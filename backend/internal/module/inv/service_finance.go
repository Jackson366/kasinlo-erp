package inv

import (
	"errors"
	"strings"
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

// financeUnionSQL 应收 / 应付的两类来源。
//
// 一类是已审核单据（发货 / 入库即形成债权债务），一类是合同付款期次
// （签了合同就按约定分期形成应收应付）。两者字段口径一致，
// 用 UNION ALL 合成一张「应收应付行」再由外层统一筛选、排序、分页 ——
// 分开查两次再在内存里合并排序，分页就再也算不准总数。
//
// 占位符顺序：bizType, 已审核状态, source, bizType, source。
//
// 合同期次那条的「待审核(0) / 已终止(3)」写成了字面量而不是占位符：
// IN (0, 3) 是合同状态编码的一部分，跟 InvContractPending 常量保持一致，
// 不在 SQL 里再传参 —— 少一个占位符就少一次「参数错位」的机会。
//
// 期次那条还带 `p.amount > 0`：0 元期次（例如比例填了 0 的占位行）会在应收列表里
// 变成永远 0 的噪音行，点了也收不到钱。
const financeUnionSQL = `
SELECT 'order' AS source, o.id AS order_id, o.order_no AS order_no,
       0::bigint AS contract_id, ''::text AS contract_no,
       0::bigint AS plan_id, 0 AS plan_seq, ''::text AS plan_name,
       o.party_id AS party_id, o.party_name AS party_name,
       o.order_date AS order_date, o.due_date AS due_date,
       o.total_amount AS total_amount, o.settled_amount AS settled_amount,
       o.created_by AS created_by
  FROM inv_orders o
 WHERE o.deleted_at IS NULL AND o.biz_type = ? AND o.status = ? AND ? <> 'plan'
UNION ALL
SELECT 'plan' AS source, 0::bigint AS order_id, ''::text AS order_no,
       c.id AS contract_id, c.contract_no AS contract_no,
       p.id AS plan_id, p.seq AS plan_seq, p.name AS plan_name,
       p.party_id AS party_id, p.party_name AS party_name,
       c.sign_date AS order_date, p.due_date AS due_date,
       p.amount AS total_amount, p.settled_amount AS settled_amount,
       c.created_by AS created_by
  FROM inv_contract_plans p
  JOIN inv_contracts c ON c.id = p.contract_id AND c.deleted_at IS NULL
 WHERE p.deleted_at IS NULL AND p.biz_type = ? AND c.status NOT IN (0, 3) AND ? <> 'order'
   AND p.amount > 0
`

// financeUnionArgs 上面那段 SQL 的参数。
func financeUnionArgs(bizType, source string) []any {
	return []any{bizType, model.InvStatusDone, source, bizType, source}
}

// financeWhere 外层筛选条件（数据范围 / 关键字 / 结算状态）。
//
// 汇总与明细共用同一段条件：两处各写一遍，迟早出现「明细筛出 5 条、
// 汇总却是全表」的错账，而对账时最不该怀疑的就是这两个数。
func (s *Service) financeWhere(actor *authz.Identity, q page.Query, settle string, today time.Time) (string, []any) {
	var sb strings.Builder
	var args []any
	if cond, a, ok := authz.ResolveScope(actor).CreatorCond("t"); ok {
		sb.WriteString(" AND " + cond)
		args = append(args, a...)
	}
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		// 合同期次要能按合同号与期次名搜到（「HT0007 验收款」）
		sb.WriteString(" AND (" + likeExpr("t.order_no") + " OR " + likeExpr("t.party_name") +
			" OR " + likeExpr("t.contract_no") + " OR " + likeExpr("t.plan_name") + ")")
		args = append(args, kw, kw, kw, kw)
	}
	switch settle {
	case model.InvSettleUnpaid:
		sb.WriteString(" AND t.settled_amount <= ?")
		args = append(args, settleEpsilon)
	case model.InvSettlePaid:
		sb.WriteString(" AND t.settled_amount + ? >= t.total_amount")
		args = append(args, settleEpsilon)
	case model.InvSettlePartial:
		sb.WriteString(" AND t.settled_amount > ? AND t.settled_amount + ? < t.total_amount")
		args = append(args, settleEpsilon, settleEpsilon)
	case "overdue":
		sb.WriteString(" AND t.total_amount - t.settled_amount > ? AND " + dueDateOverdueCond + " AND t.due_date < ?")
		args = append(args, settleEpsilon, today)
	}
	return sb.String(), args
}

// financeList 应收 / 应付明细。bizType 取 sales（应收）或 purchase（应付），
// source 取 "" 全部 / order 单据 / plan 合同期次。
// 口径：订单只有「已审核」才形成应收应付；合同期次只要合同未终止就计入。
func (s *Service) financeList(bizType string, q page.Query, settle, source string, actor *authz.Identity) ([]FinanceRow, FinanceSummary, int64, error) {
	now := time.Now().In(database.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, database.Location())

	where, whereArgs := s.financeWhere(actor, q, settle, today)
	unionArgs := financeUnionArgs(bizType, source)

	var summary FinanceSummary
	type sumRow struct {
		Total      float64
		Settled    float64
		OrderTotal float64
		PlanTotal  float64
		Overdue    float64
	}
	var sr sumRow
	// 汇总列与筛选参数的位置：CASE 里的两个占位符在 WHERE 之前
	sumSQL := `SELECT COALESCE(SUM(t.total_amount),0) AS total,
       COALESCE(SUM(t.settled_amount),0) AS settled,
       COALESCE(SUM(CASE WHEN t.source = 'order' THEN t.total_amount ELSE 0 END),0) AS order_total,
       COALESCE(SUM(CASE WHEN t.source = 'plan'  THEN t.total_amount ELSE 0 END),0) AS plan_total,
       COALESCE(SUM(CASE WHEN t.total_amount - t.settled_amount > ? AND ` + dueDateOverdueCond +
		` AND t.due_date < ? THEN t.total_amount - t.settled_amount ELSE 0 END),0) AS overdue
  FROM (` + financeUnionSQL + `) t WHERE 1=1` + where
	sumArgs := append([]any{settleEpsilon, today}, unionArgs...)
	sumArgs = append(sumArgs, whereArgs...)
	if err := s.db.Raw(sumSQL, sumArgs...).Scan(&sr).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	summary.Total = round2(sr.Total)
	summary.Settled = round2(sr.Settled)
	summary.Outstanding = round2(sr.Total - sr.Settled)
	summary.Overdue = round2(sr.Overdue)
	summary.OrderTotal = round2(sr.OrderTotal)
	summary.PlanTotal = round2(sr.PlanTotal)

	countSQL := `SELECT COUNT(*) FROM (` + financeUnionSQL + `) t WHERE 1=1` + where
	countArgs := append(append([]any{}, unionArgs...), whereArgs...)
	var total int64
	if err := s.db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	listSQL := `SELECT t.* FROM (` + financeUnionSQL + `) t WHERE 1=1` + where +
		` ORDER BY t.due_date ASC NULLS LAST, t.source ASC, t.plan_seq ASC, t.order_date DESC, t.order_id DESC
       LIMIT ? OFFSET ?`
	listArgs := append(append([]any{}, unionArgs...), whereArgs...)
	listArgs = append(listArgs, q.Limit(), q.Offset())

	type financeDBRow struct {
		Source        string
		OrderID       int64
		OrderNo       string
		ContractID    int64
		ContractNo    string
		PlanID        int64
		PlanSeq       int
		PlanName      string
		PartyID       int64
		PartyName     string
		OrderDate     time.Time
		DueDate       *time.Time
		TotalAmount   float64
		SettledAmount float64
	}
	var rowsDB []financeDBRow
	if err := s.db.Raw(listSQL, listArgs...).Scan(&rowsDB).Error; err != nil {
		return nil, summary, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	rows := make([]FinanceRow, 0, len(rowsDB))
	for _, r := range rowsDB {
		outstanding := round2(r.TotalAmount - r.SettledAmount)
		rows = append(rows, FinanceRow{
			Source: r.Source, OrderID: r.OrderID, OrderNo: r.OrderNo,
			ContractID: r.ContractID, ContractNo: r.ContractNo,
			PlanID: r.PlanID, PlanSeq: r.PlanSeq, PlanName: r.PlanName,
			PartyID: r.PartyID, PartyName: r.PartyName,
			OrderDate: r.OrderDate, DueDate: r.DueDate, TotalAmount: round2(r.TotalAmount),
			SettledAmount: round2(r.SettledAmount), Outstanding: outstanding,
			SettleStatus: settleStatus(r.TotalAmount, r.SettledAmount),
			Overdue:      isOverdue(outstanding, r.DueDate, today),
		})
	}
	return rows, summary, total, nil
}

// Receivables 应收明细。
func (s *Service) Receivables(q page.Query, settle, source string, actor *authz.Identity) ([]FinanceRow, FinanceSummary, int64, error) {
	return s.financeList(model.InvBizSales, q, settle, source, actor)
}

// Payables 应付明细。
func (s *Service) Payables(q page.Query, settle, source string, actor *authz.Identity) ([]FinanceRow, FinanceSummary, int64, error) {
	return s.financeList(model.InvBizPurchase, q, settle, source, actor)
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
