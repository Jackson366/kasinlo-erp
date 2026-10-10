package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	"nova-admin/api/docs"
	"nova-admin/internal/config"
	"nova-admin/internal/logger"
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
//	TraceID -> Logger -> OperationLog -> Recovery -> CORS -> Auth -> RequirePerm -> Handler
//
// 两处顺序是有意为之，改动前请先看完理由：
//
//  1. OperationLog 必须在 Recovery **外面**。它靠 c.Next() 返回之后读状态码来落库，
//     而 panic 会让内层中间件的「后置代码」整段跳过。若把它放在 Recovery 里层，
//     服务端 500 的那次请求在审计表里就查不到 —— 丢掉的恰好是最该留痕的记录。
//  2. 操作日志放在引擎级而不是业务组上，是为了连未匹配到路由的请求也一并记录。
func New(cfg *config.Config) *gin.Engine {
	if !cfg.App.IsDebug() {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()

	// 只信任显式配置的代理。gin 的默认值是信任所有代理，于是 X-Forwarded-For
	// （完全由客户端控制）会直接变成审计日志里的来源 IP。配置已在 config.validate 校验过。
	if err := engine.SetTrustedProxies(cfg.Security.TrustedProxies); err != nil {
		// 走到这里说明校验与解析行为不一致，按「不信任任何代理」保守处理并留痕
		logger.Error("可信代理配置解析失败，已按不信任任何代理启动", zap.Error(err))
		_ = engine.SetTrustedProxies(nil)
	}

	engine.Use(
		middleware.TraceIDMiddleware(),
		middleware.Logger(),
		middleware.OperationLog(),
		middleware.Recovery(),
		middleware.CORS(cfg.CORS.AllowOrigins),
	)
	engine.NoRoute(middleware.NoRoute())

	// 在线接口文档。默认跟随运行模式：debug 打开、release 关闭（可用 app.swagger 覆盖）。
	// BasePath 在启动时同步为真实的 api 前缀 —— 否则改了 app.version 之后，
	// 文档里的路径全是旧的，照着文档调接口只会 404。
	if cfg.App.SwaggerEnabled() {
		docs.SwaggerInfo.BasePath = cfg.App.BasePath()
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// 健康检查（不鉴权、不记日志）。DB 状态做 1 秒记忆，避免被刷成打库通道。
	engine.GET("/health", func(c *gin.Context) {
		status := "ok"
		code := http.StatusOK
		if err := healthError(); err != nil {
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
	base := engine.Group(cfg.App.BasePath())

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
