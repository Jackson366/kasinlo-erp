package inv

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

// SettleBatch 批量登记收付款：一个事务里给每张单据各记一笔。
//
// 语义与单张结算完全一致（已审核、金额不超未结、条件更新兜底），差别只有两点：
//   - **整批原子**：任何一张不满足条件就整批回滚。批量付款最怕「付了一半」——
//     账上出现一批半结清的单据，比完全没付更难对账；
//   - 默认按每张单据的未结余额结清（见 SettleBatchReq 的说明）。
//
// 行锁用 FOR UPDATE：批量时用户往往连点两下，两批请求并发读同一份余额
// 都会通过校验，只有锁住行、后到的那批重新读到新余额，才不会超额。
func (s *Service) SettleBatch(req SettleBatchReq, actor *authz.Identity) (SettleBatchResult, error) {
	var res SettleBatchResult
	settledAt, err := parseDate(req.SettledAt)
	if err != nil {
		return res, err
	}
	if settledAt.IsZero() {
		settledAt = time.Now().In(database.Location())
	}
	method := req.Method
	if method == "" {
		method = "银行转账"
	}
	// 去重：同一张单据传两次不能写出两笔结算记录
	seen := make(map[int64]bool, len(req.OrderIDs))
	ids := make([]int64, 0, len(req.OrderIDs))
	for _, id := range req.OrderIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	err = saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			var o model.InvOrder
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, id).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errcode.Newf(errcode.ErrNotFound, "单据 %d 不存在", id)
				}
				return err
			}
			if err := s.ensureCreatorVisible(o.CreatedBy, actor); err != nil {
				return err
			}
			if o.Status != model.InvStatusDone {
				return errcode.Newf(errcode.ErrConflict, "单据 %s 未审核，不能登记收付款", o.OrderNo)
			}
			outstanding := round2(o.TotalAmount - o.SettledAmount)
			if outstanding <= settleEpsilon {
				return errcode.Newf(errcode.ErrConflict, "单据 %s 已结清", o.OrderNo)
			}
			amount := outstanding
			if a, ok := req.Amounts[id]; ok {
				amount = round2(a)
				if amount <= settleEpsilon {
					return errcode.Newf(errcode.ErrBadRequest, "单据 %s 的结算金额必须大于 0", o.OrderNo)
				}
				if amount > outstanding+settleEpsilon {
					return errcode.Newf(errcode.ErrConflict, "单据 %s 结算金额超出未结余额（剩余 %.2f）", o.OrderNo, outstanding)
				}
			}
			rec := model.InvSettlement{
				OrderID: o.ID, BizType: o.BizType, PartyID: o.PartyID,
				Amount: amount, Method: method, SettledAt: settledAt,
				Remark: req.Remark, CreatedBy: actorUID(actor),
			}
			if err := tx.Create(&rec).Error; err != nil {
				return err
			}
			upd := tx.Model(&model.InvOrder{}).
				Where("id = ? AND status = ? AND settled_amount + ? <= total_amount + ?",
					o.ID, model.InvStatusDone, amount, settleEpsilon).
				UpdateColumn("settled_amount", gorm.Expr("settled_amount + ?", amount))
			if upd.Error != nil {
				return upd.Error
			}
			if upd.RowsAffected == 0 {
				return errcode.Newf(errcode.ErrConflict, "单据 %s 的余额已被改动，请刷新后重试", o.OrderNo)
			}
			res.Count++
			res.Amount = round2(res.Amount + amount)
		}
		return nil
	}))
	return res, err
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
		//
		// 同时带上 status = 已审核：反审核并发跑时，单据会被退回待处理，
		// 而这里若只校验金额，就会给一张「待处理」单据记上一笔收付款 ——
		// 应收列表按已审核过滤，这笔钱从账上消失，流水却还在。
		res := tx.Model(&model.InvOrder{}).
			Where("id = ? AND status = ? AND settled_amount + ? <= total_amount + ?",
				o.ID, model.InvStatusDone, amount, settleEpsilon).
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
