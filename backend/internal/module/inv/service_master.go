package inv

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
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
//
// 往来单位档案是共享主数据、不套范围；但「余额」是单据的派生数据，
// 必须按当前操作者的数据范围收窄 —— 否则「仅本人/本部门」账号会看到
// 全公司的应收应付。所以这里单独套 applyCreator，主数据列表不受影响。
func (s *Service) partyStats(bizType string, actor *authz.Identity) (map[int64]partyStat, error) {
	type row struct {
		PartyID int64
		Bal     float64
		Cnt     int64
	}
	var rows []row
	tx := s.db.Model(&model.InvOrder{}).
		Select("party_id, COALESCE(SUM(total_amount - settled_amount), 0) AS bal, COUNT(*) AS cnt").
		// 只统计「已审核」单据，与应收应付列表同口径。
		// 之前用 status <> 已取消（把待处理也算了进来），于是客户列表里的余额
		// 与点进去看到的应收明细合计对不上 —— 现场对账第一句就是「你这数不对」。
		Where("biz_type = ? AND status = ?", bizType, model.InvStatusDone)
	tx = s.applyCreator(tx, actor)
	if err := tx.Group("party_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make(map[int64]partyStat, len(rows))
	for _, r := range rows {
		out[r.PartyID] = partyStat{Balance: round2(r.Bal), Count: r.Cnt}
	}
	return out, nil
}

// partyList 客户 / 供应商通用列表。bizType 决定余额口径与冗余名称来源。
func (s *Service) partyList(bizType string, q page.Query, status *int16, actor *authz.Identity) ([]PartyRow, int64, error) {
	var (
		rows  []PartyRow
		total int64
	)
	stats, err := s.partyStats(bizType, actor)
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
func (s *Service) PartyList(bizType string, q page.Query, status *int16, actor *authz.Identity) ([]PartyRow, int64, error) {
	return s.partyList(bizType, q, status, actor)
}

// partyNameFree 校验编号唯一（存活数据内）。
//
// 必须带 deleted_at IS NULL：Table() 不带模型，GORM 不会自动补软删除条件，
// 否则「删掉再用同一编号新建」会被判成重复 —— 而库里的部分唯一索引
// （WHERE deleted_at IS NULL）本来就是允许复用的，代码与索引口径要一致。
func (s *Service) partyNameFree(bizType, code string, excludeID int64) error {
	var cnt int64
	tx := s.db.Table(partyTable(bizType)).Where("code = ? AND deleted_at IS NULL", code)
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

// categoryChildCounts 统计每个分类的**直接子分类**数，删除校验要用。
func (s *Service) categoryChildCounts() (map[int64]int64, error) {
	type row struct {
		ParentID int64
		Cnt      int64
	}
	var rows []row
	if err := s.db.Model(&model.InvCategory{}).
		Select("parent_id, COUNT(*) AS cnt").
		Where("parent_id > 0").Group("parent_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make(map[int64]int64, len(rows))
	for _, r := range rows {
		out[r.ParentID] = r.Cnt
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

// CategoryList 商品分类列表。返回**平表**（含 parent_id），树由前端组装 ——
// 后端不猜前端默认展开到第几层，平表也方便筛选与分页。
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
	if err := q.Paginate(tx.Order("parent_id ASC, sort ASC, id ASC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	counts, err := s.categoryRefCounts()
	if err != nil {
		return nil, 0, err
	}
	children, err := s.categoryChildCounts()
	if err != nil {
		return nil, 0, err
	}
	rows := make([]CategoryRow, 0, len(list))
	for _, it := range list {
		rows = append(rows, CategoryRow{
			ID: it.ID, ParentID: it.ParentID, Name: it.Name, Sort: it.Sort,
			Status: it.Status, Remark: it.Remark,
			ProductCount: counts[it.ID], ChildCount: children[it.ID], CreatedAt: it.CreatedAt,
		})
	}
	return rows, total, nil
}

// CategoryOptions 启用中的分类，供商品表单的下拉使用。
// 带 parent_id，前端可渲染成树形下拉；同时把 currentID 那一项也取回来 ——
// 否则编辑一个已停用分类下的商品时，该分类从下拉里消失，表单显示成「未分类」。
func (s *Service) CategoryOptions(currentID int64) ([]CategoryOption, error) {
	var list []model.InvCategory
	if err := s.db.Model(&model.InvCategory{}).
		Where("status = ? OR id = ?", model.StatusEnabled, currentID).
		Order("parent_id ASC, sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make([]CategoryOption, 0, len(list))
	for _, it := range list {
		out = append(out, CategoryOption{ID: it.ID, ParentID: it.ParentID, Name: it.Name})
	}
	return out, nil
}

// categoryNameFree 校验**同级**分类名称唯一（存活数据内）。
//
// 刻意不做全局唯一：不同父级下的「标准件」是两个合法分类
// （五金下的标准件 vs 机电下的标准件），全局唯一会把多级树逼回一级。
func (s *Service) categoryNameFree(name string, parentID, excludeID int64) error {
	var cnt int64
	tx := s.db.Model(&model.InvCategory{}).Where("name = ? AND parent_id = ?", name, parentID)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "同级分类「%s」已存在", name)
	}
	return nil
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

// categoryMaxDepth 分类树的防环/下钻深度上限，两处共用：
//   - categoryCycleFree（写路径）：改 parent_id 时沿父链上走，遇到自己即 40000；
//   - categoryScope（读路径）：报表品类筛选沿子孙下钻。
//
// 取 100 而不是 10：正常业务分类只有 2~3 层，100 早已远超实际；这个上限的
// 真正用途是给「脏数据成环」一个终止条件。写成 10 的话，一旦出现异常深的
// 合法分类，报表下钻会**静默截断**第 10 层以下 —— 数据少了却没有任何报错。
const categoryMaxDepth = 100

// categoryCycleFree 校验不会把分类挂到自己或自己的子孙下面。
//
// 这一步不能省：parent_id 一旦成环，前端按 parent_id 组装树会无限递归，
// 页面直接卡死。环只能在服务端拦 —— 在下拉里过滤掉自己不等于拦住了环
// （绕过界面直接调接口一样能造出环来）。
func (s *Service) categoryCycleFree(id, parentID int64) error {
	if parentID == id && id != 0 {
		return errcode.New(errcode.ErrBadRequest, "上级分类不能是自己")
	}
	if parentID <= 0 || id <= 0 {
		return nil
	}
	// 从 parentID 沿父链往上走，走到根（0）之前若遇到 id，说明成环
	cur := parentID
	for i := 0; i < categoryMaxDepth && cur > 0; i++ {
		var c model.InvCategory
		if err := s.db.Select("id", "parent_id").First(&c, cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.New(errcode.ErrBadRequest, "上级分类不存在")
			}
			return errcode.Wrap(errcode.ErrDatabase, err)
		}
		if c.ID == id {
			return errcode.New(errcode.ErrBadRequest, "上级分类不能是自己的子分类")
		}
		cur = c.ParentID
	}
	return nil
}

// CategoryCreate 新增商品分类。
func (s *Service) CategoryCreate(req CategoryReq) (int64, error) {
	if err := s.categoryNameFree(req.Name, req.ParentID, 0); err != nil {
		return 0, err
	}
	if err := s.categoryExists(req.ParentID); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	it := model.InvCategory{
		ParentID: req.ParentID, Name: req.Name, Sort: req.Sort, Status: status, Remark: req.Remark,
	}
	if err := s.db.Create(&it).Error; err != nil {
		return 0, saveErr(err)
	}
	return it.ID, nil
}

// CategoryUpdate 修改商品分类。
func (s *Service) CategoryUpdate(id int64, req CategoryReq) error {
	if err := s.categoryNameFree(req.Name, req.ParentID, id); err != nil {
		return err
	}
	if err := s.categoryExists(req.ParentID); err != nil {
		return err
	}
	if err := s.categoryCycleFree(id, req.ParentID); err != nil {
		return err
	}
	updates := map[string]any{
		"parent_id": req.ParentID, "name": req.Name, "sort": req.Sort, "remark": req.Remark,
	}
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

// CategoryDelete 删除商品分类。仍被商品引用、或还有子分类时禁止删除 ——
// 删掉之后那些商品的分类会变成一个悬空 ID，列表里显示为空、报表品类占比凭空少一块；
// 有子分类时更糟：子分类会集体变成根节点，树形结构凭空塌一层。
func (s *Service) CategoryDelete(id int64) error {
	var cnt int64
	if err := s.db.Model(&model.InvProduct{}).Where("category_id = ?", id).
		Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "该分类下还有 %d 个商品，无法删除（可改为停用）", cnt)
	}
	var childCnt int64
	if err := s.db.Model(&model.InvCategory{}).Where("parent_id = ?", id).
		Count(&childCnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if childCnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "该分类下还有 %d 个子分类，请先删除或移走子分类", childCnt)
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
			Warn:   sp.Status == model.StatusEnabled && sp.WarnStock > 0 && sp.Stock <= sp.WarnStock,
			Status: sp.Status,
			Units:  make([]ProductUnitVO, 0, len(units[sp.ID])),
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
		tx = tx.Where(`code ILIKE ? ESCAPE '\' OR name ILIKE ? ESCAPE '\' OR barcode ILIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM inv_product_specs sp
			WHERE sp.product_id = inv_products.id AND sp.deleted_at IS NULL AND sp.spec ILIKE ? ESCAPE '\')`,
			likeKW(q.Keyword), likeKW(q.Keyword), likeKW(q.Keyword), likeKW(q.Keyword))
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
			ID: it.ID, Code: it.Code, Name: it.Name, Barcode: it.Barcode,
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
		ID: it.ID, Code: it.Code, Name: it.Name, Barcode: it.Barcode,
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
//
// 定 1000 而不是 500：开单时明细行要靠**本地模糊匹配**（el-select 的
// filterable）即时过滤，选项太少会把「搜不到」变成家常便饭。
// 1000 个 option 的渲染在几十毫秒量级，换来的是不用每次按键都发请求。
const pickLimit = 1000

// ProductPick 商品 → 规格 → 单位 的级联选项，供单据明细的三级下拉使用。
// 停用的商品 / 规格 / 单位一律不出现在可选项里。
func (s *Service) ProductPick(keyword string) (*ProductPickResult, error) {
	tx := s.db.Model(&model.InvProduct{}).Where("status = ?", model.StatusEnabled)
	if keyword != "" {
		tx = tx.Where(`code ILIKE ? ESCAPE '\' OR name ILIKE ? ESCAPE '\' OR barcode ILIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM inv_product_specs sp
			WHERE sp.product_id = inv_products.id AND sp.deleted_at IS NULL AND sp.spec ILIKE ? ESCAPE '\')`,
			likeKW(keyword), likeKW(keyword), likeKW(keyword), likeKW(keyword))
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
		pick := ProductPickVO{ID: p.ID, Code: p.Code, Name: p.Name, Barcode: p.Barcode, Specs: []PickSpecVO{}}
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

// 规格与单位没填时的兜底值。
//
// 为什么是「自动补」而不是「允许为空」：规格在业务上是库存的归属层、
// 单位是单据折算的基准 —— 真的允许空，商品会因为「没有规格」而进不了
// 开单的商品下拉（ProductPick 只给有规格的商品），等于建了个查不到、
// 开不了单的僵尸档案。所以做法是**用户不用想，系统自己填**。
const (
	defaultSpecName = "默认"
	defaultUnitName = "个"
)

func defaultUnitReq() ProductUnitReq {
	return ProductUnitReq{Unit: defaultUnitName, Rate: 1, IsBase: true}
}

// normalizeSpecs 把用户没填的部分补成结构自洽的默认值。
//
// 这个函数让「规格 / 单位」从必填项降级为可选项：录一个「办公用品-签字笔」时
// 不必先想清楚规格和计量单位，系统会补上「默认 / 个」。
// 归一化之后 validateSpecs 只需面对干净数据、只管真正的错误（重名）。
//
// 四类兜底：
//  1. 完全没有规格 → 补一条「默认」规格 + 基本单位「个」；
//  2. 规格名为空 → 「默认」；
//  3. 规格下没有单位 / 单位为空白 / 换算率 <= 0 → 补「个」、rate 取 1；
//  4. 基本单位不是「恰一个」→ 没指定就把第一个提为基本单位，
//     指定了多个只留第一个（基本单位必须唯一，否则库存没有计数基准）。
func normalizeSpecs(reqs []ProductSpecReq) []ProductSpecReq {
	if len(reqs) == 0 {
		return []ProductSpecReq{{
			Spec:  defaultSpecName,
			Units: []ProductUnitReq{defaultUnitReq()},
		}}
	}
	out := make([]ProductSpecReq, 0, len(reqs))
	for _, sp := range reqs {
		name := strings.TrimSpace(sp.Spec)
		if name == "" {
			name = defaultSpecName
		}
		units := sp.Units
		if len(units) == 0 {
			units = []ProductUnitReq{defaultUnitReq()}
		}
		fixed := make([]ProductUnitReq, 0, len(units))
		for _, u := range units {
			un := strings.TrimSpace(u.Unit)
			if un == "" {
				un = defaultUnitName
			}
			rate := u.Rate
			if rate <= 0 {
				rate = 1
			}
			fixed = append(fixed, ProductUnitReq{
				Unit: un, Rate: rate, IsBase: u.IsBase,
				PurchasePrice: u.PurchasePrice, SalePrice: u.SalePrice, Status: u.Status,
			})
		}
		baseIdx := -1
		for i := range fixed {
			if fixed[i].IsBase {
				baseIdx = i
				break
			}
		}
		if baseIdx < 0 {
			baseIdx = 0
		}
		for i := range fixed {
			fixed[i].IsBase = i == baseIdx
			// 基本单位的换算率恒为 1（「1 个 = 1 个」），顺手钉死，
			// 免得留下「基本单位 rate = 3」这种自相矛盾的数据
			if i == baseIdx {
				fixed[i].Rate = 1
			}
		}
		out = append(out, ProductSpecReq{
			ID: sp.ID, Spec: name, WarnStock: sp.WarnStock, Status: sp.Status, Units: fixed,
		})
	}
	return out
}

// validateSpecs 校验归一化之后的规格与单位，只负责**真正的错误**。
//
// 唯一的硬约束是「不能重名」：
//   - 两行「φ20」会把库存劈成两半，各自都不预警；
//   - 同一规格下两个「箱」会让单据选单位时不知道看哪一行的价格。
//
// 缺规格、缺单位、缺基本单位、基本单位换算率不是 1 —— 这些都由
// normalizeSpecs 兜底，不在这里报错（它们不是用户的错，是系统该补的）。
func validateSpecs(reqs []ProductSpecReq) error {
	seenSpec := map[string]bool{}
	for _, sp := range reqs {
		spec := strings.TrimSpace(sp.Spec)
		if seenSpec[spec] {
			return errcode.Newf(errcode.ErrBadRequest, "规格「%s」重复", spec)
		}
		seenSpec[spec] = true

		seenUnit := map[string]bool{}
		for _, u := range sp.Units {
			unit := strings.TrimSpace(u.Unit)
			if seenUnit[unit] {
				return errcode.Newf(errcode.ErrBadRequest, "规格「%s」下的单位「%s」重复", spec, unit)
			}
			seenUnit[unit] = true
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
	// 先归一化再校验：规格 / 单位是**可选项**，缺的由 normalizeSpecs 补齐，
	// validateSpecs 只处理补完之后仍然存在的真错误（重名）
	req.Specs = normalizeSpecs(req.Specs)
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
		Code: req.Code, Name: req.Name, Barcode: req.Barcode, CategoryID: req.CategoryID,
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
	req.Specs = normalizeSpecs(req.Specs)
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
		"code": req.Code, "name": req.Name, "barcode": req.Barcode,
		"category_id": req.CategoryID, "remark": req.Remark,
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
