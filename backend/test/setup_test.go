// Package test 提供 Nova Admin 的进程内端到端测试。
// 通过 httptest 直接驱动 gin 引擎，不依赖网络端口，可在任何环境稳定运行。
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nova-admin/internal/assembly"
	"nova-admin/internal/bootstrap"
	"nova-admin/internal/model"
	"nova-admin/internal/oplog"
	"nova-admin/internal/router"
)

const (
	testConfig = "testdata/config.yaml"
	// 测试账号口令（种子数据内置）
	adminAccount = "admin"
	adminSecret  = "admin123456"
	demoAccount  = "demo"
	demoSecret   = "demo123456"
)

var (
	setupOnce sync.Once
	engine    *gin.Engine
	setupErr  error
	// testDB 是测试库句柄，供需要直接操作数据库的用例使用
	// （例如验证聚合查询出错时接口是否会报错）。
	testDB *gorm.DB
)

// setup 初始化测试环境：连接测试库 -> 迁移 -> 种子 -> 构建引擎。
func setup(t *testing.T) *gin.Engine {
	t.Helper()
	setupOnce.Do(func() {
		res, err := bootstrap.Init(testConfig, bootstrap.Options{
			SeedData: true,
			// 与 main 共用同一个组装入口：测试少装一个模块的话，
			// 问题只会在某条用例莫名其妙 404 时才暴露，代价太高。
			Modules: assembly.Modules,
		})
		if err != nil {
			setupErr = fmt.Errorf("测试环境初始化失败: %w", err)
			return
		}

		// 清理历史失败登录记录。
		// 账号锁定是按「时间窗内的失败次数」判定的，而 TestAuth 会刻意用错误口令登录，
		// 反复执行测试就会累积到阈值把账号锁死（表现为后续所有测试返回 40103 无法登录，
		// 且与代码改动毫无关系，排查成本很高）。这在测试库上重置，保证测试可重复执行。
		if err := res.DB.Where("success = ?", false).Delete(&model.LoginLog{}).Error; err != nil {
			setupErr = fmt.Errorf("清理历史失败登录记录失败: %w", err)
			return
		}

		oplog.Init(res.DB)
		testDB = res.DB
		engine = router.New(res.Config)
	})
	if setupErr != nil {
		t.Fatalf("测试环境初始化失败: %v", setupErr)
	}
	return engine
}

// menuID 按权限码返回内置菜单的主键。
//
// 用例里不再硬编码菜单主键：主键现在由数据库自增分配，「哪个节点拿到哪个 ID」
// 取决于模块清单的声明顺序。把它写死等于把声明顺序变成一条隐式契约 ——
// 调整一下清单就会让全套用例集体飘红，真正的回归信号反而被淹没。
func menuID(t *testing.T, permission string) int64 {
	t.Helper()
	var m model.Menu
	if err := testDB.Where("permission = ?", permission).Take(&m).Error; err != nil {
		t.Fatalf("按权限码 %q 查菜单失败: %v", permission, err)
	}
	return m.ID
}

// menuIDByName 按路由名返回菜单主键。目录节点没有权限码，只能按名字定位。
func menuIDByName(t *testing.T, name string) int64 {
	t.Helper()
	var m model.Menu
	if err := testDB.Where("name = ?", name).Take(&m).Error; err != nil {
		t.Fatalf("按路由名 %q 查菜单失败: %v", name, err)
	}
	return m.ID
}

// response 是统一响应体的解析结果。
type response struct {
	status int
	Code   int            `json:"code"`
	Msg    string         `json:"message"`
	Data   map[string]any `json:"data"`
	List   []any          `json:"-"`
	Raw    map[string]any `json:"-"`
}

// call 发起一次进程内请求。
func call(t *testing.T, method, path, token string, body any) *response {
	t.Helper()
	eng := setup(t)

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	res := &response{status: rec.Code}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("%s %s 响应不是合法 JSON: %s", method, path, rec.Body.String())
	}
	res.Raw = payload
	if v, ok := payload["code"].(float64); ok {
		res.Code = int(v)
	}
	if v, ok := payload["message"].(string); ok {
		res.Msg = v
	}
	if d, ok := payload["data"].(map[string]any); ok {
		res.Data = d
		if list, ok := d["list"].([]any); ok {
			res.List = list
		}
		return res
	}
	if list, ok := payload["data"].([]any); ok {
		res.List = list
	}
	return res
}

// dataMap 取 data 字段为对象。
func (r *response) obj(t *testing.T) map[string]any {
	t.Helper()
	if r.Data == nil {
		t.Fatalf("期望 data 为对象，实际: %v", r.Raw)
	}
	return r.Data
}

// str 从对象中安全取字符串。
func str(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// num 从对象中安全取整数。
func num(m map[string]any, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

// loginAs 登录并返回访问令牌。
func loginAs(t *testing.T, account, secret string) string {
	t.Helper()
	res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": account,
		"password": secret,
	})
	if res.Code != 0 {
		t.Fatalf("账号 %s 登录失败: code=%d msg=%s", account, res.Code, res.Msg)
	}
	token := str(res.obj(t), "accessToken")
	if token == "" {
		t.Fatalf("登录未返回 accessToken")
	}
	return token
}

func assertCode(t *testing.T, res *response, want int, label string) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("%s: 期望业务码 %d，实际 %d（响应: %v）", label, want, res.Code, res.Raw)
	}
}

func assertStatus(t *testing.T, res *response, want int, label string) {
	t.Helper()
	if res.status != want {
		t.Fatalf("%s: 期望 HTTP %d，实际 %d（响应: %v）", label, want, res.status, res.Raw)
	}
}

// newRequest 构造进程内请求。
func newRequest(method, path, token string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// serve 直接驱动引擎处理请求，不经过网络。
func serve(eng *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)
	return rec
}

// rawCall 发起一次请求并返回原始响应。
// 与 call 的区别：不做 JSON 解析，用于导出、模板下载等二进制响应。
func rawCall(t *testing.T, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return serve(setup(t), req)
}

// uploadCall 以 multipart/form-data 上传一个文件，返回原始响应。
func uploadCall(t *testing.T, path, token, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造上传表单失败: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("写入上传内容失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭上传表单失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return serve(setup(t), req)
}

// contains 判断字符串是否包含子串。
func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// mustJSON 把任意值序列化为 JSON 字符串，仅用于断言。
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// itoa 整数转字符串。
func itoa(n int64) string { return strconv.FormatInt(n, 10) }
