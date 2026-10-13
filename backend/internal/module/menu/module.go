package menu

import "nova-erp/internal/module"

// Module 返回菜单模块的自描述清单。
func Module() module.Module {
	return module.Module{
		Namespace: "system",
		Register:  Register,
		Menus: []module.MenuNode{
			{
				Name: "SystemMenu", Title: "菜单管理", Icon: "Menu",
				Path: "/system/menu", Component: "system/menu/index",
				Permission: "system:menu:list", Sort: 3,
				Children: []module.MenuNode{
					{Name: "SystemMenuCreate", Title: "新增菜单", Type: module.TypeButton, Permission: "system:menu:create"},
					{Name: "SystemMenuUpdate", Title: "修改菜单", Type: module.TypeButton, Permission: "system:menu:update"},
					{Name: "SystemMenuDelete", Title: "删除菜单", Type: module.TypeButton, Permission: "system:menu:delete"},
				},
			},
		},
	}
}
