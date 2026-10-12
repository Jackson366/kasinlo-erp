package role

import "nova-admin/internal/module"

// Module 返回角色模块的自描述清单。
func Module() module.Module {
	return module.Module{
		Namespace: "system",
		Register:  Register,
		Menus: []module.MenuNode{
			{
				Name: "SystemRole", Title: "角色管理", Icon: "Key",
				Path: "/system/role", Component: "system/role/index",
				Permission: "system:role:list", Sort: 2, KeepAlive: true,
				Children: []module.MenuNode{
					{Name: "SystemRoleCreate", Title: "新增角色", Type: module.TypeButton, Permission: "system:role:create"},
					{Name: "SystemRoleUpdate", Title: "修改角色", Type: module.TypeButton, Permission: "system:role:update"},
					{Name: "SystemRoleDelete", Title: "删除角色", Type: module.TypeButton, Permission: "system:role:delete"},
					{Name: "SystemRoleAssign", Title: "分配权限", Type: module.TypeButton, Permission: "system:role:assign"},
					{Name: "SystemRoleExport", Title: "导出角色", Type: module.TypeButton, Permission: "system:role:export"},
				},
			},
		},
	}
}
