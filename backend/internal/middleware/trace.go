package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"nova-erp/internal/logger"
)

const traceHeader = "X-Trace-Id"

// traceIDRe 限定链路 ID 的可用字符。
//
// 只限长度是不够的：这个值会被原样写进响应头和日志。
// 客户端可以塞入 CR/LF 制造响应头拆分，或塞入控制字符污染日志行；
// 长度受控的前提下，字符集也必须一并收窄 —— 不在集合内的一律重新生成。
var traceIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// TraceID 为每个请求生成/透传链路 ID。
func TraceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(traceHeader)
		if !traceIDRe.MatchString(id) {
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
