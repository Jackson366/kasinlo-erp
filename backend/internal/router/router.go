package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/config"
	"nova-admin/internal/database"
	"nova-admin/internal/middleware"
	"nova-admin/internal/module/auth"
	"nova-admin/internal/module/dashboard"
	"nova-admin/internal/module/dept"
	"nova-admin/internal/module/log"
	"nova-admin/internal/module/menu"
	"nova-admin/internal/module/profile"
	"nova-admin/internal/module/role"
	"nova-admin/internal/module/user"
)

// New 构建 gin 引擎并装配全局中间件与业务模块路由。
//
// 中间件顺序说明：
//
//	TraceID -> Logger -> Recovery -> CORS -> [业务组] OperationLog -> Auth -> RequirePerm -> Handler
//
// 操作日志中间件放在业务组最外层，确保能读到 Auth 写入的用户身份。
func New(cfg *config.Config) *gin.Engine {
	if !cfg.App.IsDebug() {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(
		middleware.TraceIDMiddleware(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(cfg.CORS.AllowOrigins),
	)
	engine.NoRoute(middleware.NoRoute())

	// 健康检查（不鉴权、不记日志）
	engine.GET("/health", func(c *gin.Context) {
		status := "ok"
		code := http.StatusOK
		if err := database.Health(); err != nil {
			status = "degraded: " + err.Error()
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{
			"status":  status,
			"app":     cfg.App.Name,
			"version": cfg.App.Version,
		})
	})

	// 业务路由统一前缀，例如 /api/v1
	base := engine.Group(cfg.App.BasePath(), middleware.OperationLog())

	// 公开接口（登录、刷新令牌）
	auth.Register(base, cfg.Security)

	// 需要登录的模块，各模块自行声明鉴权与权限要求
	profile.Register(base)
	dashboard.Register(base)
	user.Register(base)
	role.Register(base)
	menu.Register(base)
	dept.Register(base)
	log.Register(base)

	return engine
}
