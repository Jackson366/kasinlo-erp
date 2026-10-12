package profile

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/module"
)

// Module 返回个人中心模块的自描述清单。
//
// 个人中心没有菜单：入口在右上角头像下拉里，权限只要求「已登录」。
// 仍然登记进注册表，是为了让「路由注册」这件事只有一个出口 ——
// 组装层遍历注册表即可，不必记得补一行 profile.Register(base)。
func Module() module.Module {
	return module.Module{
		Namespace: "profile",
		Register:  Register,
	}
}

// 保证 Register 的签名与 module.Module.Register 一致，改动时编译即报错。
var _ func(*gin.RouterGroup) = Register
