package ops

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/module"
)

// Module 返回运维模块的自描述清单。
//
// 页面是「系统运维」一个页、四个页签（备份管理 / 数据清理 / 系统巡检 / 系统升级），
// 而不是四个页面：这四个动作本来就是一次运维会话里连着做的事 ——
// 巡检发现问题、备份留底、清理、升级。拆成四个页面只会让人在菜单里来回跳。
//
// 权限拆成六个点而不是一个「运维管理员」：备份只读、恢复建库、清理不可逆、
// 升级替换程序，影响面完全不同（见 route.go）。其中「日志清理」与「业务数据清理」
// 再拆成两个点：前者按保留期删流水，后者是**整表清空账本 / 重置数据库**，
// 让「能清日志的运维」不等于「能重置数据库的人」。
func Module(cfg *config.Config) module.Module {
	register := func(rg *gin.RouterGroup) { Register(rg, cfg) }
	return module.Module{
		Namespace: "ops",
		Register:  register,
		Menus: []module.MenuNode{
			{
				Name: "SystemOps", Title: "系统运维", Icon: "Tools",
				Path: "/system/ops", Component: "system/ops/index",
				Permission: "ops:list", Sort: 6, KeepAlive: false,
				Children: []module.MenuNode{
					{Name: "SystemOpsBackup", Title: "执行备份", Type: module.TypeButton, Permission: "ops:backup"},
					{Name: "SystemOpsRestore", Title: "恢复演练", Type: module.TypeButton, Permission: "ops:restore"},
					{Name: "SystemOpsCleanup", Title: "日志清理", Type: module.TypeButton, Permission: "ops:cleanup"},
					{Name: "SystemOpsReset", Title: "业务数据清理", Type: module.TypeButton, Permission: "ops:reset"},
					{Name: "SystemOpsUpgrade", Title: "系统升级", Type: module.TypeButton, Permission: "ops:upgrade"},
				},
			},
		},
	}
}
