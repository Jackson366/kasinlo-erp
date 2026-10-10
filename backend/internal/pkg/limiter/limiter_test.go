package limiter

import (
	"testing"
	"time"
)

func TestFailLimiterThresholdAndReset(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }

	for i := 1; i <= 3; i++ {
		if !l.Allow("a") {
			t.Fatalf("第 %d 次尝试应放行", i)
		}
		l.Fail("a")
	}
	if l.Allow("a") {
		t.Fatal("达到阈值后应拒绝")
	}
	if !l.Allow("b") {
		t.Fatal("不同 key 之间不应互相影响")
	}

	// 成功即清零
	l.Reset("a")
	if !l.Allow("a") {
		t.Fatal("重置后应重新放行")
	}
}

func TestFailLimiterWindowExpiry(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }

	l.Fail("a")
	l.Fail("a")
	if l.Allow("a") {
		t.Fatal("窗口内达到阈值应拒绝")
	}

	now = now.Add(2 * time.Minute)
	if !l.Allow("a") {
		t.Fatal("时间窗滑过后应重新放行")
	}
}

func TestFailLimiterDisabled(t *testing.T) {
	l := New(0, time.Minute)
	for i := 0; i < 50; i++ {
		l.Fail("a")
	}
	if !l.Allow("a") {
		t.Fatal("关闭状态下应始终放行")
	}

	// nil 接收者也不能 panic（Service 未初始化时的防御）
	var nilL *FailLimiter
	if !nilL.Allow("a") {
		t.Fatal("nil 限流器应放行")
	}
	nilL.Fail("a")
	nilL.Reset("a")
}
