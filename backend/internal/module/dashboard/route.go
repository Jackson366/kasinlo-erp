package dashboard

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/middleware"
)

// Register 注册仪表盘模块路由。
//
// 权限码与种子菜单 1（仪表盘）的 Permission 字段一致：前端按权限码决定
// 显不显示菜单，后端就必须按同一个码决定接不接待 —— 只藏菜单不挡接口，
// 等于把接口裸奔在「看得见源码的人」面前。四个接口统一收在 dashboard:view 下。
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/dashboard", middleware.Auth(), middleware.RequirePerm("dashboard:view"))
	{
		g.GET("/overview", h.Overview)
		g.GET("/trend", h.Trend)
		g.GET("/modules", h.Modules)
		g.GET("/recent-logins", h.RecentLogins)
	}
}
