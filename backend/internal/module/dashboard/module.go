package dashboard

import "nova-admin/internal/module"

// Module 返回仪表盘模块的自描述清单。
//
// 菜单权限码 dashboard:view 与 route.go 里 RequirePerm 用的是同一个字符串 ——
// 这正是把这层声明收进模块的价值：以前一个在 seed.go、一个在 route.go，
// 改一处忘另一处的表现是「菜单看得见、点进去接口 403」。
func Module() module.Module {
	return module.Module{
		Namespace: "dashboard",
		Register:  Register,
		Menus: []module.MenuNode{
			{
				Name: "Dashboard", Title: "仪表盘", Icon: "Odometer",
				Path: "/dashboard", Component: "dashboard/index",
				Permission: "dashboard:view", Sort: 1, KeepAlive: true,
			},
		},
	}
}
