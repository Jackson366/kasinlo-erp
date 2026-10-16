package test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nova-admin/internal/database"
	"nova-admin/internal/middleware"
	"nova-admin/internal/model"
)

const (
	userImportPath = "/api/v1/system/users/import"
	// userImportScope 是幂等记录的 scope，格式为「方法 + 路由模板」。
	userImportScope = "POST " + userImportPath
)

// uploadWithKey 与 uploadCall 相同，但额外携带 Idempotency-Key 头。
func uploadWithKey(t *testing.T, path, token, key, filename string, content []byte) *httptest.ResponseRecorder {
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
	if key != "" {
		req.Header.Set(middleware.IdempotencyKeyHeader, key)
	}
	return serve(setup(t), req)
}

// importKeyFor 生成一个不会与其它用例冲突的幂等键。
func importKeyFor(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// TestIdempotentImportReplay 验证「同一个键重复提交同一个导入」只写一次库，第二次回放首次响应。
func TestIdempotentImportReplay(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	key := importKeyFor("test-replay")
	user := fmt.Sprintf("idem%d", time.Now().UnixNano())
	file := buildSheet(t, importHeaders, [][]string{
		{user, "幂等测试", "", "", "", "", "", "", ""},
	})

	first := uploadWithKey(t, userImportPath, token, key, "users.xlsx", file)
	if first.Code != http.StatusOK {
		t.Fatalf("首次导入失败: HTTP %d body=%s", first.Code, first.Body.String())
	}
	created, code := parseImportResult(t, first.Body.Bytes())
	if code != 0 || created.Created != 1 {
		t.Fatalf("首次导入应创建 1 条用户，实际 code=%d created=%d", code, created.Created)
	}

	// 同一个键 + 同一个文件再发一次：应当回放，而不是再执行一遍导入
	second := uploadWithKey(t, userImportPath, token, key, "users.xlsx", file)
	if second.Code != http.StatusOK {
		t.Fatalf("重复提交应回放 200，实际 HTTP %d body=%s", second.Code, second.Body.String())
	}
	if got := second.Header().Get("Idempotent-Replay"); got != "true" {
		t.Fatalf("重复提交应被标记为回放，Idempotent-Replay=%q", got)
	}
	// 回放的必须是首次响应的原样内容（含 traceId），这样才能保证调用方看到的结果完全一致
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatalf("回放内容应与首次响应一致\n首次: %s\n回放: %s", first.Body.String(), second.Body.String())
	}

	// 最关键的一条：库中该用户只能有一条，说明第二次确实没有写库
	list := call(t, http.MethodGet, "/api/v1/system/users?keyword="+user, token, nil)
	assertCode(t, list, 0, "幂等回放后查询用户")
	if got := num(list.obj(t), "total"); got != 1 {
		t.Fatalf("幂等回放不应重复建档，期望 1 条，实际 %d 条", got)
	}
}

// TestIdempotentImportRejectsConcurrent 验证「同一键仍在处理中」时返回 409。
func TestIdempotentImportRejectsConcurrent(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	setup(t)
	key := importKeyFor("test-busy")

	// 直接造一条还在租期内的「处理中」记录，模拟同一次操作的另一个请求尚未结束
	row := model.Idempotency{
		Key:       key,
		Scope:     userImportScope,
		Status:    model.IdempotencyProcessing,
		ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := database.DB().Create(&row).Error; err != nil {
		t.Fatalf("写入处理中的幂等记录失败: %v", err)
	}

	rec := uploadWithKey(t, userImportPath, token, key, "users.xlsx",
		buildSheet(t, importHeaders, [][]string{{"busyuser", "占用中", "", "", "", "", "", "", ""}}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("同一键正在处理中应返回 409，实际 HTTP %d（body=%s）", rec.Code, rec.Body.String())
	}
}

// TestIdempotentImportReleasesOnFailure 验证业务失败后键被释放，同一个键可以修好后重试。
func TestIdempotentImportReleasesOnFailure(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	key := importKeyFor("test-retry")
	user := fmt.Sprintf("idemretry%d", time.Now().UnixNano())

	// 第一次故意用一份表头不符的文件，请求会以 400 结束
	bad := buildSheet(t, []string{"账号", "姓名"}, [][]string{{"x", "y"}})
	first := uploadWithKey(t, userImportPath, token, key, "users.xlsx", bad)
	if first.Code != http.StatusBadRequest {
		t.Fatalf("表头不符应返回 400，实际 HTTP %d（body=%s）", first.Code, first.Body.String())
	}

	// 失败已经释放了键：同一个键换一份合法文件应当正常执行，而不是 409 或回放
	good := buildSheet(t, importHeaders, [][]string{{user, "失败后重试", "", "", "", "", "", "", ""}})
	second := uploadWithKey(t, userImportPath, token, key, "users.xlsx", good)
	if second.Code != http.StatusOK {
		t.Fatalf("失败后以同一键重试应成功，实际 HTTP %d（body=%s）", second.Code, second.Body.String())
	}
	if got := second.Header().Get("Idempotent-Replay"); got != "" {
		t.Fatalf("失败后重试不是回放，不应带 Idempotent-Replay=%q", got)
	}
	result, code := parseImportResult(t, second.Body.Bytes())
	if code != 0 || result.Created != 1 {
		t.Fatalf("重试应创建 1 条用户，实际 code=%d created=%d", code, result.Created)
	}
}

// TestIdempotentImportReclaimsExpired 验证过期的「处理中」记录会被后来的请求抢占，
// 否则进程崩溃留下的记录会把键永久占死。
func TestIdempotentImportReclaimsExpired(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	setup(t)
	key := importKeyFor("test-expired")

	// 模拟一条已经超过租期的「处理中」记录
	stale := model.Idempotency{
		Key:       key,
		Scope:     userImportScope,
		Status:    model.IdempotencyProcessing,
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := database.DB().Create(&stale).Error; err != nil {
		t.Fatalf("写入过期幂等记录失败: %v", err)
	}

	user := fmt.Sprintf("idemexp%d", time.Now().UnixNano())
	rec := uploadWithKey(t, userImportPath, token, key, "users.xlsx",
		buildSheet(t, importHeaders, [][]string{{user, "抢占过期键", "", "", "", "", "", "", ""}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("过期的幂等键应可被抢占，实际 HTTP %d（body=%s）", rec.Code, rec.Body.String())
	}
	result, code := parseImportResult(t, rec.Body.Bytes())
	if code != 0 || result.Created != 1 {
		t.Fatalf("抢占后应正常导入 1 条，实际 code=%d created=%d", code, result.Created)
	}
}

// TestIdempotentKeyScopedToEndpoint 验证同一个键被挪到别的接口时会被拒绝，而不是回放别的接口的响应。
func TestIdempotentKeyScopedToEndpoint(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	setup(t)
	key := importKeyFor("test-scope")

	// 造一条 scope 不属于导入接口的「已完成」记录
	other := model.Idempotency{
		Key:        key,
		Scope:      "POST /api/v1/system/roles/import",
		Status:     model.IdempotencyCompleted,
		StatusCode: http.StatusOK,
		Response:   []byte(`{"code":0}`),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	if err := database.DB().Create(&other).Error; err != nil {
		t.Fatalf("写入跨接口幂等记录失败: %v", err)
	}

	rec := uploadWithKey(t, userImportPath, token, key, "users.xlsx",
		buildSheet(t, importHeaders, [][]string{{"scopeuser", "跨接口", "", "", "", "", "", "", ""}}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("同一键跨接口使用应返回 409，实际 HTTP %d（body=%s）", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Idempotent-Replay"); got == "true" {
		t.Fatalf("跨接口使用不应回放其它接口的响应")
	}
}
