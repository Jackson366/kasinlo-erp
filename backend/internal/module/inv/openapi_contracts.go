package inv

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// 独立出参避免把经办人、内部备注、审核人和附件信息带到开放接口。
type platformContractRow struct {
	ID            int64                  `json:"id"`
	ContractNo    string                 `json:"contractNo"`
	Name          string                 `json:"name"`
	BizType       string                 `json:"bizType"`
	PartyID       int64                  `json:"partyId"`
	PartyName     string                 `json:"partyName"`
	SignDate      time.Time              `json:"signDate"`
	StartDate     *time.Time             `json:"startDate"`
	EndDate       *time.Time             `json:"endDate"`
	Amount        float64                `json:"amount"`
	Status        int16                  `json:"status"`
	Stage         string                 `json:"stage"`
	PlanAmount    float64                `json:"planAmount"`
	SettledAmount float64                `json:"settledAmount"`
	Outstanding   float64                `json:"outstanding"`
	Plans         []platformContractPlan `json:"plans,omitempty"`
}

type platformContractPlan struct {
	ID            int64      `json:"id"`
	Seq           int        `json:"seq"`
	Name          string     `json:"name"`
	Kind          int16      `json:"kind"`
	Amount        float64    `json:"amount"`
	DueDate       *time.Time `json:"dueDate"`
	SettledAmount float64    `json:"settledAmount"`
	Outstanding   float64    `json:"outstanding"`
	Status        string     `json:"status"`
}

func projectPlatformContract(row ContractRow) platformContractRow {
	out := platformContractRow{
		ID: row.ID, ContractNo: row.ContractNo, Name: row.Name, BizType: row.BizType,
		PartyID: row.PartyID, PartyName: row.PartyName, SignDate: row.SignDate,
		StartDate: row.StartDate, EndDate: row.EndDate, Amount: row.Amount,
		Status: row.Status, Stage: row.Stage, PlanAmount: row.PlanAmount,
		SettledAmount: row.SettledAmount, Outstanding: row.Outstanding,
	}
	for _, plan := range row.Plans {
		out.Plans = append(out.Plans, platformContractPlan{
			ID: plan.ID, Seq: plan.Seq, Name: plan.Name, Kind: plan.Kind, Amount: plan.Amount,
			DueDate: plan.DueDate, SettledAmount: plan.SettledAmount, Outstanding: plan.Outstanding, Status: plan.Status,
		})
	}
	return out
}

// platformContracts 查询当前账号数据范围内的合同。
// @Summary 查询合同列表
// @Tags 开放平台内部接口
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "编号 / 名称 / 往来单位 / 业务员"
// @Param bizType query string false "purchase / sales"
// @Param status query int false "1 执行中 / 2 已完成 / 3 已终止"
// @Param stage query string false "unaudited / pending / accepted / warranty / warranty_due"
// @Param partyId query int false "往来单位 ID，0 不过滤"
// @Param startDate query string false "签订日期起 YYYY-MM-DD"
// @Param endDate query string false "签订日期止 YYYY-MM-DD"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=response.PageData{list=[]inv.platformContractRow}}
// @Failure 400 {object} response.Body
// @Router /internal/open-platform/contracts [get]
func (h *Handler) platformContracts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	filter, err := contractFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	q := page.FromContext(c)
	list, total, err := h.svc.ContractList(filter, q, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformContractRow, 0, len(list))
	for _, row := range list {
		rows = append(rows, projectPlatformContract(row))
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// platformContract 查询合同及收付款计划，越出数据范围返回 404。
// @Summary 查询合同详情
// @Tags 开放平台内部接口
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "合同 ID"
// @Success 200 {object} response.Body{data=inv.platformContractRow}
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Router /internal/open-platform/contracts/{id} [get]
func (h *Handler) platformContract(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	row, err := h.svc.ContractDetail(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, projectPlatformContract(*row))
}

// platformContractReport 查询当前账号数据范围内的合同执行报表。
//
// @Summary 查询合同执行报表
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "编号 / 名称 / 往来单位 / 业务员"
// @Param bizType query string false "purchase / sales"
// @Param status query int false "1 执行中 / 2 已完成 / 3 已终止"
// @Param stage query string false "unaudited / pending / accepted / warranty / warranty_due"
// @Param partyId query int false "往来单位 ID，0 不过滤"
// @Param startDate query string false "签订日期起 YYYY-MM-DD"
// @Param endDate query string false "签订日期止 YYYY-MM-DD"
// @Success 200 {object} response.Body{data=inv.ReportContractResult}
// @Failure 400 {object} response.Body
// @Router /internal/open-platform/contract-report [get]
func (h *Handler) platformContractReport(c *gin.Context) {
	noStore(c)
	h.ReportContract(c)
}

type platformContractLedgerRow struct {
	SettleID   int64     `json:"settleId"`
	SettledAt  time.Time `json:"settledAt"`
	ContractNo string    `json:"contractNo"`
	Name       string    `json:"name"`
	BizType    string    `json:"bizType"`
	PartyName  string    `json:"partyName"`
	PlanSeq    int       `json:"planSeq"`
	PlanName   string    `json:"planName"`
	Amount     float64   `json:"amount"`
	Method     string    `json:"method"`
}

type platformContractLedgerResult struct {
	Rows []platformContractLedgerRow `json:"rows"`
}

// platformContractLedger 查询当前账号数据范围内的合同期次实际收付款。
//
// @Summary 查询合同收付款台账
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "合同编号 / 名称 / 往来单位"
// @Param bizType query string false "purchase / sales"
// @Param status query int false "1 执行中 / 2 已完成 / 3 已终止"
// @Param from query string false "收付日期起 YYYY-MM-DD"
// @Param to query string false "收付日期止 YYYY-MM-DD"
// @Param limit query int false "返回行数上限，1 到 1000，默认 1000"
// @Success 200 {object} response.Body{data=inv.platformContractLedgerResult}
// @Failure 400 {object} response.Body
// @Router /internal/open-platform/contract-ledger [get]
func (h *Handler) platformContractLedger(c *gin.Context) {
	noStore(c)
	filter, err := contractFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	filter.Start, err = parseDatePtr(strings.TrimSpace(c.Query("from")))
	if err != nil {
		response.Fail(c, err)
		return
	}
	filter.End, err = parseDatePtr(strings.TrimSpace(c.Query("to")))
	if err != nil {
		response.Fail(c, err)
		return
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		response.FailWith(c, errcode.ErrBadRequest, "收付开始日期不能晚于结束日期")
		return
	}
	limit := reportContractLimit
	if raw, present := c.GetQuery("limit"); present {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > reportContractLimit {
			response.FailWith(c, errcode.ErrBadRequest, "查询参数 limit 须为 1 到 1000 的整数")
			return
		}
	}

	rows, err := h.svc.ReportContractLedger(filter, page.FromContext(c), middleware.CurrentIdentity(c), limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	result := platformContractLedgerResult{Rows: make([]platformContractLedgerRow, 0, len(rows))}
	for _, row := range rows {
		result.Rows = append(result.Rows, platformContractLedgerRow{
			SettleID: row.SettleID, SettledAt: row.SettledAt,
			ContractNo: row.ContractNo, Name: row.Name, BizType: row.BizType, PartyName: row.PartyName,
			PlanSeq: row.PlanSeq, PlanName: row.PlanName, Amount: row.Amount, Method: row.Method,
		})
	}
	response.OK(c, result)
}
