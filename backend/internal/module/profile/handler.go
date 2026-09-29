package profile

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/middleware"
	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/response"
)

// Handler 个人中心接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建个人中心处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Info 当前用户信息
// @Summary 获取当前登录用户信息
// @Tags    个人中心
// @Router  /profile [get]
func (h *Handler) Info(c *gin.Context) {
	id := middleware.CurrentIdentity(c)
	if id == nil {
		response.FailWith(c, errcode.ErrUnauthorized, "")
		return
	}
	data, err := h.svc.Info(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Update 修改个人资料
// @Summary 修改个人资料
// @Tags    个人中心
// @Router  /profile [put]
func (h *Handler) Update(c *gin.Context) {
	var req UpdateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改个人资料")
	if err := h.svc.Update(middleware.CurrentUserID(c), req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "资料已更新")
}

// ChangePassword 修改密码
// @Summary 修改当前用户密码
// @Tags    个人中心
// @Router  /profile/password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	var req ChangePasswordReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改密码")
	if err := h.svc.ChangePassword(middleware.CurrentUserID(c), req.OldPassword, req.NewPassword); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "密码修改成功，请重新登录")
}

// Routes 当前用户路由
// @Summary 获取当前用户可访问的菜单路由
// @Tags    个人中心
// @Router  /profile/routes [get]
func (h *Handler) Routes(c *gin.Context) {
	id := middleware.CurrentIdentity(c)
	if id == nil {
		response.FailWith(c, errcode.ErrUnauthorized, "")
		return
	}
	routes, err := h.svc.Routes(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, routes)
}
