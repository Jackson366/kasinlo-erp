// Package assembly 是组装层（composition root）：全项目唯一知道「有哪些模块」的地方。
//
// 它必须 import 具体模块 —— 这正是它存在的全部意义。与之相对，框架层的其他包
// （internal/pkg、authz、middleware、oplog、database、seed、router、bootstrap 以及
// module 契约本身）一律不许 import 具体模块。这条线由
// scripts/check-module-boundary.sh 钉住：框架包里出现 internal/module/<具体模块>
// 即视为破坏 —— 一旦框架依赖了业务，那个业务模块就再也删不掉了。
//
// 换来的能力：
//   - 新增或下线一个模块 = 改这一个文件；
//   - 框架升级只动框架目录，业务升级只动业务目录，两边 diff 互不重叠；
//   - 「加一个菜单」从改 5 处收敛成在模块自己的清单里写一段。
package assembly

import (
	"nova-admin/internal/config"
	"nova-admin/internal/module"
	"nova-admin/internal/module/auth"
	"nova-admin/internal/module/dashboard"
	"nova-admin/internal/module/dept"
	"nova-admin/internal/module/log"
	"nova-admin/internal/module/menu"
	"nova-admin/internal/module/profile"
	"nova-admin/internal/module/role"
	"nova-admin/internal/module/user"
)

// Modules 返回本进程要装配的全部模块。
// 顺序即路由注册顺序与内置菜单的插入顺序：改顺序会改变菜单主键的分配次序，
// 语义上没有影响，但会让「按 ID 排序」的观感变化，非必要不要动。
func Modules(cfg *config.Config) []module.Module {
	return []module.Module{
		// 公开接口：登录、刷新令牌
		auth.Module(cfg.Security),
		// 登录后即可用，没有菜单（入口在右上角头像下拉里）
		profile.Module(),
		// 顶层菜单：仪表盘
		dashboard.Module(),
		// 「系统管理」分组。分组挂在哪个目录下属于导航结构，
		// 是组装决策，所以在这里声明 —— 各系统模块不必知道自己被归在哪个目录。
		module.Group(
			module.MenuNode{
				Name: "System", Title: "系统管理", Icon: "Setting",
				Path: "/system", Redirect: "/system/user", Sort: 2,
			},
			user.Module(),
			role.Module(),
			menu.Module(),
			dept.Module(),
			log.Module(cfg.Security),
		),

		// 业务模块在此往下追加，例如：
		//   invflow.Module(),
	}
}

// Build 登记并校验全部模块。
//
// main 与端到端测试共用同一个入口，避免两边各维护一份清单然后慢慢漂移
// （测试里少装一个模块，问题只会在某条用例莫名其妙 404 时才暴露）。
func Build(cfg *config.Config) error {
	return module.Build(Modules(cfg)...)
}
