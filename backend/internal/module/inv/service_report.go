package inv

import (
	"time"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
)

// doneCond 是「已审核单据」状态值（= model.InvStatusDone）的 SQL 片段。
const doneCond = "status = 2"

// Overview 数据概览：概览页与报表页共用的核心指标。
func (s *Service) Overview() (*OverviewVO, error) {
	var out OverviewVO
	now := time.Now().In(database.Location())
	monthFrom := monthStart()
	activeFrom := now.AddDate(0, 0, -30)

	counts := []struct {
		dest  *int64
		model any
		where string
		args  []any
	}{
		{&out.CustomerCount, &model.InvCustomer{}, "status = ?", []any{model.StatusEnabled}},
		{&out.MonthNewCustomer, &model.InvCustomer{}, "created_at >= ?", []any{monthFrom}},
		{&out.ProductCount, &model.InvProduct{}, "status = ?", []any{model.StatusEnabled}},
		// 预警按**规格**统计：库存记在规格上，同一个商品下的 φ20 缺货、
		// φ25 充足是常态，按商品维度统计只会得出「这个商品有库存」的错误结论。
		// 还要排除「商品已停用」的规格：商品停用后规格仍可能留在启用态，
		// 若不联表校验，预警数会把早就下架的商品也算进去（与 Inventory 的口径一致）。
		{&out.WarnProductCount, &model.InvProductSpec{},
			"status = ? AND warn_stock > 0 AND stock <= warn_stock AND product_id IN (?)",
			[]any{model.StatusEnabled,
				s.db.Model(&model.InvProduct{}).Select("id").Where("status = ?", model.StatusEnabled)}},
		{&out.PendingOrders, &model.InvOrder{}, "status = ?", []any{model.InvStatusPending}},
	}
	for _, c := range counts {
		tx := s.db.Model(c.model)
		if c.where != "" {
			tx = tx.Where(c.where, c.args...)
		}
		if err := tx.Count(c.dest).Error; err != nil {
			return nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
	}

	if err := s.db.Model(&model.InvOrder{}).
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
func (s *Service) ReportSummary(rng string) (*ReportSummary, error) {
	start, label := rangeStart(rng)
	out := ReportSummary{Range: label}

	if err := s.db.Model(&model.InvOrder{}).
		Select("COALESCE(SUM(total_amount),0)").
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", model.InvBizSales, start).
		Scan(&out.SalesAmount).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.db.Model(&model.InvOrder{}).
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", model.InvBizSales, start).
		Count(&out.SalesOrders).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.db.Model(&model.InvOrder{}).
		Select("COALESCE(SUM(total_amount),0)").
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", model.InvBizPurchase, start).
		Scan(&out.Purchases).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 销售成本 = 销售明细的**基本数量** × 该规格基本单位的采购价。
	// 用 qty_base 而不是 qty：明细里的数量是按成交单位记的（2 箱），
	// 直接乘以「每个」的采购价会把成本算成 2 个的钱。
	if err := s.db.Table("inv_order_items AS i").
		Joins("JOIN inv_orders AS o ON o.id = i.order_id AND o.deleted_at IS NULL").
		Joins("JOIN inv_product_specs AS sp ON sp.id = i.spec_id AND sp.deleted_at IS NULL").
		Joins("JOIN inv_product_units AS u ON u.spec_id = sp.id AND u.is_base AND u.deleted_at IS NULL").
		Where("i.deleted_at IS NULL AND o.biz_type = ? AND o.status = ? AND o.order_date >= ?",
			model.InvBizSales, model.InvStatusDone, start).
		Select("COALESCE(SUM(i.qty_base * u.purchase_price),0)").Scan(&out.CostAmount).Error; err != nil {
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
func (s *Service) ReportTrend(days int) ([]ReportTrendPoint, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	zone := database.Location()
	now := time.Now().In(zone)
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -(days - 1))

	type dayRow struct {
		Day string
		Amt float64
	}
	byDay := func(biz string) (map[string]float64, error) {
		var rows []dayRow
		if err := s.db.Model(&model.InvOrder{}).
			Select("TO_CHAR(order_date, 'YYYY-MM-DD') AS day, COALESCE(SUM(total_amount),0) AS amt").
			Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", biz, startDate).
			Group("day").Scan(&rows).Error; err != nil {
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

// ReportRanking 销售排行：按客户统计区间内销售额。
func (s *Service) ReportRanking(rng string, limit int) ([]ReportRankRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	start, _ := rangeStart(rng)
	type row struct {
		PartyID   int64
		PartyName string
		Amount    float64
		Cnt       int64
	}
	var rows []row
	if err := s.db.Model(&model.InvOrder{}).
		Select("party_id, party_name, COALESCE(SUM(total_amount),0) AS amount, COUNT(*) AS cnt").
		Where("biz_type = ? AND "+doneCond+" AND order_date >= ?", model.InvBizSales, start).
		Group("party_id, party_name").Order("amount DESC").Limit(limit).Scan(&rows).Error; err != nil {
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
			Rank: i + 1, PartyID: r.PartyID, PartyName: r.PartyName,
			Amount: round2(r.Amount), OrderCount: r.Cnt, Ratio: ratio,
		})
	}
	return out, nil
}

// ReportCategory 品类销售占比。
func (s *Service) ReportCategory(rng string) ([]ReportCategoryRow, error) {
	start, _ := rangeStart(rng)
	type row struct {
		Category string
		Amount   float64
	}
	var rows []row
	if err := s.db.Table("inv_order_items AS i").
		Joins("JOIN inv_orders AS o ON o.id = i.order_id AND o.deleted_at IS NULL").
		Joins("JOIN inv_products AS p ON p.id = i.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN inv_categories AS c ON c.id = p.category_id AND c.deleted_at IS NULL").
		Where("i.deleted_at IS NULL AND o.biz_type = ? AND o.status = ? AND o.order_date >= ?",
			model.InvBizSales, model.InvStatusDone, start).
		Select("COALESCE(NULLIF(c.name, ''), '未分类') AS category, COALESCE(SUM(i.amount),0) AS amount").
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
