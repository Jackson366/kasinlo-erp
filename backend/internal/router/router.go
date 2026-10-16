package router

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	"nova-erp/api/docs"
	"nova-erp/internal/config"
	"nova-erp/internal/logger"
	"nova-erp/internal/metrics"
	"nova-erp/internal/middleware"
	"nova-erp/internal/module"
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

	// 指标采集只在 /metrics 启用时装配：关闭态下它没有任何消费者，
	// 装配上去只是白白给每个请求多加一次打点与加锁。
	if cfg.App.MetricsEnabled() {
		engine.Use(metrics.Middleware())
	}

	// 在线接口文档。默认跟随运行模式：debug 打开、release 关闭（可用 app.swagger 覆盖）。
	// BasePath 在启动时同步为真实的 api 前缀 —— 否则改了 app.version 之后，
	// 文档里的路径全是旧的，照着文档调接口只会 404。
	if cfg.App.SwaggerEnabled() {
		docs.SwaggerInfo.BasePath = cfg.App.BasePath()
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// 监控端点：默认关闭，开启后可选用 app.metrics_token 加一道 Bearer 校验。
	if cfg.App.MetricsEnabled() {
		if cfg.App.MetricsToken == "" {
			logger.Warn("已启用 /metrics 但未配置 app.metrics_token，端点对任何可访问者开放",
				zap.String("建议", "把该端点限制在内网，或设置 NOVA_APP_METRICS_TOKEN"))
		}
		engine.GET("/metrics", metricsGuard(cfg.App.MetricsToken, metrics.Handler()))
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

	// 各模块自己声明鉴权与权限要求，装配顺序由组装层（internal/assembly）决定。
	// 本包不再认识任何具体模块 —— 加一个模块不必改这里。
	for _, m := range module.All() {
		if m.Register != nil {
			m.Register(base)
		}
	}

	return engine
}

// metricsGuard 在配置了令牌时校验 /metrics 的访问凭据。
//
// 令牌为空则不加校验：此时安全性由「默认关闭 + 只在内网暴露」承担，
// 启动时会打一条 WARN 明确提醒，不静默放行。
func metricsGuard(token string, next gin.HandlerFunc) gin.HandlerFunc {
	if token == "" {
		return next
	}
	return func(c *gin.Context) {
		got := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		// 定长比较：普通 == 会在首个不同字节处提前返回，理论上可用于逐字节试探令牌。
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		next(c)
	}
}
