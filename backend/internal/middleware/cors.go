package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS 处理跨域请求，允许携带自定义头与凭证。
func CORS(allowOrigins []string) gin.HandlerFunc {
	allowAll := false
	allowed := make(map[string]struct{}, len(allowOrigins))
	for _, o := range allowOrigins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		}
		allowed[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			switch {
			case allowAll:
				// 配置了 * 时把来源原样反射回去，但**不能**同时声明允许携带凭证：
				// 那等于告诉浏览器「任何站点都可以发起带身份的跨站请求」。
				// 本项目用 Authorization 头而非 Cookie 承载身份，去掉这个头不影响功能。
				c.Header("Access-Control-Allow-Origin", origin)
			default:
				if _, ok := allowed[origin]; ok {
					c.Header("Access-Control-Allow-Origin", origin)
					c.Header("Access-Control-Allow-Credentials", "true")
				}
			}
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Trace-Id")
			c.Header("Access-Control-Expose-Headers", "X-Trace-Id")
			c.Header("Access-Control-Max-Age", "86400")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
