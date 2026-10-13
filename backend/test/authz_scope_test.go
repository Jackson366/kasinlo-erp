package test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"nova-erp/internal/model"

	"nova-erp/internal/database"
)

// 本文件覆盖「授权入口的唯一性」与「角色属性本身也是授权能力」这两类缺陷。
//
// 背景：上一轮加固给「把角色授予用户」加了 authz.EnsureGrantable，
// 但它只挂在 user.replaceRoles 一条路径上，于是同一件事还有三条旁路：
// 批量导入、修改角色、给角色分配菜单。这几条都能绕过它拿到超出自身的能力。

// TestImportCannotEscalate 导入不得绕过角色授予校验（P0）。
//
// 导入的写库路径直接往 sys_user_roles 插关联行，不经过 replaceRoles，
// 此前只校验「角色名是否存在」。持有 system:user:import 的非超管
// 可以在 Excel 的角色列填「超级管理员」，批量造出超管账号。
func TestImportCannotEscalate(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	// 受限管理员：技术部 + 系统管理员（数据范围=本部门及以下）
	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("imp_esc_admin%d", suffix), "password": "probe123456",
		"nickname": "导入越权探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, create, 0, "创建受限管理员")
	scopedID := num(create.obj(t), "id")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
	}()

	scopedToken := loginAs(t, fmt.Sprintf("imp_esc_admin%d", suffix), "probe123456")
	victim := fmt.Sprintf("imp_esc_super%d", suffix)

	rows := [][]string{{victim, "批量提权", "", "", "", "技术部", "超级管理员", "启用", ""}}
	book := buildSheet(t, importHeaders, rows)

	// 1. dryRun 预演不写库，统计里应显示「校验通过」
	preview := uploadCall(t, "/api/v1/system/users/import?dryRun=1", scopedToken, "users.xlsx", book)
	if preview.Code != http.StatusOK {
		t.Fatalf("导入预演失败: HTTP %d body=%s", preview.Code, preview.Body.String())
	}
	if p, code := parseImportResult(t, preview.Body.Bytes()); code != 0 || p.Valid != 1 {
		t.Fatalf("预演结果异常 code=%d valid=%d（期望 0/1，说明行校验本身是通过的）", code, p.Valid)
	}

	// 2. 真正写入时必须被 403 拦下
	rec := uploadCall(t, "/api/v1/system/users/import", scopedToken, "users.xlsx", book)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("导入授予超管角色应返回 403，实际 HTTP %d body=%s", rec.Code, rec.Body.String())
	}

	// 3. 被拒的导入不能留下任何痕迹
	var cnt int64
	if err := database.DB().Model(&model.User{}).Where("username = ?", victim).Count(&cnt).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if cnt != 0 {
		t.Fatal("被拒绝的导入仍然写入了用户记录")
	}
}

// TestRoleScopeAndMenusCannotWiden 非超管不得把角色调到「比自己更宽」（P1）。
//
// 只按「权限码 ⊆ 自身权限」判定是不够的：一个不挂任何菜单、但数据范围=全部的角色，
// 权限码集合为空，任何部门管理员都能把它授出去，受赠者直接获得全库数据可见性。
// 同理，「修改角色」本身就是一次授权：把任意角色的 dataScope 改成 1 等于发全库通行证。
func TestRoleScopeAndMenusCannotWiden(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	createRole := call(t, http.MethodPost, "/api/v1/system/roles", adminToken, map[string]any{
		"name": fmt.Sprintf("全部数据角色%d", suffix), "code": fmt.Sprintf("wide_scope_%d", suffix),
		"sort": 99, "dataScope": 1, // 1 = 全部数据
	})
	assertCode(t, createRole, 0, "创建全部数据范围角色")
	wideID := num(createRole.obj(t), "id")

	createAdmin := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("scope_esc%d", suffix), "password": "probe123456",
		"nickname": "范围越权探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, createAdmin, 0, "创建受限管理员")
	scopedID := num(createAdmin.obj(t), "id")

	defer func() {
		// 清理顺序必须是「先删用户、再删角色」：角色下还挂着用户时删除会返回 409，
		// 残留数据会让下一次运行撞上「角色标识已存在」。defer 是 LIFO，
		// 所以不能分成两条 defer，必须写在同一个闭包里顺序执行。
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/roles/"+itoa(wideID), adminToken, nil)
	}()

	scopedToken := loginAs(t, fmt.Sprintf("scope_esc%d", suffix), "probe123456")

	// 1. 受限管理员自己的范围是「本部门及以下」，不能授予「全部数据」角色
	victim := call(t, http.MethodPost, "/api/v1/system/users", scopedToken, map[string]any{
		"username": fmt.Sprintf("scope_victim%d", suffix), "password": "probe123456",
		"nickname": "范围受害者", "deptId": 2, "roleIds": []int64{wideID},
	})
	assertStatus(t, victim, http.StatusForbidden, "授予更宽数据范围的角色")
	assertCode(t, victim, 40300, "授予更宽数据范围的角色")

	// 2. 角色下拉里也不该出现这个角色（否则又是一个「选了必然 403」的选项）
	options := call(t, http.MethodGet, "/api/v1/system/roles/options", scopedToken, nil)
	assertCode(t, options, 0, "拉取角色下拉")
	for _, item := range options.List {
		if row := item.(map[string]any); row["id"].(float64) == float64(wideID) {
			t.Fatal("角色下拉不应包含数据范围比自己更宽的角色")
		}
	}

	// 3. 更不能把一个已有角色的数据范围改成 1
	widen := call(t, http.MethodPut, "/api/v1/system/roles/"+itoa(wideID), scopedToken,
		map[string]any{"name": fmt.Sprintf("全部数据角色%d", suffix), "dataScope": 1})
	assertStatus(t, widen, http.StatusForbidden, "把角色数据范围调宽")
	assertCode(t, widen, 40300, "把角色数据范围调宽")

	// 4. 反向确认：超管自己可以调（否则是误伤）
	byAdmin := call(t, http.MethodPut, "/api/v1/system/roles/"+itoa(wideID), adminToken,
		map[string]any{"name": fmt.Sprintf("全部数据角色%d", suffix), "dataScope": 1})
	assertCode(t, byAdmin, 0, "超管调整数据范围")
}

// TestUpdateCannotDisableSuperViaStatus 分支绕过（P1）。
//
// ensureTargetNotSuper 此前只在 roleIds != nil 时调用，
// 于是「不传 roleIds、只传 status」就能禁用另一个超管账号。
// 系统允许多个账号持有 super_admin，只挡内置 id=1 是不够的。
func TestUpdateCannotDisableSuperViaStatus(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	// 造第二个超管账号，挂到技术部下，让部门管理员的数据范围能覆盖它
	second := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("super2_%d", suffix), "password": "probe123456",
		"nickname": "二号超管", "deptId": 2, "roleIds": []int64{1},
	})
	assertCode(t, second, 0, "创建第二个超管账号")
	secondID := num(second.obj(t), "id")

	scoped := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("super2_esc%d", suffix), "password": "probe123456",
		"nickname": "禁用超管探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, scoped, 0, "创建受限管理员")
	scopedID := num(scoped.obj(t), "id")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(secondID), adminToken, nil)
	}()

	scopedToken := loginAs(t, fmt.Sprintf("super2_esc%d", suffix), "probe123456")

	// 只改 status、不传 roleIds —— 此前这条路径完全不校验目标是否超管
	attack := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(secondID), scopedToken,
		map[string]any{"nickname": "二号超管", "deptId": 2, "status": 0})
	assertStatus(t, attack, http.StatusForbidden, "禁用超管账号")
	assertCode(t, attack, 40300, "禁用超管账号")

	var status int16
	if err := database.DB().Model(&model.User{}).Where("id = ?", secondID).
		Select("status").Scan(&status).Error; err != nil {
		t.Fatalf("查询状态失败: %v", err)
	}
	if status != model.StatusEnabled {
		t.Fatalf("超管账号被禁用了，status=%d", status)
	}
}

// TestPartialUpdateKeepsDepartment 半更新不能把字段清空（P1）。
//
// GORM 对 map 形式的 Updates 不做零值省略：请求里没带的字段会写成零值。
// dept_id 是指针，nil 表示「没带」，若原样写进 map 就会把用户踢出部门，
// 数据范围随之退化成仅本人 —— 一个只改昵称的请求就能造成这种后果。
func TestPartialUpdateKeepsDepartment(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": fmt.Sprintf("partial_%d", suffix), "password": "probe123456",
		"nickname": "半更新受害者", "deptId": 2,
	})
	assertCode(t, create, 0, "创建用户")
	id := num(create.obj(t), "id")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(id), adminToken, nil)
	}()

	// 只带 nickname，不带 deptId
	res := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(id), adminToken,
		map[string]any{"nickname": "半更新受害者改"})
	assertCode(t, res, 0, "半更新")

	var deptID *int64
	if err := database.DB().Model(&model.User{}).Where("id = ?", id).
		Select("dept_id").Scan(&deptID).Error; err != nil {
		t.Fatalf("查询部门失败: %v", err)
	}
	if deptID == nil || *deptID != 2 {
		t.Fatalf("未传 deptId 时部门不应被改动，实际 dept_id=%v", deptID)
	}
}
