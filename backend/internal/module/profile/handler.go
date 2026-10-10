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
// @Summary      获取当前登录用户信息
// @Description  返回个人资料、角色与权限码集合（超管不返回权限码，前端以其 isSuper 判定放行）。
// @Tags         个人中心
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=profile.Info}
// @Failure      401  {object}  response.Body  "未登录或令牌失效"
// @Router       /profile [get]
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
// @Summary      修改个人资料
// @Tags         个人中心
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      UpdateReq  true  "个人资料"
// @Success      200   {object}  response.Body  "资料已更新"
// @Failure      400   {object}  response.Body  "参数校验失败"
// @Router       /profile [put]
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
// @Summary      修改当前用户密码
// @Description  修改成功后该用户所有已签发令牌立即失效，需重新登录。
// @Tags         个人中心
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      ChangePasswordReq  true  "原密码与新密码"
// @Success      200   {object}  response.Body  "密码修改成功，请重新登录"
// @Failure      400   {object}  response.Body  "新密码不能与原密码相同"
// @Failure      401   {object}  response.Body  "原密码错误"
// @Router       /profile/password [put]
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
// @Summary      获取当前用户可访问的菜单路由
// @Description  返回前端动态路由树（dir / menu 两类，已补齐父级目录）；超管返回全部。
// @Tags         个人中心
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=[]menu.Route}
// @Failure      401  {object}  response.Body  "未登录或令牌失效"
// @Router       /profile/routes [get]
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
