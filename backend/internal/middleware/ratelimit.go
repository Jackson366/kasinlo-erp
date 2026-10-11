package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/limiter"
	"nova-admin/internal/pkg/response"
)

// RateLimit 返回一个「每 IP 请求数」的滑动窗口限流中间件。
//
// 与 auth 服务里的失败限流分工不同：
//   - 失败限流按「失败次数」计，只用于登录 —— 它要能不误伤正常登录；
//   - 本中间件按「请求次数」计，用于 /auth/refresh 这种没有凭据猜测面、
//     但可能被脚本反复打的接口，纯粹做防刷。
//
// max <= 0 或 window <= 0 时直接放行（相当于关闭）。
func RateLimit(max int, window time.Duration) gin.HandlerFunc {
	l := limiter.New(max, window)
	if max <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		// 客户端 IP 由 gin 依据可信代理配置解析，未配置代理时即 TCP 对端地址
		key := c.ClientIP()
		// 用原子方法而不是 Allow + Fail 两步：后者在并发下会被同时通过的
		// N 个请求一起突破阈值，限流器恰好在爆破场景下失效。
		if !l.AllowAndConsume(key) {
			response.FailWith(c, errcode.ErrTooManyReq, "")
			return
		}
		c.Next()
	}
}
