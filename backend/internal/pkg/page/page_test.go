package page

import "testing"

func TestAtoiOr(t *testing.T) {
	if got := AtoiOr("", 5); got != 5 {
		t.Fatalf("空串应返回默认值，实际 %d", got)
	}
	if got := AtoiOr("abc", 5); got != 5 {
		t.Fatalf("非法值应返回默认值，实际 %d", got)
	}
	if got := AtoiOr("42", 5); got != 42 {
		t.Fatalf("合法值应正确转换，实际 %d", got)
	}
}

// Normalize 必须把客户端可控参数收敛到安全区间，尤其防住
// 「超大页码 → 巨型 OFFSET → 单请求拖垮数据库」的拒绝服务。
func TestNormalize(t *testing.T) {
	cases := []struct {
		name         string
		in           Query
		wantPage     int
		wantPageSize int
		wantOrder    string
	}{
		{"全空走默认", Query{}, 1, 10, "desc"},
		{"负数收敛", Query{Page: -3, PageSize: -1}, 1, 10, "desc"},
		{"超限条数封顶", Query{Page: 1, PageSize: 9999}, 1, 200, "desc"},
		{"asc 原样保留", Query{Order: "asc"}, 1, 10, "asc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.in
			q.Normalize()
			if q.Page != tc.wantPage || q.PageSize != tc.wantPageSize || q.Order != tc.wantOrder {
				t.Fatalf("got (page=%d, size=%d, order=%s), want (%d, %d, %s)",
					q.Page, q.PageSize, q.Order, tc.wantPage, tc.wantPageSize, tc.wantOrder)
			}
		})
	}

	// 巨型页码：9e18 * 200 会整型溢出，Normalize 必须先按 OFFSET 上界反推页码上界
	q := Query{Page: 9_000_000_000_000_000_000, PageSize: 200}
	q.Normalize()
	if q.Page != maxOffset/200+1 {
		t.Fatalf("巨型页码应收敛到 %d，实际 %d", maxOffset/200+1, q.Page)
	}
}

func TestOffset(t *testing.T) {
	if got := (Query{Page: 1, PageSize: 10}).Offset(); got != 0 {
		t.Fatalf("第 1 页 OFFSET 应为 0，实际 %d", got)
	}
	if got := (Query{Page: 3, PageSize: 10}).Offset(); got != 20 {
		t.Fatalf("第 3 页 OFFSET 应为 20，实际 %d", got)
	}
	// 未 Normalize 的野参数也必须被 Offset 兜底，不越 maxOffset
	if got := (Query{Page: 999_999_999, PageSize: 10}).Offset(); got != maxOffset {
		t.Fatalf("野 OFFSET 应封顶 %d，实际 %d", maxOffset, got)
	}
	if got := (Query{Page: -5, PageSize: 10}).Offset(); got != 0 {
		t.Fatalf("负 OFFSET 应归 0，实际 %d", got)
	}
}

// SafeOrderBy 是 SQL 注入防线：OrderBy 只能取白名单列，方向只能是 ASC/DESC。
func TestSafeOrderBy(t *testing.T) {
	allowed := map[string]string{"createdAt": "created_at", "latencyMs": "latency_ms"}

	if got := (Query{OrderBy: "createdAt", Order: "asc"}).SafeOrderBy(allowed, "created_at DESC"); got != "created_at ASC" {
		t.Fatalf("白名单命中应生成排序子句，实际 %q", got)
	}
	// 大小写不敏感
	if got := (Query{OrderBy: "latencyMs", Order: "ASC"}).SafeOrderBy(allowed, "x"); got != "latency_ms ASC" {
		t.Fatalf("大写 ASC 应识别，实际 %q", got)
	}
	// 白名单外：列名带注入载荷也必须落回 fallback
	for _, evil := range []string{"id; DROP TABLE users", "created_at--", "\"", ""} {
		if got := (Query{OrderBy: evil}).SafeOrderBy(allowed, "created_at DESC"); got != "created_at DESC" {
			t.Fatalf("白名单外 %q 应落回 fallback，实际 %q", evil, got)
		}
	}
	// 方向载荷也只接受 asc/ASC
	if got := (Query{OrderBy: "createdAt", Order: "asc; --"}).SafeOrderBy(allowed, "x"); got != "created_at DESC" {
		t.Fatalf("非法方向应落回 DESC，实际 %q", got)
	}
}
