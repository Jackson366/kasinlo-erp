package test

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/oplog"
)

// 本文件收录「曾经真实出现过、且只靠编译/常规测试抓不到」的缺陷回归用例。
// 每个用例都对应一次实际踩坑，注释里写清了触发条件与表现。

// randomHex 生成不可压缩的随机内容。
// 刻意不用重复字符：xlsx 是 zip，重复内容会被压得很小，
// 这样就造不出「超过 64KB 的测试文件」。
func randomHex(rnd *rand.Rand, n int) string {
	const alphabet = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rnd.Intn(len(alphabet))]
	}
	return string(b)
}

// TestImportAcceptsLargeFile 回归「请求体被截断」缺陷。
//
// 原实现里操作日志中间件只把读到的前 64KB 回填给 c.Request.Body，
// 超过 64KB 的 multipart 上传因此丢掉结尾的 boundary，c.FormFile 解析失败，
// 最终返回「请选择要上传的 Excel 文件」—— 提示指向的原因与真实原因完全不符，
// 用户只会反复重新选文件。1000 行接近上限的导入文件很容易触发。
func TestImportAcceptsLargeFile(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	const rowCount = 1000 // 与 maxImportRows 一致，同时覆盖「刚好不超上限」
	suffix := time.Now().UnixNano()
	rnd := rand.New(rand.NewSource(20260929))

	rows := make([][]string, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		rows = append(rows, []string{
			fmt.Sprintf("bigchk%d_%d", suffix, i),
			"大文件测试", "", "", "", "", "", "启用",
			randomHex(rnd, 200),
		})
	}
	content := buildSheet(t, importHeaders, rows)
	if len(content) <= maxBodyCaptureBytes {
		t.Fatalf("测试文件仅 %d 字节，未超过 %d 字节的截断阈值，无法覆盖该回归点",
			len(content), maxBodyCaptureBytes)
	}

	rec := uploadCall(t, "/api/v1/system/users/import?dryRun=1", token, "big.xlsx", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("超过 64KB 的上传被拒绝: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
	preview, code := parseImportResult(t, rec.Body.Bytes())
	if code != 0 {
		t.Fatalf("大文件校验返回业务码 %d: %s", code, rec.Body.String())
	}
	if preview.Total != rowCount || preview.Invalid != 0 {
		t.Fatalf("大文件校验结果异常: total=%d invalid=%d（期望 %d/0）",
			preview.Total, preview.Invalid, rowCount)
	}
}

// maxBodyCaptureBytes 与 middleware.maxCaptureBytes 保持一致。
// 中间件里的常量不导出，这里按契约值断言（改动阈值时该用例会同步提醒）。
const maxBodyCaptureBytes = 64 * 1024

// TestImportDedupesRepeatedRoles 回归「重复角色导致整单回滚」缺陷。
//
// sys_user_roles 的主键是 (user_id, role_id)。角色列里把同一个角色写两遍
// （「只读访客、只读访客」，或用不同的分隔符混写）会产生重复关联，
// 批量 INSERT 直接主键冲突 → 整批回滚，而错误信息只有「数据库操作失败」。
// 最恶劣的地方在于：预览阶段显示「全部通过」，用户完全无法预判。
func TestImportDedupesRepeatedRoles(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	name := fmt.Sprintf("impdup%d", time.Now().UnixNano())

	rows := [][]string{
		// 同一角色用「、」「；」「,」分别写一遍
		{name, "重复角色", "", "", "", "", "只读访客、只读访客；只读访客, 只读访客", "启用", "去重回归"},
	}
	content := buildSheet(t, importHeaders, rows)

	previewRec := uploadCall(t, "/api/v1/system/users/import?dryRun=1", token, "dup.xlsx", content)
	preview, code := parseImportResult(t, previewRec.Body.Bytes())
	if code != 0 {
		t.Fatalf("预览返回业务码 %d: %s", code, previewRec.Body.String())
	}
	if preview.Valid != 1 || preview.Invalid != 0 {
		t.Fatalf("重复角色名属于书写冗余，应校验通过：valid=%d invalid=%d rows=%v",
			preview.Valid, preview.Invalid, preview.Rows)
	}

	rec := uploadCall(t, "/api/v1/system/users/import", token, "dup.xlsx", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("重复角色名导致整单导入失败: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
	result, code := parseImportResult(t, rec.Body.Bytes())
	if code != 0 {
		t.Fatalf("导入返回业务码 %d: %s", code, rec.Body.String())
	}
	if result.Created != 1 {
		t.Fatalf("期望创建 1 个用户，实际 %d", result.Created)
	}

	db := database.DB()
	var u model.User
	if err := db.Where("username = ?", name).First(&u).Error; err != nil {
		t.Fatalf("导入的用户未落库: %v", err)
	}
	var links int64
	if err := db.Model(&model.UserRole{}).Where("user_id = ?", u.ID).Count(&links).Error; err != nil {
		t.Fatalf("统计角色关联失败: %v", err)
	}
	if links != 1 {
		t.Fatalf("重复角色应去重为 1 条关联，实际 %d 条", links)
	}

	// 收尾：这个用例会真正写库，必须自己清理，避免污染后续用例
	db.Where("user_id = ?", u.ID).Delete(&model.UserRole{})
	db.Delete(&model.User{}, u.ID)
}

// TestExportRejectsInvalidIDs 回归「非法 ids 静默降级为全表导出」缺陷。
//
// 原实现用 `if len(ids) > 0 { 按 ids 导出 } else { 按筛选导出 }`，
// 于是 ids=abc 这类一个都解析不出来的请求会掉进 else 分支 ——
// 用户请求「导出选中项」，实际拿到的是整张表。请求语义被悄悄放大，
// 数据量越大越危险，而且响应完全正常，没有任何迹象。
func TestExportRejectsInvalidIDs(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	rec := rawCall(t, http.MethodGet, "/api/v1/system/users/export?ids=abc,def", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("无效 ids 应被拒绝，实际 HTTP %d（响应体 %d 字节，说明可能返回了表格）",
			rec.Code, rec.Body.Len())
	}
	if !strings.Contains(rec.Body.String(), "选中项参数无效") {
		t.Fatalf("错误提示不符合预期: %s", rec.Body.String())
	}

	// 合法 ids 不受影响，仍能正常导出
	ok := rawCall(t, http.MethodGet, "/api/v1/system/users/export?ids=1", token)
	assertXlsx(t, ok, "按选中项导出")
}

// TestAuditLogCoversExportAndImport 回归「审计缺两处」缺陷。
//
// 两个独立问题被同一个用例覆盖，因为它们都表现为「日志里查不到」：
//
//  1. 导出接口走 GET，而中间件只对 POST/PUT/PATCH/DELETE 落库，
//     于是 handler 里 c.Set("opAction", "导出用户") 白设了 ——
//     数据外带风险最高的操作反而没有任何审计记录。
//  2. 导入上传的 multipart 请求体含 0x00 等非法 UTF-8 字节，
//     被当成文本写进 sys_operation_logs.request（text 列）后
//     PostgreSQL 直接拒绝（SQLSTATE 22021），整条日志写入失败 ——
//     最该审计的批量写操作一条都没留下。
func TestAuditLogCoversExportAndImport(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	db := database.DB()

	const (
		exportPath = "/api/v1/system/users/export"
		importPath = "/api/v1/system/users/import"
	)

	// 用「前后计数差值」断言，避免为清理历史数据而删日志
	countLogs := func(path string) int64 {
		var n int64
		if err := db.Model(&model.OperationLog{}).Where("path = ?", path).Count(&n).Error; err != nil {
			t.Fatalf("统计操作日志失败: %v", err)
		}
		return n
	}
	beforeExport := countLogs(exportPath)
	beforeImport := countLogs(importPath)

	rawCall(t, http.MethodGet, exportPath, token)

	name := fmt.Sprintf("audit%d", time.Now().UnixNano())
	uploadCall(t, "/api/v1/system/users/import?dryRun=1", token, "audit.xlsx",
		buildSheet(t, importHeaders, [][]string{
			{name, "审计测试", "", "", "", "", "", "启用", ""},
		}))

	if err := oplog.Flush(5 * time.Second); err != nil {
		t.Fatalf("刷新日志缓冲失败: %v", err)
	}

	t.Run("导出被记入操作日志", func(t *testing.T) {
		if countLogs(exportPath) <= beforeExport {
			t.Fatal("导出接口（GET）没有产生操作日志")
		}
		var row model.OperationLog
		if err := db.Where("path = ?", exportPath).Order("id DESC").First(&row).Error; err != nil {
			t.Fatalf("读取导出日志失败: %v", err)
		}
		if row.Action != "导出用户" {
			t.Fatalf("导出日志的动作名应为「导出用户」，实际 %q", row.Action)
		}
		if row.Method != http.MethodGet {
			t.Fatalf("导出日志的方法应为 GET，实际 %q", row.Method)
		}
	})

	t.Run("上传接口的日志能落库且不含二进制", func(t *testing.T) {
		if countLogs(importPath) <= beforeImport {
			t.Fatal("上传接口的操作日志没有落库（二进制请求体导致整条日志写入失败）")
		}
		var row model.OperationLog
		if err := db.Where("path = ?", importPath).Order("id DESC").First(&row).Error; err != nil {
			t.Fatalf("读取上传日志失败: %v", err)
		}
		if row.Action != "校验导入文件" {
			t.Fatalf("预校验的日志动作名应为「校验导入文件」，实际 %q", row.Action)
		}
		if strings.ContainsRune(row.Request, 0) {
			t.Fatalf("日志中的请求体含 NUL 字节: %q", row.Request)
		}
		if !strings.Contains(row.Request, "未记录") {
			t.Fatalf("二进制请求体应标记为未记录，实际 %q", row.Request)
		}
	})
}

// TestAuditLogSurvivesHostileInput 回归「客户端内容可以抑制审计记录」缺陷。
//
// 审计日志的写入不能受请求内容摆布，否则攻击者只要构造一个让 INSERT 失败的值，
// 就能让自己的操作不留痕迹。实测有两条这样的路径：
//
//  1. Path 列是 varchar(255) 且从未截断 —— 一个超长 URL 就能让整条日志写不进去。
//  2. 请求体按「字节」截断，切断多字节字符后产生非法 UTF-8，同样被数据库拒绝。
//
// 另外顺带覆盖嵌套脱敏：敏感字段挂在嵌套对象里时，只遍历顶层的旧写法
// 会原样把口令写进日志。
func TestAuditLogSurvivesHostileInput(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	db := database.DB()

	// 超长路径：会命中 PUT /system/users/:id 这条路由并走一遍操作日志中间件
	longPath := "/api/v1/system/users/" + strings.Repeat("a", 300)
	call(t, http.MethodPut, longPath, token, map[string]any{"nickname": "超长路径"})

	// 嵌套口令：请求体本身不合法（缺少必填项），但依然会被记入操作日志
	call(t, http.MethodPost, "/api/v1/system/depts", token, map[string]any{
		"nested": map[string]any{"password": "leakme12345"},
	})

	if err := oplog.Flush(5 * time.Second); err != nil {
		t.Fatalf("刷新日志缓冲失败: %v", err)
	}

	t.Run("超长路径的日志能落库", func(t *testing.T) {
		var n int64
		if err := db.Model(&model.OperationLog{}).
			Where("path LIKE ?", "/api/v1/system/users/aaa%").Count(&n).Error; err != nil {
			t.Fatalf("统计失败: %v", err)
		}
		if n == 0 {
			t.Fatal("超长路径的日志没有落库（Path 超过 varchar(255) 被数据库拒绝）")
		}
	})

	t.Run("嵌套口令不入日志", func(t *testing.T) {
		var row model.OperationLog
		if err := db.Where("request LIKE ?", "%nested%").Order("id DESC").First(&row).Error; err != nil {
			t.Fatalf("读取日志失败: %v", err)
		}
		if strings.Contains(row.Request, "leakme12345") {
			t.Fatalf("嵌套的口令被原样写入审计日志: %s", row.Request)
		}
		if !strings.Contains(row.Request, "******") {
			t.Fatalf("嵌套口令应被替换为掩码，实际: %s", row.Request)
		}
	})
}

// TestAuditLogTruncatesLongFields 直接覆盖写入边界的收敛逻辑。
//
// 用「大量多字节中文」把请求体推过 2000 字符的保留上限，验证两件事同时成立：
// 字符数收敛到上限以内，且结果仍是合法 UTF-8。
// 旧实现按字节截断（s[:2000]），切在中文中间就会造出非法 UTF-8，
// 结果是这条日志被数据库整条拒绝 —— 而且是客户端可以主动触发的。
func TestAuditLogTruncatesLongFields(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	db := database.DB()

	name := fmt.Sprintf("trunc%d", time.Now().UnixNano())
	body := map[string]any{
		// 键名以 "1" 开头，json.Marshal 按字典序排列后它位于最前，
		// 保证用这个标记一定能命中日志（截断会砍掉末尾的 username）
		"1tag":     name,
		"username": name,
		"password": "123456",
		"nickname": "截断测试",
		"remark":   strings.Repeat("中文备注", 40), // 160 个字符，满足 max=255
		// 未声明的字段会被 binding 忽略，但依然留在请求体里，用来把正文撑过上限
		"padding": strings.Repeat("中文填充", 600),
	}
	call(t, http.MethodPost, "/api/v1/system/users", token, body)

	if err := oplog.Flush(5 * time.Second); err != nil {
		t.Fatalf("刷新日志缓冲失败: %v", err)
	}

	var row model.OperationLog
	if err := db.Where("request LIKE ?", "%"+name+"%").Order("id DESC").First(&row).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if !utf8.ValidString(row.Request) {
		t.Fatal("日志中的请求体不是合法 UTF-8（被按字节切坏了）")
	}
	got := len([]rune(row.Request))
	if got != 2000 {
		t.Fatalf("请求体应被截断到 2000 个字符，实际 %d（未覆盖到截断分支？）", got)
	}
	if len([]rune(row.Path)) > 255 {
		t.Fatalf("Path 超出列长度上限: %d 个字符", len([]rune(row.Path)))
	}

	// 收尾：这个用例会真实建号
	var u model.User
	if err := db.Where("username = ?", name).First(&u).Error; err == nil {
		db.Where("user_id = ?", u.ID).Delete(&model.UserRole{})
		db.Delete(&model.User{}, u.ID)
	}
}
