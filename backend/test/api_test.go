package test

import (
	"net/http"
	"testing"
	"time"

	"nova-erp/internal/oplog"
)

// TestHealth 健康检查。
func TestHealth(t *testing.T) {
	eng := setup(t)
	req := newRequest(http.MethodGet, "/health", "", nil)
	rec := serve(eng, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if !contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("健康检查响应异常: %s", rec.Body.String())
	}
}

// TestAuth 认证相关：登录、失败、禁用、未授权、刷新、登出。
func TestAuth(t *testing.T) {
	t.Run("错误口令返回 40101", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": adminAccount, "password": "definitely-wrong",
		})
		assertStatus(t, res, http.StatusUnauthorized, "错误口令")
		assertCode(t, res, 40101, "错误口令")
	})

	t.Run("不存在的账号同样返回 40101", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "ghost_account", "password": "whatever123",
		})
		assertCode(t, res, 40101, "账号不存在")
	})

	t.Run("参数校验中文提示", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": ""})
		assertCode(t, res, 40000, "空参数")
		if !contains(res.Msg, "用户名不能为空") || !contains(res.Msg, "密码不能为空") {
			t.Fatalf("未返回中文校验提示: %s", res.Msg)
		}
	})

	token := loginAs(t, adminAccount, adminSecret)

	t.Run("管理员登录返回完整身份", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": adminAccount, "password": adminSecret,
		})
		assertCode(t, res, 0, "管理员登录")
		obj := res.obj(t)
		if str(obj, "accessToken") == "" || str(obj, "refreshToken") == "" {
			t.Fatalf("令牌缺失: %v", obj)
		}
		if num(obj, "expiresIn") <= 0 {
			t.Fatalf("expiresIn 异常: %v", obj["expiresIn"])
		}
		user, ok := obj["user"].(map[string]any)
		if !ok {
			t.Fatalf("登录响应缺少 user 对象: %v", obj)
		}
		if user["isSuper"] != true {
			t.Fatalf("超级管理员应返回 isSuper=true，实际: %v", user)
		}
		if num(user, "userId") != 1 {
			t.Fatalf("管理员 userId 应为 1，实际 %v", user["userId"])
		}
		if str(user, "username") != adminAccount {
			t.Fatalf("用户名不匹配: %v", user["username"])
		}
	})

	t.Run("无令牌访问受保护接口返回 401", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/users", "", nil)
		assertStatus(t, res, http.StatusUnauthorized, "无令牌")
		assertCode(t, res, 40100, "无令牌")
	})

	t.Run("非法令牌返回 401", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/users", "invalid.token.value", nil)
		assertStatus(t, res, http.StatusUnauthorized, "非法令牌")
	})

	t.Run("刷新令牌", func(t *testing.T) {
		login := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": adminAccount, "password": adminSecret,
		})
		refresh := str(login.obj(t), "refreshToken")
		res := call(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": refresh})
		assertCode(t, res, 0, "刷新令牌")
		if str(res.obj(t), "accessToken") == "" {
			t.Fatal("刷新后未返回 accessToken")
		}
	})

	t.Run("访问令牌不能当刷新令牌使用", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": token})
		assertCode(t, res, 40105, "类型混用")
	})

	t.Run("退出登录", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/logout", token, nil)
		assertCode(t, res, 0, "退出登录")
	})
}

// TestProfile 个人中心与动态路由。
func TestProfile(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	t.Run("获取个人信息", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/profile", token, nil)
		assertCode(t, res, 0, "个人信息")
		if str(res.obj(t), "username") != adminAccount {
			t.Fatalf("用户名不匹配: %v", res.Data)
		}
	})

	t.Run("动态路由树", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/profile/routes", token, nil)
		assertCode(t, res, 0, "动态路由")
		if len(res.List) < 2 {
			t.Fatalf("顶级路由数量不足: %d", len(res.List))
		}
		flat := mustJSON(res.Raw)
		if !contains(flat, "数据概览") || !contains(flat, "进销存") || !contains(flat, "系统管理") {
			t.Fatalf("路由缺少预期节点: %s", flat)
		}
		if contains(flat, "system:user:create") {
			t.Fatal("路由树中不应包含按钮级权限点")
		}
		if !contains(flat, "system/user/index") {
			t.Fatal("路由缺少组件路径，前端无法渲染")
		}
	})

	t.Run("修改个人资料", func(t *testing.T) {
		res := call(t, http.MethodPut, "/api/v1/profile", token, map[string]any{
			"nickname": "超级管理员", "email": "admin@example.com", "phone": "13800000000",
		})
		assertCode(t, res, 0, "修改资料")
	})

	t.Run("原密码错误返回 40106", func(t *testing.T) {
		res := call(t, http.MethodPut, "/api/v1/profile/password", token, map[string]any{
			"oldPassword": "not-the-old-one", "newPassword": "brandnew123456",
		})
		assertCode(t, res, 40106, "原密码错误")
	})
}

// TestPermissionIsolation 权限隔离：只读账号不能执行写操作。
func TestPermissionIsolation(t *testing.T) {
	demoToken := loginAs(t, demoAccount, demoSecret)

	t.Run("只读账号可读列表", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/users?page=1&pageSize=5", demoToken, nil)
		assertCode(t, res, 0, "只读列表")
	})

	t.Run("只读账号新增用户被拒绝", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/system/users", demoToken, map[string]any{
			"username": "hacker", "password": "hacker123456", "nickname": "越权测试",
		})
		assertStatus(t, res, http.StatusForbidden, "越权新增")
		assertCode(t, res, 40300, "越权新增")
	})

	t.Run("只读账号删除用户被拒绝", func(t *testing.T) {
		res := call(t, http.MethodDelete, "/api/v1/system/users/2", demoToken, nil)
		assertStatus(t, res, http.StatusForbidden, "越权删除")
	})

	t.Run("只读账号修改角色被拒绝", func(t *testing.T) {
		res := call(t, http.MethodPut, "/api/v1/system/roles/3", demoToken, map[string]any{"name": "改名"})
		assertStatus(t, res, http.StatusForbidden, "越权改角色")
	})

	t.Run("只读账号清空日志被拒绝", func(t *testing.T) {
		res := call(t, http.MethodDelete, "/api/v1/system/logs/operations", demoToken, map[string]any{})
		assertStatus(t, res, http.StatusForbidden, "越权清日志")
	})

	// 只读账号不应看到写操作按钮权限
	t.Run("只读账号权限集合不含写权限", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/profile", demoToken, nil)
		perms, _ := res.obj(t)["permissions"].([]any)
		for _, p := range perms {
			if s, ok := p.(string); ok && s == "system:user:create" {
				t.Fatal("只读账号不应拥有 system:user:create 权限")
			}
		}
	})
}

// TestUserCRUD 用户管理完整流程。
func TestUserCRUD(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	// 新增
	res := call(t, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "e2e_user", "password": "e2e12345678", "nickname": "端到端账号",
		"email": "e2e@example.com", "deptId": 2, "roleIds": []int64{3},
	})
	assertCode(t, res, 0, "新增用户")
	userID := num(res.obj(t), "id")
	if userID == 0 {
		t.Fatalf("新增用户未返回 id: %v", res.Raw)
	}

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(userID), token, nil)
	}()

	t.Run("重复用户名返回 40900", func(t *testing.T) {
		dup := call(t, http.MethodPost, "/api/v1/system/users", token, map[string]any{
			"username": "e2e_user", "password": "e2e12345678", "nickname": "重复",
		})
		assertCode(t, dup, 40900, "重复用户名")
	})

	t.Run("列表支持关键字搜索", func(t *testing.T) {
		list := call(t, http.MethodGet, "/api/v1/system/users?page=1&pageSize=10&keyword=e2e", token, nil)
		assertCode(t, list, 0, "用户搜索")
		if total := num(list.obj(t), "total"); total < 1 {
			t.Fatalf("搜索结果为空: %v", list.Data)
		}
	})

	t.Run("详情包含部门与角色", func(t *testing.T) {
		detail := call(t, http.MethodGet, "/api/v1/system/users/"+itoa(userID), token, nil)
		assertCode(t, detail, 0, "用户详情")
		obj := detail.obj(t)
		if str(obj, "deptName") == "" {
			t.Fatalf("详情缺少部门名称: %v", obj)
		}
		roles, _ := obj["roleIds"].([]any)
		if len(roles) != 1 {
			t.Fatalf("角色数量应为 1，实际 %v", obj["roleIds"])
		}
		if str(obj, "password") != "" {
			t.Fatal("响应不应包含密码字段")
		}
	})

	t.Run("新增用户可用新口令登录", func(t *testing.T) {
		res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "e2e_user", "password": "e2e12345678",
		})
		assertCode(t, res, 0, "新账号登录")
	})

	t.Run("修改用户", func(t *testing.T) {
		upd := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(userID), token, map[string]any{
			"nickname": "端到端账号-已改", "roleIds": []int64{3}, "status": 1,
		})
		assertCode(t, upd, 0, "修改用户")
		detail := call(t, http.MethodGet, "/api/v1/system/users/"+itoa(userID), token, nil)
		if str(detail.obj(t), "nickname") != "端到端账号-已改" {
			t.Fatalf("昵称未更新: %v", detail.Data)
		}
	})

	t.Run("禁用后无法登录", func(t *testing.T) {
		off := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(userID)+"/status", token, map[string]any{"status": 0})
		assertCode(t, off, 0, "禁用用户")
		login := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "e2e_user", "password": "e2e12345678",
		})
		assertCode(t, login, 40102, "禁用账号登录")
		// 恢复启用
		on := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(userID)+"/status", token, map[string]any{"status": 1})
		assertCode(t, on, 0, "启用用户")
	})

	t.Run("重置密码后新口令生效", func(t *testing.T) {
		rp := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(userID)+"/password", token, map[string]any{
			"password": "reset12345678",
		})
		assertCode(t, rp, 0, "重置密码")
		login := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "e2e_user", "password": "reset12345678",
		})
		assertCode(t, login, 0, "新口令登录")
	})

	t.Run("内置管理员受保护", func(t *testing.T) {
		del := call(t, http.MethodDelete, "/api/v1/system/users/1", token, nil)
		assertCode(t, del, 40000, "删除内置管理员")
		off := call(t, http.MethodPut, "/api/v1/system/users/1/status", token, map[string]any{"status": 0})
		assertCode(t, off, 40000, "禁用内置管理员")
	})

	t.Run("删除不存在的用户返回 40400", func(t *testing.T) {
		del := call(t, http.MethodDelete, "/api/v1/system/users/999999", token, nil)
		assertCode(t, del, 40400, "删除不存在用户")
	})

	t.Run("路径参数非法返回 40000", func(t *testing.T) {
		bad := call(t, http.MethodGet, "/api/v1/system/users/abc", token, nil)
		assertCode(t, bad, 40000, "非法 id")
	})
}

// TestRoleCRUD 角色管理与授权。
func TestRoleCRUD(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	res := call(t, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
		"name": "端到端角色", "code": "e2e_role", "sort": 99, "dataScope": 4,
		"menuIds": []int64{menuID(t, "inv:overview:view"), menuIDByName(t, "System"), menuID(t, "system:user:list")},
	})
	assertCode(t, res, 0, "新增角色")
	roleID := num(res.obj(t), "id")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/roles/"+itoa(roleID), token, nil)
	}()

	t.Run("角色列表", func(t *testing.T) {
		list := call(t, http.MethodGet, "/api/v1/system/roles?page=1&pageSize=10", token, nil)
		assertCode(t, list, 0, "角色列表")
		if num(list.obj(t), "total") < 3 {
			t.Fatalf("内置角色数量不足: %v", list.Data)
		}
	})

	t.Run("角色下拉选项", func(t *testing.T) {
		opts := call(t, http.MethodGet, "/api/v1/system/roles/options", token, nil)
		assertCode(t, opts, 0, "角色选项")
		if len(opts.List) < 3 {
			t.Fatalf("角色选项不足: %d", len(opts.List))
		}
	})

	t.Run("详情返回已授权菜单", func(t *testing.T) {
		detail := call(t, http.MethodGet, "/api/v1/system/roles/"+itoa(roleID), token, nil)
		assertCode(t, detail, 0, "角色详情")
		menus, _ := detail.obj(t)["menuIds"].([]any)
		if len(menus) != 3 {
			t.Fatalf("授权菜单应为 3 个，实际 %v", detail.obj(t)["menuIds"])
		}
	})

	t.Run("重复角色标识返回 40900", func(t *testing.T) {
		dup := call(t, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
			"name": "重复角色", "code": "e2e_role",
		})
		assertCode(t, dup, 40900, "重复角色标识")
	})

	t.Run("重新分配菜单权限", func(t *testing.T) {
		assign := call(t, http.MethodPut, "/api/v1/system/roles/"+itoa(roleID)+"/menus", token, map[string]any{
			"menuIds": []int64{menuID(t, "inv:overview:view"), menuIDByName(t, "System"),
				menuID(t, "system:user:list"), menuID(t, "system:role:list")},
		})
		assertCode(t, assign, 0, "分配权限")
		detail := call(t, http.MethodGet, "/api/v1/system/roles/"+itoa(roleID), token, nil)
		menus, _ := detail.obj(t)["menuIds"].([]any)
		if len(menus) != 4 {
			t.Fatalf("授权菜单应为 4 个，实际 %v", detail.obj(t)["menuIds"])
		}
	})

	t.Run("内置角色不可删除", func(t *testing.T) {
		del := call(t, http.MethodDelete, "/api/v1/system/roles/1", token, nil)
		assertCode(t, del, 40000, "删除内置角色")
	})

	t.Run("超管角色不可裁剪权限", func(t *testing.T) {
		assign := call(t, http.MethodPut, "/api/v1/system/roles/1/menus", token, map[string]any{"menuIds": []int64{menuID(t, "inv:overview:view")}})
		assertCode(t, assign, 40000, "裁剪超管权限")
	})
}

// TestMenuTree 菜单树管理。
func TestMenuTree(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	t.Run("菜单树结构", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/menus", token, nil)
		assertCode(t, res, 0, "菜单树")
		if len(res.List) < 2 {
			t.Fatalf("顶级菜单不足: %d", len(res.List))
		}
	})

	t.Run("菜单详情", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/menus/"+itoa(menuID(t, "system:user:list")), token, nil)
		assertCode(t, res, 0, "菜单详情")
		if str(res.obj(t), "permission") != "system:user:list" {
			t.Fatalf("权限标识不匹配: %v", res.Data)
		}
	})

	t.Run("存在子菜单时不可删除", func(t *testing.T) {
		res := call(t, http.MethodDelete, "/api/v1/system/menus/"+itoa(menuIDByName(t, "System")), token, nil)
		assertCode(t, res, 40900, "删除含子节点菜单")
	})

	t.Run("新增与删除菜单", func(t *testing.T) {
		create := call(t, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
			"parentId": 10, "name": "E2EMenu", "path": "/system/e2e", "component": "system/e2e/index",
			"title": "端到端菜单", "type": "menu", "permission": "system:e2e:list", "sort": 99,
		})
		assertCode(t, create, 0, "新增菜单")
		id := num(create.obj(t), "id")
		// 内置菜单与用户自建菜单共用同一条自增序列，主键不可能撞车 ——
		// 这正是「内置菜单预留 1-999 段」那条历史约束可以撤掉的原因。
		if id <= 0 {
			t.Fatalf("新增菜单未返回有效主键: %d", id)
		}

		dup := call(t, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
			"parentId": 10, "name": "E2EMenu", "title": "重复", "type": "menu",
		})
		assertCode(t, dup, 40900, "重复路由名")

		del := call(t, http.MethodDelete, "/api/v1/system/menus/"+itoa(id), token, nil)
		assertCode(t, del, 0, "删除菜单")
	})

	t.Run("不能把菜单挂到自己的子节点下", func(t *testing.T) {
		res := call(t, http.MethodPut, "/api/v1/system/menus/"+itoa(menuIDByName(t, "System")), token, map[string]any{
			"parentId": menuIDByName(t, "SystemUser"), "name": "System", "path": "/system", "title": "系统管理", "type": "dir",
		})
		assertCode(t, res, 40000, "环路校验")
	})
}

// TestDeptTree 部门树管理。
func TestDeptTree(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	t.Run("部门树含统计", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/depts", token, nil)
		assertCode(t, res, 0, "部门树")
		if len(res.List) < 1 {
			t.Fatal("部门树为空")
		}
		root, _ := res.List[0].(map[string]any)
		children, _ := root["children"].([]any)
		if len(children) < 2 {
			t.Fatalf("根部门子节点不足: %v", root)
		}
	})

	t.Run("有用户的部门不可删除", func(t *testing.T) {
		res := call(t, http.MethodDelete, "/api/v1/system/depts/2", token, nil)
		assertCode(t, res, 40900, "删除有用户部门")
	})

	t.Run("新增与删除部门", func(t *testing.T) {
		create := call(t, http.MethodPost, "/api/v1/system/depts", token, map[string]any{
			"parentId": 1, "name": "端到端部门", "leader": "测试负责人", "sort": 99,
		})
		assertCode(t, create, 0, "新增部门")
		id := num(create.obj(t), "id")

		dup := call(t, http.MethodPost, "/api/v1/system/depts", token, map[string]any{
			"parentId": 1, "name": "端到端部门",
		})
		assertCode(t, dup, 40900, "同级重名")

		del := call(t, http.MethodDelete, "/api/v1/system/depts/"+itoa(id), token, nil)
		assertCode(t, del, 0, "删除部门")
	})
}

// TestInvOverview 进销存数据概览（登录落地页的接口）。
func TestInvOverview(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	t.Run("概览统计", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/inv/overview", token, nil)
		assertCode(t, res, 0, "概览")
		obj := res.obj(t)
		// 种子数据：5 客户、8 商品、14 张单据（含待处理）
		if num(obj, "customerCount") < 5 {
			t.Fatalf("客户总数异常: %v", obj["customerCount"])
		}
		if num(obj, "productCount") < 8 {
			t.Fatalf("商品总数异常: %v", obj["productCount"])
		}
		if num(obj, "pendingOrders") < 1 {
			t.Fatalf("待处理单据数异常: %v", obj["pendingOrders"])
		}
	})

	t.Run("报表汇总", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/inv/report/summary?range=month", token, nil)
		assertCode(t, res, 0, "报表汇总")
		if num(res.obj(t), "salesOrders") < 1 {
			t.Fatalf("本月销售单数异常: %v", res.obj(t)["salesOrders"])
		}
	})

	t.Run("趋势按天补齐", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/inv/report/trend?days=7", token, nil)
		assertCode(t, res, 0, "趋势")
		if len(res.List) != 7 {
			t.Fatalf("趋势点数应为 7，实际 %d", len(res.List))
		}
	})
}

// TestLogs 日志查询与清空。
func TestLogs(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)

	// 清掉历史操作日志，再制造一条写操作，确保断言到的是本次产生的日志
	call(t, http.MethodDelete, "/api/v1/system/logs/operations", token, map[string]any{"before": "2100-01-01"})
	if err := oplog.Flush(5 * time.Second); err != nil {
		t.Fatalf("刷新日志缓冲失败: %v", err)
	}
	call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": demoAccount, "password": demoSecret,
	})
	if err := oplog.Flush(5 * time.Second); err != nil {
		t.Fatalf("刷新日志缓冲失败: %v", err)
	}

	t.Run("登录日志有记录", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/logs/logins?page=1&pageSize=5", token, nil)
		assertCode(t, res, 0, "登录日志")
		if num(res.obj(t), "total") < 1 {
			t.Fatalf("登录日志为空: %v", res.Data)
		}
	})

	t.Run("操作日志有记录", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/logs/operations?page=1&pageSize=5", token, nil)
		assertCode(t, res, 0, "操作日志")
		if len(res.List) < 1 {
			t.Fatal("操作日志为空")
		}
		first, _ := res.List[0].(map[string]any)
		if str(first, "method") == "" || str(first, "module") == "" {
			t.Fatalf("操作日志字段缺失: %v", first)
		}
		if num(first, "status") < 200 {
			t.Fatalf("操作日志状态码异常: %v", first["status"])
		}
	})

	// 回归：超管不传 before 清空全部 = 无条件 DELETE，曾被 GORM 的
	// ErrMissingWhereClause 拦下，接口报「数据库错误」且清不掉。
	// 上面 516 行那条 before=2100 的用例带 WHERE，覆盖不到这条线上路径。
	t.Run("操作日志不传before清空全部", func(t *testing.T) {
		res := call(t, http.MethodDelete, "/api/v1/system/logs/operations", token, nil)
		assertCode(t, res, 0, "不传 before 清空操作日志")
		if num(res.obj(t), "deleted") < 1 {
			t.Fatalf("应至少清掉一条日志: %v", res.Data)
		}
		// 清空请求自身会再产生一条审计日志，缓冲落库后列表应恰好回到 1 条
		if err := oplog.Flush(5 * time.Second); err != nil {
			t.Fatalf("刷新日志缓冲失败: %v", err)
		}
		after := call(t, http.MethodGet, "/api/v1/system/logs/operations?page=1&pageSize=5", token, nil)
		assertCode(t, after, 0, "清空后查询")
		if total := num(after.obj(t), "total"); total != 1 {
			t.Fatalf("清空全部后应只剩本次审计日志 1 条，实际 %d", total)
		}
	})

	t.Run("请求体中的敏感字段被脱敏", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/logs/operations?page=1&pageSize=20&module=auth", token, nil)
		assertCode(t, res, 0, "按模块筛选日志")
		for _, item := range res.List {
			row, _ := item.(map[string]any)
			req := str(row, "request")
			if req == "" {
				continue
			}
			if contains(req, adminSecret) || contains(req, demoSecret) {
				t.Fatalf("操作日志泄露了明文密码: %s", req)
			}
		}
	})

	t.Run("日志模块下拉", func(t *testing.T) {
		res := call(t, http.MethodGet, "/api/v1/system/logs/modules", token, nil)
		assertCode(t, res, 0, "模块下拉")
	})

	t.Run("清空指定时间之前的数据", func(t *testing.T) {
		res := call(t, http.MethodDelete, "/api/v1/system/logs/logins", token, map[string]any{"before": "2000-01-01"})
		assertCode(t, res, 0, "清空登录日志")
	})
}
