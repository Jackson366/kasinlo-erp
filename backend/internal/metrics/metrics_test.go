package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMiddlewareRecordsRequest 验证一次请求会被计入「请求总数」与「耗时分位」，
// 且 /metrics 输出是可被抓取端解析的文本格式。
func TestMiddlewareRecordsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Middleware())
	engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	engine.GET("/metrics", Handler())

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	want := []string{
		`nova_http_requests_total{method="GET",route="/ping",status="200"} 1`,
		`nova_http_request_duration_seconds_count{route="/ping"} 1`,
		`nova_http_request_duration_seconds_bucket{route="/ping",le="+Inf"} 1`,
		"nova_uptime_seconds ",
	}
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Fatalf("指标输出缺少 %q\n完整输出:\n%s", w, body)
		}
	}
}

// TestUnmatchedRouteIsBucketed 验证未匹配到路由的请求不会把真实路径写进标签。
//
// 标签用真实路径会让基数随扫描流量无限膨胀（每个不存在的 URL 都是新序列），
// 是 Prometheus 使用中最典型的自伤方式。
func TestUnmatchedRouteIsBucketed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Middleware())
	engine.GET("/metrics", Handler())

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/definitely-not-here", nil))

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	if strings.Contains(body, "definitely-not-here") {
		t.Fatalf("真实路径泄漏进指标标签:\n%s", body)
	}
	if !strings.Contains(body, `route="unmatched"`) {
		t.Fatalf("未匹配请求未计入 unmatched:\n%s", body)
	}
}

// TestLabelsText 验证带标签渲染的拼接边界（渲染时要在末尾插入 le）。
func TestLabelsText(t *testing.T) {
	if got := labelsText(map[string]string{"route": "/api/v1/x"}); got != `{route="/api/v1/x"}` {
		t.Fatalf("labelsText 结果异常: %s", got)
	}
	if got := withLe(`{route="/a"}`, "0.5"); got != `{route="/a",le="0.5"}` {
		t.Fatalf("withLe 结果异常: %s", got)
	}
	if got := withLe("{}", "+Inf"); got != `{le="+Inf"}` {
		t.Fatalf("空标签 withLe 结果异常: %s", got)
	}
}
