package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"nova-admin/internal/logger"
	"nova-admin/internal/pkg/errcode"
)

// Body 是所有接口的统一响应结构。
type Body struct {
	Code    int    `json:"code"`              // 业务码，0 表示成功
	Message string `json:"message"`           // 提示信息
	Data    any    `json:"data,omitempty"`    // 业务数据
	TraceID string `json:"traceId,omitempty"` // 链路追踪 ID
}

// PageData 是分页数据的统一结构。
type PageData struct {
	List     any   `json:"list"`     // 当前页数据
	Total    int64 `json:"total"`    // 总条数
	Page     int   `json:"page"`     // 当前页码
	PageSize int   `json:"pageSize"` // 每页条数
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "success", Data: data, TraceID: traceOf(c)})
}

// OKPage 返回分页成功响应。
func OKPage(c *gin.Context, list any, total int64, page, pageSize int) {
	OK(c, PageData{List: list, Total: total, Page: page, PageSize: pageSize})
}

// OKMsg 返回仅带消息的成功响应。
func OKMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: msg, TraceID: traceOf(c)})
}

// Fail 返回失败响应；传入普通 error 时归一到 50000。
func Fail(c *gin.Context, err error) {
	code := errcode.ErrInternal
	msg := ""

	var be *errcode.Error
	if errors.As(err, &be) {
		code = be.Code
		msg = be.Msg
		// 业务错误对外只暴露提示语，底层原因记录到服务端日志便于排查
		if be.Err != nil {
			logger.Error("请求处理失败",
				zap.String("traceId", traceOf(c)),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("code", int(code)),
				zap.Error(be.Err),
			)
		}
	} else if err != nil {
		msg = err.Error()
		logger.Error("请求处理失败",
			zap.String("traceId", traceOf(c)),
			zap.String("path", c.Request.URL.Path),
			zap.Error(err),
		)
	}
	FailWith(c, code, msg)
}

// FailWith 返回指定业务码的失败响应。
func FailWith(c *gin.Context, code errcode.Code, msg string) {
	if msg == "" {
		msg = code.Message()
	}
	status := code.HTTPStatus()
	// 供操作日志中间件采集失败原因
	c.Set("errorMsg", msg)
	c.JSON(status, Body{Code: int(code), Message: msg, TraceID: traceOf(c)})
	c.Abort()
}

// FailBadRequest 参数错误快捷方法。
func FailBadRequest(c *gin.Context, msg string) { FailWith(c, errcode.ErrBadRequest, msg) }

// FailUnauthorized 未认证快捷方法。
func FailUnauthorized(c *gin.Context, msg string) { FailWith(c, errcode.ErrUnauthorized, msg) }

// FailForbidden 无权限快捷方法。
func FailForbidden(c *gin.Context, msg string) { FailWith(c, errcode.ErrForbidden, msg) }

func traceOf(c *gin.Context) string {
	if v, ok := c.Get("traceId"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
