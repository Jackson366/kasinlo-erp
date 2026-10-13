package auth

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/middleware"
)

// Register 注册认证模块路由。
func Register(rg *gin.RouterGroup, cfg config.SecurityConfig) {
	h := NewHandler(NewService(cfg))
	g := rg.Group("/auth")
	{
		// 登录的防爆破靠「按账号+IP 锁定」与「按 IP 失败限流」，
		// 二者都在 Service 里，因为判定依赖登录结果，中间件拿不到。
		g.POST("/login", h.Login)
		// 刷新接口只做防刷：阈值放宽（默认 120/分钟），阈值用尽时返回 429。
		g.POST("/refresh", middleware.RateLimit(cfg.RefreshRateLimitPerMin(), time.Minute), h.Refresh)
		g.POST("/logout", middleware.Auth(), h.Logout)
	}
}
