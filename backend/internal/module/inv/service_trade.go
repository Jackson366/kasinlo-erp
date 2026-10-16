package inv

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// orderPrefix 单据编号前缀。
func orderPrefix(bizType string) string {
	if bizType == model.InvBizPurchase {
		return "CG"
	}
	return "XS"
}

// itemCounts 统计每个单据的明细行数（一次聚合，避免逐行查询）。
func (s *Service) itemCounts(orderIDs []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	type row struct {
		OrderID int64
		Cnt     int64
	}
	var rows []row
	if err := s.db.Model(&model.InvOrderItem{}).
		Select("order_id, COUNT(*) AS cnt").Where("order_id IN ?", orderIDs).
		Group("order_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, r := range rows {
		out[r.OrderID] = r.Cnt
	}
	return out, nil
}

// OrderList 单据列表。bizType 为 purchase / sales；status 为空表示不过滤。
func (s *Service) OrderList(bizType string, q page.Query, status *int16, actor *authz.Identity) ([]OrderRow, int64, error) {
	tx := s.db.Model(&model.InvOrder{}).Where("biz_type = ?", bizType)
	tx = s.applyCreator(tx, actor)
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		tx = tx.Where(likeExpr("order_no")+" OR "+likeExpr("party_name"), kw, kw)
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvOrder
	if err := q.Paginate(tx.Order("order_date DESC, id DESC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	ids := make([]int64, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	counts, err := s.itemCounts(ids)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]OrderRow, 0, len(list))
	for _, o := range list {
		rows = append(rows, toOrderRow(o, counts[o.ID]))
	}
	return rows, total, nil
}

func toOrderRow(o model.InvOrder, itemCount int64) OrderRow {
	return OrderRow{
		ID: o.ID, OrderNo: o.OrderNo, BizType: o.BizType, PartyID: o.PartyID, PartyName: o.PartyName,
		OrderDate: o.OrderDate, DueDate: o.DueDate, Status: o.Status,
		TotalAmount: o.TotalAmount, SettledAmount: o.SettledAmount,
		SettleStatus: settleStatus(o.TotalAmount, o.SettledAmount),
		ItemCount:    itemCount, Remark: o.Remark, CreatedAt: o.CreatedAt,
	}
}

// OrderDetail 单据详情。
func (s *Service) OrderDetail(id int64, actor *authz.Identity) (*OrderDetail, error) {
	var o model.InvOrder
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	items, err := s.orderItems(id)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{
		OrderRow: toOrderRow(o, int64(len(items))),
		Operator: s.operatorName(o.CreatedBy),
		Items:    items,
	}, nil
}

func (s *Service) orderItems(orderID int64) ([]OrderItemVO, error) {
	var list []model.InvOrderItem
	if err := s.db.Where("order_id = ?", orderID).Order("id ASC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 明细快照里没有商品编码，从商品档案补齐；商品已删则留空——打印不因档案变动而失败
	ids := make([]int64, 0, len(list))
	for _, it := range list {
		ids = append(ids, it.ProductID)
	}
	products, err := s.productByIDs(ids)
	if err != nil {
		return nil, err
	}
	out := make([]OrderItemVO, 0, len(list))
	for _, it := range list {
		code := ""
		if p, ok := products[it.ProductID]; ok {
			code = p.Code
		}
		out = append(out, OrderItemVO{
			ID: it.ID, ProductID: it.ProductID, ProductCode: code, ProductName: it.ProductName,
			SpecID: it.SpecID, Spec: it.Spec, UnitID: it.UnitID, Unit: it.Unit, Rate: it.Rate,
			Qty: it.Qty, QtyBase: it.QtyBase, Price: it.Price, Amount: it.Amount,
		})
	}
	return out, nil
}

// buildItems 依据请求装配明细：价格缺省时取该**计量单位**的采购价 / 销售价。
//
// 价格挂在单位上而不是商品上，所以「1 箱」与「1 个」的默认价天然不同 ——
// 这正是一箱 100 元、一个 10.5 元这类批发/零售价差能被正确带出来的原因。
func (s *Service) buildItems(bizType string, req []OrderItemReq) ([]model.InvOrderItem, float64, error) {
	refs := make([]specUnitRef, 0, len(req))
	for _, it := range req {
		refs = append(refs, specUnitRef{ProductID: it.ProductID, SpecID: it.SpecID, UnitID: it.UnitID})
	}
	resolved, err := s.resolveSpecUnits(refs)
	if err != nil {
		return nil, 0, err
	}
	items := make([]model.InvOrderItem, 0, len(req))
	var total float64
	for i, it := range req {
		r := resolved[i]
		price := it.Price
		if price <= 0 {
			if bizType == model.InvBizPurchase {
				price = r.Unit.PurchasePrice
			} else {
				price = r.Unit.SalePrice
			}
		}
		amount := round2(it.Qty * price)
		total += amount
		items = append(items, model.InvOrderItem{
			ProductID: r.Product.ID, ProductName: r.Product.Name,
			SpecID: r.Spec.ID, Spec: r.Spec.Spec,
			UnitID: r.Unit.ID, Unit: r.Unit.Unit, Rate: r.Unit.Rate,
			Qty: it.Qty, QtyBase: round4(it.Qty * r.Unit.Rate),
			Price: price, Amount: amount,
		})
	}
	return items, round2(total), nil
}

// OrderCreate 新增单据。
func (s *Service) OrderCreate(req OrderReq, actor *authz.Identity) (int64, error) {
	partyName, err := s.findParty(req.BizType, req.PartyID)
	if err != nil {
		return 0, err
	}
	orderDate, err := parseDate(req.OrderDate)
	if err != nil {
		return 0, err
	}
	dueDate, err := parseDatePtr(req.DueDate)
	if err != nil {
		return 0, err
	}
	items, total, err := s.buildItems(req.BizType, req.Items)
	if err != nil {
		return 0, err
	}
	no, err := s.nextNo("inv_orders", orderPrefix(req.BizType))
	if err != nil {
		return 0, err
	}
	order := model.InvOrder{
		OrderNo: no, BizType: req.BizType, PartyID: req.PartyID, PartyName: partyName,
		OrderDate: orderDate, DueDate: dueDate, Status: model.InvStatusPending,
		TotalAmount: total, Remark: req.Remark, CreatedBy: actorUID(actor),
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].OrderID = order.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return 0, saveErr(err)
	}
	return order.ID, nil
}

// OrderUpdate 修改单据。仅「待处理」状态可改，且整体覆盖明细。
func (s *Service) OrderUpdate(id int64, req OrderReq, actor *authz.Identity) error {
	var o model.InvOrder
	if err := s.db.First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureCreatorVisible(o.CreatedBy, actor); err != nil {
		return err
	}
	if o.Status != model.InvStatusPending {
		return errcode.New(errcode.ErrConflict, "仅待处理单据可修改")
	}
	if o.BizType != req.BizType {
		return errcode.New(errcode.ErrBadRequest, "单据类型不可更改")
	}
	partyName, err := s.findParty(req.BizType, req.PartyID)
	if err != nil {
		return err
	}
	orderDate, err := parseDate(req.OrderDate)
	if err != nil {
		return err
	}
	dueDate, err := parseDatePtr(req.DueDate)
	if err != nil {
		return err
	}
	items, total, err := s.buildItems(req.BizType, req.Items)
	if err != nil {
		return err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.InvOrder{}).Where("id = ?", id).Updates(map[string]any{
			"party_id": req.PartyID, "party_name": partyName, "order_date": orderDate,
			"due_date": dueDate, "total_amount": total, "remark": req.Remark,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("order_id = ?", id).Delete(&model.InvOrderItem{}).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].OrderID = id
		}
		return tx.Create(&items).Error
	})
	return saveErr(err)
}

// OrderDelete 删除单据。已审核（已产生出入库）的单据不允许删除。
func (s *Service) OrderDelete(id int64, actor *authz.Identity) error {
	var o model.InvOrder
	if err := s.db.First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureCreatorVisible(o.CreatedBy, actor); err != nil {
		return err
	}
	if o.Status == model.InvStatusDone {
		return errcode.New(errcode.ErrConflict, "已审核单据不可删除，请先作废对应的出入库单")
	}
	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("order_id = ?", id).Delete(&model.InvOrderItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.InvOrder{}, id).Error
	}))
}

// OrderChangeStatus 流转单据状态：1 待处理 / 2 审核通过 / 3 取消。
// 置为「审核通过」时会自动生成出入库单并同步商品库存（同一事务）。
func (s *Service) OrderChangeStatus(id int64, status int16, actor *authz.Identity) error {
	var o model.InvOrder
	if err := s.db.First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureCreatorVisible(o.CreatedBy, actor); err != nil {
		return err
	}
	if o.Status == model.InvStatusDone {
		return errcode.New(errcode.ErrConflict, "已审核单据不可再流转状态")
	}
	if o.Status == status {
		return nil
	}
	if status != model.InvStatusDone {
		return saveErr(s.db.Model(&model.InvOrder{}).Where("id = ?", id).
			Update("status", status).Error)
	}

	// 审核通过：生成出入库单 + 调整库存
	var items []model.InvOrderItem
	if err := s.db.Where("order_id = ?", id).Find(&items).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	direction := model.InvStockIn
	if o.BizType == model.InvBizSales {
		direction = model.InvStockOut
	}
	sourceType := model.InvSourcePurchase
	if o.BizType == model.InvBizSales {
		sourceType = model.InvSourceSales
	}
	stockNo, err := s.nextNo("inv_stock_orders", "CK")
	if err != nil {
		return err
	}
	stockItems := make([]model.InvStockItem, 0, len(items))
	for _, it := range items {
		stockItems = append(stockItems, model.InvStockItem{
			ProductID: it.ProductID, ProductName: it.ProductName,
			SpecID: it.SpecID, Spec: it.Spec,
			UnitID: it.UnitID, Unit: it.Unit, Rate: it.Rate,
			Qty: it.Qty, QtyBase: it.QtyBase,
		})
	}
	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		so := model.InvStockOrder{
			OrderNo: stockNo, Direction: direction, SourceType: sourceType, SourceID: o.ID,
			OrderDate: o.OrderDate, Status: 1, CreatedBy: actorUID(actor),
		}
		if err := tx.Create(&so).Error; err != nil {
			return err
		}
		for i := range stockItems {
			stockItems[i].StockOrderID = so.ID
		}
		if err := tx.Create(&stockItems).Error; err != nil {
			return err
		}
		if err := s.applyStock(tx, direction, stockItems); err != nil {
			return err
		}
		// 状态推进放在最后、且带条件：上面那句 o.Status 是在事务外读的，
		// 两次并发审核都会读到「待处理」，于是生成两张出入库单、扣两次库存。
		// 用「status <> 已审核」的条件更新把第二次挡在数据库层 ——
		// 影响 0 行就回滚整个事务，库存与出入库单一起撤销。
		res := tx.Model(&model.InvOrder{}).
			Where("id = ? AND status <> ?", o.ID, model.InvStatusDone).
			Update("status", model.InvStatusDone)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.New(errcode.ErrConflict, "单据已被审核，请勿重复操作")
		}
		return nil
	}))
}

// applyStock 在事务内按方向增减**规格**库存（基数为基本单位）。
//
// 库存记在规格上而不是商品上：同一个商品的不同规格是各自独立的库存，
// 把「φ20 出库 5 根」记到商品级，φ25 的库存会被一起扣掉。
//
// 先按规格聚合净变动再逐条校验：同一张单子上同一个规格出现两行时，
// 逐行校验会「每行都够、合起来不够」，把库存扣成负数。
func (s *Service) applyStock(tx *gorm.DB, direction string, items []model.InvStockItem) error {
	sign := 1.0
	if direction == model.InvStockOut {
		sign = -1.0
	}
	delta := map[int64]float64{}
	label := map[int64]string{}
	order := make([]int64, 0, len(items))
	for _, it := range items {
		if it.SpecID <= 0 {
			return errcode.New(errcode.ErrBadRequest, "出入库明细缺少规格，无法过账")
		}
		if _, ok := delta[it.SpecID]; !ok {
			order = append(order, it.SpecID)
			label[it.SpecID] = fmt.Sprintf("%s %s", it.ProductName, it.Spec)
		}
		delta[it.SpecID] += sign * it.QtyBase
	}
	for _, specID := range order {
		d := round4(delta[specID])
		if d == 0 {
			continue
		}
		if d < 0 {
			var sp model.InvProductSpec
			if err := tx.First(&sp, specID).Error; err != nil {
				return err
			}
			if sp.Stock+d < -0.00005 {
				return errcode.Newf(errcode.ErrConflict,
					"「%s」库存不足（现有 %s）", label[specID], fmtQty(sp.Stock))
			}
		}
		if err := tx.Model(&model.InvProductSpec{}).Where("id = ?", specID).
			UpdateColumn("stock", gorm.Expr("stock + ?", d)).Error; err != nil {
			return err
		}
	}
	return nil
}
