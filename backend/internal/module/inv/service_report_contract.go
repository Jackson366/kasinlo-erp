package inv

import (
	"strings"
	"time"

	"nova-erp/internal/authz"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// 合同报表：执行情况 + 收付款台账。
//
// 「合同执行」回答的是「这份合同还有多少钱没付/没收、质保金压了多少」；
// 「台账」回答的是「哪一天、按哪一期、实际收付了多少钱」。
// 两者口径必须能对上 —— 台账按 plan_id 汇总后，应当等于执行表里的已收付金额。

// reportContractLimit 执行报表最多返回的合同数。
// 报表是一次性全量渲染（不分页），超过上限给出 truncated 让前端提示收窄筛选。
const reportContractLimit = 1000

// ReportContract 合同执行报表。
func (s *Service) ReportContract(f ContractFilter, q page.Query, actor *authz.Identity) (*ReportContractResult, error) {
	today := contractToday()
	var list []model.InvContract
	if err := s.contractQuery(f, q.Keyword, actor, today).
		Order("sign_date DESC, id DESC").Limit(reportContractLimit + 1).Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}

	res := &ReportContractResult{Rows: make([]ReportContractRow, 0, len(list))}
	if len(list) > reportContractLimit {
		res.Truncated = true
		list = list[:reportContractLimit]
	}

	ids := make([]int64, 0, len(list))
	for _, it := range list {
		ids = append(ids, it.ID)
	}
	plans, err := s.plansByContract(ids)
	if err != nil {
		return nil, err
	}

	for _, it := range list {
		stage, stageLabel := contractStage(it, today)
		planCount, planAmount, settled, retentionOut := planSummary(plans[it.ID])
		res.Rows = append(res.Rows, ReportContractRow{
			ContractID: it.ID, ContractNo: it.ContractNo, Name: it.Name, BizType: it.BizType,
			PartyName: it.PartyName, SignDate: it.SignDate, Amount: round2(it.Amount),
			PlanCount: planCount, PlanAmount: planAmount,
			SettledAmount: settled, Outstanding: round2(planAmount - settled),
			RetentionAmount: round2(it.RetentionAmount), RetentionOutstanding: retentionOut,
			Stage: stage, StageLabel: stageLabel,
			AcceptedAt: it.AcceptedAt, WarrantyEndAt: it.WarrantyEndAt,
		})
		res.Count++
		res.Amount += it.Amount
		res.PlanAmount += planAmount
		res.SettledAmount += settled
		res.RetentionAmount += it.RetentionAmount
		res.RetentionOutstanding += retentionOut
	}
	res.Amount = round2(res.Amount)
	res.PlanAmount = round2(res.PlanAmount)
	res.SettledAmount = round2(res.SettledAmount)
	res.Outstanding = round2(res.PlanAmount - res.SettledAmount)
	res.RetentionAmount = round2(res.RetentionAmount)
	res.RetentionOutstanding = round2(res.RetentionOutstanding)
	return res, nil
}

// ReportContractLedger 合同收付款台账（实际发生的流水，按日期倒序）。
//
// 只取 source=plan 的结算记录：订单结算不属于合同台账，
// 混进来会让「合同收了多少」这个数虚高。
func (s *Service) ReportContractLedger(f ContractFilter, q page.Query, actor *authz.Identity, limit int) ([]ReportContractLedgerRow, error) {
	if limit <= 0 || limit > reportContractLimit {
		limit = reportContractLimit
	}
	var sb strings.Builder
	sb.WriteString(`SELECT st.id AS settle_id, st.settled_at, c.contract_no, c.name,
       c.biz_type, c.party_name, p.seq AS plan_seq, p.name AS plan_name,
       st.amount, st.method, st.remark
  FROM inv_settlements st
  JOIN inv_contract_plans p ON p.id = st.plan_id AND p.deleted_at IS NULL
  JOIN inv_contracts c ON c.id = p.contract_id AND c.deleted_at IS NULL
 WHERE st.deleted_at IS NULL AND st.source = ?`)
	args := []any{model.InvSettleSourcePlan}

	if f.BizType != "" {
		sb.WriteString(" AND c.biz_type = ?")
		args = append(args, f.BizType)
	}
	if f.Status != nil {
		sb.WriteString(" AND c.status = ?")
		args = append(args, *f.Status)
	}
	if f.Start != nil {
		sb.WriteString(" AND st.settled_at >= ?")
		args = append(args, *f.Start)
	}
	if f.End != nil {
		sb.WriteString(" AND st.settled_at <= ?")
		args = append(args, *f.End)
	}
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		sb.WriteString(" AND (c.contract_no ILIKE ? ESCAPE '\\' OR c.name ILIKE ? ESCAPE '\\' OR c.party_name ILIKE ? ESCAPE '\\')")
		args = append(args, kw, kw, kw)
	}
	if cond, a, ok := authz.ResolveScope(actor).CreatorCond("c"); ok {
		sb.WriteString(" AND " + cond)
		args = append(args, a...)
	}
	sb.WriteString(" ORDER BY st.settled_at DESC, st.id DESC LIMIT ?")
	args = append(args, limit)

	type ledgerDBRow struct {
		SettleID   int64
		SettledAt  time.Time
		ContractNo string
		Name       string
		BizType    string
		PartyName  string
		PlanSeq    int
		PlanName   string
		Amount     float64
		Method     string
		Remark     string
	}
	var raw []ledgerDBRow
	if err := s.db.Raw(sb.String(), args...).Scan(&raw).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make([]ReportContractLedgerRow, 0, len(raw))
	for _, r := range raw {
		out = append(out, ReportContractLedgerRow{
			SettleID: r.SettleID, SettledAt: r.SettledAt,
			ContractNo: r.ContractNo, Name: r.Name, BizType: r.BizType, PartyName: r.PartyName,
			PlanSeq: r.PlanSeq, PlanName: r.PlanName,
			Amount: round2(r.Amount), Method: r.Method, Remark: r.Remark,
		})
	}
	return out, nil
}
