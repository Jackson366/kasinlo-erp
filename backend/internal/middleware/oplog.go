package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/oplog"
	"nova-erp/internal/pkg/textutil"
)

// sensitiveKeys 命中这些关键字的字段在入库前会被脱敏。
var sensitiveKeys = []string{"password", "oldpassword", "newpassword", "token", "secret", "refreshToken"}

const (
	// maxCaptureBytes 是单个请求体「最多读取并留存」的字节数。
	// 超过这个长度的请求体不会被记录，但**必须完整回填**给后续 handler。
	maxCaptureBytes = 64 * 1024
	// maxRawBodyChars 是非 JSON 请求体裁剪后的最大字符数。
	maxRawBodyChars = 500
)

// OperationLog 记录写操作日志（GET/OPTIONS 只统计不落库）。
func OperationLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		var body string
		if shouldCaptureBody(c.Request.Method) {
			body = readBody(c)
		}

		c.Next()

		// 落库条件：写操作，或者 handler 显式声明了操作名。
		// 后者是为导出接口准备的 —— 导出走 GET（返回二进制流），
		// 但它是数据外带风险最高的操作，漏记等于没有审计。
		action := c.GetString("opAction")
		if !shouldRecord(c.Request.Method) && action == "" {
			return
		}

		status := c.Writer.Status()
		errMsg := ""
		if len(c.Errors) > 0 {
			errMsg = c.Errors.String()
		}
		if errMsg == "" && status >= 400 {
			errMsg = c.GetString("errorMsg")
		}

		if action == "" {
			action = methodAction(c.Request.Method)
		}

		oplog.Write(oplog.Entry{
			UserID:    CurrentUserID(c),
			Username:  CurrentUsername(c),
			Module:    resolveModule(c),
			Action:    action,
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Request:   body,
			Status:    status,
			LatencyMS: time.Since(start).Milliseconds(),
			ErrorMsg:  errMsg,
			CreatedAt: time.Now(),
		})
	}
}

func shouldCaptureBody(method string) bool {
	return method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE"
}

func shouldRecord(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func methodAction(method string) string {
	switch method {
	case "POST":
		return "新增"
	case "PUT", "PATCH":
		return "修改"
	case "DELETE":
		return "删除"
	case "GET":
		return "查询"
	}
	return method
}

// readBody 读取请求体用于记录，并把**完整**请求体回填给后续 handler。
//
// 这里有两个必须同时成立的约束，处理不当会造成线上缺陷：
//
//  1. 回填必须完整。曾经只把读到的前 64KB 回填回 c.Request.Body，
//     超过 64KB 的 multipart 上传会因此丢掉 boundary，c.FormFile 解析失败，
//     最终报出「请选择要上传的 Excel 文件」这种指向完全错误原因的提示。
//  2. 非文本请求体不记录。xlsx 等二进制内容含 0x00 等非法 UTF-8 字节，
//     写进 PostgreSQL 的 text 列会被直接拒绝（SQLSTATE 22021），
//     结果是上传接口的操作日志一条都落不了库 —— 恰好丢的是最该审计的记录。
func readBody(c *gin.Context) string {
	body := c.Request.Body
	if body == nil {
		return ""
	}

	// 二进制请求体（multipart 上传等）整体跳过，既不脱敏也不入库
	if !isTextContentType(c.Request.Header.Get("Content-Type")) {
		return "(二进制请求体，未记录)"
	}

	// 多读 1 字节用于判断「是否还有剩余内容未读」
	raw, err := io.ReadAll(io.LimitReader(body, maxCaptureBytes+1))
	// 无论成败都把「已读部分 + 未读部分」接回去，保证 handler 拿到完整请求体。
	// 注意 io.MultiReader 要在 ReadAll 之后构造：此时 body 正好停在未读处。
	c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), body))
	if err != nil {
		return ""
	}
	if len(raw) == 0 {
		return ""
	}
	if len(raw) > maxCaptureBytes {
		return fmt.Sprintf("(请求体超过 %d KB，未记录)", maxCaptureBytes>>10)
	}
	return sanitize(raw, c.Request.Header.Get("Content-Type"))
}

// isTextContentType 判断请求体是否值得作为文本记录。
// Content-Type 缺失时按文本处理（兼容老客户端直传 JSON 的场景）。
func isTextContentType(ct string) bool {
	if ct == "" {
		return true
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "application/json",
		"text/plain",
		"application/x-www-form-urlencoded":
		return true
	default:
		return false
	}
}

func sanitize(raw []byte, contentType string) string {
	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		// 表单提交同样要脱敏：application/x-www-form-urlencoded 不是 JSON，
		// 之前直接截断原样入库，等于把 password=xxx 明文写进审计表。
		// 解析成 map 后走同一套 redact，与 JSON 口径一致。
		if isFormContentType(contentType) {
			if form, ferr := url.ParseQuery(string(raw)); ferr == nil && len(form) > 0 {
				m := make(map[string]any, len(form))
				for k, vs := range form {
					if len(vs) > 0 {
						m[k] = vs[0]
					}
				}
				if out, merr := json.Marshal(redact(m)); merr == nil {
					return string(out)
				}
			}
		}
		// 非 JSON 提交：先剔除非法 UTF-8 字节，再按「字符」截断。
		// 两处都必要 —— 按字节截断会把多字节中文切成两半，同样造出非法 UTF-8。
		// 截断逻辑收敛在 textutil，与导出表格、日志落盘同一份实现。
		return textutil.TruncateEllipsis(strings.ToValidUTF8(string(raw), ""), maxRawBodyChars, "...")
	}
	out, err := json.Marshal(redact(obj))
	if err != nil {
		return ""
	}
	return string(out)
}

// isFormContentType 判断是否为表单提交（忽略 charset 等参数）。
func isFormContentType(ct string) bool {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.EqualFold(strings.TrimSpace(ct), "application/x-www-form-urlencoded")
}

// redact 递归脱敏。
//
// 必须递归：敏感字段不一定在顶层，{"data":{"password":"..."}} 这种结构
// 用「只遍历顶层 key」的写法会原样把口令写进审计日志。
func redact(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, sub := range val {
			if isSensitiveKey(k) {
				out[k] = "******"
				continue
			}
			out[k] = redact(sub)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, sub := range val {
			out[i] = redact(sub)
		}
		return out
	default:
		return v
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// resolveModule 从路径推导业务模块名，例如 /api/v1/system/users/3 -> users。
func resolveModule(c *gin.Context) string {
	path := strings.Trim(c.Request.URL.Path, "/")
	segs := strings.Split(path, "/")
	// 跳过 api / 版本段 / system 等固定前缀
	skip := map[string]bool{"api": true, "system": true, "v1": true, "v2": true}
	var last string
	for _, s := range segs {
		if s == "" || skip[s] {
			continue
		}
		if isNumeric(s) {
			continue
		}
		last = s
		break
	}
	if last == "" && len(segs) > 0 {
		last = segs[len(segs)-1]
	}
	return last
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
