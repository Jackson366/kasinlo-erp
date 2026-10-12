package test

// 本文件钉住「改角色 = 授权行为」这一防线的两条旁路（详见 authz_scope_test.go 的背景说明）：
//
//  1. Update 挂菜单此前没有 EnsureRoleMenusGrantable：持有 system:role:update 的
//     非超管可以把一个已挂用户的角色改成任意菜单组合（比如加上自己都没有的
//     system:user:delete），该角色现有持有者下一次 authz 缓存失效后就拿到新权限。
//  2. Create 此前完全不看 actor：持有 system:role:create 的非超管可以直接
//     造出 data_scope=全部数据 + 任意菜单的角色，等于给自己发全库通行证。

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestRoleCreateCannotExceedGranter 非超管创建角色不得超过自身能力（P2）。
func TestRoleCreateCannotExceedGranter(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	createAdmin := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("role_cre%d", suffix), "password": "probe123456",
		"nickname": "创建越权探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, createAdmin, 0, "创建受限管理员")
	scopedID := num(createAdmin.obj(t), "id")
	scopedToken := loginAs(t, fmt.Sprintf("role_cre%d", suffix), "probe123456")

	// 1. 不传 dataScope 时默认「全部数据」，比自己（本部门及以下）宽 → 403
	def := call(t, http.MethodPost, "/api/v1/system/roles", scopedToken, map[string]any{
		"name": fmt.Sprintf("默认范围角色%d", suffix), "code": fmt.Sprintf("cre_def_%d", suffix), "sort": 99,
	})
	assertStatus(t, def, http.StatusForbidden, "默认创建全部数据范围角色")

	// 2. 显式全部数据 → 403
	wide := call(t, http.MethodPost, "/api/v1/system/roles", scopedToken, map[string]any{
		"name": fmt.Sprintf("全部数据角色%d", suffix), "code": fmt.Sprintf("cre_wide_%d", suffix),
		"sort": 99, "dataScope": 1,
	})
	assertStatus(t, wide, http.StatusForbidden, "显式创建全部数据范围角色")

	// 3. 范围合法但挂上自己没有的权限码（103 = system:user:delete）→ 403
	unowned := call(t, http.MethodPost, "/api/v1/system/roles", scopedToken, map[string]any{
		"name": fmt.Sprintf("越权菜单角色%d", suffix), "code": fmt.Sprintf("cre_menu_%d", suffix),
		"sort": 99, "dataScope": 3, "menuIds": []int64{menuID(t, "system:user:list"), menuID(t, "system:user:delete")},
	})
	assertStatus(t, unowned, http.StatusForbidden, "挂上自己没有的权限码")

	// 4. 反向确认：本部门及以下 + 自己有的菜单 → 0（否则是误伤）
	ok := call(t, http.MethodPost, "/api/v1/system/roles", scopedToken, map[string]any{
		"name": fmt.Sprintf("合规角色%d", suffix), "code": fmt.Sprintf("cre_ok_%d", suffix),
		"sort": 99, "dataScope": 3, "menuIds": []int64{menuID(t, "system:user:list"), menuID(t, "system:user:create")},
	})
	assertCode(t, ok, 0, "创建合规角色")
	roleID := num(ok.obj(t), "id")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/roles/"+itoa(roleID), adminToken, nil)
	}()
}

// TestRoleUpdateCannotGrantUnownedMenus 修改角色不得挂上自己没有的权限码（P1）。
func TestRoleUpdateCannotGrantUnownedMenus(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	// 受限管理员先造一个自己能力范围内的角色
	createAdmin := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("role_upd%d", suffix), "password": "probe123456",
		"nickname": "修改越权探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, createAdmin, 0, "创建受限管理员")
	scopedID := num(createAdmin.obj(t), "id")
	scopedToken := loginAs(t, fmt.Sprintf("role_upd%d", suffix), "probe123456")

	createRole := call(t, http.MethodPost, "/api/v1/system/roles", scopedToken, map[string]any{
		"name": fmt.Sprintf("回显角色%d", suffix), "code": fmt.Sprintf("upd_%d", suffix),
		"sort": 99, "dataScope": 3, "menuIds": []int64{menuID(t, "system:user:list"), menuID(t, "system:user:create")},
	})
	assertCode(t, createRole, 0, "受限管理员创建合规角色")
	roleID := num(createRole.obj(t), "id")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/roles/"+itoa(roleID), adminToken, nil)
	}()

	// 1. 把角色菜单改成包含 103（system:user:delete，自己都没有）→ 403
	escalate := call(t, http.MethodPut, "/api/v1/system/roles/"+itoa(roleID), scopedToken,
		map[string]any{"name": fmt.Sprintf("回显角色%d", suffix), "menuIds": []int64{menuID(t, "system:user:list"), menuID(t, "system:user:delete")}})
	assertStatus(t, escalate, http.StatusForbidden, "修改角色挂上自己没有的权限码")
	assertCode(t, escalate, 40300, "修改角色挂上自己没有的权限码")

	// 2. 反向确认：换成自己有的菜单组合 → 0
	legit := call(t, http.MethodPut, "/api/v1/system/roles/"+itoa(roleID), scopedToken,
		map[string]any{"name": fmt.Sprintf("回显角色%d", suffix), "menuIds": []int64{menuID(t, "system:user:list"), menuID(t, "system:user:update")}})
	assertCode(t, legit, 0, "修改角色为自身能力范围内的菜单")
}
