package ops

import (
	"testing"
	"time"
)

// 边界算错一天，要么少留一天日志（审计缺口），要么白留一年（磁盘）。
func TestResolveCutoffs(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.Local)
	opCut, loginCut, idemCut := resolveCutoffs(CleanupOptions{
		OpLogDays:        180,
		LoginLogDays:     180,
		IdempotencyDays:  7,
		LoginLockSeconds: 300,
		Now:              now,
	})

	if want := now.AddDate(0, 0, -180); !opCut.Equal(want) {
		t.Fatalf("操作日志边界=%v，期望 %v", opCut, want)
	}
	// 180 天远大于锁定窗口，登录日志边界应与保留期一致
	if want := now.AddDate(0, 0, -180); !loginCut.Equal(want) {
		t.Fatalf("登录日志边界=%v，期望 %v", loginCut, want)
	}
	if want := now.AddDate(0, 0, -7); !idemCut.Equal(want) {
		t.Fatalf("幂等键边界=%v，期望 %v", idemCut, want)
	}
}

// 保留期被配成 0 天时，锁定窗口必须兜住：否则「清理」等于「解除所有账号锁定」。
func TestResolveCutoffsRespectsLockWindow(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.Local)
	_, loginCut, _ := resolveCutoffs(CleanupOptions{
		OpLogDays:        1,
		LoginLogDays:     0, // 配错：想删掉全部
		LoginLockSeconds: 300,
		Now:              now,
	})
	if want := now.Add(-5 * time.Minute); !loginCut.Equal(want) {
		t.Fatalf("锁定窗口未生效：边界=%v，期望 %v", loginCut, want)
	}
}
