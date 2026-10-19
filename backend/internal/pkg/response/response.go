package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"nova-erp/internal/logger"
	"nova-erp/internal/pkg/errcode"
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

// IDData 是「只返回资源 ID」的统一结构（新增接口使用）。
// 用具体类型而不是 gin.H：gin.H 是 map，Swagger 无法据此推导出字段，
// 生成的文档里 data 会是一片空白。
type IDData struct {
	ID int64 `json:"id"`
}

// DeletedData 是「返回被清理条数」的统一结构（清空类接口使用）。
type DeletedData struct {
	Deleted int64 `json:"deleted"`
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
		// 非业务错误（内部 bug / 数据库 / IO / 文件系统）不把 err.Error() 透给客户端：
		// 里面可能带文件路径、SQL、底层错误链，既无益于用户判断，也扩大了信息暴露面。
		// 对外归一到 50000 通用文案，完整错误只进服务端日志（下面已记）。
		msg = ""
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

func traceOf(c *gin.Context) string {
	if v, ok := c.Get("traceId"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
