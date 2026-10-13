package role

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册角色模块路由。
// 权限标识：system:role:list / create / update / delete / assign / export
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/system/roles", middleware.Auth())
	{
		g.GET("", middleware.RequirePerm("system:role:list"), h.List)
		g.GET("/options", middleware.RequirePerm("system:role:list", "system:user:list"), h.Options)
		g.GET("/export", middleware.RequirePerm("system:role:export"), h.Export)
		g.GET("/:id", middleware.RequirePerm("system:role:list"), h.Detail)
		g.POST("", middleware.RequirePerm("system:role:create"), h.Create)
		g.PUT("/:id", middleware.RequirePerm("system:role:update"), h.Update)
		g.DELETE("/:id", middleware.RequirePerm("system:role:delete"), h.Delete)
		g.PUT("/:id/menus", middleware.RequirePerm("system:role:assign"), h.AssignMenus)
	}
}
