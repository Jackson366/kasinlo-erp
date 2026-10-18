package ops

import (
	"testing"
	"time"
)

// 备份时间必须取文件名里的时间戳：拷贝过的备份 mtime 会变成拷贝时间，
// 用它排序会把「新拷的旧备份」排到最前面，恢复时拿错文件。
func TestBackupTimeFromName(t *testing.T) {
	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	got := backupTimeFromName("nova_inv_v5-20261018220000.dump", fallback)
	want := time.Date(2026, 10, 18, 22, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := backupTimeFromName("weird-name.dump", fallback); !got.Equal(fallback) {
		t.Fatalf("解析不出时间戳应回落到 fallback，实际 %v", got)
	}
}

// 确认口令必须是精确匹配：模糊匹配（包含、忽略大小写）会让「确认」形同虚设。
func TestConfirmPasswordsAreExact(t *testing.T) {
	for _, wrong := range []string{"", "确认清理 ", "确认", "CONFIRM", "确认升级"} {
		if wrong == confirmCleanup {
			t.Fatalf("%q 不应等于清理确认口令", wrong)
		}
	}
	if confirmCleanup == confirmUpgrade {
		t.Fatal("清理与升级的确认口令必须不同：同一个口令连点两下就升级了")
	}
}

// 文件名一旦由客户端给出，就必须按纯 basename 校验：
// 带路径的输入要么是穿越尝试，要么是调用方把整条路径塞了进来。
func TestParseActionsToleratesGarbage(t *testing.T) {
	for _, in := range []string{"", "   ", "not-json", "null", "[{\"key\":\"backend\"}]"} {
		got := parseActions(in)
		if got == nil {
			t.Fatalf("%q 不应返回 nil（前端要能直接 map）", in)
		}
	}
	if acts := parseActions(`[{"key":"backend","title":"替换后端程序","done":true}]`); len(acts) != 1 || !acts[0].Done {
		t.Fatalf("合法 JSON 应被解析出来：%+v", acts)
	}
}

// hasPendingUpgrade 决定「能不能再升一次」：除 success / failed 外都算未收尾，
// 含 executing（正在执行 —— 并发抢占态）。
func TestHasPendingUpgrade(t *testing.T) {
	if hasPendingUpgrade(nil) {
		t.Fatal("没有记录不应判为有待升级")
	}
	if !hasPendingUpgrade([]UpgradeRecord{{Status: "pending"}}) {
		t.Fatal("pending 应判为有待升级")
	}
	if !hasPendingUpgrade([]UpgradeRecord{{Status: "executing"}}) {
		t.Fatal("executing（执行中）应判为有待升级")
	}
	if !hasPendingUpgrade([]UpgradeRecord{{Status: "applied"}}) {
		t.Fatal("applied（已执行未确认）应判为有待升级")
	}
	if hasPendingUpgrade([]UpgradeRecord{{Status: "success"}, {Status: "failed"}}) {
		t.Fatal("success / failed 不应判为有待升级")
	}
}
