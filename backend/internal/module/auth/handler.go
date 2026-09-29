package auth

import (
	"github.com/gin-gonic/gin"

	"nova-admin/internal/middleware"
	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/response"
)

// Handler 认证接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建认证处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Login 用户登录
// @Summary  用户登录
// @Tags     认证
// @Router   /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "登录")
	res, err := h.svc.Login(req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Refresh 刷新令牌
// @Summary  刷新访问令牌
// @Tags     认证
// @Router   /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	res, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Logout 退出登录
// @Summary  退出登录
// @Tags     认证
// @Router   /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	c.Set("opAction", "退出登录")
	if err := h.svc.Logout(middleware.CurrentUserID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "已退出登录")
}
