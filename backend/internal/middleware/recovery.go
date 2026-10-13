package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"nova-erp/internal/logger"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/response"
)

// Recovery 捕获 panic，记录堆栈并返回统一错误响应。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					zap.String("traceId", TraceID(c)),
					zap.String("path", c.Request.URL.Path),
					zap.Any("error", err),
					zap.ByteString("stack", debug.Stack()),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, response.Body{
					Code:    int(errcode.ErrInternal),
					Message: errcode.ErrInternal.Message(),
					TraceID: TraceID(c),
				})
			}
		}()
		c.Next()
	}
}

// NoRoute 统一 404 响应。
func NoRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		response.FailWith(c, errcode.ErrNotFound, "接口不存在: "+c.Request.URL.Path)
	}
}
