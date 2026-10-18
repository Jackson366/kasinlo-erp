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

// 调用方传 0 表示「用默认值」，而不是「删掉此刻之前的全部记录」。
// 0 天的字面含义对操作日志就是清空审计，所以框架层必须自己兜底，
// 不能指望每个调用方（命令行、HTTP 接口、将来的定时任务）都记得换算。
func TestCleanupDefaultsGuardAgainstZeroDays(t *testing.T) {
	// 默认值必须是正的：任何一项 <= 0 都会让对应边界退化成「现在」，
	// 于是「清理」变成「删掉全部」。
	if defaultOpLogDays <= 0 || defaultLoginLogDays <= 0 ||
		defaultIdempotencyDays <= 0 || defaultLoginLockSec <= 0 {
		t.Fatal("清理默认值必须为正")
	}
	now := time.Now()
	opCut, loginCut, idemCut := resolveCutoffs(CleanupOptions{
		OpLogDays:        defaultOpLogDays,
		LoginLogDays:     defaultLoginLogDays,
		IdempotencyDays:  defaultIdempotencyDays,
		LoginLockSeconds: defaultLoginLockSec,
		Now:              now,
	})
	// 边界必须早于「现在」，否则等于全删
	for name, cut := range map[string]time.Time{"操作日志": opCut, "登录日志": loginCut, "幂等键": idemCut} {
		if cut.IsZero() || !cut.Before(now) {
			t.Fatalf("%s 边界异常：%v", name, cut)
		}
	}
}
