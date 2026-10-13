package profile

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册个人中心模块路由，全部需要登录。
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/profile", middleware.Auth())
	{
		g.GET("", h.Info)
		g.PUT("", h.Update)
		g.PUT("/password", h.ChangePassword)
		g.GET("/routes", h.Routes)
	}
}
