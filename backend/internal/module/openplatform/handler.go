package openplatform

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/response"
)

type handler struct {
	bridge bridge
}

type deviceRequest struct {
	UserCode string `json:"userCode" binding:"required,len=12" label:"设备验证码"`
}

type decisionRequest struct {
	UserCode string `json:"userCode" binding:"required,len=12" label:"设备验证码"`
	Approve  *bool  `json:"approve" binding:"required" label:"授权决定"`
	Scope    string `json:"scope" label:"授权范围"`
}

func (h handler) inspect(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var in deviceRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		response.FailWith(c, errcode.ErrBadRequest, "请输入 12 位设备验证码")
		return
	}
	actor := middleware.CurrentIdentity(c)
	var details deviceDetails
	if err := h.bridge.call(c.Request.Context(), "device/inspect",
		request{UserID: actor.UserID, Epoch: actor.TokenEpoch, UserCode: in.UserCode}, &details); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, details)
}

func (h handler) decide(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var in decisionRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		response.FailWith(c, errcode.ErrBadRequest, "授权决定无效")
		return
	}
	actor := middleware.CurrentIdentity(c)
	if in.Scope == "" {
		in.Scope = "inventory.read"
	}
	if *in.Approve {
		for _, scope := range strings.Split(in.Scope, " ") {
			var permission string
			switch scope {
			case "inventory.read":
				permission = "inv:stock:list"
			case "product.read":
				permission = "inv:product:list"
			case "purchase.read":
				permission = "inv:purchase:list"
			case "sales.read":
				permission = "inv:sales:list"
			default:
				response.FailWith(c, errcode.ErrBadRequest, "授权范围无效")
				return
			}
			if !actor.HasAnyPerm(permission) {
				response.FailWith(c, errcode.ErrForbidden, "当前账号没有所申请的查询权限")
				return
			}
		}
	}
	var result map[string]bool
	if err := h.bridge.call(c.Request.Context(), "device/decision",
		request{UserID: actor.UserID, Epoch: actor.TokenEpoch, UserCode: in.UserCode, Approve: in.Approve, Scope: in.Scope}, &result); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "授权决定已保存")
}

func (h handler) list(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor := middleware.CurrentIdentity(c)
	var grants []grant
	if err := h.bridge.call(c.Request.Context(), "grants/list",
		request{UserID: actor.UserID, Epoch: actor.TokenEpoch}, &grants); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, grants)
}

func (h handler) revoke(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor := middleware.CurrentIdentity(c)
	var result map[string]bool
	if err := h.bridge.call(c.Request.Context(), "grants/revoke",
		request{UserID: actor.UserID, Epoch: actor.TokenEpoch, GrantID: c.Param("id")}, &result); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "授权已撤销")
}
