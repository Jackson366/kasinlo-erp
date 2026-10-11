package test

// 本文件钉住两条授权扩散旁路（详见 authz_scope_test.go / grant_bypass_test.go 背景）：
//
//  1. 日志模块此前完全不接数据范围：内置 system_admin 就持有 system:log:list/export，
//     部门管理员能看到全库登录日志（含 IP / UA）与操作日志——与 dashboard
//     模块自己声明的「范围管理员只看范围内」口径直接矛盾。
//     修复后列表 / 导出 / 模块下拉 / 清空四条路径统一走 authz.Scope.ApplyLog。
//  2. 菜单 permission 此前任何持 system:menu:update 的人都能随意改写：
//     改一个已挂角色的菜单权限码等价于批量改发该角色全部持有者的有效权限。
//     方案（用户确认）：非超管只能设置自己持有的权限码（子集校验）。

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestLogScopeRestrictsDeptAdmin 部门管理员的日志可见性收窄到本部门及以下（P1）。
func TestLogScopeRestrictsDeptAdmin(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	dept2User := fmt.Sprintf("log_dep2_%d", suffix) // 技术部管理员（本部门及以下）
	dept3User := fmt.Sprintf("log_dep3_%d", suffix) // 运营部普通用户（范围外）

	// system_admin（角色2）按设计不含删除权限，给它单独造一个
	// 「本部门及以下 + system:log:list/delete」的角色，才能覆盖清空路径。
	createRole := call(t, http.MethodPost, "/api/v1/system/roles", adminToken, map[string]any{
		"name": fmt.Sprintf("日志清理角色%d", suffix), "code": fmt.Sprintf("log_clr_%d", suffix),
		"sort": 99, "dataScope": 3, "menuIds": []int64{15, 141},
	})
	assertCode(t, createRole, 0, "创建日志清理角色")
	roleID := num(createRole.obj(t), "id")

	createA := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": dept2User, "password": "probe123456",
		"nickname": "日志范围探针A", "deptId": 2, "roleIds": []int64{roleID},
	})
	assertCode(t, createA, 0, "创建技术部管理员")
	aID := num(createA.obj(t), "id")

	createB := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": dept3User, "password": "probe123456",
		"nickname": "日志范围探针B", "deptId": 3, "roleIds": []int64{3},
	})
	assertCode(t, createB, 0, "创建运营部用户")
	bID := num(createB.obj(t), "id")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(aID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(bID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/roles/"+itoa(roleID), adminToken, nil)
	}()

	// 两个探针各自登录一次，产生登录日志（B 的日志在技术部管理员的范围外）
	aToken := loginAs(t, dept2User, "probe123456")
	bToken := loginAs(t, dept3User, "probe123456")
	if bToken == "" {
		t.Fatal("探针B登录失败，无法产生范围外日志")
	}

	// 1. A 查 B（运营部）的登录日志 → 一条都看不到
	ab := call(t, http.MethodGet, "/api/v1/system/logs/logins?username="+dept3User, aToken, nil)
	assertCode(t, ab, 0, "范围内用户查登录日志不应报错")
	if total := num(ab.obj(t), "total"); total != 0 {
		t.Fatalf("技术部管理员看到了运营部用户的登录日志：total=%d", total)
	}

	// 2. A 查自己的登录日志 → 至少一条（登录动作本身已产生记录）
	aa := call(t, http.MethodGet, "/api/v1/system/logs/logins?username="+dept2User, aToken, nil)
	assertCode(t, aa, 0, "查自己的登录日志")
	if total := num(aa.obj(t), "total"); total < 1 {
		t.Fatalf("自己的登录日志应至少一条，实际 total=%d", total)
	}

	// 3. 对照：admin 查 B 的登录日志 → 至少一条（证明 B 的日志确实存在，
	//    上面第 1 步的 0 是范围过滤的结果而不是没有数据）
	abAdmin := call(t, http.MethodGet, "/api/v1/system/logs/logins?username="+dept3User, adminToken, nil)
	assertCode(t, abAdmin, 0, "超管查范围外日志")
	if total := num(abAdmin.obj(t), "total"); total < 1 {
		t.Fatalf("超管应能看到运营部用户的登录日志，实际 total=%d", total)
	}

	// 4. 清空同样受范围约束。范围外信号源：B（运营部）做一次写操作产生
	//    操作日志，A 清空后它必须原样保留。
	//    不拿 admin 的记录计数做断言：其他测试建的探针用户名（dept_admin_probe
	//    等）含 "admin" 且恰在 dept2，ILIKE 模糊匹配会把它们计入 admin 的计数，
	//    A 清掉自己范围内的记录属正确行为，会造成误报。dept3User 不含 admin
	//    字样且全库唯一，不受此干扰。
	bUpdate := call(t, http.MethodPut, "/api/v1/profile", bToken, map[string]any{
		"nickname": "日志范围探针B", "email": fmt.Sprintf("logb%d@example.com", suffix),
	})
	assertCode(t, bUpdate, 0, "B 产生范围外操作日志")

	// oplog 是异步批量落盘（64 条或 2 秒），轮询等待 B 的记录可见
	var bTotalBefore int64
	for i := 0; i < 20; i++ {
		res := call(t, http.MethodGet, "/api/v1/system/logs/operations?username="+dept3User, adminToken, nil)
		assertCode(t, res, 0, "查 B 的操作日志")
		bTotalBefore = num(res.obj(t), "total")
		if bTotalBefore >= 1 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if bTotalBefore < 1 {
		t.Fatalf("前置条件不满足：B 的操作日志未落盘（等了 6 秒）")
	}

	clear := call(t, http.MethodDelete, "/api/v1/system/logs/operations", aToken, map[string]any{})
	assertCode(t, clear, 0, "范围内清空操作日志")

	res := call(t, http.MethodGet, "/api/v1/system/logs/operations?username="+dept3User, adminToken, nil)
	assertCode(t, res, 0, "复查 B 的操作日志")
	bTotalAfter := num(res.obj(t), "total")
	if bTotalAfter < bTotalBefore {
		t.Fatalf("技术部管理员清空日志影响到了范围外数据（B 的记录被删）：before=%d after=%d",
			bTotalBefore, bTotalAfter)
	}
	_ = bToken // B 的令牌仅用于产生登录日志
}

// TestMenuPermissionSubset 非超管只能把自己持有的权限码设到菜单上（P2）。
func TestMenuPermissionSubset(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	// system_admin（角色2）持有 system:user:list/create/update，没有 system:user:delete
	createA := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("menu_sub%d", suffix), "password": "probe123456",
		"nickname": "菜单子集探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, createA, 0, "创建受限管理员")
	aID := num(createA.obj(t), "id")
	aToken := loginAs(t, fmt.Sprintf("menu_sub%d", suffix), "probe123456")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(aID), adminToken, nil)
	}()

	// 幂等自愈：本测试历史版本曾直接改种子菜单 11 的权限码，防线禁用时
	// 会把脏数据永久写进测试库（menu 11 → system:user:delete）。
	// 开头无条件恢复一次，保证测试库状态可重复。
	// 注意必须传全量字段：menu Update 是整体覆盖，漏掉 component/icon/keepAlive
	// 会把它们清空，破坏 TestProfile 的路由树断言。
	restore := call(t, http.MethodPut, "/api/v1/system/menus/11", adminToken, map[string]any{
		"parentId": 10, "name": "SystemUser", "path": "/system/user",
		"component": "system/user/index", "title": "用户管理", "icon": "User",
		"type": "menu", "permission": "system:user:list", "sort": 1, "keepAlive": true,
	})
	assertCode(t, restore, 0, "恢复种子菜单权限码")

	// 1. A 先自建一个持有码按钮（自建资源，不碰种子数据），再尝试把它的
	//    permission 改成自己没有的码 → 403
	btn := call(t, http.MethodPost, "/api/v1/system/menus", aToken, map[string]any{
		"parentId": 11, "name": fmt.Sprintf("SubBtn%d", suffix), "title": "子集探针按钮",
		"type": "button", "permission": "system:user:update", "sort": 99,
	})
	assertCode(t, btn, 0, "自建持有码按钮")
	btnID := num(btn.obj(t), "id")

	escalate := call(t, http.MethodPut, "/api/v1/system/menus/"+itoa(btnID), aToken, map[string]any{
		"parentId": 11, "name": fmt.Sprintf("SubBtn%d", suffix), "title": "子集探针按钮",
		"type": "button", "permission": "system:user:delete", "sort": 99,
	})
	assertStatus(t, escalate, http.StatusForbidden, "把自己没有的权限码设到菜单上")
	assertCode(t, escalate, 40300, "把自己没有的权限码设到菜单上")

	// 2. 反向确认：设回自己持有的码 → 0
	legit := call(t, http.MethodPut, "/api/v1/system/menus/"+itoa(btnID), aToken, map[string]any{
		"parentId": 11, "name": fmt.Sprintf("SubBtn%d", suffix), "title": "子集探针按钮",
		"type": "button", "permission": "system:user:update", "sort": 99,
	})
	assertCode(t, legit, 0, "设置自己持有的权限码应放行")

	// 3. 新建带自己没有的权限码的按钮 → 403
	createEvil := call(t, http.MethodPost, "/api/v1/system/menus", aToken, map[string]any{
		"parentId": 11, "name": fmt.Sprintf("EvilBtn%d", suffix), "title": "越权按钮",
		"type": "button", "permission": "system:user:delete", "sort": 99,
	})
	assertStatus(t, createEvil, http.StatusForbidden, "新建带越权权限码的菜单")

	// 4. 反向确认：新建自己持有的权限码按钮 → 0（并清理两个探针按钮）
	createOk := call(t, http.MethodPost, "/api/v1/system/menus", aToken, map[string]any{
		"parentId": 11, "name": fmt.Sprintf("OwnBtn%d", suffix), "title": "自有按钮",
		"type": "button", "permission": "system:user:list", "sort": 99,
	})
	assertCode(t, createOk, 0, "新建自带有权限码的菜单")
	if okID := num(createOk.obj(t), "id"); okID > 0 {
		call(t, http.MethodDelete, "/api/v1/system/menus/"+itoa(okID), adminToken, nil)
	}
	if btnID > 0 {
		call(t, http.MethodDelete, "/api/v1/system/menus/"+itoa(btnID), adminToken, nil)
	}
}
