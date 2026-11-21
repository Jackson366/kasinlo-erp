package inv

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/response"
)

type platformContractActivityRow struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type platformContractActivityData struct {
	ContractID int64                         `json:"contractId"`
	Rows       []platformContractActivityRow `json:"rows"`
}

// platformContractActivity 查询合同最近 50 条已落库的操作记录。
//
// @Summary 查询合同操作记录
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "合同 ID"
// @Success 200 {object} response.Body{data=inv.platformContractActivityData}
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Router /internal/open-platform/contract-activities/{id} [get]
func (h *Handler) platformContractActivity(c *gin.Context) {
	noStore(c)
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	history, err := h.svc.ContractLogs(id, middleware.CurrentIdentity(c), 0)
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformContractActivityRow, 0, len(history))
	for _, row := range history {
		rows = append(rows, platformContractActivityRow{
			ID: row.ID, Action: row.Action, Method: row.Method, Status: row.Status, CreatedAt: row.CreatedAt,
		})
	}
	response.OK(c, platformContractActivityData{ContractID: id, Rows: rows})
}

type platformContractPlanSettlementRow struct {
	ID        int64     `json:"id"`
	Amount    float64   `json:"amount"`
	Method    string    `json:"method"`
	SettledAt time.Time `json:"settledAt"`
}

type platformContractPlanSettlementData struct {
	PlanID int64                               `json:"planId"`
	Rows   []platformContractPlanSettlementRow `json:"rows"`
}

// platformContractPlanSettlements 查询当前账号可见合同的收付款期次流水。
//
// @Summary 查询合同期次收付款流水
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "期次 ID"
// @Success 200 {object} response.Body{data=inv.platformContractPlanSettlementData}
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Router /internal/open-platform/contract-plan-settlements/{id} [get]
func (h *Handler) platformContractPlanSettlements(c *gin.Context) {
	noStore(c)
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	history, err := h.svc.PlanSettleHistory(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformContractPlanSettlementRow, 0, len(history))
	for _, row := range history {
		rows = append(rows, platformContractPlanSettlementRow{
			ID: row.ID, Amount: row.Amount, Method: row.Method, SettledAt: row.SettledAt,
		})
	}
	response.OK(c, platformContractPlanSettlementData{PlanID: id, Rows: rows})
}
