package log

import (
	"errors"
	"testing"

	"nova-erp/internal/pkg/errcode"
)

// 锁定 parseTime/parseTimeStrict 的语义契约：
// 空串 =「不筛」，两者都必须放行；只有「非空但解析失败」两者才有分歧。
func TestParseTime(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool // 是否应成功解析
	}{
		{"空串", "", false},
		{"RFC3339", "2026-01-02T03:04:05Z", true},
		{"带时区偏移", "2026-01-02T03:04:05+08:00", true},
		{"SQL 格式", "2026-01-02 03:04:05", true},
		{"无秒无区", "2026-01-02T03:04:05", true},
		{"仅日期", "2026-01-02", true},
		{"乱码", "not-a-time", false},
		{"格式残缺", "2026-13", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTime(tc.in)
			if tc.want != (got != nil) {
				t.Fatalf("parseTime(%q) = %v, 期望非nil=%v", tc.in, got, tc.want)
			}
			if got != nil && got.Year() != 2026 {
				t.Fatalf("parseTime(%q) 年份 = %d, 期望 2026", tc.in, got.Year())
			}
		})
	}
}

func TestParseTimeStrict(t *testing.T) {
	// 空串：nil + nil error，表示「未传时间条件」
	if v, err := parseTimeStrict(""); v != nil || err != nil {
		t.Fatalf("空串应返回 (nil, nil)，实际 (%v, %v)", v, err)
	}

	// 合法值：与 parseTime 同口径
	if v, err := parseTimeStrict("2026-01-02 03:04:05"); err != nil || v == nil {
		t.Fatalf("合法格式不应报错：%v", err)
	}

	// 非法值：必须报错。清空接口依赖这一点——静默放行会把
	// 「按时间清空」放大成「清空范围内全部」。
	v, err := parseTimeStrict("not-a-time")
	if v != nil {
		t.Fatalf("解析失败时应返回 nil 值，实际 %v", v)
	}
	if err == nil {
		t.Fatal("解析失败必须返回错误")
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) || ec.Code != errcode.ErrBadRequest {
		t.Fatalf("错误码应为 ErrBadRequest，实际 %v", err)
	}
}

// 两个入口共用 parseFlexible，同一输入必须得到同一时刻。
func TestParseTimeConsistency(t *testing.T) {
	for _, in := range []string{"2026-01-02T03:04:05Z", "2026-01-02 03:04:05", "2026-01-02"} {
		a := parseTime(in)
		b, err := parseTimeStrict(in)
		if a == nil || b == nil || err != nil {
			t.Fatalf("%q: 两侧解析结果不一致 (%v, %v, %v)", in, a, b, err)
		}
		if !a.Equal(*b) {
			t.Fatalf("%q: 时刻不同 %v vs %v", in, a, b)
		}
	}
	// ParseInLocation 对无时区串按本地时区解释：确保年月日不被挪动
	tm := parseTime("2026-01-02 23:59:59")
	if tm == nil || tm.Day() != 2 || tm.Hour() != 23 {
		t.Fatalf("本地时区解析日/时被挪动：%v", tm)
	}
}
