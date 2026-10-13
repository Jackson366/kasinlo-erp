package user

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册用户模块路由。
// 权限标识：system:user:list / create / update / delete / reset / export / import
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/system/users", middleware.Auth())
	{
		g.GET("", middleware.RequirePerm("system:user:list"), h.List)
		g.GET("/options", middleware.RequirePerm("system:user:list", "system:role:list", "system:dept:list"), h.Options)

		// 导出与导入必须注册在 /:id 之前语义更清晰；
		// Gin 的路由树对静态段优先匹配，不会与 /:id 冲突。
		g.GET("/export", middleware.RequirePerm("system:user:export"), h.Export)
		g.GET("/import-template", middleware.RequirePerm("system:user:import"), h.ImportTemplate)
		g.POST("/import", middleware.RequirePerm("system:user:import"), h.Import)

		g.GET("/:id", middleware.RequirePerm("system:user:list"), h.Detail)
		g.POST("", middleware.RequirePerm("system:user:create"), h.Create)
		g.PUT("/:id", middleware.RequirePerm("system:user:update"), h.Update)
		g.DELETE("/:id", middleware.RequirePerm("system:user:delete"), h.Delete)
		g.PUT("/:id/status", middleware.RequirePerm("system:user:update"), h.UpdateStatus)
		g.PUT("/:id/password", middleware.RequirePerm("system:user:reset"), h.ResetPassword)
	}
}
