package dept

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册部门模块路由。
// 权限标识：system:dept:list / create / update / delete
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/system/depts", middleware.Auth())
	{
		g.GET("", middleware.RequirePerm("system:dept:list", "system:user:list"), h.List)
		// /export 必须排在 /:id 之前：gin 的路由树里静态段与参数段在同一层会冲突
		g.GET("/export", middleware.RequirePerm("system:dept:list", "system:user:list"), h.Export)
		g.GET("/:id", middleware.RequirePerm("system:dept:list"), h.Detail)
		g.POST("", middleware.RequirePerm("system:dept:create"), h.Create)
		g.PUT("/:id", middleware.RequirePerm("system:dept:update"), h.Update)
		g.DELETE("/:id", middleware.RequirePerm("system:dept:delete"), h.Delete)
	}
}
