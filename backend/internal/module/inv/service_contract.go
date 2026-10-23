package inv

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// 合同台账。
//
// 合同不是单据的上游约束（见 model.InvContract 注释），所以这里没有
// 「审核 / 反审核 / 生成出入库」这类状态机，只有一张可增删改查的台账：
// 编号、对方、金额、履约期、状态、附件。三条最容易被忽略的规则写在下面：
//
//   - 编号留空自动生成 HT + 流水，填了就用填的（重复会拒），编辑时留空沿用原号；
//   - 签订日期必填，履约起止选填；两个都填时结束不能早于开始；
//   - 往来单位必须真的存在（客户 / 供应商按合同类型二选一），
//     否则会留下一条「挂了不存在的客户」的合同，列表点进去什么都没有。

// contractToday 当天零点（日期列比较用；避免 23:59:59 这种边界差一天）。
func contractToday() time.Time {
	now := time.Now().In(database.Location())
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, database.Location())
}

// contractQuery 合同台账的查询条件。
//
// 列表、报表、导出共用这一份：三处各写一遍，迟早出现「报表里的合同数
// 和列表里的总数不一致」—— 而这种差异会被当成数据丢失来排查。
func (s *Service) contractQuery(f ContractFilter, keyword string, actor *authz.Identity, today time.Time) *gorm.DB {
	tx := s.applyCreator(s.db.Model(&model.InvContract{}), actor)
	if f.BizType != "" {
		tx = tx.Where("biz_type = ?", f.BizType)
	}
	if f.Status != nil {
		tx = tx.Where("status = ?", *f.Status)
	}
	if f.PartyID > 0 {
		tx = tx.Where("party_id = ?", f.PartyID)
	}
	// 履约阶段是派生的（见 contractStage），所以这里把它翻译成日期 / 状态条件。
	switch f.Stage {
	case "unaudited":
		tx = tx.Where("status = ?", model.InvContractPending)
	case "pending":
		// 待验收只指「已审核、还没验收」：待审核的合同还没进入履约
		tx = tx.Where("accepted_at IS NULL AND status NOT IN ?",
			[]int16{model.InvContractPending, model.InvContractTerminated})
	case "accepted":
		tx = tx.Where("accepted_at IS NOT NULL")
	case "warranty":
		tx = tx.Where("warranty_end_at IS NOT NULL AND warranty_end_at >= ?", today)
	case "warranty_due":
		tx = tx.Where("warranty_end_at IS NOT NULL AND warranty_end_at < ?", today)
	}
	if keyword != "" {
		// 业务员也进关键字：现场最常见的问法是「张伟签的那份」，
		// 而业务员既不是编号也不是对方单位。
		kw := likeKW(keyword)
		tx = tx.Where(
			likeExpr("contract_no")+" OR "+likeExpr("name")+" OR "+likeExpr("party_name")+" OR "+likeExpr("salesman"),
			kw, kw, kw, kw)
	}
	if f.Start != nil {
		tx = tx.Where("sign_date >= ?", *f.Start)
	}
	if f.End != nil {
		tx = tx.Where("sign_date <= ?", *f.End)
	}
	return tx
}

// ContractList 分页查询合同台账。
func (s *Service) ContractList(f ContractFilter, q page.Query, actor *authz.Identity) ([]ContractRow, int64, error) {
	today := contractToday()
	tx := s.contractQuery(f, q.Keyword, actor, today)

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvContract
	if err := q.Paginate(tx.Order("sign_date DESC, id DESC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	ids := make([]int64, 0, len(list))
	creators := make([]int64, 0, len(list))
	for _, it := range list {
		ids = append(ids, it.ID)
		creators = append(creators, it.CreatedBy)
	}
	counts, err := s.AttachmentCounts(model.InvAttachContract, ids)
	if err != nil {
		return nil, 0, err
	}
	operators, err := s.operatorNames(creators)
	if err != nil {
		return nil, 0, err
	}
	// 一页合同的期次一次取回，不逐条合同再查一遍（N+1）
	plans, err := s.plansByContract(ids)
	if err != nil {
		return nil, 0, err
	}

	rows := make([]ContractRow, 0, len(list))
	for _, it := range list {
		rows = append(rows, toContractRow(it, counts[it.ID], operators[it.CreatedBy], plans[it.ID], today, false))
	}
	return rows, total, nil
}

// ContractDetail 合同详情（编辑回填用，带完整付款计划）。
func (s *Service) ContractDetail(id int64, actor *authz.Identity) (*ContractRow, error) {
	var it model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&it).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	operators, err := s.operatorNames([]int64{it.CreatedBy})
	if err != nil {
		return nil, err
	}
	counts, err := s.AttachmentCounts(model.InvAttachContract, []int64{it.ID})
	if err != nil {
		return nil, err
	}
	plans, err := s.plansByContract([]int64{it.ID})
	if err != nil {
		return nil, err
	}
	row := toContractRow(it, counts[it.ID], operators[it.CreatedBy], plans[it.ID], contractToday(), true)
	return &row, nil
}

// ContractCreate 新增合同（含付款计划），返回新记录 ID。
func (s *Service) ContractCreate(req ContractReq, actor *authz.Identity) (int64, error) {
	sign, start, end, err := contractDates(req)
	if err != nil {
		return 0, err
	}
	partyName, err := s.contractPartyName(req.BizType, req.PartyID)
	if err != nil {
		return 0, err
	}
	no := strings.TrimSpace(req.ContractNo)
	if no == "" {
		no, err = s.nextCode("inv_contracts", "contract_no", "HT", 4)
		if err != nil {
			return 0, err
		}
	} else if err := s.contractNoFree(no, 0); err != nil {
		return 0, err
	}

	// 新建一律「待审核」：合同一保存就会按付款计划生成应收 / 应付，
	// 没有这道门槛，录错一份合同等于当场虚增一笔债权。
	// 请求里的 status 在这里被忽略 —— 否则「新增时直接传 status=1」就绕过了审核。
	status := model.InvContractPending
	amount := round2(req.Amount)
	retentionAmount, retentionRate := retentionValues(req.RetentionAmount, req.RetentionRate, amount)
	it := model.InvContract{
		ContractNo: no, Name: strings.TrimSpace(req.Name), BizType: req.BizType,
		PartyID: req.PartyID, PartyName: partyName,
		SignDate: sign, StartDate: start, EndDate: end,
		Amount: amount, Status: status,
		Salesman: strings.TrimSpace(req.Salesman), Remark: req.Remark,
		WarrantyMonths: req.WarrantyMonths, RetentionRate: retentionRate, RetentionAmount: retentionAmount,
		CreatedBy: actorID(actor),
	}
	// 合同与计划必须一起落库：只写进合同、计划写失败会得到一份
	// 「金额 100 万、一期都没有」的合同，应收里一分钱都看不到。
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&it).Error; err != nil {
			return err
		}
		return s.savePlans(tx, it, req.Plans, actorID(actor))
	})
	if err != nil {
		return 0, saveErr(err)
	}
	return it.ID, nil
}

// ContractUpdate 修改合同。编号留空表示沿用原编号。
func (s *Service) ContractUpdate(id int64, req ContractReq, actor *authz.Identity) error {
	var old model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&old).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	sign, start, end, err := contractDates(req)
	if err != nil {
		return err
	}
	// 往来单位没变就沿用原名称。
	//
	// 对方档案被删掉的历史合同也要能改备注、改状态：否则一条「指向已删客户」的
	// 数据会把整份合同锁死 —— 保存时才报「选中的客户不存在，请刷新后重选」，
	// 而界面上根本没有可选的客户。换单位时仍然严格校验。
	partyName := old.PartyName
	if req.PartyID != old.PartyID || req.BizType != old.BizType {
		partyName, err = s.contractPartyName(req.BizType, req.PartyID)
		if err != nil {
			return err
		}
	}
	no := strings.TrimSpace(req.ContractNo)
	if no == "" {
		no = old.ContractNo
	} else if err := s.contractNoFree(no, id); err != nil {
		return err
	}

	amount := round2(req.Amount)
	retentionAmount, retentionRate := retentionValues(req.RetentionAmount, req.RetentionRate, amount)

	// 已审核的合同，金额 / 往来单位 / 类型不能直接改。
	//
	// 这三个字段决定「应收多少钱、向谁收」，改了等于换了一份合同 ——
	// 而钱可能已经收了一部分，报表、账龄、台账全部要跟着变。
	// 需要改就走「反审核 → 修改 → 重新审核」，让改动有明确的边界。
	if old.Status != model.InvContractPending {
		if amount != round2(old.Amount) || req.PartyID != old.PartyID || req.BizType != old.BizType {
			return errcode.New(errcode.ErrConflict,
				"合同已审核，金额与往来单位不能直接修改，请先反审核")
		}
	}

	updates := map[string]any{
		"contract_no": no, "name": strings.TrimSpace(req.Name), "biz_type": req.BizType,
		"party_id": req.PartyID, "party_name": partyName,
		"sign_date": sign, "start_date": start, "end_date": end,
		"amount": amount, "salesman": strings.TrimSpace(req.Salesman),
		"remark":          req.Remark,
		"warranty_months": req.WarrantyMonths,
		"retention_rate":  retentionRate, "retention_amount": retentionAmount,
	}
	// 状态：待审核不能自己「改」成已审核（那等于绕过审核），
	// 已审核不能直接退回待审核（那要走反审核，会连带清理验收记录）。
	if req.Status != nil {
		next := *req.Status
		switch {
		case old.Status == model.InvContractPending && next != model.InvContractPending:
			return errcode.New(errcode.ErrConflict, "合同尚未审核，请先审核通过")
		case old.Status != model.InvContractPending && next == model.InvContractPending:
			return errcode.New(errcode.ErrConflict, "已审核的合同不能直接退回待审核，请使用反审核")
		default:
			updates["status"] = next
		}
	}
	// 已验收的合同改了质保月数，质保到期日要跟着重算 ——
	// 否则报表里的「质保到期」还是按老月数算出来的日期。
	if old.AcceptedAt != nil {
		if req.WarrantyMonths > 0 {
			e := old.AcceptedAt.AddDate(0, req.WarrantyMonths, 0).In(database.Location())
			updates["warranty_end_at"] = &e
		} else {
			updates["warranty_end_at"] = nil
		}
	}

	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.InvContract{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		// 比例算金额要用**改后**的合同金额，所以重新读一次再存计划
		var cur model.InvContract
		if err := tx.First(&cur, id).Error; err != nil {
			return err
		}
		if req.Plans == nil {
			return nil // 没传 plans 表示本次不动计划（局部更新，如只改状态）
		}
		// 期次的往来单位是冗余字段，合同换了对方单位要一并刷新，
		// 否则应收列表里同一份合同的客户名前后不一致。
		if err := tx.Model(&model.InvContractPlan{}).Where("contract_id = ?", id).
			Updates(map[string]any{"biz_type": cur.BizType, "party_id": cur.PartyID, "party_name": cur.PartyName}).Error; err != nil {
			return err
		}
		return s.savePlans(tx, cur, req.Plans, actorID(actor))
	}))
}

// ContractDelete 删除合同（软删除，附件与付款计划一并不可见）。
func (s *Service) ContractDelete(id int64, actor *authz.Identity) error {
	var old model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&old).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 有收付款的合同不能直接删：钱已经在账上了，删掉合同会让那几笔收付款
	// 变成「没有出处的流水」，账龄与台账都解释不通。
	var settled float64
	if err := s.db.Model(&model.InvContractPlan{}).
		Select("COALESCE(SUM(settled_amount),0)").Where("contract_id = ?", id).
		Scan(&settled).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if settled > settleEpsilon {
		return errcode.Newf(errcode.ErrConflict, "该合同已发生收付款 %.2f，不能删除", round2(settled))
	}
	return saveErr(s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("contract_id = ?", id).Delete(&model.InvContractPlan{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.InvContract{}, id).Error
	}))
}

// retentionValues 归算质保金「比例 ⇄ 金额」：填了金额就以金额为准，
// 只填比例则按合同金额折算。两个都留空表示没有质保金。
func retentionValues(amount, rate, contractAmount float64) (float64, float64) {
	a := round2(amount)
	r := round2(rate)
	if a > 0 {
		if r == 0 && contractAmount > 0 {
			r = round2(a / contractAmount * 100)
		}
		return a, r
	}
	if r > 0 && contractAmount > 0 {
		a = round2(contractAmount * r / 100)
	}
	return a, r
}

// ContractAudit 审核合同：待审核 → 执行中。
//
// 审核通过意味着「这份合同的钱可以入账了」：只有到这一步，付款计划才会
// 出现在应收 / 应付里，也才能验收与登记收付款。
func (s *Service) ContractAudit(id int64, actor *authz.Identity) error {
	var c model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if c.Status != model.InvContractPending {
		return errcode.New(errcode.ErrConflict, "该合同已审核，无需重复操作")
	}
	now := time.Now().In(database.Location())
	// 条件更新兜底并发：两个人同时点审核，后到的那个拿到 0 行而不是写两次
	res := s.db.Model(&model.InvContract{}).
		Where("id = ? AND status = ?", id, model.InvContractPending).
		Updates(map[string]any{
			"status":     model.InvContractActive,
			"audited_at": now,
			"auditor":    actorName(actor),
		})
	if res.Error != nil {
		return saveErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrConflict, "该合同状态已被改动，请刷新后重试")
	}
	return nil
}

// ContractUnaudit 反审核：退回待审核，并同时撤销验收。
//
// 两个条件 / 动作都是为了不留下自相矛盾的数据：
//   - 已有收付款不能反审核：钱在账上了，退回待审核会让它从应收里消失；
//   - 同时清空验收与质保日期：合同要回炉改条款了，留着「已验收 + 质保到期」
//     会让阶段显示成「待审核却已验收」。质保月数（条款本身）保留。
func (s *Service) ContractUnaudit(id int64, actor *authz.Identity) error {
	var c model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if c.Status == model.InvContractPending {
		return errcode.New(errcode.ErrConflict, "该合同尚未审核")
	}
	var settled float64
	if err := s.db.Model(&model.InvContractPlan{}).
		Select("COALESCE(SUM(settled_amount),0)").Where("contract_id = ?", id).
		Scan(&settled).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if settled > settleEpsilon {
		return errcode.Newf(errcode.ErrConflict,
			"该合同已发生收付款 %.2f，不能反审核", round2(settled))
	}
	res := s.db.Model(&model.InvContract{}).
		Where("id = ? AND status <> ?", id, model.InvContractPending).
		Updates(map[string]any{
			"status":          model.InvContractPending,
			"audited_at":      nil,
			"auditor":         "",
			"accepted_at":     nil,
			"accept_result":   "",
			"warranty_end_at": nil,
		})
	if res.Error != nil {
		return saveErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrConflict, "该合同状态已被改动，请刷新后重试")
	}
	return nil
}

// ContractLogs 合同的操作记录（新增 / 修改 / 审核 / 验收 / 删除）。
//
// 数据来自系统操作日志，按请求路径归集：中间件已经记下了「谁、什么动作、
// 什么请求体、什么结果」，再单独建一张合同变更表只会出现两份不一致的记录。
//
// 注意：操作日志是**异步批量落库**（channel + 批量 insert），所以刚做完的操作
// 可能晚几百毫秒才出现在这里 —— 这是刻意的取舍，日志不该拖慢业务响应。
func (s *Service) ContractLogs(id int64, actor *authz.Identity, limit int) ([]ContractLogRow, error) {
	var c model.InvContract
	if err := s.applyCreator(s.db.Select("id").Where("id = ?", id), actor).Take(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// 路径不写死前缀（app.prefix / version 可配），用「后缀 + 后缀/下级」匹配，
	// 避免 /contracts/12 把 /contracts/120 的记录也捞进来。
	suffix := fmt.Sprintf("/inv/contracts/%d", id)
	var list []model.OperationLog
	if err := s.db.Where("path LIKE ? OR path LIKE ?", "%"+suffix, "%"+suffix+"/%").
		Order("created_at DESC, id DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make([]ContractLogRow, 0, len(list))
	for _, it := range list {
		out = append(out, ContractLogRow{
			ID: it.ID, Username: it.Username, Action: it.Action,
			Method: it.Method, IP: it.IP, Status: it.Status, CreatedAt: it.CreatedAt,
		})
	}
	return out, nil
}

// actorName 取操作人显示名：优先昵称，其次账号。
func actorName(actor *authz.Identity) string {
	if actor == nil {
		return ""
	}
	if n := strings.TrimSpace(actor.Nickname); n != "" {
		return n
	}
	return actor.Username
}

// contractDates 解析签订日期与履约起止，并校验先后顺序。
func contractDates(req ContractReq) (time.Time, *time.Time, *time.Time, error) {
	sign, err := parseDate(strings.TrimSpace(req.SignDate))
	if err != nil {
		return time.Time{}, nil, nil, errcode.New(errcode.ErrBadRequest, "签订日期格式不正确，应为 yyyy-MM-dd")
	}
	start, err := parseDatePtr(strings.TrimSpace(req.StartDate))
	if err != nil {
		return time.Time{}, nil, nil, errcode.New(errcode.ErrBadRequest, "开始日期格式不正确，应为 yyyy-MM-dd")
	}
	end, err := parseDatePtr(strings.TrimSpace(req.EndDate))
	if err != nil {
		return time.Time{}, nil, nil, errcode.New(errcode.ErrBadRequest, "结束日期格式不正确，应为 yyyy-MM-dd")
	}
	if start != nil && end != nil && end.Before(*start) {
		return time.Time{}, nil, nil, errcode.New(errcode.ErrBadRequest, "履约结束日期不能早于开始日期")
	}
	return sign, start, end, nil
}

// contractPartyName 按合同类型取对方单位名称（采购=供应商，销售=客户）。
func (s *Service) contractPartyName(bizType string, partyID int64) (string, error) {
	if bizType == model.InvBizPurchase {
		var it model.InvSupplier
		if err := s.db.Select("id", "name").Where("id = ?", partyID).Take(&it).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", errcode.New(errcode.ErrBadRequest, "选中的供应商不存在，请刷新后重选")
			}
			return "", errcode.Wrap(errcode.ErrDatabase, err)
		}
		return it.Name, nil
	}
	var it model.InvCustomer
	if err := s.db.Select("id", "name").Where("id = ?", partyID).Take(&it).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errcode.New(errcode.ErrBadRequest, "选中的客户不存在，请刷新后重选")
		}
		return "", errcode.Wrap(errcode.ErrDatabase, err)
	}
	return it.Name, nil
}

// contractNoFree 校验合同编号未被占用（excludeID 用于编辑时排除自己）。
func (s *Service) contractNoFree(no string, excludeID int64) error {
	q := s.db.Model(&model.InvContract{}).Where("contract_no = ?", no)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var cnt int64
	if err := q.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "合同编号「%s」已存在", no)
	}
	return nil
}

func toContractRow(it model.InvContract, attachCount int, operator string,
	plans []model.InvContractPlan, today time.Time, withPlans bool) ContractRow {
	stage, stageLabel := contractStage(it, today)
	planCount, planAmount, settled, retentionOut := planSummary(plans)
	row := ContractRow{
		ID: it.ID, ContractNo: it.ContractNo, Name: it.Name, BizType: it.BizType,
		PartyID: it.PartyID, PartyName: it.PartyName,
		SignDate: it.SignDate, StartDate: it.StartDate, EndDate: it.EndDate,
		Amount: it.Amount, Status: it.Status, Salesman: it.Salesman,
		Operator: operator, Remark: it.Remark,
		AuditedAt: it.AuditedAt, Auditor: it.Auditor,
		AcceptedAt: it.AcceptedAt, AcceptResult: it.AcceptResult,
		WarrantyMonths: it.WarrantyMonths, WarrantyEndAt: it.WarrantyEndAt,
		RetentionRate: it.RetentionRate, RetentionAmount: it.RetentionAmount,
		Stage: stage, StageLabel: stageLabel,
		PlanCount: planCount, PlanAmount: planAmount,
		SettledAmount: settled, Outstanding: round2(planAmount - settled),
		RetentionOutstanding: retentionOut,
		AttachmentCount:      attachCount, CreatedAt: it.CreatedAt,
	}
	if withPlans {
		items := make([]ContractPlanItem, 0, len(plans))
		for _, p := range plans {
			items = append(items, toPlanItem(p, it, today))
		}
		row.Plans = items
	}
	return row
}

// ---------------- 导出 ----------------

// exportContractRows 合同导出单元格。
//
// 走的是 ContractList，和页面上同一套筛选 —— 否则「页面筛了类型与日期、
// 导出却是全表」这类口径漂移会在对账时才发现。
func (s *Service) exportContractRows(q page.Query, f ContractFilter, actor *authz.Identity) ([][]string, error) {
	rows, _, err := s.ContractList(f, q, actor)
	if err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.ContractNo, r.Name, contractBizLabel(r.BizType), r.PartyName,
			r.SignDate.Format("2006-01-02"),
			contractDateText(r.StartDate), contractDateText(r.EndDate),
			money(r.Amount), money(r.SettledAmount), money(r.Outstanding),
			r.StageLabel, contractDateText(r.AcceptedAt), contractDateText(r.WarrantyEndAt),
			money(r.RetentionOutstanding), r.Salesman, r.Remark,
		})
	}
	return out, nil
}

func contractBizLabel(v string) string {
	if v == model.InvBizPurchase {
		return "采购合同"
	}
	return "销售合同"
}

// contractDateText 可选日期转文本：未填就是空串，不写 0001-01-01。
func contractDateText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}
