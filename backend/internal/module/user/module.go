package user

import "nova-admin/internal/module"

// Module 返回用户模块的自描述清单。
//
// 菜单与按钮权限点从 seed.go 搬到这里：权限码归声明它的模块所有，
// 于是「加一个按钮权限」只需要动这一个文件，不必再去框架的 seed.go 里
// 手工分配菜单 ID。
func Module() module.Module {
	return module.Module{
		Namespace: "system",
		Register:  Register,
		Menus: []module.MenuNode{
			{
				Name: "SystemUser", Title: "用户管理", Icon: "User",
				Path: "/system/user", Component: "system/user/index",
				Permission: "system:user:list", Sort: 1, KeepAlive: true,
				Children: []module.MenuNode{
					{Name: "SystemUserCreate", Title: "新增用户", Type: module.TypeButton, Permission: "system:user:create"},
					{Name: "SystemUserUpdate", Title: "修改用户", Type: module.TypeButton, Permission: "system:user:update"},
					{Name: "SystemUserDelete", Title: "删除用户", Type: module.TypeButton, Permission: "system:user:delete"},
					{Name: "SystemUserReset", Title: "重置密码", Type: module.TypeButton, Permission: "system:user:reset"},
					{Name: "SystemUserExport", Title: "导出用户", Type: module.TypeButton, Permission: "system:user:export"},
					{Name: "SystemUserImport", Title: "导入用户", Type: module.TypeButton, Permission: "system:user:import"},
				},
			},
		},
	}
}
