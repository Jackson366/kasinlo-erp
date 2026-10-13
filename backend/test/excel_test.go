package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/sheet"
)

// importHeaders 是导入模板的表头，必须与后端列定义一致。
var importHeaders = []string{"用户名", "昵称", "密码", "邮箱", "手机号", "部门", "角色", "状态", "备注"}

// buildSheet 用「写」组件反向生成 xlsx，作为导入测试的输入文件。
// 这样测试数据与真实模板同源，不会因为手工拼 XML 而与实现脱节。
func buildSheet(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()

	tbl := sheet.NewTable[[]string]("数据")
	for i, h := range headers {
		idx := i
		tbl.Col(h, 0, func(r []string) string { return sheet.Cell(r, idx) })
	}
	tbl.Rows(rows)

	data, err := tbl.Build()
	if err != nil {
		t.Fatalf("生成测试表格失败: %v", err)
	}
	return data
}

type importDetail struct {
	Row      int      `json:"row"`
	Username string   `json:"username"`
	Errors   []string `json:"errors"`
}

type importPayload struct {
	Total   int            `json:"total"`
	Valid   int            `json:"valid"`
	Invalid int            `json:"invalid"`
	Created int            `json:"created"`
	Rows    []importDetail `json:"rows"`
}

// parseImportResult 解析导入接口的响应体。
func parseImportResult(t *testing.T, body []byte) (importPayload, int) {
	t.Helper()
	var out struct {
		Code int           `json:"code"`
		Data importPayload `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("导入响应不是合法 JSON: %s", string(body))
	}
	return out.Data, out.Code
}

// describeErrors 把逐行校验结果拼成一段文本，便于断言。
func describeErrors(errs []string) string {
	if len(errs) == 0 {
		return "通过"
	}
	return strings.Join(errs, "；")
}

// assertXlsx 校验响应体确实是一个 xlsx（ZIP 容器）且响应头正确。
func assertXlsx(t *testing.T, rec *httptest.ResponseRecorder, label string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: 期望 HTTP 200，实际 %d（body=%s）", label, rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	if len(body) < 4 || body[0] != 'P' || body[1] != 'K' {
		t.Fatalf("%s: 内容不是有效的 xlsx（应以 ZIP 魔数 PK 开头），前 16 字节: %x", label, body[:min(16, len(body))])
	}
	if ct := rec.Header().Get("Content-Type"); !contains(ct, "spreadsheetml") {
		t.Fatalf("%s: Content-Type 不正确: %q", label, ct)
	}
	// 中文文件名必须通过 RFC 5987 的 filename* 传递
	disp := rec.Header().Get("Content-Disposition")
	if !contains(disp, "attachment") {
		t.Fatalf("%s: Content-Disposition 缺少 attachment: %q", label, disp)
	}
	if !contains(disp, "filename*=UTF-8''") {
		t.Fatalf("%s: Content-Disposition 缺少 UTF-8 文件名: %q", label, disp)
	}
}

func TestUserExcelExport(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	// ---- 导出用户 ----
	rec := rawCall(t, http.MethodGet, "/api/v1/system/users/export", token)
	assertXlsx(t, rec, "导出用户")

	// 导出的文件必须能被重新解析，且表头正确 —— 闭环校验
	rows, err := sheet.Parse(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("解析导出的用户表格失败: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("导出的表格至少应包含表头与一行数据，实际 %d 行", len(rows))
	}
	wantHeaders := []string{"ID", "用户名", "昵称", "邮箱", "手机号", "部门", "角色", "状态"}
	for i, h := range wantHeaders {
		if got := sheet.Cell(rows[0], i); got != h {
			t.Fatalf("第 %d 列表头期望 %q，实际 %q", i+1, h, got)
		}
	}
	// 内置管理员一定在导出结果里
	if !contains(fmt.Sprint(rows), adminAccount) {
		t.Fatalf("导出结果中未包含内置管理员 %s", adminAccount)
	}

	// ---- 按选中项导出 ----
	rec = rawCall(t, http.MethodGet, "/api/v1/system/users/export?ids=1", token)
	assertXlsx(t, rec, "按选中项导出")
	selected, err := sheet.Parse(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("解析选中项导出的表格失败: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("按 ids=1 导出应只有表头 + 1 行，实际 %d 行", len(selected))
	}

	// ---- 导出模板 ----
	rec = rawCall(t, http.MethodGet, "/api/v1/system/users/import-template", token)
	assertXlsx(t, rec, "下载导入模板")
	tpl, err := sheet.Parse(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("解析导入模板失败: %v", err)
	}
	for i, h := range importHeaders {
		if got := sheet.Cell(tpl[0], i); got != h {
			t.Fatalf("模板第 %d 列表头期望 %q，实际 %q", i+1, h, got)
		}
	}
	// 模板数据区必须为空，否则用户不删示例行就会把示例数据导进系统
	if len(tpl) != 1 {
		t.Fatalf("导入模板的数据区应为空，实际有 %d 行数据", len(tpl)-1)
	}

	// ---- 角色 / 日志导出 ----
	assertXlsx(t, rawCall(t, http.MethodGet, "/api/v1/system/roles/export", token), "导出角色")
	assertXlsx(t, rawCall(t, http.MethodGet, "/api/v1/system/logs/operations/export", token), "导出操作日志")
	assertXlsx(t, rawCall(t, http.MethodGet, "/api/v1/system/logs/logins/export", token), "导出登录日志")
}

func TestUserImport(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	// 用户名带时间戳后缀，避免与上一次测试运行留下的数据冲突
	suffix := time.Now().UnixNano()
	okUser := fmt.Sprintf("impok%d", suffix)
	dupUser := okUser // 与上面重复

	rows := [][]string{
		{okUser, "导入测试", "", "import@example.com", "13800138000", "技术部", "只读访客", "启用", "自动化测试"},
		{dupUser, "重复用户名", "", "", "", "", "", "", ""},
		{"ab", "用户名过短", "", "", "", "", "", "", ""},
		{fmt.Sprintf("impbad%d", suffix), "部门不存在", "", "", "", "不存在的部门", "", "", ""},
		{fmt.Sprintf("impst%d", suffix), "状态非法", "", "", "", "", "", "未知状态", ""},
		{"", "用户名为空", "", "", "", "", "", "", ""},
	}

	// ---- 1. 校验预览：只校验不写库 ----
	rec := uploadCall(t, "/api/v1/system/users/import?dryRun=1", token, "users.xlsx", buildSheet(t, importHeaders, rows))
	if rec.Code != http.StatusOK {
		t.Fatalf("导入预览失败: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
	preview, code := parseImportResult(t, rec.Body.Bytes())
	if code != 0 {
		t.Fatalf("导入预览返回业务码 %d", code)
	}
	if preview.Total != 6 || preview.Valid != 1 || preview.Invalid != 5 {
		t.Fatalf("预览统计不符合预期：total=%d valid=%d invalid=%d（期望 6/1/5）",
			preview.Total, preview.Valid, preview.Invalid)
	}
	if preview.Created != 0 {
		t.Fatalf("预览阶段不应写入数据，created=%d", preview.Created)
	}
	if len(preview.Rows) != 6 {
		t.Fatalf("预览应返回 6 行明细，实际 %d", len(preview.Rows))
	}

	// 失败原因必须逐行可见，行号对应 Excel 中的真实行号。
	// 注意用行号做索引而不是用户名 —— 重复用户名的用例两条记录同名，用 map[用户名] 会互相覆盖。
	byRow := map[int]importDetail{}
	for _, r := range preview.Rows {
		byRow[r.Row] = r
	}
	steps := []struct {
		row      int
		contains string
		desc     string
	}{
		{2, "通过", "合法行"},
		{3, "与文件中的其他行用户名重复", "文件内用户名重复"},
		{4, "用户名长度需为 3-64 个字符", "用户名过短"},
		{5, "部门「不存在的部门」不存在", "部门不存在"},
		{6, "状态只能填写", "状态非法"},
		{7, "用户名不能为空", "用户名为空"},
	}
	for _, s := range steps {
		detail, ok := byRow[s.row]
		if !ok {
			t.Fatalf("预览明细中缺少第 %d 行", s.row)
		}
		got := describeErrors(detail.Errors)
		if !contains(got, s.contains) {
			t.Fatalf("第 %d 行（%s）应提示 %q，实际: %q", s.row, s.desc, s.contains, got)
		}
	}

	// ---- 预览不应写库 ----
	list := call(t, http.MethodGet, "/api/v1/system/users?keyword="+okUser, token, nil)
	assertCode(t, list, 0, "预览后查询用户")
	if num(list.obj(t), "total") != 0 {
		t.Fatalf("预览阶段不应写入用户，但已存在 %d 条", num(list.obj(t), "total"))
	}

	// ---- 2. 确认导入：合法的 1 行写入 ----
	rec = uploadCall(t, "/api/v1/system/users/import", token, "users.xlsx", buildSheet(t, importHeaders, rows))
	if rec.Code != http.StatusOK {
		t.Fatalf("确认导入失败: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
	result, code := parseImportResult(t, rec.Body.Bytes())
	if code != 0 {
		t.Fatalf("确认导入返回业务码 %d", code)
	}
	if result.Created != 1 {
		t.Fatalf("应写入 1 条用户，实际 %d（valid=%d invalid=%d）", result.Created, result.Valid, result.Invalid)
	}

	// ---- 3. 落库校验：新用户确实存在，且部门/角色关联生效 ----
	list = call(t, http.MethodGet, "/api/v1/system/users?keyword="+okUser, token, nil)
	assertCode(t, list, 0, "导入后查询用户")
	if num(list.obj(t), "total") != 1 {
		t.Fatalf("导入后应能查到 1 条用户，实际 %d", num(list.obj(t), "total"))
	}

	// ---- 4. 重复导入同一文件：用户名已存在，应全部失败 ----
	rec = uploadCall(t, "/api/v1/system/users/import?dryRun=1", token, "users.xlsx", buildSheet(t, importHeaders, rows))
	again, _ := parseImportResult(t, rec.Body.Bytes())
	if again.Valid != 0 {
		t.Fatalf("重复导入时不应有校验通过的行，实际 valid=%d", again.Valid)
	}
	foundTaken := false
	for _, r := range again.Rows {
		if r.Username == okUser && contains(describeErrors(r.Errors), "已被系统中其他用户使用") {
			foundTaken = true
		}
	}
	if !foundTaken {
		t.Fatalf("重复导入未提示用户名已被占用")
	}
}

func TestUserImportRejectsBadFiles(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	cases := []struct {
		name     string
		filename string
		content  []byte
		wantMsg  string
	}{
		{
			name:     "非 xlsx 扩展名",
			filename: "users.csv",
			content:  []byte("a,b,c"),
			wantMsg:  "仅支持 .xlsx 格式",
		},
		{
			name:     "内容不是有效的 xlsx",
			filename: "users.xlsx",
			content:  []byte("这不是一个 xlsx 文件"),
			wantMsg:  "无法解析该文件",
		},
		{
			name:     "表头被改动",
			filename: "users.xlsx",
			content:  buildSheet(t, []string{"账号", "姓名", "密码", "邮箱", "手机号", "部门", "角色", "状态", "备注"}, [][]string{{"x", "y", "", "", "", "", "", "", ""}}),
			wantMsg:  "表头应为",
		},
		{
			name:     "只有表头没有数据",
			filename: "users.xlsx",
			content:  buildSheet(t, importHeaders, nil),
			wantMsg:  "没有可导入的数据行",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := uploadCall(t, "/api/v1/system/users/import?dryRun=1", token, tc.filename, tc.content)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("期望 HTTP 400，实际 %d（body=%s）", rec.Code, rec.Body.String())
			}
			_, code := parseImportResult(t, rec.Body.Bytes())
			if code != 40000 {
				t.Fatalf("期望业务码 40000，实际 %d（body=%s）", code, rec.Body.String())
			}
			var out struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			if !contains(out.Message, tc.wantMsg) {
				t.Fatalf("错误信息期望包含 %q，实际 %q", tc.wantMsg, out.Message)
			}
		})
	}
}

func TestExcelPermissionIsolation(t *testing.T) {
	demoToken := loginAs(t, demoAccount, demoSecret)

	// 只读账号不应具备导出/导入能力（导出属于批量取数，与「能看列表」不是一个风险等级）
	readonlyCases := []struct {
		method string
		path   string
		label  string
	}{
		{http.MethodGet, "/api/v1/system/users/export", "只读账号导出用户"},
		{http.MethodGet, "/api/v1/system/users/import-template", "只读账号下载模板"},
		{http.MethodGet, "/api/v1/system/roles/export", "只读账号导出角色"},
		{http.MethodGet, "/api/v1/system/logs/operations/export", "只读账号导出操作日志"},
		{http.MethodGet, "/api/v1/system/logs/logins/export", "只读账号导出登录日志"},
	}
	for _, tc := range readonlyCases {
		rec := rawCall(t, tc.method, tc.path, demoToken)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s：期望 HTTP 403，实际 %d（body=%s）", tc.label, rec.Code, rec.Body.String())
		}
	}

	// 只读账号也不允许导入
	rec := uploadCall(t, "/api/v1/system/users/import", demoToken, "users.xlsx",
		buildSheet(t, importHeaders, [][]string{{"permcheck", "权限校验", "", "", "", "", "", "", ""}}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("只读账号导入用户：期望 HTTP 403，实际 %d（body=%s）", rec.Code, rec.Body.String())
	}

	// 未登录应被鉴权中间件拦下
	anon := rawCall(t, http.MethodGet, "/api/v1/system/users/export", "")
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("未登录导出：期望 HTTP 401，实际 %d", anon.Code)
	}
}

// TestBuiltinPermissionsBackfilled 验证新增的权限点会被幂等地补进「已经初始化过的数据库」。
//
// 这是本次改动的关键风险点：种子数据原先是「表不为空就整段跳过」，
// 对已经跑过初始化的库，新增权限点不会生效。改为按 ID 补齐后，
// 必须确保菜单与内置角色的授权都能补上，且不会给只读角色越权。
func TestBuiltinPermissionsBackfilled(t *testing.T) {
	setup(t)
	db := database.DB()

	// 只校验「补齐了、类型对、不重复」——主键由库分配，会随种子顺序变化，
	// 断言具体 ID 等于把测试绑死在某一份种子上（dashboard 下线后全表 ID 都变过一轮）。
	newMenus := []string{
		"system:user:export", "system:user:import", "system:role:export", "system:log:export",
	}
	for _, perm := range newMenus {
		var found model.Menu
		if err := db.Where("permission = ?", perm).First(&found).Error; err != nil {
			t.Fatalf("权限点 %s 未补进菜单表: %v", perm, err)
		}
		if found.Type != model.MenuTypeButton {
			t.Fatalf("权限点 %s 应为按钮类型，实际 %s", perm, found.Type)
		}
	}

	// 内置菜单补齐后不应出现重复记录（幂等性）
	var totalMenu int64
	db.Model(&model.Menu{}).Where("permission IN ?", []string{
		"system:user:export", "system:user:import", "system:role:export", "system:log:export",
	}).Count(&totalMenu)
	if totalMenu != 4 {
		t.Fatalf("4 个新增权限点应恰好各 1 条，实际共 %d 条", totalMenu)
	}

	// 系统管理员应已获得这 4 个按钮的授权
	var adminGrants int64
	db.Model(&model.RoleMenu{}).
		Where("role_id = ? AND menu_id IN ?", 2, []int64{
			menuID(t, "system:user:export"), menuID(t, "system:user:import"),
			menuID(t, "system:role:export"), menuID(t, "system:log:export"),
		}).
		Count(&adminGrants)
	if adminGrants != 4 {
		t.Fatalf("系统管理员应获得 4 个新增按钮授权，实际 %d 个", adminGrants)
	}

	// 只读访客不得获得导出/导入授权
	var viewerGrants int64
	db.Model(&model.RoleMenu{}).
		Where("role_id = ? AND menu_id IN ?", 3, []int64{
			menuID(t, "system:user:export"), menuID(t, "system:user:import"),
			menuID(t, "system:role:export"), menuID(t, "system:log:export"),
		}).
		Count(&viewerGrants)
	if viewerGrants != 0 {
		t.Fatalf("只读访客不应获得导出/导入授权，实际获得 %d 个", viewerGrants)
	}

	// 超级管理员应拥有全部菜单授权
	var superGrants, menuCount int64
	db.Model(&model.RoleMenu{}).Where("role_id = ?", 1).Count(&superGrants)
	db.Model(&model.Menu{}).Count(&menuCount)
	if superGrants != menuCount {
		t.Fatalf("超级管理员应拥有全部 %d 个菜单授权，实际 %d 个", menuCount, superGrants)
	}
}
