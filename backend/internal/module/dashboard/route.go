package dashboard

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/middleware"
)

// Register 注册仪表盘模块路由。
// 登录即可访问；如需权限控制可加 RequirePerm("dashboard:view")。
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/dashboard", middleware.Auth())
	{
		g.GET("/overview", h.Overview)
		g.GET("/trend", h.Trend)
		g.GET("/modules", h.Modules)
		g.GET("/recent-logins", h.RecentLogins)
	}
}
