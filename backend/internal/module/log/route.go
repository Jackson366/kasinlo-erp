package log

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/config"
	"nova-admin/internal/middleware"
)

// Register 注册日志模块路由。
// 权限标识：system:log:list / delete / export
// sec 用于取登录锁定期：清空登录日志时必须保留锁定期内的失败记录（见 ClearLogins）。
func Register(rg *gin.RouterGroup, sec config.SecurityConfig) {
	h := NewHandler(NewService(sec))
	g := rg.Group("/system/logs", middleware.Auth())
	{
		g.GET("/operations", middleware.RequirePerm("system:log:list"), h.Operations)
		g.GET("/operations/export", middleware.RequirePerm("system:log:export"), h.ExportOperations)
		g.GET("/logins", middleware.RequirePerm("system:log:list"), h.Logins)
		g.GET("/logins/export", middleware.RequirePerm("system:log:export"), h.ExportLogins)
		g.GET("/modules", middleware.RequirePerm("system:log:list"), h.Modules)
		g.DELETE("/operations", middleware.RequirePerm("system:log:delete"), h.ClearOperations)
		g.DELETE("/logins", middleware.RequirePerm("system:log:delete"), h.ClearLogins)
	}
}
