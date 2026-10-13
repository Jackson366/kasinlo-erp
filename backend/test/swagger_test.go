package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nova-erp/internal/config"
	"nova-erp/internal/router"
)

// TestSwaggerDocServedAndGated 校验在线接口文档「能生成、能访问、且默认不对外暴露」。
//
// 这条用例直接把 swaggo 生成的 docs 包接进真实路由里跑，避免出现
// 「swag init 生成了文件，但路由没挂 / 挂了但路径对不上」这类只在联调时才发现的问题。
func TestSwaggerDocServedAndGated(t *testing.T) {
	setup(t)

	cfg := config.Get()
	orig := cfg.App.Swagger
	defer func() { cfg.App.Swagger = orig }()

	// ---- 1) 显式开启：文档可访问，且 basePath 与运行时配置一致 ----
	enabled := true
	cfg.App.Swagger = &enabled
	engine := router.New(cfg)

	rec := serve(engine, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("开启后 /swagger/doc.json 期望 HTTP 200，实际 %d", rec.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("文档不是合法 JSON: %v", err)
	}
	if got := doc["basePath"]; got != cfg.App.BasePath() {
		t.Fatalf("basePath 必须与运行配置一致：期望 %s，实际 %v", cfg.App.BasePath(), got)
	}
	paths, _ := doc["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("生成的文档里没有任何 path")
	}
	// 抽查关键接口是否在文档中（漏了说明注解没被解析到）
	for _, p := range []string{"/auth/login", "/system/users", "/system/roles/{id}/menus", "/profile/routes"} {
		if _, ok := paths[p]; !ok {
			t.Fatalf("文档缺少接口 %s", p)
		}
	}
	if _, ok := doc["securityDefinitions"].(map[string]any)["BearerAuth"]; !ok {
		t.Fatal("文档缺少 BearerAuth 安全定义")
	}

	// ---- 2) 未开启（release 默认）：不应暴露文档 ----
	cfg.App.Swagger = nil
	off := router.New(cfg)
	if cfg.App.SwaggerEnabled() {
		t.Fatal("前置条件不成立：release 模式默认不应开启 swagger")
	}
	rec2 := serve(off, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))
	if rec2.Code == http.StatusOK {
		t.Fatal("未开启 swagger 时 /swagger/doc.json 不应可访问")
	}
}
