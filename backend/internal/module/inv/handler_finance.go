package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// Receivables 应收明细
// @Summary      应收明细
// @Tags         进销存-应收应付
// @Produce      json
// @Security     BearerAuth
// @Param        settle  query  string  false  "unpaid | partial | paid | overdue"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.FinanceRow}}
// @Router       /inv/finance/receivables [get]
func (h *Handler) Receivables(c *gin.Context) { h.financeList(c, true) }

// Payables 应付明细
// @Summary      应付明细
// @Tags         进销存-应收应付
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.FinanceRow}}
// @Router       /inv/finance/payables [get]
func (h *Handler) Payables(c *gin.Context) { h.financeList(c, false) }

func (h *Handler) financeList(c *gin.Context, receivable bool) {
	q := page.FromContext(c)
	settle := c.Query("settle")
	var (
		rows    []FinanceRow
		summary FinanceSummary
		total   int64
		err     error
	)
	if receivable {
		rows, summary, total, err = h.svc.Receivables(q, settle)
	} else {
		rows, summary, total, err = h.svc.Payables(q, settle)
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
	if err := h.svc.Settle(req, actorID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "登记成功")
}
