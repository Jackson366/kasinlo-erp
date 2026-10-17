package sheet

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"nova-erp/internal/pkg/errcode"
)

// buildTable 用与业务模块相同的方式构造一张表，保证测试与真实导出路径同源。
func buildTable(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()

	tbl := NewTable[[]string]("用户数据")
	for i, h := range headers {
		idx := i
		tbl.Col(h, 0, func(r []string) string { return Cell(r, idx) })
	}
	tbl.Rows(rows)

	data, err := tbl.Build()
	if err != nil {
		t.Fatalf("生成表格失败: %v", err)
	}
	return data
}

// TestBuildParseRoundTrip 覆盖导出/导入的闭环：写出去的内容必须能原样读回来。
func TestBuildParseRoundTrip(t *testing.T) {
	headers := []string{"用户名", "手机号", "备注"}
	rows := [][]string{
		{"alice", "13800138000", "普通用户"},
		{"长名字用户", "13900139000", "含中文"},
	}
	data := buildTable(t, headers, rows)

	if len(data) < 4 || data[0] != 'P' || data[1] != 'K' {
		t.Fatalf("生成的内容不是 xlsx（应以 ZIP 魔数 PK 开头）")
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("解析自身生成的表格失败: %v", err)
	}
	if len(parsed) != 1+len(rows) {
		t.Fatalf("解析结果应为表头 + %d 行数据，实际 %d 行", len(rows), len(parsed))
	}
	for i, h := range headers {
		if got := Cell(parsed[0], i); got != h {
			t.Fatalf("第 %d 列表头期望 %q，实际 %q", i+1, h, got)
		}
	}
	for r, row := range rows {
		for i, want := range row {
			if got := Cell(parsed[r+1], i); got != want {
				t.Fatalf("第 %d 行第 %d 列期望 %q，实际 %q", r+2, i+1, want, got)
			}
		}
	}
}

// TestBuildKeepsLongNumbersAsText 锁定「文本格式」这一导出约定：
// 手机号/编号必须原样保留，不能变成科学计数法（1.38E+10）。
func TestBuildKeepsLongNumbersAsText(t *testing.T) {
	data := buildTable(t, []string{"编号"}, [][]string{{"13800138000"}})
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := Cell(parsed[1], 0); got != "13800138000" {
		t.Fatalf("长数字应原样保留为文本，实际 %q", got)
	}
}

// TestBuildStoresFormulaLookingTextAsLiteral 确认以 = 开头的内容不会被当成公式，
// 导出内容来自数据库，被 Excel 当公式执行不是我们想要的行为。
func TestBuildStoresFormulaLookingTextAsLiteral(t *testing.T) {
	data := buildTable(t, []string{"备注"}, [][]string{{"=1+1"}})
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := Cell(parsed[1], 0); got != "=1+1" {
		t.Fatalf("以 = 开头的内容应作为字面文本，实际 %q", got)
	}
}

func TestBuildWithoutColumnsFails(t *testing.T) {
	if _, err := NewTable[[]string]("空表").Build(); err == nil {
		t.Fatalf("没有定义列时应报错")
	}
	var be *errcode.Error
	if _, err := NewTable[[]string]("空表").Build(); !errors.As(err, &be) || be.Code != errcode.ErrInternal {
		t.Fatalf("未定义列的错误应为 ErrInternal，实际 %v", err)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantMsg string
	}{
		{"空内容", nil, "上传的文件为空"},
		{"不是 xlsx", []byte("这不是一个 xlsx 文件"), "无法解析该文件"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("应返回错误")
			}
			var be *errcode.Error
			if !errors.As(err, &be) {
				t.Fatalf("错误类型应为 *errcode.Error，实际 %T", err)
			}
			if be.Code != errcode.ErrBadRequest {
				t.Fatalf("错误码应为 ErrBadRequest，实际 %d", be.Code)
			}
			if !strings.Contains(be.Msg, tc.wantMsg) {
				t.Fatalf("错误信息期望包含 %q，实际 %q", tc.wantMsg, be.Msg)
			}
		})
	}
}

func TestCellSafeAndTrimmed(t *testing.T) {
	row := []string{" a ", "b"}
	if got := Cell(row, 0); got != "a" {
		t.Fatalf("Cell 应裁剪空白，实际 %q", got)
	}
	if got := Cell(row, -1); got != "" {
		t.Fatalf("负下标应返回空串，实际 %q", got)
	}
	if got := Cell(row, len(row)); got != "" {
		t.Fatalf("越界下标应返回空串，实际 %q", got)
	}
	if got := Cell(nil, 0); got != "" {
		t.Fatalf("空行应返回空串，实际 %q", got)
	}
}

func TestIsBlankRow(t *testing.T) {
	cases := []struct {
		name string
		row  []string
		want bool
	}{
		{"nil 行", nil, true},
		{"空行", []string{}, true},
		{"全空白", []string{"", "   ", "\t"}, true},
		{"有内容", []string{"", " x "}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBlankRow(tc.row); got != tc.want {
				t.Fatalf("IsBlankRow(%v) 期望 %v，实际 %v", tc.row, tc.want, got)
			}
		})
	}
}

// TestSanitizeSheetName 覆盖 Excel 工作表名的硬限制：非法字符替换与 31 字符上限。
// 超过上限会让整个导出直接失败，所以这条边界必须有测试兜住。
func TestSanitizeSheetName(t *testing.T) {
	if got := sanitizeSheetName("  "); got != "Sheet1" {
		t.Fatalf("空名应回退为 Sheet1，实际 %q", got)
	}
	if got := sanitizeSheetName("/a\\b?c*d[e]f:g"); got != "_a_b_c_d_e_f_g" {
		t.Fatalf("非法字符应被替换为下划线，实际 %q", got)
	}
	long := ""
	for i := 0; i < 40; i++ {
		long += "中"
	}
	got := sanitizeSheetName(long)
	if n := utf8.RuneCountInString(got); n != 31 {
		t.Fatalf("工作表名应按字符截断到 31，实际 %d", n)
	}
}

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 3},
		{"中文", 4},
		{"中a", 3},
		{"", 0},
	}
	for _, tc := range cases {
		if got := displayWidth(tc.in); got != tc.want {
			t.Fatalf("displayWidth(%q) 期望 %d，实际 %d", tc.in, tc.want, got)
		}
	}
}

func TestContentDispositionHelpers(t *testing.T) {
	// 中文文件名必须同时给出 ASCII 回退名与 RFC 5987 编码名
	fallback := asciiFallback("用户列表_20260304.xlsx")
	if fallback != "_____20260304.xlsx" {
		t.Fatalf("ASCII 回退名不符合预期: %q", fallback)
	}
	if got := asciiFallback("用户列表"); got != "export.xlsx" {
		t.Fatalf("全非 ASCII 且去掉占位符后为空时应回退为 export.xlsx，实际 %q", got)
	}
	if got := asciiFallback(`a"b\c`); got != "a_b_c" {
		t.Fatalf("引号与反斜杠应被替换，实际 %q", got)
	}

	escaped := rfc5987Escape("用户 列表.xlsx")
	if strings.Contains(escaped, "+") {
		t.Fatalf("RFC 5987 要求空格编码为 %%20 而不是 +，实际 %q", escaped)
	}
	if got := rfc5987Escape("用户列表.xlsx"); got != "%E7%94%A8%E6%88%B7%E5%88%97%E8%A1%A8.xlsx" {
		t.Fatalf("百分号编码不符合预期: %q", got)
	}
}
