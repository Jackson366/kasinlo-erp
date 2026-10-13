package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// Overview 数据概览
// @Summary      数据概览
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=inv.OverviewVO}
// @Router       /inv/overview [get]
func (h *Handler) Overview(c *gin.Context) {
	data, err := h.svc.Overview()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportSummary 报表汇总
// @Summary      报表汇总（营业收入 / 成本 / 毛利）
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        range  query  string  false  "today | week | month | quarter | year"
// @Success      200  {object}  response.Body{data=inv.ReportSummary}
// @Router       /inv/report/summary [get]
func (h *Handler) ReportSummary(c *gin.Context) {
	data, err := h.svc.ReportSummary(c.Query("range"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportTrend 销售 / 采购趋势
// @Summary      销售与采购趋势
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        days  query  int  false  "天数，默认 30"
// @Success      200  {object}  response.Body{data=[]inv.ReportTrendPoint}
// @Router       /inv/report/trend [get]
func (h *Handler) ReportTrend(c *gin.Context) {
	days := page.AtoiOr(c.Query("days"), 30)
	data, err := h.svc.ReportTrend(days)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportRanking 销售排行
// @Summary      销售排行（按客户）
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        range  query  string  false  "时间范围"
// @Param        limit  query  int     false  "条数，默认 5"
// @Success      200  {object}  response.Body{data=[]inv.ReportRankRow}
// @Router       /inv/report/ranking [get]
func (h *Handler) ReportRanking(c *gin.Context) {
	limit := page.AtoiOr(c.Query("limit"), 5)
	data, err := h.svc.ReportRanking(c.Query("range"), limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportCategory 品类销售占比
// @Summary      品类销售占比
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        range  query  string  false  "时间范围"
// @Success      200  {object}  response.Body{data=[]inv.ReportCategoryRow}
// @Router       /inv/report/category [get]
func (h *Handler) ReportCategory(c *gin.Context) {
	data, err := h.svc.ReportCategory(c.Query("range"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
