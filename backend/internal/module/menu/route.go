package menu

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册菜单模块路由。
// 权限标识：system:menu:list / create / update / delete
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/system/menus", middleware.Auth())
	{
		g.GET("", middleware.RequirePerm("system:menu:list"), h.List)
		// /export 必须排在 /:id 之前：gin 的路由树里静态段与参数段在同一层会冲突
		g.GET("/export", middleware.RequirePerm("system:menu:list"), h.Export)
		g.GET("/:id", middleware.RequirePerm("system:menu:list"), h.Detail)
		g.POST("", middleware.RequirePerm("system:menu:create"), h.Create)
		g.PUT("/:id", middleware.RequirePerm("system:menu:update"), h.Update)
		g.DELETE("/:id", middleware.RequirePerm("system:menu:delete"), h.Delete)
	}
}
