package ops

import (
	"strings"
	"testing"
	"time"
)

// 每档的确认口令必须不同：口令相同的时候，手滑点到「确认」
// 就能从「清理单据」跳到「重置数据库」。
func TestBizCleanupConfirmPhrasesAreDistinct(t *testing.T) {
	scopes := BizCleanupScopes()
	if len(scopes) != 3 {
		t.Fatalf("应有 3 档清理范围，实际 %d", len(scopes))
	}
	seen := map[string]string{}
	for _, s := range scopes {
		if s.Confirm == "" {
			t.Fatalf("%s 缺少确认口令", s.Key)
		}
		if prev, ok := seen[s.Confirm]; ok {
			t.Fatalf("确认口令 %q 被 %s 与 %s 共用", s.Confirm, prev, s.Key)
		}
		seen[s.Confirm] = s.Key
		if s.Title == "" || s.Desc == "" || s.Keep == "" {
			t.Fatalf("%s 的展示字段不完整", s.Key)
		}
	}
}

// 框架表免疫是这套清理的安全底线，用测试钉住：
// 将来若有人往范围里加了 sys_users，这里立刻红。
func TestBizCleanupNeverTouchesFrameworkTables(t *testing.T) {
	for _, s := range BizCleanupScopes() {
		for _, tbl := range s.Tables {
			if frameworkTables[tbl] && !scopeLogTables[tbl] {
				t.Fatalf("范围 %s 包含受保护的表 %s", s.Key, tbl)
			}
			if isFrameworkTableForTest(tbl) {
				t.Fatalf("范围 %s 包含框架表 %s", s.Key, tbl)
			}
		}
	}
	// 日志表只允许出现在 database 档
	for _, s := range BizCleanupScopes() {
		for _, tbl := range s.Tables {
			if scopeLogTables[tbl] && s.Key != ScopeDatabase {
				t.Fatalf("日志表 %s 只允许出现在 database 档，却出现在 %s", tbl, s.Key)
			}
		}
	}
}

// 白名单必须**按档位**判定，不能只看「是不是日志表」：
// 之前的实现把「属于 scopeLogTables」当成豁免条件，于是任何档都能删日志表，
// 只是恰好没有档登记了它。这里钉住运行时判据本身。
func TestIsAllowedBizTableIsScopeAware(t *testing.T) {
	cases := []struct {
		table string
		scope string
		want  bool
	}{
		{"sys_operation_logs", ScopeDatabase, true},
		{"sys_login_logs", ScopeDatabase, true},
		{"sys_idempotencies", ScopeDatabase, true},
		{"sys_operation_logs", ScopeInvDocuments, false},
		{"sys_login_logs", ScopeInvAll, false},
		{"sys_idempotencies", ScopeInvDocuments, false},
		{"sys_users", ScopeDatabase, false},
		{"sys_upgrade_records", ScopeDatabase, false},
		{"inv_orders", ScopeInvDocuments, true},
		{"inv_products", ScopeInvAll, true},
	}
	for _, c := range cases {
		if got := isAllowedBizTable(c.table, c.scope); got != c.want {
			t.Fatalf("isAllowedBizTable(%s, %s) = %v，期望 %v", c.table, c.scope, got, c.want)
		}
	}
}

func isFrameworkTableForTest(t string) bool {
	switch t {
	case "sys_users", "sys_roles", "sys_menus", "sys_depts", "sys_user_roles", "sys_role_menus",
		"sys_schema_versions", "sys_upgrade_records":
		return true
	}
	return false
}

// 删除顺序必须是子表在前：库内现在没有外键，但顺序按依赖排，
// 将来补上外键时不必再改一遍代码（也不会出现「父表删不掉」的报错）。
func TestBizCleanupDeleteOrderIsChildFirst(t *testing.T) {
	for _, s := range BizCleanupScopes() {
		idx := map[string]int{}
		for i, tbl := range s.Tables {
			idx[tbl] = i
		}
		pairs := [][2]string{
			{"inv_order_items", "inv_orders"},
			{"inv_stock_items", "inv_stock_orders"},
			{"inv_product_specs", "inv_products"},
			{"inv_product_units", "inv_product_specs"},
		}
		for _, p := range pairs {
			ci, okc := idx[p[0]]
			pi, okp := idx[p[1]]
			if !okc || !okp {
				continue
			}
			if ci > pi {
				t.Fatalf("范围 %s：%s 必须排在 %s 之前", s.Key, p[0], p[1])
			}
		}
	}
}

// 未知范围必须报错，且错误里要带合法值 —— 现场照着报错就能改对，
// 不用回来翻文档。
func TestBizCleanupRejectsUnknownScope(t *testing.T) {
	_, err := FindBizCleanupScope("drop_everything")
	if err == nil {
		t.Fatal("未知范围应报错")
	}
	if !strings.Contains(err.Error(), "inv_documents") {
		t.Fatalf("错误信息应列出合法值：%v", err)
	}
}

// 「先有备份」这道闸门：没有备份、备份太老都要拦下。
func TestBizCleanupGate(t *testing.T) {
	now := parseTime(t, "2026-10-18T12:00:00+08:00")
	ok, msg := BizCleanupGate(parseTime(t, "2026-10-18T11:00:00+08:00"), now, 24*time.Hour)
	if !ok || msg == "" {
		t.Fatalf("1 小时前的备份应通过：%v %s", ok, msg)
	}
	if ok, _ := BizCleanupGate(parseTime(t, "2026-10-16T11:00:00+08:00"), now, 24*time.Hour); ok {
		t.Fatal("两天前的备份应被拦下")
	}
	if ok, msg := BizCleanupGate(timeZero(), now, 24*time.Hour); ok || msg == "" {
		t.Fatalf("没有备份必须拦下并给出提示：%v %s", ok, msg)
	}
}

func timeZero() time.Time { return time.Time{} }

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("解析时间失败：%v", err)
	}
	return v
}
