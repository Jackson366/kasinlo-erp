package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTestEngine(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	for _, h := range handlers {
		r.Use(h)
	}
	r.GET("/probe", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

func probe(engine *gin.Engine, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// TestRateLimitThrottlesAfterQuota 超过配额的请求必须被 429 挡下。
func TestRateLimitThrottlesAfterQuota(t *testing.T) {
	engine := newTestEngine(RateLimit(2, time.Minute))

	if got := probe(engine, nil).Code; got != http.StatusOK {
		t.Fatalf("第 1 次请求应放行，实际 HTTP %d", got)
	}
	if got := probe(engine, nil).Code; got != http.StatusOK {
		t.Fatalf("第 2 次请求应放行，实际 HTTP %d", got)
	}
	if got := probe(engine, nil).Code; got != http.StatusTooManyRequests {
		t.Fatalf("超出配额应返回 429，实际 HTTP %d", got)
	}
}

// TestRateLimitDisabled 配额为 0 时相当于关闭，不应有任何拦截。
func TestRateLimitDisabled(t *testing.T) {
	engine := newTestEngine(RateLimit(0, time.Minute))
	for i := 0; i < 20; i++ {
		if got := probe(engine, nil).Code; got != http.StatusOK {
			t.Fatalf("关闭限流时第 %d 次请求被拦截，实际 HTTP %d", i+1, got)
		}
	}
}

// TestTraceIDSanitized 客户端传入的 X-Trace-Id 必须做字符集收敛后再透传。
//
// 只限长度不够：该值会原样进入响应头与日志，含 CRLF / 空格 / 控制字符时
// 可用于响应头拆分与日志注入。
func TestTraceIDSanitized(t *testing.T) {
	engine := newTestEngine(TraceIDMiddleware())

	cases := []struct {
		name string
		in   string
		keep bool
	}{
		{"合法字符原样透传", "abc-123_XYZ", true},
		{"含 CRLF 应重新生成", "abc\r\nX-Evil: 1", false},
		{"含空格应重新生成", "abc def", false},
		{"超长应重新生成", strings.Repeat("a", 65), false},
		{"空值应生成新 ID", "", false},
	}
	for _, tc := range cases {
		rec := probe(engine, map[string]string{"X-Trace-Id": tc.in})
		got := rec.Header().Get("X-Trace-Id")
		if got == "" {
			t.Fatalf("%s: 响应应始终带 X-Trace-Id", tc.name)
		}
		if tc.keep && got != tc.in {
			t.Fatalf("%s: 期望原样透传 %q，实际 %q", tc.name, tc.in, got)
		}
		if !tc.keep && got == tc.in && tc.in != "" {
			t.Fatalf("%s: 非法值不应被原样透传", tc.name)
		}
	}
}
