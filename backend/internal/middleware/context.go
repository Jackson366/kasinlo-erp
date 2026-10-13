package middleware

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/authz"
)

// gin.Context 中存放键的常量，避免魔法字符串散落各处。
const (
	CtxTraceID  = "traceId"
	CtxUserID   = "userId"
	CtxUsername = "username"
	CtxIdentity = "identity"
)

// CurrentUserID 从上下文取当前用户 ID，未登录返回 0。
func CurrentUserID(c *gin.Context) int64 {
	if v, ok := c.Get(CtxUserID); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

// CurrentUsername 从上下文取当前用户名。
func CurrentUsername(c *gin.Context) string {
	if v, ok := c.Get(CtxUsername); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// CurrentIdentity 从上下文取完整身份，未登录返回 nil。
func CurrentIdentity(c *gin.Context) *authz.Identity {
	if v, ok := c.Get(CtxIdentity); ok {
		if id, ok := v.(*authz.Identity); ok {
			return id
		}
	}
	return nil
}

// TraceID 从上下文取链路 ID。
func TraceID(c *gin.Context) string {
	if v, ok := c.Get(CtxTraceID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func setTraceID(c *gin.Context, id string) { c.Set(CtxTraceID, id) }
