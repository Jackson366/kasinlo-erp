package inv

import (
	"errors"

	"gorm.io/gorm"

	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// stockStats 统计出入库单据的行数与总数量。
//
// 总数量按**基本数量**（qty_base）汇总：一张单子上同时有「2 箱」和「12 米」时，
// 把成交数量直接相加得到的数没有单位、也没有意义。
func (s *Service) stockStats(ids []int64) (map[int64][2]float64, error) {
	out := map[int64][2]float64{}
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		StockOrderID int64
		Cnt          int64
		Qty          float64
	}
	var rows []row
	if err := s.db.Model(&model.InvStockItem{}).
		Select("stock_order_id, COUNT(*) AS cnt, COALESCE(SUM(qty_base),0) AS qty").
		Where("stock_order_id IN ?", ids).Group("stock_order_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, r := range rows {
		out[r.StockOrderID] = [2]float64{float64(r.Cnt), round4(r.Qty)}
	}
	return out, nil
}

// StockOrderList 出入库单据列表。direction 为空表示全部。
func (s *Service) StockOrderList(q page.Query, direction string) ([]StockOrderRow, int64, error) {
	tx := s.db.Model(&model.InvStockOrder{}).Where("status = 1")
	if direction == model.InvStockIn || direction == model.InvStockOut {
		tx = tx.Where("direction = ?", direction)
	}
	if q.Keyword != "" {
		tx = tx.Where(likeExpr("order_no"), likeKW(q.Keyword))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvStockOrder
	if err := q.Paginate(tx.Order("order_date DESC, id DESC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	ids := make([]int64, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	stats, err := s.stockStats(ids)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]StockOrderRow, 0, len(list))
	for _, o := range list {
		st := stats[o.ID]
		rows = append(rows, StockOrderRow{
			ID: o.ID, OrderNo: o.OrderNo, Direction: o.Direction, SourceType: o.SourceType,
			SourceID: o.SourceID, OrderDate: o.OrderDate, Status: o.Status,
			ItemCount: int64(st[0]), TotalQty: st[1], Remark: o.Remark, CreatedAt: o.CreatedAt,
		})
	}
	return rows, total, nil
}

// StockOrderCreate 新增出入库单据并即时过账（调整库存）。
func (s *Service) StockOrderCreate(req StockOrderReq, actorID int64) (int64, error) {
	orderDate, err := parseDate(req.OrderDate)
	if err != nil {
		return 0, err
	}
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = model.InvSourceManual
	}
	resolved, err := s.resolveSpecUnits(stockRefs(req.Items))
	if err != nil {
		return 0, err
	}
	items := make([]model.InvStockItem, 0, len(req.Items))
	for i, it := range req.Items {
		r := resolved[i]
		items = append(items, model.InvStockItem{
			ProductID: r.Product.ID, ProductName: r.Product.Name,
			SpecID: r.Spec.ID, Spec: r.Spec.Spec,
			UnitID: r.Unit.ID, Unit: r.Unit.Unit, Rate: r.Unit.Rate,
			Qty: it.Qty, QtyBase: round4(it.Qty * r.Unit.Rate),
		})
	}
	no, err := s.nextNo("inv_stock_orders", "CK")
	if err != nil {
		return 0, err
	}
	order := model.InvStockOrder{
		OrderNo: no, Direction: req.Direction, SourceType: sourceType,
		OrderDate: orderDate, Status: 1, Remark: req.Remark, CreatedBy: actorID,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].StockOrderID = order.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		return s.applyStock(tx, req.Direction, items)
	})
	if err != nil {
		return 0, saveErr(err)
	}
	return order.ID, nil
}

// StockOrderVoid 作废出入库单据并回滚库存。
func (s *Service) StockOrderVoid(id int64) error {
	var o model.InvStockOrder
	if err := s.db.First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if o.Status != 1 {
		return errcode.New(errcode.ErrConflict, "单据已作废")
	}
	var items []model.InvStockItem
	if err := s.db.Where("stock_order_id = ?", id).Find(&items).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 回滚方向与原方向相反
	reverse := model.InvStockOut
	if o.Direction == model.InvStockOut {
		reverse = model.InvStockIn
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.applyStock(tx, reverse, items); err != nil {
			return err
		}
		// 同上：状态推进带条件且放最后。并发点两次作废会读到同一份「有效」状态，
		// 回滚两次库存 —— 库存凭空多出来。影响 0 行即回滚。
		res := tx.Model(&model.InvStockOrder{}).
			Where("id = ? AND status = ?", id, 1).
			Update("status", 2)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.New(errcode.ErrConflict, "单据已作废，请勿重复操作")
		}
		return nil
	})
	return saveErr(err)
}

// stockRefs 把出入库明细请求转换成待解析的三元组。
func stockRefs(items []StockItemReq) []specUnitRef {
	out := make([]specUnitRef, 0, len(items))
	for _, it := range items {
		out = append(out, specUnitRef{ProductID: it.ProductID, SpecID: it.SpecID, UnitID: it.UnitID})
	}
	return out
}

// StockOrderDetail 出入库单打印详情。
// 已作废单据不可打印——单据已回滚库存，再打出来就是一张与实物对不上的凭证。
func (s *Service) StockOrderDetail(id int64) (*StockOrderDetail, error) {
	var o model.InvStockOrder
	if err := s.db.First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "出入库单不存在")
		}
		return nil, saveErr(err)
	}
	if o.Status == 2 {
		return nil, errcode.New(errcode.ErrConflict, "已作废单据不可打印")
	}

	var items []model.InvStockItem
	if err := s.db.Where("stock_order_id = ?", id).Order("id ASC").Find(&items).Error; err != nil {
		return nil, saveErr(err)
	}

	// 明细行只存了商品名与规格快照，编码从商品档案补齐；
	// 商品可能已被删除，查不到就留空——打印不因档案变动而失败
	ids := make([]int64, 0, len(items))
	specIDs := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ProductID)
		if it.SpecID > 0 {
			specIDs = append(specIDs, it.SpecID)
		}
	}
	products, err := s.productByIDs(ids)
	if err != nil {
		return nil, err
	}
	units, err := s.unitsBySpec(specIDs)
	if err != nil {
		return nil, err
	}
	// 基本单位名：纸面要同时写清「2 箱」与「20 个」，缺了单位名，
	// 基本数量那一列就只是一个没有计量含义的数字
	baseUnit := map[int64]string{}
	for specID, list := range units {
		for _, u := range list {
			if u.IsBase {
				baseUnit[specID] = u.Unit
				break
			}
		}
	}

	printItems := make([]StockPrintItem, 0, len(items))
	total := 0.0
	for _, it := range items {
		code := ""
		if p, ok := products[it.ProductID]; ok {
			code = p.Code
		}
		total += it.QtyBase
		printItems = append(printItems, StockPrintItem{
			ProductCode: code, ProductName: it.ProductName, Spec: it.Spec,
			Unit: it.Unit, Rate: it.Rate, Qty: it.Qty, QtyBase: it.QtyBase,
			UnitBase: baseUnit[it.SpecID],
		})
	}

	return &StockOrderDetail{
		ID: o.ID, OrderNo: o.OrderNo, Direction: o.Direction, SourceType: o.SourceType,
		SourceOrderNo: s.sourceOrderNo(o.SourceType, o.SourceID),
		OrderDate:     o.OrderDate, Remark: o.Remark,
		Operator: s.operatorName(o.CreatedBy), TotalQty: round4(total), Items: printItems,
	}, nil
}

// sourceOrderNo 查来源采购 / 销售单号；手工单没有来源，返回空。
func (s *Service) sourceOrderNo(sourceType string, sourceID int64) string {
	if sourceID <= 0 || (sourceType != model.InvSourcePurchase && sourceType != model.InvSourceSales) {
		return ""
	}
	var no string
	if err := s.db.Model(&model.InvOrder{}).Where("id = ?", sourceID).
		Pluck("order_no", &no).Error; err != nil || no == "" {
		return ""
	}
	return no
}

// operatorName 制单人显示名：昵称优先，退回用户名；用户已删除则留空。
func (s *Service) operatorName(userID int64) string {
	if userID <= 0 {
		return ""
	}
	var row struct{ Nickname, Username string }
	if err := s.db.Table("sys_users").Select("nickname", "username").
		Where("id = ?", userID).Scan(&row).Error; err != nil {
		return ""
	}
	if row.Nickname != "" {
		return row.Nickname
	}
	return row.Username
}
