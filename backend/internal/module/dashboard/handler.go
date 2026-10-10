package dashboard

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/pkg/response"
)

// Handler 仪表盘接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建仪表盘处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Overview 概览统计
// @Summary      仪表盘概览统计
// @Description  返回用户、角色、部门、菜单总数以及今日登录/操作/失败次数、近一周活跃用户数。
// @Tags         仪表盘
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=dashboard.Overview}
// @Failure      401  {object}  response.Body  "未登录或令牌失效"
// @Router       /dashboard/overview [get]
func (h *Handler) Overview(c *gin.Context) {
	data, err := h.svc.Overview()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Trend 趋势统计
// @Summary      登录与操作趋势
// @Tags         仪表盘
// @Produce      json
// @Security     BearerAuth
// @Param        days  query     int  false  "统计天数"  default(7)
// @Success      200   {object}  response.Body{data=[]dashboard.TrendPoint}
// @Failure      401   {object}  response.Body  "未登录或令牌失效"
// @Router       /dashboard/trend [get]
func (h *Handler) Trend(c *gin.Context) {
	days := atoiOr(c.Query("days"), 7)
	data, err := h.svc.Trend(days)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Modules 模块排行
// @Summary      模块操作量排行
// @Tags         仪表盘
// @Produce      json
// @Security     BearerAuth
// @Param        days   query     int  false  "统计天数"    default(7)
// @Param        limit  query     int  false  "返回条数"    default(8)
// @Success      200    {object}  response.Body{data=[]dashboard.ModuleStat}
// @Failure      401    {object}  response.Body  "未登录或令牌失效"
// @Router       /dashboard/modules [get]
func (h *Handler) Modules(c *gin.Context) {
	days := atoiOr(c.Query("days"), 7)
	limit := atoiOr(c.Query("limit"), 8)
	data, err := h.svc.ModuleStats(days, limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// RecentLogins 最近登录
// @Summary      最近登录记录
// @Tags         仪表盘
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query     int  false  "返回条数"  default(8)
// @Success      200    {object}  response.Body{data=[]model.LoginLog}
// @Failure      401    {object}  response.Body  "未登录或令牌失效"
// @Router       /dashboard/recent-logins [get]
func (h *Handler) RecentLogins(c *gin.Context) {
	limit := atoiOr(c.Query("limit"), 8)
	data, err := h.svc.RecentLogins(limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
