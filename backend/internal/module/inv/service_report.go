package inv

import (
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
)

// doneCond 是「已审核单据」状态值（= model.InvStatusDone）的 SQL 片段。
const doneCond = "status = 2"

// Overview 数据概览：概览页与报表页共用的核心指标。
//
// 口径刻意分成两类：客户数 / 商品数 / 预警数是**共享主数据**的统计（保持全局，
// 与列表一致）；待处理单数 / 活跃客户 / 应收应付 / 月度销售采购是**单据派生**，
// 按数据范围收窄 —— 概览只反映当前身份能看到的那部分经营数据。
func (s *Service) Overview(actor *authz.Identity) (*OverviewVO, error) {
	var out OverviewVO
	now := time.Now().In(database.Location())
	monthFrom := monthStart()
	activeFrom := now.AddDate(0, 0, -30)

	counts := []struct {
		dest  *int64
		model any
		where string
		args  []any
		scope bool // true 表示按数据范围收窄（单据统计）
	}{
		{&out.CustomerCount, &model.InvCustomer{}, "status = ?", []any{model.StatusEnabled}, false},
		{&out.MonthNewCustomer, &model.InvCustomer{}, "created_at >= ?", []any{monthFrom}, false},
		{&out.ProductCount, &model.InvProduct{}, "status = ?", []any{model.StatusEnabled}, false},
		// 预警按**规格**统计：库存记在规格上，同一个商品下的 φ20 缺货、
		// φ25 充足是常态，按商品维度统计只会得出「这个商品有库存」的错误结论。
		// 还要排除「商品已停用」的规格：商品停用后规格仍可能留在启用态，
		// 若不联表校验，预警数会把早就下架的商品也算进去（与 Inventory 的口径一致）。
		{&out.WarnProductCount, &model.InvProductSpec{},
			"status = ? AND warn_stock > 0 AND stock <= warn_stock AND product_id IN (?)",
			[]any{model.StatusEnabled,
				s.db.Model(&model.InvProduct{}).Select("id").Where("status = ?", model.StatusEnabled)}, false},
		{&out.PendingOrders, &model.InvOrder{}, "status = ?", []any{model.InvStatusPending}, true},
	}
	for _, c := range counts {
		tx := s.db.Model(c.model)
		if c.where != "" {
			tx = tx.Where(c.where, c.args...)
		}
		if c.scope {
			tx = s.applyCreator(tx, actor)
		}
		if err := tx.Count(c.dest).Error; err != nil {
			return nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
	}

	if err := s.applyCreator(s.db.Model(&model.InvOrder{}), actor).
		Where("biz_type = ? AND status = ? AND order_date >= ?", model.InvBizSales, model.InvStatusDone, activeFrom).
		Distinct("party_id").Count(&out.ActiveCustomer).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}

	sums := []struct {
		dest  *float64
		biz   string
		extra string
		args  []any
	}{
		{&out.ReceivableTotal, model.InvBizSales, "total_amount - settled_amount", nil},
		{&out.PayableTotal, model.InvBizPurchase, "total_amount - settled_amount", nil},
		{&out.MonthSales, model.InvBizSales, "total_amount", []any{monthFrom}},
		{&out.MonthPurchase, model.InvBizPurchase, "total_amount", []any{monthFrom}},
	}
	for _, q := range sums {
		tx := s.db.Model(&model.InvOrder{}).
			Select("COALESCE(SUM("+q.extra+"),0)").
			Where("biz_type = ? AND status = ?", q.biz, model.InvStatusDone)
		tx = s.applyCreator(tx, actor)
		if len(q.args) > 0 {
			tx = tx.Where("order_date >= ?", q.args...)
		}
		if err := tx.Scan(q.dest).Error; err != nil {
			return nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
		*q.dest = round2(*q.dest)
	}
	return &out, nil
}

// ReportSummary 报表汇总：营业收入 / 销售成本 / 毛利 / 毛利率。
// f 携带时间范围与品类筛选；品类命中自身 + 全部子孙分类。
func (s *Service) ReportSummary(f ReportFilter, actor *authz.Identity) (*ReportSummary, error) {
	start, label, err := f.start()
	if err != nil {
		return nil, err
	}
	end, err := f.end()
	if err != nil {
		return nil, err
	}
	out := ReportSummary{Range: label}
	if f.CategoryID > 0 {
		out.Range = label + " · 已按品类筛选"
	}

	salesTx := s.db.Model(&model.InvOrder{}).
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", model.InvBizSales, start)
	purchTx := s.db.Model(&model.InvOrder{}).
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", model.InvBizPurchase, start)
	salesTx = s.applyCreator(salesTx, actor)
	purchTx = s.applyCreator(purchTx, actor)
	if end != nil {
		salesTx = salesTx.Where("order_date <= ?", *end)
		purchTx = purchTx.Where("order_date <= ?", *end)
	}
	// 品类筛选要下到明细：单据主表上没有品类，只有明细行能关联到商品 → 分类
	if f.CategoryID > 0 {
		cond, args, err := s.categoryScope(f.CategoryID, "p.category_id")
		if err != nil {
			return nil, err
		}
		sub := s.db.Model(&model.InvOrderItem{}).
			Joins("JOIN inv_products AS p ON p.id = inv_order_items.product_id AND p.deleted_at IS NULL").
			Where("inv_order_items.order_id = inv_orders.id AND "+cond, args...)
		salesTx = salesTx.Where("EXISTS (?)", sub)
		purchTx = purchTx.Where("EXISTS (?)", sub)
	}

	if err := salesTx.Select("COALESCE(SUM(total_amount),0)").Scan(&out.SalesAmount).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := salesTx.Count(&out.SalesOrders).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := purchTx.Count(&out.PurchaseOrders).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := purchTx.Select("COALESCE(SUM(total_amount),0)").Scan(&out.Purchases).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 销售成本 = 销售明细的**基本数量** × 该规格基本单位的采购价。
	// 用 qty_base 而不是 qty：明细里的数量是按成交单位记的（2 箱），
	// 直接乘以「每个」的采购价会把成本算成 2 个的钱。
	costTx := s.db.Table("inv_order_items AS i").
		Joins("JOIN inv_orders AS o ON o.id = i.order_id AND o.deleted_at IS NULL").
		Joins("JOIN inv_product_specs AS sp ON sp.id = i.spec_id AND sp.deleted_at IS NULL").
		Joins("JOIN inv_product_units AS u ON u.spec_id = sp.id AND u.is_base AND u.deleted_at IS NULL").
		Joins("JOIN inv_products AS p ON p.id = i.product_id AND p.deleted_at IS NULL").
		Where("i.deleted_at IS NULL AND o.biz_type = ? AND o.status = ? AND o.order_date >= ?",
			model.InvBizSales, model.InvStatusDone, start)
	costTx = s.applyCreatorAs(costTx, actor, "o")
	if end != nil {
		costTx = costTx.Where("o.order_date <= ?", *end)
	}
	if f.CategoryID > 0 {
		cond, args, err := s.categoryScope(f.CategoryID, "p.category_id")
		if err != nil {
			return nil, err
		}
		costTx = costTx.Where(cond, args...)
	}
	if err := costTx.Select("COALESCE(SUM(i.qty_base * u.purchase_price),0)").
		Scan(&out.CostAmount).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out.SalesAmount = round2(out.SalesAmount)
	out.Purchases = round2(out.Purchases)
	out.CostAmount = round2(out.CostAmount)
	out.GrossProfit = round2(out.SalesAmount - out.CostAmount)
	if out.SalesAmount > 0 {
		out.GrossRate = round2(out.GrossProfit / out.SalesAmount * 100)
	}
	return &out, nil
}

// ReportTrend 近 days 天的销售 / 采购趋势（按单据日期分桶）。
func (s *Service) ReportTrend(days int, f ReportFilter, actor *authz.Identity) ([]ReportTrendPoint, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	zone := database.Location()
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	startDate := today.AddDate(0, 0, -(days - 1))
	// 自定义区间时，**天数由区间自身决定**，不能沿用 days 参数：
	// 只用 from 时算到今天，from + to 都给了就严格按 to 收口。
	// 少这一步会画出未来日期的空点（今天才 14 号，图却画到 16 号），
	// 而且后端 days 上限 90 会把长区间静默截断。
	if f.From != "" {
		t, err := parseDate(f.From)
		if err != nil {
			return nil, err
		}
		startDate = t
		until := today
		if f.To != "" {
			e, err := parseDate(f.To)
			if err != nil {
				return nil, err
			}
			until = e
		}
		days = int(until.Sub(startDate).Hours()/24) + 1
		if days < 1 {
			days = 1
		}
		if days > 90 {
			days = 90
		}
	}
	var endPtr *time.Time
	if f.To != "" {
		t, err := parseDate(f.To)
		if err != nil {
			return nil, err
		}
		e := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, zone)
		endPtr = &e
	}

	type dayRow struct {
		Day string
		Amt float64
	}
	byDay := func(biz string) (map[string]float64, error) {
		tx := s.db.Model(&model.InvOrder{}).
			Select("TO_CHAR(order_date, 'YYYY-MM-DD') AS day, COALESCE(SUM(total_amount),0) AS amt").
			Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", biz, startDate)
		tx = s.applyCreator(tx, actor)
		if endPtr != nil {
			tx = tx.Where("order_date <= ?", *endPtr)
		}
		if f.CategoryID > 0 {
			cond, args, err := s.categoryScope(f.CategoryID, "p.category_id")
			if err != nil {
				return nil, err
			}
			sub := s.db.Model(&model.InvOrderItem{}).
				Joins("JOIN inv_products AS p ON p.id = inv_order_items.product_id AND p.deleted_at IS NULL").
				Where("inv_order_items.order_id = inv_orders.id AND "+cond, args...)
			tx = tx.Where("EXISTS (?)", sub)
		}
		var rows []dayRow
		if err := tx.Group("day").Scan(&rows).Error; err != nil {
			return nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
		out := make(map[string]float64, len(rows))
		for _, r := range rows {
			out[r.Day] = round2(r.Amt)
		}
		return out, nil
	}
	sales, err := byDay(model.InvBizSales)
	if err != nil {
		return nil, err
	}
	purchases, err := byDay(model.InvBizPurchase)
	if err != nil {
		return nil, err
	}
	out := make([]ReportTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		d := startDate.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, ReportTrendPoint{Date: d, SalesAmount: sales[d], PurchaseAmount: purchases[d]})
	}
	return out, nil
}

// ReportRanking 排行：按客户 / 供应商 / 商品统计区间内的销售（或采购）额。
//
// dimension 取 customer（按交易对手）或 product（按商品明细）；
// biz 取 sales（销售排行）或 purchase（采购排行，交易对手即供应商）。
func (s *Service) ReportRanking(f ReportFilter, dimension, biz string, limit int, actor *authz.Identity) ([]ReportRankRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	if biz != model.InvBizSales && biz != model.InvBizPurchase {
		biz = model.InvBizSales
	}
	start, _, err := f.start()
	if err != nil {
		return nil, err
	}
	end, err := f.end()
	if err != nil {
		return nil, err
	}
	if dimension == "product" {
		return s.reportProductRanking(f, biz, start, end, limit, actor)
	}
	return s.reportPartyRanking(f, biz, start, end, limit, actor)
}

// reportPartyRanking 按交易对手（客户 / 供应商）聚合排行。
func (s *Service) reportPartyRanking(f ReportFilter, biz string, start time.Time, end *time.Time, limit int, actor *authz.Identity) ([]ReportRankRow, error) {
	type row struct {
		PartyID   int64
		PartyName string
		Amount    float64
		Cnt       int64
	}
	var rows []row
	tx := s.db.Model(&model.InvOrder{}).
		Select("party_id, party_name, COALESCE(SUM(total_amount),0) AS amount, COUNT(*) AS cnt").
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", biz, start)
	tx = s.applyCreator(tx, actor)
	if end != nil {
		tx = tx.Where("order_date <= ?", *end)
	}
	if f.CategoryID > 0 {
		cond, args, err := s.categoryScope(f.CategoryID, "p.category_id")
		if err != nil {
			return nil, err
		}
		sub := s.db.Model(&model.InvOrderItem{}).
			Joins("JOIN inv_products AS p ON p.id = inv_order_items.product_id AND p.deleted_at IS NULL").
			Where("inv_order_items.order_id = inv_orders.id AND "+cond, args...)
		tx = tx.Where("EXISTS (?)", sub)
	}
	if err := tx.Group("party_id, party_name").Order("amount DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var sum float64
	for _, r := range rows {
		sum += r.Amount
	}
	out := make([]ReportRankRow, 0, len(rows))
	for i, r := range rows {
		ratio := 0.0
		if sum > 0 {
			ratio = round2(r.Amount / sum * 100)
		}
		out = append(out, ReportRankRow{
			Rank: i + 1, Name: r.PartyName, ID: r.PartyID,
			Amount: round2(r.Amount), OrderCount: r.Cnt, Ratio: ratio,
		})
	}
	return out, nil
}

// reportProductRanking 按商品明细聚合排行。
//
// 名称用明细里的 ProductName 快照而不是联表取当前档案名：商品档案可能被删，
// 联表过滤后那部分销售会从排行里凭空消失，金额对不上。品类筛选时才联表
// （用户主动按品类筛，本就需要商品档案的 category_id，删了的商品归不进任何品类是合理的）。
func (s *Service) reportProductRanking(f ReportFilter, biz string, start time.Time, end *time.Time, limit int, actor *authz.Identity) ([]ReportRankRow, error) {
	type row struct {
		ProductID   int64
		ProductName string
		Amount      float64
		Cnt         int64
		Qty         float64
	}
	var rows []row
	tx := s.db.Table("inv_order_items AS i").
		Joins("JOIN inv_orders AS o ON o.id = i.order_id AND o.deleted_at IS NULL").
		Where("i.deleted_at IS NULL AND o.biz_type = ? AND o.status = ? AND o.order_date >= ?",
			biz, model.InvStatusDone, start).
		Select("i.product_id AS product_id, i.product_name AS product_name, " +
			"COALESCE(SUM(i.amount),0) AS amount, COUNT(DISTINCT i.order_id) AS cnt, " +
			"COALESCE(SUM(i.qty_base),0) AS qty")
	tx = s.applyCreatorAs(tx, actor, "o")
	if end != nil {
		tx = tx.Where("o.order_date <= ?", *end)
	}
	if f.CategoryID > 0 {
		cond, args, err := s.categoryScope(f.CategoryID, "p.category_id")
		if err != nil {
			return nil, err
		}
		tx = tx.Joins("JOIN inv_products AS p ON p.id = i.product_id AND p.deleted_at IS NULL").
			Where(cond, args...)
	}
	if err := tx.Group("i.product_id, i.product_name").Order("amount DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var sum float64
	for _, r := range rows {
		sum += r.Amount
	}
	out := make([]ReportRankRow, 0, len(rows))
	for i, r := range rows {
		ratio := 0.0
		if sum > 0 {
			ratio = round2(r.Amount / sum * 100)
		}
		out = append(out, ReportRankRow{
			Rank: i + 1, Name: r.ProductName, ID: r.ProductID,
			Amount: round2(r.Amount), OrderCount: r.Cnt, Quantity: round2(r.Qty), Ratio: ratio,
		})
	}
	return out, nil
}

// ReportCategory 品类销售占比。
func (s *Service) ReportCategory(f ReportFilter, actor *authz.Identity) ([]ReportCategoryRow, error) {
	start, _, err := f.start()
	if err != nil {
		return nil, err
	}
	end, err := f.end()
	if err != nil {
		return nil, err
	}
	type row struct {
		Category string
		Amount   float64
	}
	var rows []row
	tx := s.db.Table("inv_order_items AS i").
		Joins("JOIN inv_orders AS o ON o.id = i.order_id AND o.deleted_at IS NULL").
		Joins("JOIN inv_products AS p ON p.id = i.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN inv_categories AS c ON c.id = p.category_id AND c.deleted_at IS NULL").
		Where("i.deleted_at IS NULL AND o.biz_type = ? AND o.status = ? AND o.order_date >= ?",
			model.InvBizSales, model.InvStatusDone, start)
	tx = s.applyCreatorAs(tx, actor, "o")
	if end != nil {
		tx = tx.Where("o.order_date <= ?", *end)
	}
	if f.CategoryID > 0 {
		cond, args, err := s.categoryScope(f.CategoryID, "p.category_id")
		if err != nil {
			return nil, err
		}
		tx = tx.Where(cond, args...)
	}
	if err := tx.Select("COALESCE(NULLIF(c.name, ''), '未分类') AS category, COALESCE(SUM(i.amount),0) AS amount").
		Group("category").Order("amount DESC").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var sum float64
	for _, r := range rows {
		sum += r.Amount
	}
	out := make([]ReportCategoryRow, 0, len(rows))
	for _, r := range rows {
		ratio := 0.0
		if sum > 0 {
			ratio = round2(r.Amount / sum * 100)
		}
		out = append(out, ReportCategoryRow{Category: r.Category, Amount: round2(r.Amount), Ratio: ratio})
	}
	return out, nil
}

// ReportInventory 库存分析：库存总金额、规格数、低库存预警数，
// 以及品类库存金额分布与低库存预警清单。
//
// 库存是**共享主数据**（记在规格上，没有「谁建的」语义），
// 与商品 / 客户一致不按数据范围收窄 —— 全店共享一套库存。
func (s *Service) ReportInventory() (*ReportInventory, error) {
	var out ReportInventory

	// 品类库存金额分布：库存成本 = 规格库存 × 基本单位采购价。
	// 只算启用商品 + 启用规格，与 Inventory 列表口径一致。
	type catRow struct {
		Category string
		Value    float64
	}
	var cats []catRow
	if err := s.db.Table("inv_product_specs AS sp").
		Joins("JOIN inv_products AS p ON p.id = sp.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN inv_categories AS c ON c.id = p.category_id AND c.deleted_at IS NULL").
		Joins("LEFT JOIN inv_product_units AS u ON u.spec_id = sp.id AND u.is_base AND u.deleted_at IS NULL").
		Where("sp.deleted_at IS NULL AND p.status = ? AND sp.status = ?",
			model.StatusEnabled, model.StatusEnabled).
		Select("COALESCE(NULLIF(c.name, ''), '未分类') AS category, " +
			"COALESCE(SUM(sp.stock * COALESCE(u.purchase_price, 0)), 0) AS value").
		Group("category").Order("value DESC").Scan(&cats).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var sum float64
	for _, c := range cats {
		sum += c.Value
	}
	out.CategoryValue = make([]InvCategoryValueRow, 0, len(cats))
	for _, c := range cats {
		ratio := 0.0
		if sum > 0 {
			ratio = round2(c.Value / sum * 100)
		}
		out.CategoryValue = append(out.CategoryValue, InvCategoryValueRow{
			Category: c.Category, Value: round2(c.Value), Ratio: ratio,
		})
	}
	out.TotalValue = round2(sum)

	// 启用规格数。预警数的条件是「设了下限且当前库存已触及」，与概览的
	// WarnProductCount 口径一致（下限没设的规格不参与预警）。
	countBase := func() *gorm.DB {
		return s.db.Table("inv_product_specs AS sp").
			Joins("JOIN inv_products AS p ON p.id = sp.product_id AND p.deleted_at IS NULL").
			Where("sp.deleted_at IS NULL AND p.status = ? AND sp.status = ?",
				model.StatusEnabled, model.StatusEnabled)
	}
	if err := countBase().Count(&out.SpecCount).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := countBase().Where("sp.warn_stock > 0 AND sp.stock <= sp.warn_stock").
		Count(&out.WarnCount).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}

	// 低库存预警清单：按缺货程度升序（最缺的在前），取前 20。
	type warnRow struct {
		ProductID   int64
		ProductCode string
		ProductName string
		SpecID      int64
		Spec        string
		Unit        string
		Stock       float64
		WarnStock   float64
	}
	var warns []warnRow
	if err := s.db.Table("inv_product_specs AS sp").
		Joins("JOIN inv_products AS p ON p.id = sp.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN inv_product_units AS u ON u.spec_id = sp.id AND u.is_base AND u.deleted_at IS NULL").
		Where("sp.deleted_at IS NULL AND p.status = ? AND sp.status = ? AND sp.warn_stock > 0 AND sp.stock <= sp.warn_stock",
			model.StatusEnabled, model.StatusEnabled).
		Select("p.id AS product_id, p.code AS product_code, p.name AS product_name, " +
			"sp.id AS spec_id, sp.spec AS spec, COALESCE(u.unit, '') AS unit, " +
			"sp.stock AS stock, sp.warn_stock AS warn_stock").
		Order("(sp.stock - sp.warn_stock) ASC, sp.id ASC").Limit(20).Scan(&warns).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out.WarnList = make([]InvWarnStockRow, 0, len(warns))
	for _, w := range warns {
		out.WarnList = append(out.WarnList, InvWarnStockRow{
			ProductID: w.ProductID, ProductCode: w.ProductCode, ProductName: w.ProductName,
			SpecID: w.SpecID, Spec: w.Spec, Unit: w.Unit,
			Stock: w.Stock, WarnStock: w.WarnStock,
		})
	}
	return &out, nil
}
