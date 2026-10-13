package log

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/module"
)

// Module 返回日志模块的自描述清单。
//
// 需要配置的模块在自己的包内构造闭包（这里把 security 段固化进 Register），
// 于是 module 包不必依赖 config 包。
func Module(sec config.SecurityConfig) module.Module {
	register := func(rg *gin.RouterGroup) { Register(rg, sec) }
	return module.Module{
		Namespace: "system",
		Register:  register,
		Menus: []module.MenuNode{
			{
				Name: "SystemLog", Title: "操作日志", Icon: "Document",
				Path: "/system/log", Component: "system/log/index",
				Permission: "system:log:list", Sort: 5,
				Children: []module.MenuNode{
					{Name: "SystemLogDelete", Title: "清空日志", Type: module.TypeButton, Permission: "system:log:delete"},
					{Name: "SystemLogExport", Title: "导出日志", Type: module.TypeButton, Permission: "system:log:export"},
				},
			},
		},
	}
}
