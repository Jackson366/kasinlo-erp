package inv

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// partyStat 是「往来单位 -> 未结金额 / 单据数」的聚合结果。
type partyStat struct {
	Balance float64
	Count   int64
}

// partyStats 按业务类型统计每个往来单位的未结金额与单据数。
// 一次性聚合，避免列表里逐行查库（N+1）。
func (s *Service) partyStats(bizType string) (map[int64]partyStat, error) {
	type row struct {
		PartyID int64
		Bal     float64
		Cnt     int64
	}
	var rows []row
	if err := s.db.Model(&model.InvOrder{}).
		Select("party_id, COALESCE(SUM(total_amount - settled_amount), 0) AS bal, COUNT(*) AS cnt").
		Where("biz_type = ? AND status <> ?", bizType, model.InvStatusCanceled).
		Group("party_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make(map[int64]partyStat, len(rows))
	for _, r := range rows {
		out[r.PartyID] = partyStat{Balance: round2(r.Bal), Count: r.Cnt}
	}
	return out, nil
}

// partyList 客户 / 供应商通用列表。bizType 决定余额口径与冗余名称来源。
func (s *Service) partyList(bizType string, q page.Query, status *int16) ([]PartyRow, int64, error) {
	var (
		rows  []PartyRow
		total int64
	)
	stats, err := s.partyStats(bizType)
	if err != nil {
		return nil, 0, err
	}

	if bizType == model.InvBizPurchase {
		tx := s.db.Model(&model.InvSupplier{})
		if q.Keyword != "" {
			kw := likeKW(q.Keyword)
			tx = tx.Where(likeExpr("code")+" OR "+likeExpr("name")+" OR "+likeExpr("contact"), kw, kw, kw)
		}
		if status != nil {
			tx = tx.Where("status = ?", *status)
		}
		if err := tx.Count(&total).Error; err != nil {
			return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
		}
		var list []model.InvSupplier
		if err := q.Paginate(tx.Order("id DESC")).Find(&list).Error; err != nil {
			return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
		}
		for _, it := range list {
			st := stats[it.ID]
			rows = append(rows, PartyRow{
				ID: it.ID, Code: it.Code, Name: it.Name, Contact: it.Contact, Phone: it.Phone,
				Address: it.Address, TaxNo: it.TaxNo, Status: it.Status, Remark: it.Remark,
				Balance: st.Balance, OrderCount: st.Count, CreatedAt: it.CreatedAt,
			})
		}
		return rows, total, nil
	}

	tx := s.db.Model(&model.InvCustomer{})
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		tx = tx.Where(likeExpr("code")+" OR "+likeExpr("name")+" OR "+likeExpr("contact"), kw, kw, kw)
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvCustomer
	if err := q.Paginate(tx.Order("id DESC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, it := range list {
		st := stats[it.ID]
		rows = append(rows, PartyRow{
			ID: it.ID, Code: it.Code, Name: it.Name, Contact: it.Contact, Phone: it.Phone,
			Address: it.Address, TaxNo: it.TaxNo, Status: it.Status, Remark: it.Remark,
			Balance: st.Balance, OrderCount: st.Count, CreatedAt: it.CreatedAt,
		})
	}
	return rows, total, nil
}

// PartyList 客户 / 供应商列表入口，bizType 取 purchase（供应商）或 sales（客户）。
func (s *Service) PartyList(bizType string, q page.Query, status *int16) ([]PartyRow, int64, error) {
	return s.partyList(bizType, q, status)
}

// partyNameFree 校验编号唯一（存活数据内）。
func (s *Service) partyNameFree(bizType, code string, excludeID int64) error {
	var cnt int64
	tx := s.db.Table(partyTable(bizType)).Where("code = ?", code)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "编号 %s 已存在", code)
	}
	return nil
}

func partyTable(bizType string) string {
	if bizType == model.InvBizPurchase {
		return "inv_suppliers"
	}
	return "inv_customers"
}

// PartyCreate 新增客户 / 供应商。
func (s *Service) PartyCreate(bizType string, req PartyReq) (int64, error) {
	if err := s.partyNameFree(bizType, req.Code, 0); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	if bizType == model.InvBizPurchase {
		it := model.InvSupplier{
			Code: req.Code, Name: req.Name, Contact: req.Contact, Phone: req.Phone,
			Address: req.Address, TaxNo: req.TaxNo, Status: status, Remark: req.Remark,
		}
		if err := s.db.Create(&it).Error; err != nil {
			return 0, saveErr(err)
		}
		return it.ID, nil
	}
	it := model.InvCustomer{
		Code: req.Code, Name: req.Name, Contact: req.Contact, Phone: req.Phone,
		Address: req.Address, TaxNo: req.TaxNo, Status: status, Remark: req.Remark,
	}
	if err := s.db.Create(&it).Error; err != nil {
		return 0, saveErr(err)
	}
	return it.ID, nil
}

// PartyUpdate 修改客户 / 供应商。
func (s *Service) PartyUpdate(bizType string, id int64, req PartyReq) error {
	if err := s.partyNameFree(bizType, req.Code, id); err != nil {
		return err
	}
	updates := map[string]any{
		"code": req.Code, "name": req.Name, "contact": req.Contact, "phone": req.Phone,
		"address": req.Address, "tax_no": req.TaxNo, "remark": req.Remark,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	res := s.db.Table(partyTable(bizType)).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return saveErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "记录不存在")
	}
	return nil
}

// PartyDelete 删除客户 / 供应商。已被单据引用时禁止删除，
// 否则历史单据会指向一个查不到的往来单位。
func (s *Service) PartyDelete(bizType string, id int64) error {
	var cnt int64
	if err := s.db.Model(&model.InvOrder{}).Where("biz_type = ? AND party_id = ?", bizType, id).
		Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "该往来单位已被 %d 张单据引用，无法删除", cnt)
	}
	var res *gorm.DB
	if bizType == model.InvBizPurchase {
		res = s.db.Delete(&model.InvSupplier{}, id)
	} else {
		res = s.db.Delete(&model.InvCustomer{}, id)
	}
	if res.Error != nil {
		return errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "记录不存在")
	}
	return nil
}

// ============================================================
// 商品分类
// ============================================================

// categoryRefCounts 统计每个分类下的商品数（一次聚合，避免列表里逐行查库）。
func (s *Service) categoryRefCounts() (map[int64]int64, error) {
	type row struct {
		CategoryID int64
		Cnt        int64
	}
	var rows []row
	if err := s.db.Model(&model.InvProduct{}).
		Select("category_id, COUNT(*) AS cnt").
		Where("category_id > 0").Group("category_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make(map[int64]int64, len(rows))
	for _, r := range rows {
		out[r.CategoryID] = r.Cnt
	}
	return out, nil
}

// categoryNames 取「分类 ID -> 名称」映射。分类表很小，取全量比按 ID 拼 IN 更简单。
func (s *Service) categoryNames() (map[int64]string, error) {
	var list []model.InvCategory
	if err := s.db.Model(&model.InvCategory{}).Select("id", "name").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make(map[int64]string, len(list))
	for _, it := range list {
		out[it.ID] = it.Name
	}
	return out, nil
}

// CategoryList 商品分类列表。
func (s *Service) CategoryList(q page.Query, status *int16) ([]CategoryRow, int64, error) {
	tx := s.db.Model(&model.InvCategory{})
	if q.Keyword != "" {
		tx = tx.Where(likeExpr("name"), likeKW(q.Keyword))
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvCategory
	if err := q.Paginate(tx.Order("sort ASC, id ASC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	counts, err := s.categoryRefCounts()
	if err != nil {
		return nil, 0, err
	}
	rows := make([]CategoryRow, 0, len(list))
	for _, it := range list {
		rows = append(rows, CategoryRow{
			ID: it.ID, Name: it.Name, Sort: it.Sort, Status: it.Status, Remark: it.Remark,
			ProductCount: counts[it.ID], CreatedAt: it.CreatedAt,
		})
	}
	return rows, total, nil
}

// CategoryOptions 启用中的分类，供商品表单的下拉使用。
func (s *Service) CategoryOptions() ([]CategoryOption, error) {
	var list []model.InvCategory
	if err := s.db.Model(&model.InvCategory{}).
		Where("status = ?", model.StatusEnabled).
		Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make([]CategoryOption, 0, len(list))
	for _, it := range list {
		out = append(out, CategoryOption{ID: it.ID, Name: it.Name})
	}
	return out, nil
}

// categoryNameFree 校验分类名称唯一（存活数据内）。
func (s *Service) categoryNameFree(name string, excludeID int64) error {
	var cnt int64
	tx := s.db.Model(&model.InvCategory{}).Where("name = ?", name)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "分类「%s」已存在", name)
	}
	return nil
}

// CategoryCreate 新增商品分类。
func (s *Service) CategoryCreate(req CategoryReq) (int64, error) {
	if err := s.categoryNameFree(req.Name, 0); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	it := model.InvCategory{Name: req.Name, Sort: req.Sort, Status: status, Remark: req.Remark}
	if err := s.db.Create(&it).Error; err != nil {
		return 0, saveErr(err)
	}
	return it.ID, nil
}

// CategoryUpdate 修改商品分类。
func (s *Service) CategoryUpdate(id int64, req CategoryReq) error {
	if err := s.categoryNameFree(req.Name, id); err != nil {
		return err
	}
	updates := map[string]any{"name": req.Name, "sort": req.Sort, "remark": req.Remark}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	res := s.db.Model(&model.InvCategory{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return saveErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "分类不存在")
	}
	return nil
}

// CategoryDelete 删除商品分类。仍被商品引用时禁止删除 —— 删掉之后那些商品的
// 分类会变成一个悬空 ID，列表里显示为空，报表的品类占比则凭空少一块。
func (s *Service) CategoryDelete(id int64) error {
	var cnt int64
	if err := s.db.Model(&model.InvProduct{}).Where("category_id = ?", id).
		Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "该分类下还有 %d 个商品，无法删除（可改为停用）", cnt)
	}
	res := s.db.Delete(&model.InvCategory{}, id)
	if res.Error != nil {
		return errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "分类不存在")
	}
	return nil
}

// ============================================================
// 商品（商品 → 规格 → 计量单位）
// ============================================================

// specsByProduct 批量取规格，按 product_id 归拢。
func (s *Service) specsByProduct(productIDs []int64) (map[int64][]model.InvProductSpec, error) {
	out := map[int64][]model.InvProductSpec{}
	if len(productIDs) == 0 {
		return out, nil
	}
	var list []model.InvProductSpec
	if err := s.db.Where("product_id IN ?", productIDs).
		Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, sp := range list {
		out[sp.ProductID] = append(out[sp.ProductID], sp)
	}
	return out, nil
}

// unitsBySpec 批量取计量单位，按 spec_id 归拢。
func (s *Service) unitsBySpec(specIDs []int64) (map[int64][]model.InvProductUnit, error) {
	out := map[int64][]model.InvProductUnit{}
	if len(specIDs) == 0 {
		return out, nil
	}
	var list []model.InvProductUnit
	if err := s.db.Where("spec_id IN ?", specIDs).
		Order("is_base DESC, sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, u := range list {
		out[u.SpecID] = append(out[u.SpecID], u)
	}
	return out, nil
}

// buildSpecVOs 把「商品 -> 规格 -> 单位」装配成响应结构，并算出规格与商品的汇总。
func (s *Service) buildSpecVOs(specs []model.InvProductSpec) ([]ProductSpecVO, int, bool, error) {
	if len(specs) == 0 {
		return []ProductSpecVO{}, 0, false, nil
	}
	ids := make([]int64, 0, len(specs))
	for _, sp := range specs {
		ids = append(ids, sp.ID)
	}
	units, err := s.unitsBySpec(ids)
	if err != nil {
		return nil, 0, false, err
	}
	out := make([]ProductSpecVO, 0, len(specs))
	unitCount := 0
	warn := false
	for _, sp := range specs {
		vo := ProductSpecVO{
			ID: sp.ID, Spec: sp.Spec, Stock: sp.Stock, WarnStock: sp.WarnStock,
			// 停用的规格不算预警：它已经退出新单据，库存只会原地停留或清零，
			// 把它标红只会误导「还欠这个规格的货」。与 Inventory / 概览口径一致。
			Warn: sp.Status == model.StatusEnabled && sp.WarnStock > 0 && sp.Stock <= sp.WarnStock,
			Status: sp.Status,
			Units: make([]ProductUnitVO, 0, len(units[sp.ID])),
		}
		if vo.Warn {
			warn = true
		}
		for _, u := range units[sp.ID] {
			vo.Units = append(vo.Units, ProductUnitVO{
				ID: u.ID, Unit: u.Unit, Rate: u.Rate, IsBase: u.IsBase,
				PurchasePrice: u.PurchasePrice, SalePrice: u.SalePrice, Status: u.Status,
			})
		}
		unitCount += len(vo.Units)
		out = append(out, vo)
	}
	return out, unitCount, warn, nil
}

// ProductList 商品列表（含规格与计量单位明细）。
// status / categoryID 为空表示不过滤；同一份接口既供商品管理页，也供单据明细的商品下拉。
func (s *Service) ProductList(q page.Query, status *int16, categoryID int64) ([]ProductRow, int64, error) {
	tx := s.db.Model(&model.InvProduct{})
	if q.Keyword != "" {
		// 规格也算「关键字」：现场沟通时用户报的往往是「φ20」而不是编码。
		// 用 EXISTS 子查询而不是 JOIN —— JOIN 会让同一个商品被多规格放大成多行，
		// 分页与总数随即对不上（AGENTS.md §2 Step 7 同一个坑）。
		tx = tx.Where(`code ILIKE ? ESCAPE '\' OR name ILIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM inv_product_specs sp
			WHERE sp.product_id = inv_products.id AND sp.deleted_at IS NULL AND sp.spec ILIKE ? ESCAPE '\')`,
			likeKW(q.Keyword), likeKW(q.Keyword), likeKW(q.Keyword))
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	if categoryID > 0 {
		tx = tx.Where("category_id = ?", categoryID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvProduct
	if err := q.Paginate(tx.Order("id ASC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if len(list) == 0 {
		return []ProductRow{}, total, nil
	}

	ids := make([]int64, 0, len(list))
	for _, it := range list {
		ids = append(ids, it.ID)
	}
	specs, err := s.specsByProduct(ids)
	if err != nil {
		return nil, 0, err
	}
	categories, err := s.categoryNames()
	if err != nil {
		return nil, 0, err
	}

	rows := make([]ProductRow, 0, len(list))
	for _, it := range list {
		specVOs, unitCount, warn, err := s.buildSpecVOs(specs[it.ID])
		if err != nil {
			return nil, 0, err
		}
		var totalStock float64
		for _, sp := range specVOs {
			totalStock += sp.Stock
		}
		rows = append(rows, ProductRow{
			ID: it.ID, Code: it.Code, Name: it.Name,
			CategoryID: it.CategoryID, CategoryName: categories[it.CategoryID],
			SpecCount: len(specVOs), UnitCount: unitCount,
			TotalStock: round4(totalStock), Warn: warn, Status: it.Status,
			Remark: it.Remark, CreatedAt: it.CreatedAt, Specs: specVOs,
		})
	}
	return rows, total, nil
}

// ProductDetail 单个商品的完整档案（含规格与单位）。
func (s *Service) ProductDetail(id int64) (*ProductRow, error) {
	var it model.InvProduct
	if err := s.db.First(&it, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "商品不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	specs, err := s.specsByProduct([]int64{id})
	if err != nil {
		return nil, err
	}
	categories, err := s.categoryNames()
	if err != nil {
		return nil, err
	}
	specVOs, unitCount, warn, err := s.buildSpecVOs(specs[id])
	if err != nil {
		return nil, err
	}
	var totalStock float64
	for _, sp := range specVOs {
		totalStock += sp.Stock
	}
	row := ProductRow{
		ID: it.ID, Code: it.Code, Name: it.Name,
		CategoryID: it.CategoryID, CategoryName: categories[it.CategoryID],
		SpecCount: len(specVOs), UnitCount: unitCount,
		TotalStock: round4(totalStock), Warn: warn, Status: it.Status,
		Remark: it.Remark, CreatedAt: it.CreatedAt, Specs: specVOs,
	}
	return &row, nil
}

// pickLimit 级联选项一次最多返回多少个商品。
//
// 下拉不是列表页，不该把几千个商品灌进浏览器；但也不能因此静默截断 ——
// 超出部分通过 ProductPickResult.Truncated 明确告知前端。
const pickLimit = 500

// ProductPick 商品 → 规格 → 单位 的级联选项，供单据明细的三级下拉使用。
// 停用的商品 / 规格 / 单位一律不出现在可选项里。
func (s *Service) ProductPick(keyword string) (*ProductPickResult, error) {
	tx := s.db.Model(&model.InvProduct{}).Where("status = ?", model.StatusEnabled)
	if keyword != "" {
		tx = tx.Where(`code ILIKE ? ESCAPE '\' OR name ILIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM inv_product_specs sp
			WHERE sp.product_id = inv_products.id AND sp.deleted_at IS NULL AND sp.spec ILIKE ? ESCAPE '\')`,
			likeKW(keyword), likeKW(keyword), likeKW(keyword))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 多取一条用来判断「后面还有没有」，避免为了判截断再发一次 COUNT
	var list []model.InvProduct
	if err := tx.Order("id ASC").Limit(pickLimit + 1).Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	truncated := int64(len(list)) > pickLimit
	if truncated {
		list = list[:pickLimit]
	}
	if len(list) == 0 {
		return &ProductPickResult{Items: []ProductPickVO{}, Total: total, Truncated: truncated}, nil
	}
	ids := make([]int64, 0, len(list))
	for _, it := range list {
		ids = append(ids, it.ID)
	}
	specs, err := s.specsByProduct(ids)
	if err != nil {
		return nil, err
	}
	specIDs := make([]int64, 0, len(specs))
	for _, ss := range specs {
		for _, sp := range ss {
			specIDs = append(specIDs, sp.ID)
		}
	}
	units, err := s.unitsBySpec(specIDs)
	if err != nil {
		return nil, err
	}
	out := make([]ProductPickVO, 0, len(list))
	for _, p := range list {
		pick := ProductPickVO{ID: p.ID, Code: p.Code, Name: p.Name, Specs: []PickSpecVO{}}
		for _, sp := range specs[p.ID] {
			if sp.Status != model.StatusEnabled {
				continue
			}
			spVO := PickSpecVO{ID: sp.ID, Spec: sp.Spec, Stock: sp.Stock, Units: []PickUnitVO{}}
			for _, u := range units[sp.ID] {
				if u.Status != model.StatusEnabled {
					continue
				}
				spVO.Units = append(spVO.Units, PickUnitVO{
					ID: u.ID, Unit: u.Unit, Rate: u.Rate, IsBase: u.IsBase,
					PurchasePrice: u.PurchasePrice, SalePrice: u.SalePrice,
				})
			}
			// 没有可用单位的规格等于不可选：选进去也下单不了
			if len(spVO.Units) == 0 {
				continue
			}
			pick.Specs = append(pick.Specs, spVO)
		}
		if len(pick.Specs) == 0 {
			continue
		}
		out = append(out, pick)
	}
	if out == nil {
		out = []ProductPickVO{}
	}
	return &ProductPickResult{Items: out, Total: total, Truncated: truncated}, nil
}

// Inventory 库存查询：粒度是规格（库存就记在规格上）。
// 只看启用的商品与规格 —— 已停用的档案不该出现在「现在有多少」的视图里。
func (s *Service) Inventory(q page.Query) ([]InventoryRow, int64, error) {
	var total int64
	countTx := s.db.Table("inv_product_specs AS sp").
		Joins("JOIN inv_products AS p ON p.id = sp.product_id AND p.deleted_at IS NULL").
		Where("sp.deleted_at IS NULL AND p.status = ? AND sp.status = ?",
			model.StatusEnabled, model.StatusEnabled)
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		countTx = countTx.Where(likeExpr("p.code")+" OR "+likeExpr("p.name")+" OR "+likeExpr("sp.spec"), kw, kw, kw)
	}
	if err := countTx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	type invRow struct {
		ProductID     int64
		ProductCode   string
		ProductName   string
		CategoryID    int64
		SpecID        int64
		Spec          string
		UnitID        int64
		Unit          string
		Stock         float64
		WarnStock     float64
		PurchasePrice float64
	}
	var raw []invRow
	dataTx := s.db.Table("inv_product_specs AS sp").
		Joins("JOIN inv_products AS p ON p.id = sp.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN inv_product_units AS u ON u.spec_id = sp.id AND u.is_base AND u.deleted_at IS NULL").
		Where("sp.deleted_at IS NULL AND p.status = ? AND sp.status = ?",
			model.StatusEnabled, model.StatusEnabled)
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		dataTx = dataTx.Where(likeExpr("p.code")+" OR "+likeExpr("p.name")+" OR "+likeExpr("sp.spec"), kw, kw, kw)
	}
	if err := q.Paginate(dataTx.
		Select(`p.id AS product_id, p.code AS product_code, p.name AS product_name,
			p.category_id AS category_id, sp.id AS spec_id, sp.spec AS spec,
			COALESCE(u.id, 0) AS unit_id, COALESCE(u.unit, '') AS unit,
			sp.stock AS stock, sp.warn_stock AS warn_stock,
			COALESCE(u.purchase_price, 0) AS purchase_price`).
		Order("p.id ASC, sp.sort ASC, sp.id ASC")).Scan(&raw).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	categories, err := s.categoryNames()
	if err != nil {
		return nil, 0, err
	}
	rows := make([]InventoryRow, 0, len(raw))
	for _, r := range raw {
		rows = append(rows, InventoryRow{
			ProductID: r.ProductID, ProductCode: r.ProductCode, ProductName: r.ProductName,
			CategoryName: categories[r.CategoryID],
			SpecID:       r.SpecID, Spec: r.Spec, UnitID: r.UnitID, Unit: r.Unit,
			Stock: r.Stock, WarnStock: r.WarnStock,
			Warn:          r.WarnStock > 0 && r.Stock <= r.WarnStock,
			PurchasePrice: r.PurchasePrice,
		})
	}
	return rows, total, nil
}

// ProductDuplicates 录入时的同名商品提示。
//
// 只提示、不拦截：同名不同规格是合法场景（两家供应商都叫「纸箱」），
// 硬拦会把正常录入挡在门外。命中范围放宽到「名称包含关键字」，
// 因为录「热轧钢板」时想知道的往往是「是不是已经有『热轧钢板 Q235B』了」。
func (s *Service) ProductDuplicates(name string, excludeID int64) ([]ProductDupRow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return []ProductDupRow{}, nil
	}
	tx := s.db.Model(&model.InvProduct{}).Where(likeExpr("name"), likeKW(name))
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	var list []model.InvProduct
	if err := tx.Order("name ASC, id ASC").Limit(10).Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if len(list) == 0 {
		return []ProductDupRow{}, nil
	}
	ids := make([]int64, 0, len(list))
	for _, it := range list {
		ids = append(ids, it.ID)
	}
	specs, err := s.specsByProduct(ids)
	if err != nil {
		return nil, err
	}
	categories, err := s.categoryNames()
	if err != nil {
		return nil, err
	}
	out := make([]ProductDupRow, 0, len(list))
	for _, it := range list {
		names := make([]string, 0, len(specs[it.ID]))
		for _, sp := range specs[it.ID] {
			names = append(names, sp.Spec)
		}
		out = append(out, ProductDupRow{
			ID: it.ID, Code: it.Code, Name: it.Name,
			CategoryName: categories[it.CategoryID],
			SpecSummary:  strings.Join(names, "、"),
			Status:       it.Status,
		})
	}
	return out, nil
}

// productCodeFree 校验商品编码唯一（存活数据内）。
// 部分唯一索引 uk_inv_products_code 是最终防线，这里先给一句能看懂的提示。
func (s *Service) productCodeFree(code string, excludeID int64) error {
	var cnt int64
	tx := s.db.Model(&model.InvProduct{}).Where("code = ?", code)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "商品编码 %s 已存在", code)
	}
	return nil
}

// validateSpecs 校验商品的规格与单位结构，并归一化状态与排序。
//
// 两条硬约束：
//  1. 规格名在同一商品下不能重复 —— 两行「φ20」会把库存劈成两半，各自都不预警；
//  2. 每个规格必须有且只有一个基本单位，且基本单位的换算率是 1 ——
//     「按哪个单位记账」必须唯一且自洽，否则库存的数没有意义。
func validateSpecs(reqs []ProductSpecReq) error {
	seenSpec := map[string]bool{}
	for _, sp := range reqs {
		spec := strings.TrimSpace(sp.Spec)
		if seenSpec[spec] {
			return errcode.Newf(errcode.ErrBadRequest, "规格「%s」重复", spec)
		}
		seenSpec[spec] = true

		baseCount := 0
		seenUnit := map[string]bool{}
		for _, u := range sp.Units {
			unit := strings.TrimSpace(u.Unit)
			if seenUnit[unit] {
				return errcode.Newf(errcode.ErrBadRequest, "规格「%s」下的单位「%s」重复", spec, unit)
			}
			seenUnit[unit] = true
			if u.IsBase {
				baseCount++
				if u.Rate != 1 {
					return errcode.Newf(errcode.ErrBadRequest,
						"基本单位「%s」的换算率必须为 1（当前 %g）", unit, u.Rate)
				}
			}
		}
		if baseCount != 1 {
			return errcode.Newf(errcode.ErrBadRequest,
				"规格「%s」必须指定且只指定一个基本单位（当前 %d 个）", spec, baseCount)
		}
	}
	return nil
}

// saveSpecs 在事务内整体覆盖一个商品的规格与计量单位。
//
// 采用「比对后增删改」而不是「全删重建」：规格 ID 被历史单据明细引用着
// （InvOrderItem.SpecID 用于引用校验），删掉再建会让那些引用指向不存在的行。
// 库存也只在规格行上原地更新，不会被搬走。
func (s *Service) saveSpecs(tx *gorm.DB, productID int64, reqs []ProductSpecReq) error {
	var existing []model.InvProductSpec
	if err := tx.Where("product_id = ?", productID).Find(&existing).Error; err != nil {
		return err
	}
	kept := map[int64]bool{}
	for i, req := range reqs {
		specStatus := model.StatusEnabled
		if req.Status != nil {
			specStatus = *req.Status
		}
		specName := strings.TrimSpace(req.Spec)
		var specID int64
		// 优先按 ID 匹配：改名时前端会把原 ID 带回来，否则「φ20 → φ20A」
		// 会被误判成新增 + 删除，白白留下一条停用的僵尸规格
		if req.ID > 0 {
			for _, old := range existing {
				if old.ID == req.ID && !kept[old.ID] {
					specID = req.ID
					break
				}
			}
		}
		if specID == 0 {
			for _, old := range existing {
				if strings.TrimSpace(old.Spec) == specName && !kept[old.ID] {
					specID = old.ID
					break
				}
			}
		}
		if specID > 0 {
			kept[specID] = true
			if err := tx.Model(&model.InvProductSpec{}).Where("id = ?", specID).Updates(map[string]any{
				"spec": specName, "warn_stock": req.WarnStock, "sort": i, "status": specStatus,
			}).Error; err != nil {
				return err
			}
		} else {
			sp := model.InvProductSpec{
				ProductID: productID, Spec: specName, WarnStock: req.WarnStock,
				Sort: i, Status: specStatus,
			}
			if err := tx.Create(&sp).Error; err != nil {
				return err
			}
			specID = sp.ID
		}
		if err := s.saveUnits(tx, specID, req.Units); err != nil {
			return err
		}
	}

	// 档案里被移除的规格：已被单据或出入库引用过就保留（否则历史纸面对不上），
	// 只是停用；没有引用的直接删。
	for _, old := range existing {
		if kept[old.ID] {
			continue
		}
		used, err := s.specInUse(tx, old.ID)
		if err != nil {
			return err
		}
		if used {
			if err := tx.Model(&model.InvProductSpec{}).Where("id = ?", old.ID).
				Update("status", model.StatusDisabled).Error; err != nil {
				return err
			}
			continue
		}
		var unitIDs []int64
		if err := tx.Model(&model.InvProductUnit{}).Where("spec_id = ?", old.ID).
			Pluck("id", &unitIDs).Error; err != nil {
			return err
		}
		if err := tx.Where("spec_id = ?", old.ID).Delete(&model.InvProductUnit{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.InvProductSpec{}, old.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// saveUnits 在事务内覆盖一个规格下的计量单位。
//
// 与 saveSpecs 同一口径：按单位名「比对后增删改」，而不是全删重建。
// 单位行虽然不承载库存，但它的 ID 被单据明细的 unit_id 引用着，
// 而单据是**可修改**的 —— 全删重建的后果是「昨天下的单，今天改了一次
// 商品档案」，那张单再打开时明细里的单位 ID 已经不存在，
// 保存就被 resolveSpecUnits 以「规格下不存在所选计量单位」打回。
// 保留 ID 才能让历史单据始终可回看、可修改。
func (s *Service) saveUnits(tx *gorm.DB, specID int64, reqs []ProductUnitReq) error {
	var existing []model.InvProductUnit
	if err := tx.Where("spec_id = ?", specID).Find(&existing).Error; err != nil {
		return err
	}
	kept := make(map[int64]bool, len(reqs))
	for i, u := range reqs {
		status := model.StatusEnabled
		if u.Status != nil {
			status = *u.Status
		}
		name := strings.TrimSpace(u.Unit)

		var unitID int64
		for _, old := range existing {
			if old.Unit == name && !kept[old.ID] {
				unitID = old.ID
				break
			}
		}
		if unitID > 0 {
			kept[unitID] = true
			if err := tx.Model(&model.InvProductUnit{}).Where("id = ?", unitID).Updates(map[string]any{
				"unit": name, "rate": u.Rate, "is_base": u.IsBase,
				"purchase_price": u.PurchasePrice, "sale_price": u.SalePrice,
				"sort": i, "status": status,
			}).Error; err != nil {
				return err
			}
			continue
		}
		row := model.InvProductUnit{
			SpecID: specID, Unit: name, Rate: u.Rate, IsBase: u.IsBase,
			PurchasePrice: u.PurchasePrice, SalePrice: u.SalePrice,
			Sort: i, Status: status,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		kept[row.ID] = true
	}

	// 被移除的单位：明细里存的是单位名与换算率的快照，没有任何地方靠 ID
	// 反查它，删掉不影响历史纸面。
	del := tx.Where("spec_id = ?", specID)
	if len(kept) > 0 {
		ids := make([]int64, 0, len(kept))
		for id := range kept {
			ids = append(ids, id)
		}
		del = del.Where("id NOT IN ?", ids)
	}
	return del.Delete(&model.InvProductUnit{}).Error
}

// specInUse 判断规格是否已被单据明细或出入库明细引用。
func (s *Service) specInUse(tx *gorm.DB, specID int64) (bool, error) {
	var cnt int64
	if err := tx.Model(&model.InvOrderItem{}).Where("spec_id = ?", specID).Count(&cnt).Error; err != nil {
		return false, err
	}
	if cnt > 0 {
		return true, nil
	}
	if err := tx.Model(&model.InvStockItem{}).Where("spec_id = ?", specID).Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ProductCreate 新增商品（连同规格与计量单位）。库存不从表单录入 ——
// 它由出入库单据驱动，建档时一律从 0 起，避免「档案上的库存」与
// 「单据算出来的库存」两套账。
func (s *Service) ProductCreate(req ProductReq) (int64, error) {
	if err := validateSpecs(req.Specs); err != nil {
		return 0, err
	}
	if err := s.productCodeFree(req.Code, 0); err != nil {
		return 0, err
	}
	if err := s.categoryExists(req.CategoryID); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	it := model.InvProduct{
		Code: req.Code, Name: req.Name, CategoryID: req.CategoryID,
		Status: status, Remark: req.Remark,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&it).Error; err != nil {
			return err
		}
		return s.saveSpecs(tx, it.ID, req.Specs)
	})
	if err != nil {
		return 0, saveErr(err)
	}
	return it.ID, nil
}

// ProductUpdate 修改商品。规格与单位整体覆盖，库存字段不受影响。
func (s *Service) ProductUpdate(id int64, req ProductReq) error {
	if err := validateSpecs(req.Specs); err != nil {
		return err
	}
	if err := s.productCodeFree(req.Code, id); err != nil {
		return err
	}
	if err := s.categoryExists(req.CategoryID); err != nil {
		return err
	}
	var cnt int64
	if err := s.db.Model(&model.InvProduct{}).Where("id = ?", id).Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt == 0 {
		return errcode.New(errcode.ErrNotFound, "商品不存在")
	}
	updates := map[string]any{
		"code": req.Code, "name": req.Name, "category_id": req.CategoryID, "remark": req.Remark,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.InvProduct{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return s.saveSpecs(tx, id, req.Specs)
	})
	return saveErr(err)
}

// categoryExists 校验分类存在（0 表示「未分类」，允许）。
func (s *Service) categoryExists(id int64) error {
	if id <= 0 {
		return nil
	}
	var cnt int64
	if err := s.db.Model(&model.InvCategory{}).Where("id = ?", id).Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt == 0 {
		return errcode.New(errcode.ErrBadRequest, "所选商品分类不存在")
	}
	return nil
}

// ProductDelete 删除商品。已被单据引用时禁止删除。
//
// 单据明细虽然存的是商品快照（名称 / 规格 / 单位），删掉档案纸面也还能打出来，
// 但**商品编码来自档案补齐**：档案一旦消失，历史单据与报表就再也对不上
// 「这条明细到底是哪个商品」。因此与往来单位同一口径：有引用就 409。
func (s *Service) ProductDelete(id int64) error {
	var orderCnt, stockCnt int64
	if err := s.db.Model(&model.InvOrderItem{}).Where("product_id = ?", id).
		Count(&orderCnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.db.Model(&model.InvStockItem{}).Where("product_id = ?", id).
		Count(&stockCnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if used := orderCnt + stockCnt; used > 0 {
		return errcode.Newf(errcode.ErrConflict,
			"该商品已被 %d 条单据明细引用，无法删除（可改为停用）", used)
	}
	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		var specIDs []int64
		if err := tx.Model(&model.InvProductSpec{}).Where("product_id = ?", id).
			Pluck("id", &specIDs).Error; err != nil {
			return err
		}
		if len(specIDs) > 0 {
			if err := tx.Where("spec_id IN ?", specIDs).Delete(&model.InvProductUnit{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("product_id = ?", id).Delete(&model.InvProductSpec{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&model.InvProduct{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.New(errcode.ErrNotFound, "商品不存在")
		}
		return nil
	}))
}

// specUnit 是「商品 + 规格 + 计量单位」三元组的解析结果。
type specUnit struct {
	Product model.InvProduct
	Spec    model.InvProductSpec
	Unit    model.InvProductUnit
}

// specUnitRef 是明细行里待解析的三个 ID。
type specUnitRef struct {
	ProductID int64
	SpecID    int64
	UnitID    int64
}

// resolveSpecUnits 校验并解析明细请求里的 商品 / 规格 / 单位 三元组。
//
// 三个 ID 必须构成一条真实存在的链（单位属于该规格、规格属于该商品）。
// 中间任何一环对不上都必须在这里拦住：客户端把下拉串了、或者有人直接
// 构造请求，放过去的结果就是库存被记到一个不属于它的规格上 —— 账面上
// 完全看不出来，要等盘点才发现某个规格莫名其妙多了一批货。
func (s *Service) resolveSpecUnits(refs []specUnitRef) ([]specUnit, error) {
	out := make([]specUnit, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	productIDs := make([]int64, 0, len(refs))
	specIDs := make([]int64, 0, len(refs))
	unitIDs := make([]int64, 0, len(refs))
	for _, r := range refs {
		productIDs = append(productIDs, r.ProductID)
		specIDs = append(specIDs, r.SpecID)
		unitIDs = append(unitIDs, r.UnitID)
	}

	var products []model.InvProduct
	if err := s.db.Where("id IN ?", productIDs).Find(&products).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	productMap := make(map[int64]model.InvProduct, len(products))
	for _, p := range products {
		productMap[p.ID] = p
	}
	var specs []model.InvProductSpec
	if err := s.db.Where("id IN ?", specIDs).Find(&specs).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	specMap := make(map[int64]model.InvProductSpec, len(specs))
	for _, sp := range specs {
		specMap[sp.ID] = sp
	}
	var units []model.InvProductUnit
	if err := s.db.Where("id IN ?", unitIDs).Find(&units).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	unitMap := make(map[int64]model.InvProductUnit, len(units))
	for _, u := range units {
		unitMap[u.ID] = u
	}

	for i, r := range refs {
		p, ok := productMap[r.ProductID]
		if !ok {
			return nil, errcode.Newf(errcode.ErrBadRequest, "商品 %d 不存在", r.ProductID)
		}
		sp, ok := specMap[r.SpecID]
		if !ok || sp.ProductID != p.ID {
			return nil, errcode.Newf(errcode.ErrBadRequest,
				"商品「%s」下不存在所选规格", p.Name)
		}
		u, ok := unitMap[r.UnitID]
		if !ok || u.SpecID != sp.ID {
			return nil, errcode.Newf(errcode.ErrBadRequest,
				"规格「%s」下不存在所选计量单位", sp.Spec)
		}
		// 三层都必须**启用**：停用就是「不再进新单据」的意思，
		// 只查「存在」的话，档案停用了照样能开单，库存会继续记到停用规格上。
		// 下拉（ProductPick）已经过滤过启用项，这里挡的是「页面打开后
		// 管理员停用」这段竞态，以及绕过界面直接调接口的请求。
		if p.Status != model.StatusEnabled {
			return nil, errcode.Newf(errcode.ErrBadRequest, "商品「%s」已停用", p.Name)
		}
		if sp.Status != model.StatusEnabled {
			return nil, errcode.Newf(errcode.ErrBadRequest,
				"规格「%s %s」已停用", p.Name, sp.Spec)
		}
		if u.Status != model.StatusEnabled {
			return nil, errcode.Newf(errcode.ErrBadRequest,
				"计量单位「%s」已停用", u.Unit)
		}
		out[i] = specUnit{Product: p, Spec: sp, Unit: u}
	}
	return out, nil
}

// productByIDs 按 ID 取商品，返回 map 便于明细装配。
func (s *Service) productByIDs(ids []int64) (map[int64]model.InvProduct, error) {
	out := map[int64]model.InvProduct{}
	if len(ids) == 0 {
		return out, nil
	}
	var list []model.InvProduct
	if err := s.db.Where("id IN ?", ids).Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, p := range list {
		out[p.ID] = p
	}
	return out, nil
}

// findParty 取往来单位名称，用于单据冗余存储；不存在时报错。
func (s *Service) findParty(bizType string, id int64) (string, error) {
	var name string
	var err error
	if bizType == model.InvBizPurchase {
		var sp model.InvSupplier
		err = s.db.Select("name").First(&sp, id).Error
		name = sp.Name
	} else {
		var cu model.InvCustomer
		err = s.db.Select("name").First(&cu, id).Error
		name = cu.Name
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errcode.New(errcode.ErrBadRequest, "往来单位不存在或已被删除")
	}
	if err != nil {
		return "", errcode.Wrap(errcode.ErrDatabase, err)
	}
	return name, nil
}
