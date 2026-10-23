package inv

import (
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
)

// 合同付款计划与履约（验收 / 质保 / 期次收付款）。
//
// 这一层实现的是「合同怎么变成钱」：
//
//	签订 ──► 付款计划（N 个期次）──► 期次收付款 ──► 应收 / 应付 ──► 报表
//
// 三个刻意的取舍：
//
//  1. **期次不存状态**。未到期 / 待收付 / 部分 / 已结清全部由金额差额、
//     约定日期与触发节点现算（见 planStatusOf）。存一份状态就要在改金额、
//     改日期、登记收付款、改验收日期四处同步，漏一处就是「已收满却显示待收付」。
//  2. **收付款复用 inv_settlements**。合同期次与订单的记账字段完全一致，
//     分表只会让账龄、台账、按往来单位汇总各写两遍。
//  3. **已发生收付款的期次不可改金额、不可删**。金额一改，
//     已收付累计与合同金额就对不上，而历史结算记录是无法追溯修正的。

// ---------------- 派生状态 ----------------

// contractStage 派生合同的履约阶段。
//
// 不新增 status 码：老的 1 执行中 / 2 已完成 / 3 已终止 含义不变，
// 验收与质保由两个日期派生。这样历史合同不需要数据迁移，
// 也不会出现「状态说质保中、日期说早过期」的自相矛盾。
func contractStage(c model.InvContract, today time.Time) (string, string) {
	if c.Status == model.InvContractPending {
		return "unaudited", "待审核"
	}
	switch c.Status {
	case model.InvContractTerminated:
		return "terminated", "已终止"
	case model.InvContractFinished:
		return "finished", "已完成"
	}
	if c.AcceptedAt == nil {
		return "pending", "待验收"
	}
	if c.WarrantyEndAt == nil {
		return "accepted", "已验收"
	}
	if !c.WarrantyEndAt.After(today) {
		return "warranty_due", "质保到期"
	}
	return "warranty", "质保中"
}

// planDue 判断一期是否已到收付时间。
//
// 触发节点优先：合同里常见的写法是「验收后 30 天内付验收款」，
// 这种期次没有固定日期，跟着验收动作走 —— 验收没做，它就不该出现在待收里。
func planDue(p model.InvContractPlan, c model.InvContract, today time.Time) bool {
	switch p.TriggerNode {
	case model.InvPlanTriggerSign:
		return true // 签订即待收（合同已存在便满足）
	case model.InvPlanTriggerAccept:
		return c.AcceptedAt != nil
	case model.InvPlanTriggerWarranty:
		return c.WarrantyEndAt != nil && !c.WarrantyEndAt.After(today)
	}
	if p.DueDate != nil && !p.DueDate.IsZero() {
		return !p.DueDate.After(today)
	}
	return false // 既无止期也无节点：不限期，不算待收、也不进逾期
}

// planStatusOf 派生一期期次的收付状态。
func planStatusOf(p model.InvContractPlan, c model.InvContract, today time.Time) (string, string) {
	outstanding := round2(p.Amount - p.SettledAmount)
	if outstanding <= settleEpsilon {
		return "settled", "已结清"
	}
	if p.SettledAmount > settleEpsilon {
		return "partial", "部分收付"
	}
	if planDue(p, c, today) {
		return "due", "待收付"
	}
	return "pending", "未到期"
}

// planKindLabel 期次类型的中文名。
func planKindLabel(k int16) string {
	switch k {
	case model.InvPlanAdvance:
		return "预付款"
	case model.InvPlanProgress:
		return "进度款"
	case model.InvPlanAccept:
		return "验收款"
	case model.InvPlanRetention:
		return "质保金"
	}
	return "其他"
}

// ---------------- 查询 ----------------

// plansByContract 批量取合同的全部期次（一次查询，不逐条合同请求）。
func (s *Service) plansByContract(ids []int64) (map[int64][]model.InvContractPlan, error) {
	out := make(map[int64][]model.InvContractPlan, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var list []model.InvContractPlan
	if err := s.db.Where("contract_id IN ?", ids).Order("contract_id ASC, seq ASC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, p := range list {
		out[p.ContractID] = append(out[p.ContractID], p)
	}
	return out, nil
}

// planSummary 汇总一份合同的计划与收付进度。
//
// retentionOutstanding 单列：质保金是「以后才能要的钱」，
// 和当期就该收的款混在一个「未收付」里，会让催款时看错重点。
func planSummary(plans []model.InvContractPlan) (count int, total, settled, retentionOut float64) {
	count = len(plans)
	for _, p := range plans {
		total += p.Amount
		settled += p.SettledAmount
		if p.Kind == model.InvPlanRetention {
			out := round2(p.Amount - p.SettledAmount)
			if out > 0 {
				retentionOut += out
			}
		}
	}
	return count, round2(total), round2(settled), round2(retentionOut)
}

// toPlanItem 期次模型转接口行。
func toPlanItem(p model.InvContractPlan, c model.InvContract, today time.Time) ContractPlanItem {
	status, label := planStatusOf(p, c, today)
	return ContractPlanItem{
		ID: p.ID, Seq: p.Seq, Name: p.Name, Kind: p.Kind, KindLabel: planKindLabel(p.Kind),
		Rate: round2(p.Rate), Amount: round2(p.Amount), DueDate: p.DueDate,
		TriggerNode:   p.TriggerNode,
		SettledAmount: round2(p.SettledAmount),
		Outstanding:   round2(p.Amount - p.SettledAmount),
		Status:        status, StatusLabel: label, Remark: p.Remark,
	}
}

// ---------------- 保存计划 ----------------

// savePlans 保存一份合同的付款计划（整份替换语义）。
//
// 传进来的就是全部期次：没出现的既有期次会被删除 —— 但**已发生收付款的期次
// 既不能删、也不能改金额**。理由很直接：那笔钱已经在账上了，
// 把承载它的计划行删掉，应收列表里就会凭空少一笔，收付款记录却还指着它。
func (s *Service) savePlans(tx *gorm.DB, c model.InvContract, reqs []ContractPlanReq, uid int64) error {
	var old []model.InvContractPlan
	if err := tx.Where("contract_id = ?", c.ID).Find(&old).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	oldByID := make(map[int64]model.InvContractPlan, len(old))
	for _, p := range old {
		oldByID[p.ID] = p
	}

	seenSeq := make(map[int]bool, len(reqs))
	kept := make(map[int64]bool, len(reqs))
	var planTotal, retentionTotal float64

	for _, r := range reqs {
		seq := r.Seq
		if seq < 1 || seq > 99 {
			return errcode.New(errcode.ErrBadRequest, "期次号需在 1 ~ 99 之间")
		}
		if seenSeq[seq] {
			return errcode.Newf(errcode.ErrBadRequest, "付款计划第 %d 期的期次号重复", seq)
		}
		seenSeq[seq] = true

		name := strings.TrimSpace(r.Name)
		if name == "" {
			return errcode.Newf(errcode.ErrBadRequest, "付款计划第 %d 期缺少名称", seq)
		}
		kind := r.Kind
		if kind == 0 {
			kind = model.InvPlanOther
		}
		// 比例优先：合同写「30%」时，金额是比例算出来的结果，
		// 合同金额一改，金额就该跟着变。想直接定金额，就不要填比例。
		amount := round2(r.Amount)
		rate := round2(r.Rate)
		if rate > 0 {
			amount = round2(c.Amount * rate / 100)
		}
		if amount < 0 {
			return errcode.Newf(errcode.ErrBadRequest, "付款计划第 %d 期金额不能为负", seq)
		}
		due, err := parseDatePtr(strings.TrimSpace(r.DueDate))
		if err != nil {
			return errcode.Newf(errcode.ErrBadRequest, "付款计划第 %d 期的约定日期格式不正确，应为 yyyy-MM-dd", seq)
		}
		planTotal += amount
		if kind == model.InvPlanRetention {
			retentionTotal += amount
		}

		if r.ID > 0 {
			op, ok := oldByID[r.ID]
			if !ok || op.ContractID != c.ID {
				return errcode.Newf(errcode.ErrBadRequest, "付款计划第 %d 期不属于该合同，请刷新后重试", seq)
			}
			kept[r.ID] = true
			// 已收付的期次：金额与比例必须原样保留，只允许改名称、日期、备注。
			// 比例也要一起判：只改比例不改金额，下一次保存时金额会被比例重算成别的数。
			if op.SettledAmount > settleEpsilon &&
				(math.Abs(round2(op.Amount)-amount) > settleEpsilon || math.Abs(op.Rate-rate) > settleEpsilon) {
				return errcode.Newf(errcode.ErrConflict,
					"第 %d 期已发生收付款 %.2f，金额与比例不能修改", op.Seq, round2(op.SettledAmount))
			}
			updates := map[string]any{
				"seq": seq, "name": name, "kind": kind, "rate": rate, "amount": amount,
				"due_date": due, "trigger_node": strings.TrimSpace(r.TriggerNode),
				"remark": r.Remark,
			}
			if err := tx.Model(&model.InvContractPlan{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
				return errcode.Wrap(errcode.ErrDatabase, err)
			}
			continue
		}
		np := model.InvContractPlan{
			ContractID: c.ID, BizType: c.BizType, PartyID: c.PartyID, PartyName: c.PartyName,
			Seq: seq, Name: name, Kind: kind, Rate: rate, Amount: amount,
			DueDate: due, TriggerNode: strings.TrimSpace(r.TriggerNode),
			Remark: r.Remark, CreatedBy: uid,
		}
		if err := tx.Create(&np).Error; err != nil {
			return errcode.Wrap(errcode.ErrDatabase, err)
		}
	}

	for _, p := range old {
		if kept[p.ID] {
			continue
		}
		if p.SettledAmount > settleEpsilon {
			return errcode.Newf(errcode.ErrConflict,
				"第 %d 期已发生收付款 %.2f，不能删除", p.Seq, round2(p.SettledAmount))
		}
		if err := tx.Delete(&model.InvContractPlan{}, p.ID).Error; err != nil {
			return errcode.Wrap(errcode.ErrDatabase, err)
		}
	}

	// 合计不能超过合同金额：超额的计划会让应收总额大于合同额，
	// 报表「合同金额 vs 应收」两栏永远对不上。
	if planTotal > c.Amount+settleEpsilon {
		return errcode.Newf(errcode.ErrBadRequest, "付款计划合计 %.2f 超过合同金额 %.2f", planTotal, c.Amount)
	}
	if c.RetentionAmount > settleEpsilon && retentionTotal > c.RetentionAmount+settleEpsilon {
		return errcode.Newf(errcode.ErrBadRequest,
			"质保金期次合计 %.2f 超过质保金金额 %.2f", retentionTotal, c.RetentionAmount)
	}
	return nil
}

// ---------------- 验收 ----------------

// ContractAccept 登记合同验收：写验收日期与结论，并按质保月数算出质保到期日。
//
// 质保到期日是存下来的（不是每次现算）：应收应付要按它筛「质保已到期」，
// 报表也要按它排序，SQL 比较一个日期列比在应用层算完再过滤便宜得多。
func (s *Service) ContractAccept(id int64, req ContractAcceptReq, actor *authz.Identity) error {
	var c model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if c.Status == model.InvContractPending {
		return errcode.New(errcode.ErrConflict, "合同尚未审核，不能登记验收")
	}
	if c.Status == model.InvContractTerminated {
		return errcode.New(errcode.ErrConflict, "合同已终止，不能登记验收")
	}
	acceptedAt, err := parseDate(strings.TrimSpace(req.AcceptedAt))
	if err != nil {
		return errcode.New(errcode.ErrBadRequest, "验收日期格式不正确，应为 yyyy-MM-dd")
	}
	if c.SignDate.After(acceptedAt) {
		return errcode.New(errcode.ErrBadRequest, "验收日期不能早于签订日期")
	}
	months := req.WarrantyMonths
	if months == 0 {
		months = c.WarrantyMonths
	}
	var end *time.Time
	if months > 0 {
		e := acceptedAt.AddDate(0, months, 0).In(database.Location())
		end = &e
	}
	updates := map[string]any{
		"accepted_at":     acceptedAt,
		"accept_result":   req.AcceptResult,
		"warranty_months": months,
		"warranty_end_at": end,
	}
	// 验收备注**追加**到合同备注，不覆盖。
	//
	// 合同备注里通常写着付款条件、交货要求这类约定，验收时一句「现场无问题」
	// 把它冲掉，等于把原始约定删了 —— 而且是静默删除。追加再加长度保护，
	// 备注列是 varchar(255)，超了会让整条更新失败。
	if r := strings.TrimSpace(req.Remark); r != "" {
		merged := strings.TrimSpace(c.Remark)
		add := "验收：" + r
		if merged == "" {
			merged = add
		} else {
			merged = merged + "｜" + add
		}
		updates["remark"] = clipRunes(merged, 255)
	}
	if err := s.db.Model(&model.InvContract{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return saveErr(err)
	}
	return nil
}

// clipRunes 按字符（不是字节）截断，避免把多字节的中文切成半个字。
func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// ---------------- 期次收付款 ----------------

// PlanSettle 登记一期合同的收款（销售合同）或付款（采购合同）。
//
// 并发防线与订单结算一致：事务外只做友好提示，事务内用条件更新兜底 ——
// 两个并发请求都读到同一份余额时，后到的那个 UPDATE 会拿到 0 行并整单回滚，
// 不会把 settled_amount 顶到 amount 之上。
func (s *Service) PlanSettle(req PlanSettleReq, actor *authz.Identity) error {
	var p model.InvContractPlan
	if err := s.db.First(&p, req.PlanID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "付款期次不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	var c model.InvContract
	if err := s.db.First(&c, p.ContractID).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureCreatorVisible(c.CreatedBy, actor); err != nil {
		return err
	}
	if c.Status == model.InvContractPending {
		return errcode.New(errcode.ErrConflict, "合同尚未审核，不能登记收付款")
	}
	if c.Status == model.InvContractTerminated {
		return errcode.New(errcode.ErrConflict, "合同已终止，不能登记收付款")
	}
	outstanding := round2(p.Amount - p.SettledAmount)
	if outstanding <= settleEpsilon {
		return errcode.Newf(errcode.ErrConflict, "第 %d 期已结清", p.Seq)
	}
	amount := round2(req.Amount)
	if amount <= settleEpsilon {
		return errcode.New(errcode.ErrBadRequest, "收付金额必须大于 0")
	}
	if amount > outstanding+settleEpsilon {
		return errcode.Newf(errcode.ErrConflict, "收付金额超出该期未收付余额（剩余 %.2f）", outstanding)
	}
	settledAt, err := parseDate(strings.TrimSpace(req.SettledAt))
	if err != nil {
		return errcode.New(errcode.ErrBadRequest, "收付日期格式不正确，应为 yyyy-MM-dd")
	}
	if settledAt.IsZero() {
		settledAt = time.Now().In(database.Location())
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = "银行转账"
	}

	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		rec := model.InvSettlement{
			OrderID: 0, PlanID: p.ID, Source: model.InvSettleSourcePlan,
			BizType: c.BizType, PartyID: c.PartyID,
			Amount: amount, Method: method, SettledAt: settledAt,
			Remark: req.Remark, CreatedBy: actorUID(actor),
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		res := tx.Model(&model.InvContractPlan{}).
			Where("id = ? AND settled_amount + ? <= amount + ?", p.ID, amount, settleEpsilon).
			UpdateColumn("settled_amount", gorm.Expr("settled_amount + ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.New(errcode.ErrConflict, "该期余额已被改动，请刷新后重试")
		}
		return s.maybeFinishContract(tx, c)
	}))
}

// maybeFinishContract 期次全部结清、且质保已了结时，把合同置为「已完成」。
//
// 只在「钱收齐 + 该等的都等完了」时才自动完成：
// 质保金还没到期就标完成，会让质保期内的售后责任从报表里消失。
func (s *Service) maybeFinishContract(tx *gorm.DB, c model.InvContract) error {
	if c.Status != model.InvContractActive {
		return nil
	}
	var stat struct {
		Total   float64
		Settled float64
	}
	if err := tx.Model(&model.InvContractPlan{}).
		Select("COALESCE(SUM(amount),0) AS total, COALESCE(SUM(settled_amount),0) AS settled").
		Where("contract_id = ?", c.ID).Scan(&stat).Error; err != nil {
		return err
	}
	if stat.Total <= settleEpsilon || stat.Settled+settleEpsilon < stat.Total {
		return nil
	}
	today := time.Now().In(database.Location())
	if c.WarrantyEndAt != nil && c.WarrantyEndAt.After(today) {
		return nil // 质保期未满，先留在执行中
	}
	return tx.Model(&model.InvContract{}).Where("id = ?", c.ID).
		Update("status", model.InvContractFinished).Error
}

// PlanSettleHistory 某一期的收付款流水（详情页展开用）。
func (s *Service) PlanSettleHistory(planID int64, actor *authz.Identity) ([]SettleRecord, error) {
	var p model.InvContractPlan
	if err := s.db.First(&p, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "付款期次不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var c model.InvContract
	if err := s.db.First(&c, p.ContractID).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureCreatorVisible(c.CreatedBy, actor); err != nil {
		return nil, err
	}
	var list []model.InvSettlement
	if err := s.db.Where("source = ? AND plan_id = ?", model.InvSettleSourcePlan, planID).
		Order("settled_at DESC, id DESC").Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make([]SettleRecord, 0, len(list))
	for _, it := range list {
		out = append(out, SettleRecord{
			ID: it.ID, Amount: round2(it.Amount), Method: it.Method,
			SettledAt: it.SettledAt, Remark: it.Remark,
		})
	}
	return out, nil
}
