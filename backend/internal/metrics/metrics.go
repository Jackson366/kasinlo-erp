// Package metrics 提供极简的 Prometheus 文本格式指标端点。
//
// 为什么手写而不是引入 prometheus/client_golang：本项目要暴露的指标只有个位数
// （请求量、延迟直方图、连接池读数、日志丢弃与限流拒绝计数），而 client_golang
// 会连带引入 protobuf 等一整套依赖。用固定文案的文本格式（version=0.0.4）手写，
// 依赖为零、成本更低，抓取端（Prometheus / Grafana Agent）完全兼容。
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/database"
	"nova-erp/internal/oplog"
)

// latencyBuckets 是请求耗时分桶上界（秒），渲染时自动补 +Inf 桶。
// 覆盖 5ms~10s：低于 5ms 与高于 10s 的请求分别落进最小桶与 +Inf 桶。
var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

const durationName = "nova_http_request_duration_seconds"

var (
	mu        sync.Mutex
	counters  = map[string]uint64{}     // 渲染串 -> 累计值（键即 `name{labels}` 全文）
	hists     = map[string]*histEntry{} // 同一个渲染串 -> 直方图
	startedAt = time.Now()
)

// histEntry 把直方图与它的标签集放在一起，渲染时要额外插入 le 标签。
type histEntry struct {
	labels map[string]string
	h      *histogram
}

// histogram 是固定分桶的延迟直方图。
//
// 桶计数在写入时就写成累计值（每次观测把「所有 le >= 观测值」的桶 +1），
// 这样渲染时无需再累加一遍，也避免了「忘了累计」这类最容易出错的细节。
type histogram struct {
	mu      sync.Mutex
	buckets []uint64
	sum     float64
	count   uint64
}

// IncCounter 给带标签的计数器 +1。labels 为 nil 表示无标签。
func IncCounter(name string, labels map[string]string) {
	key := renderKey(name, labels, "")
	mu.Lock()
	counters[key]++
	mu.Unlock()
}

// Observe 记录一次请求耗时（秒）。route 用路由模板而非真实路径，避免标签基数爆炸。
func Observe(route string, seconds float64) {
	labels := map[string]string{"route": route}
	key := renderKey(durationName, labels, "")
	mu.Lock()
	e := hists[key]
	if e == nil {
		e = &histEntry{labels: labels, h: &histogram{buckets: make([]uint64, len(latencyBuckets))}}
		hists[key] = e
	}
	mu.Unlock()

	e.h.mu.Lock()
	for i, le := range latencyBuckets {
		if seconds <= le {
			e.h.buckets[i]++
		}
	}
	e.h.sum += seconds
	e.h.count++
	e.h.mu.Unlock()
}

// Middleware 采集每个请求的方法 / 路由 / 状态码计数与耗时。
// 只在 /metrics 启用时装配，关闭态不产生任何额外开销。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		// 路由模板必须在 c.Next() 之后取：请求进入本中间件时还没匹配到 handler，
		// 此时 FullPath() 为空。未匹配到路由的请求（404）统一归入 unmatched。
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		IncCounter("nova_http_requests_total", map[string]string{
			"method": c.Request.Method,
			"route":  route,
			"status": strconv.Itoa(c.Writer.Status()),
		})
		Observe(route, time.Since(start).Seconds())
	}
}

// Handler 渲染 /metrics。
func Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var b strings.Builder
		writeCounters(&b)
		writeHistograms(&b)
		writeRuntime(&b)
		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()))
	}
}

func writeCounters(b *strings.Builder) {
	mu.Lock()
	keys := make([]string, 0, len(counters))
	for k := range counters {
		keys = append(keys, k)
	}
	snapshot := make(map[string]uint64, len(counters))
	for k, v := range counters {
		snapshot[k] = v
	}
	mu.Unlock()
	sort.Strings(keys)

	last := ""
	for _, k := range keys {
		family := familyOf(k)
		if family != last {
			writeHeader(b, family, "counter", helpOf(family))
			last = family
		}
		fmt.Fprintf(b, "%s %d\n", k, snapshot[k])
	}
}

func writeHistograms(b *strings.Builder) {
	mu.Lock()
	keys := make([]string, 0, len(hists))
	entries := make([]*histEntry, 0, len(hists))
	for k, e := range hists {
		keys = append(keys, k)
		entries = append(entries, e)
	}
	mu.Unlock()
	sort.Strings(keys)

	writeHeader(b, durationName, "histogram", "HTTP 请求耗时分布（秒）")
	for _, e := range entries {
		e.h.mu.Lock()
		buckets, sum, count := append([]uint64(nil), e.h.buckets...), e.h.sum, e.h.count
		e.h.mu.Unlock()

		labelText := labelsText(e.labels)
		for i, le := range latencyBuckets {
			fmt.Fprintf(b, "%s_bucket%s %d\n", durationName, withLe(labelText, strconv.FormatFloat(le, 'g', -1, 64)), buckets[i])
		}
		fmt.Fprintf(b, "%s_bucket%s %d\n", durationName, withLe(labelText, "+Inf"), count)
		fmt.Fprintf(b, "%s_sum%s %s\n", durationName, labelText, strconv.FormatFloat(sum, 'g', -1, 64))
		fmt.Fprintf(b, "%s_count%s %d\n", durationName, labelText, count)
	}
}

func writeRuntime(b *strings.Builder) {
	writeHeader(b, "nova_uptime_seconds", "gauge", "服务运行时长（秒）")
	fmt.Fprintf(b, "nova_uptime_seconds %d\n", int64(time.Since(startedAt).Seconds()))

	writeHeader(b, "nova_oplog_dropped_total", "counter", "因缓冲满被丢弃的操作日志条数")
	fmt.Fprintf(b, "nova_oplog_dropped_total %d\n", oplog.Dropped())

	writeHeader(b, "nova_ratelimit_rejected_total", "counter", "被每 IP 请求限流拒绝的请求数")
	fmt.Fprintf(b, "nova_ratelimit_rejected_total %d\n", counterValue("nova_ratelimit_rejected_total"))

	// 连接池读数：in_use 长期贴着 max_open_conns 说明池子被打满，
	// 这时请求会排在连接上而不是排在 CPU 上。
	if stats, err := database.Stats(); err == nil {
		writeHeader(b, "nova_db_pool_connections", "gauge", "数据库连接池连接数")
		fmt.Fprintf(b, "nova_db_pool_connections{state=\"open\"} %d\n", stats.OpenConnections)
		fmt.Fprintf(b, "nova_db_pool_connections{state=\"in_use\"} %d\n", stats.InUse)
		fmt.Fprintf(b, "nova_db_pool_connections{state=\"idle\"} %d\n", stats.Idle)
	}
}

// counterValue 读取「无标签计数器」的当前值。
func counterValue(name string) uint64 {
	mu.Lock()
	defer mu.Unlock()
	return counters[name]
}

// renderKey 拼出 `name{k="v",...}` 形式的稳定键。额外标签 extra 用于 le。
func renderKey(name string, labels map[string]string, le string) string {
	all := labels
	if le != "" {
		all = make(map[string]string, len(labels)+1)
		for k, v := range labels {
			all[k] = v
		}
		all["le"] = le
	}
	if len(all) == 0 {
		return name
	}
	ks := make([]string, 0, len(all))
	for k := range all {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, k := range ks {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `%s="%s"`, k, escapeLabel(all[k]))
	}
	b.WriteByte('}')
	return b.String()
}

func labelsText(labels map[string]string) string {
	return renderKey("", labels, "")
}

func withLe(labelText, le string) string {
	if labelText == "{}" {
		return `{le="` + le + `"}`
	}
	return labelText[:len(labelText)-1] + `,le="` + le + `"}`
}

func familyOf(key string) string {
	if i := strings.IndexByte(key, '{'); i >= 0 {
		return key[:i]
	}
	return key
}

func writeHeader(b *strings.Builder, family, typ, help string) {
	if help != "" {
		fmt.Fprintf(b, "# HELP %s %s\n", family, help)
	}
	fmt.Fprintf(b, "# TYPE %s %s\n", family, typ)
}

// helpOf 为已知计数器族提供描述；未登记的族不加 HELP（避免输出误导性文案）。
func helpOf(family string) string {
	switch family {
	case "nova_http_requests_total":
		return "HTTP 请求总数"
	default:
		return ""
	}
}

// escapeLabel 按文本格式要求转义标签值中的反斜杠、引号与换行。
func escapeLabel(v string) string {
	if !strings.ContainsAny(v, "\\\"\n") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}
