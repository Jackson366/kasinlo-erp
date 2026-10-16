package textutil

import (
	"regexp"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFormatTime(t *testing.T) {
	if got := FormatTime(time.Time{}); got != "" {
		t.Fatalf("零值时间应返回空串，实际 %q", got)
	}

	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if got := FormatTime(at); got != "2026-03-04 05:06:07" {
		t.Fatalf("时间格式不符合预期: %q", got)
	}
}

func TestTimestampFormat(t *testing.T) {
	// 格式串曾漂移过一次（少一位年份），文件名会静默变短，这里锁定形状
	re := regexp.MustCompile(`^\d{8}_\d{6}$`)
	if got := Timestamp(); !re.MatchString(got) {
		t.Fatalf("时间戳应为 yyyyMMdd_HHmmss 形状，实际 %q", got)
	}
}

func TestTruncateByRuneNotByte(t *testing.T) {
	// 中文一个字 3 字节；按字节截断会得到非法 UTF-8，这里断言的是按字符
	// 而不是按字节计算长度
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"ASCII 截断", "abcdef", 3, "abc"},
		{"中文截断", "你好世界", 2, "你好"},
		{"未超长原样返回", "你好", 5, "你好"},
		{"刚好等于上限", "你好", 2, "你好"},
		{"limit 为 0 不限长", "你好世界", 0, "你好世界"},
		{"limit 为负不限长", "你好世界", -1, "你好世界"},
		{"空串", "", 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.in, tc.limit)
			if got != tc.want {
				t.Fatalf("Truncate(%q, %d) 期望 %q，实际 %q", tc.in, tc.limit, tc.want, got)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("Truncate(%q, %d) 产生了非法 UTF-8: %q", tc.in, tc.limit, got)
			}
		})
	}
}

func TestTruncateEllipsisOnlyWhenTruncated(t *testing.T) {
	if got := TruncateEllipsis("你好世界", 2, "…"); got != "你好…" {
		t.Fatalf("截断时应追加后缀，实际 %q", got)
	}
	if got := TruncateEllipsis("你好", 5, "…"); got != "你好" {
		t.Fatalf("未截断时不应追加后缀，实际 %q", got)
	}
	if got := TruncateEllipsis("你好世界", 0, "…"); got != "你好世界" {
		t.Fatalf("limit<=0 表示不限长，不应追加后缀，实际 %q", got)
	}
}

func TestFitRemovesInvalidUTF8(t *testing.T) {
	// 非法字节必须被剔除：直接写库会触发 SQLSTATE 22021（非法 UTF-8）
	got := Fit("a\xffb", 0)
	if got != "ab" {
		t.Fatalf("非法字节应被剔除，实际 %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("Fit 结果必须是合法 UTF-8，实际 %q", got)
	}

	// 非法字节剔除后仍要按字符收敛长度
	got = Fit("你好\xff世界", 3)
	if got != "你好世" {
		t.Fatalf("Fit 应在剔除非法字节后按字符截断，实际 %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("Fit 结果必须是合法 UTF-8，实际 %q", got)
	}
}

// TestFitNeverExceedsLimit 用一组混合输入确认「写入边界的长度严格收敛」这一硬约束。
func TestFitNeverExceedsLimit(t *testing.T) {
	inputs := []string{
		"abc",
		"你好世界",
		"a\xffb\xfe",
		"emoji😀测试",
	}
	for _, in := range inputs {
		for _, limit := range []int{1, 2, 3, 7} {
			got := Fit(in, limit)
			if !utf8.ValidString(got) {
				t.Fatalf("Fit(%q, %d) 产生了非法 UTF-8: %q", in, limit, got)
			}
			if n := utf8.RuneCountInString(got); n > limit {
				t.Fatalf("Fit(%q, %d) 结果长度 %d 超过了上限", in, limit, n)
			}
		}
	}
}
