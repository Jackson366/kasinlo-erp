package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"nova-admin/internal/logger"
)

const traceHeader = "X-Trace-Id"

// TraceID 为每个请求生成/透传链路 ID。
func TraceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(traceHeader)
		if id == "" || len(id) > 64 {
			id = newTraceID()
		}
		setTraceID(c, id)
		c.Header(traceHeader, id)
		c.Next()
	}
}

func newTraceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "trace-fallback"
	}
	return hex.EncodeToString(b)
}

// Logger 记录访问日志。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		path := c.Request.URL.Path
		// 静态资源与健康检查不记录，避免日志噪音
		if strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/favicon") {
			return
		}

		fields := []zap.Field{
			zap.String("traceId", TraceID(c)),
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("ip", c.ClientIP()),
			zap.Int("size", c.Writer.Size()),
		}
		if uid := CurrentUserID(c); uid > 0 {
			fields = append(fields, zap.Int64("userId", uid), zap.String("username", CurrentUsername(c)))
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		switch {
		case c.Writer.Status() >= 500:
			logger.Error("request", fields...)
		case c.Writer.Status() >= 400:
			logger.Warn("request", fields...)
		default:
			logger.Info("request", fields...)
		}
	}
}
