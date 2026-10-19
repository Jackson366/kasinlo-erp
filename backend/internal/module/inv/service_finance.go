package inv

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// dueDateOverdueCond 是 SQL 侧的「到期日有效」判定。
//
// 三重条件缺一不可，且与 Go 侧的 isOverdue 保持一致（同一口径两处实现，
// 明细与汇总必须算出同一个数）：
//   - due_date IS NOT NULL：未填到期日的单据不参与逾期（新数据）；
//   - due_date > '1900-01-01'：兜住历史库里被写成 Go 零值 0001-01-01 的行
//     （当时 DueDate 是 time.Time，空串解析出零值后原样落库）。
//     数据库侧的这条防线与启动时的数据修补互为兜底 —— 修补脚本没跑成、
//     或将来又有别的写入路径漏掉，汇总也不会凭空多出一笔「逾期」。
const dueDateOverdueCond = `due_date IS NOT NULL AND due_date > '1900-01-01'`

// isOverdue 判定一笔应收 / 应付是否逾期。口径与 dueDateOverdueCond 一致。
func isOverdue(outstanding float64, due *time.Time, today time.Time) bool {
	if outstanding <= 0 || due == nil || due.IsZero() {
		return false
	}
	// 与 SQL 侧 > '1900-01-01' 对齐，挡住零值时间
	if due.Before(time.Date(1900, 1, 1, 0, 0, 0, 0, database.Location())) {
		return false
	}
	return due.Before(today)
}

// financeList 应收 / 应付明细。bizType 取 sales（应收）或 purchase（应付）。
// 口径：只有「已审核」的单据才形成应收应付（货物已交付 / 已入库）。
func (s *Service) financeList(bizType string, q page.Query, settle string, actor *authz.Identity) ([]FinanceRow, FinanceSummary, int64, error) {
	now := time.Now().In(database.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, database.Location())

	var summary FinanceSummary
	type sumRow struct {
		Total   float64
		Settled float64
	}
	var sr sumRow
	if err := s.applyCreator(s.db.Model(&model.InvOrder{}), actor).
		Select("COALESCE(SUM(total_amount),0) AS total, COALESCE(SUM(settled_amount),0) AS settled").
		Where("biz_type = ? AND status = ?", bizType, model.InvStatusDone).Scan(&sr).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	summary.Total = round2(sr.Total)
	summary.Settled = round2(sr.Settled)
	summary.Outstanding = round2(sr.Total - sr.Settled)

	if err := s.applyCreator(s.db.Model(&model.InvOrder{}), actor).
		Select("COALESCE(SUM(total_amount - settled_amount),0)").
		Where("biz_type = ? AND status = ? AND total_amount - settled_amount > 0 AND "+dueDateOverdueCond+" AND due_date < ?",
			bizType, model.InvStatusDone, today).Scan(&summary.Overdue).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	summary.Overdue = round2(summary.Overdue)

	tx := s.db.Model(&model.InvOrder{}).
		Where("biz_type = ? AND status = ?", bizType, model.InvStatusDone)
	tx = s.applyCreator(tx, actor)
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		tx = tx.Where(likeExpr("order_no")+" OR "+likeExpr("party_name"), kw, kw)
	}
	switch settle {
	case model.InvSettleUnpaid:
		tx = tx.Where("settled_amount <= ?", settleEpsilon)
	case model.InvSettlePaid:
		tx = tx.Where("settled_amount + ? >= total_amount", settleEpsilon)
	case model.InvSettlePartial:
		tx = tx.Where("settled_amount > ? AND settled_amount + ? < total_amount", settleEpsilon, settleEpsilon)
	case "overdue":
		tx = tx.Where("total_amount - settled_amount > ? AND "+dueDateOverdueCond+" AND due_date < ?", settleEpsilon, today)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvOrder
	if err := q.Paginate(tx.Order("due_date ASC NULLS LAST, id DESC")).Find(&list).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	rows := make([]FinanceRow, 0, len(list))
	for _, o := range list {
		outstanding := round2(o.TotalAmount - o.SettledAmount)
		rows = append(rows, FinanceRow{
			OrderID: o.ID, OrderNo: o.OrderNo, PartyID: o.PartyID, PartyName: o.PartyName,
			OrderDate: o.OrderDate, DueDate: o.DueDate, TotalAmount: o.TotalAmount,
			SettledAmount: o.SettledAmount, Outstanding: outstanding,
			SettleStatus: settleStatus(o.TotalAmount, o.SettledAmount),
			Overdue:      isOverdue(outstanding, o.DueDate, today),
		})
	}
	return rows, summary, total, nil
}

// Receivables 应收明细。
func (s *Service) Receivables(q page.Query, settle string, actor *authz.Identity) ([]FinanceRow, FinanceSummary, int64, error) {
	return s.financeList(model.InvBizSales, q, settle, actor)
}

// Payables 应付明细。
func (s *Service) Payables(q page.Query, settle string, actor *authz.Identity) ([]FinanceRow, FinanceSummary, int64, error) {
	return s.financeList(model.InvBizPurchase, q, settle, actor)
}

// Settle 登记收付款：写入结算记录并累加单据已结算金额（同一事务）。
func (s *Service) Settle(req SettleReq, actor *authz.Identity) error {
	var o model.InvOrder
	if err := s.db.First(&o, req.OrderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureCreatorVisible(o.CreatedBy, actor); err != nil {
		return err
	}
	if o.Status != model.InvStatusDone {
		return errcode.New(errcode.ErrConflict, "仅已审核单据可登记收付款")
	}
	// 事务外的校验只是「快速失败 + 友好提示」；真正的防线在事务内的条件更新。
	// 两个并发登记读到同一份 outstanding 都能通过下面的 if，
	// 各自累加就会把 settled_amount 顶到 total_amount 之上 —— 超额结算。
	outstanding := round2(o.TotalAmount - o.SettledAmount)
	if outstanding <= settleEpsilon {
		return errcode.New(errcode.ErrConflict, "该单据已结清")
	}
	if req.Amount > outstanding+settleEpsilon {
		return errcode.Newf(errcode.ErrConflict, "结算金额超出未结余额（剩余 %.2f）", outstanding)
	}
	settledAt, err := parseDate(req.SettledAt)
	if err != nil {
		return err
	}
	if settledAt.IsZero() {
		settledAt = time.Now().In(database.Location())
	}
	method := req.Method
	if method == "" {
		method = "银行转账"
	}
	amount := round2(req.Amount)
	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		rec := model.InvSettlement{
			OrderID: o.ID, BizType: o.BizType, PartyID: o.PartyID,
			Amount: amount, Method: method, SettledAt: settledAt,
			Remark: req.Remark, CreatedBy: actorUID(actor),
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		// 条件更新：把「结算后不能超过总额」放进 WHERE。并发时第二个事务的
		// UPDATE 会等第一个提交后按最新 settled_amount 重新评估条件，
		// 超限则 0 行 → 回滚整个事务（结算记录也一起撤销）。
		res := tx.Model(&model.InvOrder{}).
			Where("id = ? AND settled_amount + ? <= total_amount + ?", o.ID, amount, settleEpsilon).
			UpdateColumn("settled_amount", gorm.Expr("settled_amount + ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.New(errcode.ErrConflict, "结算金额超出未结余额，请刷新后重试")
		}
		return nil
	}))
}
