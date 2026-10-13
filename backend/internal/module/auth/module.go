package auth

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/module"
)

// Module 返回认证模块的自描述清单。
//
// 认证接口是公开的（登录、刷新），不挂菜单也没有权限码，
// 但它同样从注册表出发注册路由，保证「新增模块要改的地方」只有组装层那一处。
func Module(cfg config.SecurityConfig) module.Module {
	register := func(rg *gin.RouterGroup) { Register(rg, cfg) }
	return module.Module{
		Namespace: "auth",
		Register:  register,
	}
}
