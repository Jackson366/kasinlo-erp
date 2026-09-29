package auth

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/config"
	"nova-admin/internal/middleware"
)

// Register 注册认证模块路由。
func Register(rg *gin.RouterGroup, cfg config.SecurityConfig) {
	h := NewHandler(NewService(cfg))
	g := rg.Group("/auth")
	{
		g.POST("/login", h.Login)
		g.POST("/refresh", h.Refresh)
		g.POST("/logout", middleware.Auth(), h.Logout)
	}
}
