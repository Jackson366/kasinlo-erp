package inv

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// Receivables 应收明细
// @Summary      应收明细
// @Tags         进销存-应收应付
// @Produce      json
// @Security     BearerAuth
// @Param        settle  query  string  false  "unpaid | partial | paid | overdue"
// @Param        source  query  string  false  "order | plan（空=全部）"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.FinanceRow}}
// @Router       /inv/finance/receivables [get]
func (h *Handler) Receivables(c *gin.Context) { h.financeList(c, true) }

// Payables 应付明细
// @Summary      应付明细
// @Tags         进销存-应收应付
// @Produce      json
// @Security     BearerAuth
// @Param        source  query  string  false  "order | plan（空=全部）"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.FinanceRow}}
// @Router       /inv/finance/payables [get]
func (h *Handler) Payables(c *gin.Context) { h.financeList(c, false) }

func (h *Handler) financeList(c *gin.Context, receivable bool) {
	q := page.FromContext(c)
	settle := c.Query("settle")
	source := c.Query("source")
	var (
		rows    []FinanceRow
		summary FinanceSummary
		total   int64
		err     error
	)
	if receivable {
		rows, summary, total, err = h.svc.Receivables(q, settle, source, middleware.CurrentIdentity(c))
	} else {
		rows, summary, total, err = h.svc.Payables(q, settle, source, middleware.CurrentIdentity(c))
	}
	if err != nil {
		response.Fail(c, err)
		return
	}
	// 汇总随列表一并返回，前端无需再发一次请求
	response.OK(c, gin.H{
		"list":     rows,
		"total":    total,
		"page":     q.Page,
		"pageSize": q.PageSize,
		"summary":  summary,
	})
}

// Settle 登记收付款
// @Summary      登记收付款
// @Tags         进销存-应收应付
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  SettleReq  true  "结算信息"
// @Success      200  {object}  response.Body
// @Router       /inv/finance/settlements [post]
func (h *Handler) Settle(c *gin.Context) {
	var req SettleReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "登记收付款")
	if err := h.svc.Settle(req, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "登记成功")
}

// SettleBatch 批量登记收付款（一张单据一笔流水，整批原子）
// @Summary      批量登记收付款
// @Tags         进销存-应收应付
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  SettleBatchReq  true  "批量结算信息"
// @Success      200  {object}  response.Body
// @Router       /inv/finance/settlements/batch [post]
func (h *Handler) SettleBatch(c *gin.Context) {
	var req SettleBatchReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "批量登记收付款")
	res, err := h.svc.SettleBatch(req, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// OrderSettlementHistory 查看原单收付款与处理记录
// @Summary      查看单据收付款记录
// @Tags         进销存-应收应付
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "单据 ID"
// @Success      200 {object} response.Body{data=inv.OrderSettlements}
// @Router       /inv/finance/orders/{id}/settlements [get]
func (h *Handler) OrderSettlementHistory(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	history, err := h.svc.OrderSettlementHistory(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, history)
}

// SettlementOffsetTargets 查询同一往来单位的未结单据
// @Summary      查询可抵扣单据
// @Tags         进销存-应收应付
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "原单 ID"
// @Param        keyword query string false "单据编号"
// @Success      200 {object} response.Body{data=[]inv.SettlementOffsetTarget}
// @Router       /inv/finance/orders/{id}/offset-targets [get]
func (h *Handler) SettlementOffsetTargets(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows, err := h.svc.SettlementOffsetTargets(id, c.Query("keyword"), middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rows)
}

// AdjustOrderSettlement 追加收付款更正、退款或往来抵扣流水
// @Summary      处理已登记收付款
// @Tags         进销存-应收应付
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "原单 ID"
// @Param        settlementId path int true "原收付款记录 ID"
// @Param        body body SettlementAdjustmentReq true "处理信息"
// @Success      200 {object} response.Body{data=response.IDData}
// @Failure      409 {object} response.Body
// @Router       /inv/finance/orders/{id}/settlements/{settlementId}/adjustments [post]
func (h *Handler) AdjustOrderSettlement(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	settlementID, err := strconv.ParseInt(c.Param("settlementId"), 10, 64)
	if err != nil || settlementID <= 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "收付款记录编号非法"))
		return
	}
	var req SettlementAdjustmentReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "处理收付款")
	adjustmentID, err := h.svc.AdjustOrderSettlement(id, settlementID, req, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: adjustmentID})
}
