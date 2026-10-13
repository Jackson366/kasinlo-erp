package auth

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/response"
)

// Handler 认证接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建认证处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Login 用户登录
// @Summary      用户登录
// @Description  校验用户名与密码，成功后返回访问令牌与刷新令牌。
// @Description  连续失败会触发「账号 + 来源 IP」锁定（40103）；同一来源 IP 失败过多会返回 429。
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        body  body      LoginReq  true  "登录参数"
// @Success      200   {object}  response.Body{data=auth.LoginResp}
// @Failure      400   {object}  response.Body  "参数校验失败"
// @Failure      401   {object}  response.Body  "用户名或密码错误 / 账号被禁用 / 账号被锁定"
// @Failure      429   {object}  response.Body  "来源 IP 请求过于频繁"
// @Failure      500   {object}  response.Body  "服务端错误"
// @Router       /auth/login [post]
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
// @Summary      刷新访问令牌
// @Description  用刷新令牌换取新的令牌对。改密码 / 重置密码 / 禁用账号后，旧刷新令牌会立即失效。
// @Description  该接口按 IP 做请求频率限制，超出返回 429。
// @Tags         认证
// @Accept       json
// @Produce      json
// @Param        body  body      RefreshReq  true  "刷新令牌"
// @Success      200   {object}  response.Body{data=auth.LoginResp}
// @Failure      400   {object}  response.Body  "参数校验失败"
// @Failure      401   {object}  response.Body  "刷新令牌无效或已过期"
// @Failure      429   {object}  response.Body  "请求过于频繁"
// @Router       /auth/refresh [post]
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
// @Summary      退出登录
// @Description  JWT 无状态，服务端仅清理身份缓存并记录审计日志；客户端应同时丢弃本地令牌。
// @Tags         认证
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body  "已退出登录"
// @Failure      401  {object}  response.Body  "未登录或令牌失效"
// @Router       /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	c.Set("opAction", "退出登录")
	if err := h.svc.Logout(middleware.CurrentUserID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "已退出登录")
}
