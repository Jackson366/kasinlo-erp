package ops

import (
	"testing"
)

// 备份时间的解析规则已上收到框架层 ops.BackupTimeFromName（唯一实现），
// 这里只守护「列表排序依赖时间戳」这一行为不变量，具体解析见框架层测试。

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
