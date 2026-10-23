package inv

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
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

// ContractList 分页查询合同台账。
func (s *Service) ContractList(f ContractFilter, q page.Query, actor *authz.Identity) ([]ContractRow, int64, error) {
	tx := s.db.Model(&model.InvContract{})
	// 合同是业务单据性质，同样受数据范围约束（只认创建人）
	tx = s.applyCreator(tx, actor)
	if f.BizType != "" {
		tx = tx.Where("biz_type = ?", f.BizType)
	}
	if f.Status != nil {
		tx = tx.Where("status = ?", *f.Status)
	}
	if q.Keyword != "" {
		// 业务员也进关键字：现场最常见的问法是「张伟签的那份」，
		// 而业务员既不是编号也不是对方单位。
		kw := likeKW(q.Keyword)
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

	rows := make([]ContractRow, 0, len(list))
	for _, it := range list {
		rows = append(rows, toContractRow(it, counts[it.ID], operators[it.CreatedBy]))
	}
	return rows, total, nil
}

// ContractDetail 合同详情（编辑回填用）。
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
	row := toContractRow(it, counts[it.ID], operators[it.CreatedBy])
	return &row, nil
}

// ContractCreate 新增合同，返回新记录 ID。
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

	status := model.InvContractActive
	if req.Status != nil {
		status = *req.Status
	}
	it := model.InvContract{
		ContractNo: no, Name: strings.TrimSpace(req.Name), BizType: req.BizType,
		PartyID: req.PartyID, PartyName: partyName,
		SignDate: sign, StartDate: start, EndDate: end,
		Amount: round2(req.Amount), Status: status,
		Salesman: strings.TrimSpace(req.Salesman), Remark: req.Remark,
		CreatedBy: actorID(actor),
	}
	if err := s.db.Create(&it).Error; err != nil {
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
	partyName, err := s.contractPartyName(req.BizType, req.PartyID)
	if err != nil {
		return err
	}
	no := strings.TrimSpace(req.ContractNo)
	if no == "" {
		no = old.ContractNo
	} else if err := s.contractNoFree(no, id); err != nil {
		return err
	}

	updates := map[string]any{
		"contract_no": no, "name": strings.TrimSpace(req.Name), "biz_type": req.BizType,
		"party_id": req.PartyID, "party_name": partyName,
		"sign_date": sign, "start_date": start, "end_date": end,
		"amount": round2(req.Amount), "salesman": strings.TrimSpace(req.Salesman),
		"remark": req.Remark,
	}
	// 状态是可选字段：不传就是「不改」，避免编辑基础信息时被默认值打回执行中
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	// 改了往来单位 / 类型时，party_name 已经是新名字，无需额外处理
	if err := s.db.Model(&model.InvContract{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return saveErr(err)
	}
	return nil
}

// ContractDelete 删除合同（软删除，附件一并不可见）。
func (s *Service) ContractDelete(id int64, actor *authz.Identity) error {
	var old model.InvContract
	if err := s.applyCreator(s.db.Where("id = ?", id), actor).Take(&old).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.db.Delete(&model.InvContract{}, id).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	return nil
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

func toContractRow(it model.InvContract, attachCount int, operator string) ContractRow {
	return ContractRow{
		ID: it.ID, ContractNo: it.ContractNo, Name: it.Name, BizType: it.BizType,
		PartyID: it.PartyID, PartyName: it.PartyName,
		SignDate: it.SignDate, StartDate: it.StartDate, EndDate: it.EndDate,
		Amount: it.Amount, Status: it.Status, Salesman: it.Salesman,
		Operator: operator, Remark: it.Remark,
		AttachmentCount: attachCount, CreatedAt: it.CreatedAt,
	}
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
			money(r.Amount), contractStatusLabel(r.Status), r.Salesman, r.Remark,
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

func contractStatusLabel(v int16) string {
	switch v {
	case model.InvContractActive:
		return "执行中"
	case model.InvContractFinished:
		return "已完成"
	case model.InvContractTerminated:
		return "已终止"
	}
	return "-"
}

// contractDateText 可选日期转文本：未填就是空串，不写 0001-01-01。
func contractDateText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}
