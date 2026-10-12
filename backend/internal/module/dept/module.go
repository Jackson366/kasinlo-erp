package dept

import "nova-admin/internal/module"

// Module 返回部门模块的自描述清单。
func Module() module.Module {
	return module.Module{
		Namespace: "system",
		Register:  Register,
		Menus: []module.MenuNode{
			{
				Name: "SystemDept", Title: "部门管理", Icon: "OfficeBuilding",
				Path: "/system/dept", Component: "system/dept/index",
				Permission: "system:dept:list", Sort: 4,
				Children: []module.MenuNode{
					{Name: "SystemDeptCreate", Title: "新增部门", Type: module.TypeButton, Permission: "system:dept:create"},
					{Name: "SystemDeptUpdate", Title: "修改部门", Type: module.TypeButton, Permission: "system:dept:update"},
					{Name: "SystemDeptDelete", Title: "删除部门", Type: module.TypeButton, Permission: "system:dept:delete"},
				},
			},
		},
	}
}
