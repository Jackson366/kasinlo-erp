package menu

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/response"
)

// Handler 菜单接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建菜单处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 菜单树
// @Summary 查询菜单树
// @Tags    菜单管理
// @Router  /system/menus [get]
func (h *Handler) List(c *gin.Context) {
	var status *int16
	if v := c.Query("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			s := int16(n)
			status = &s
		}
	}
	tree, err := h.svc.List(c.Query("keyword"), status)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, tree)
}

// Detail 菜单详情
// @Summary 查询菜单详情
// @Tags    菜单管理
// @Router  /system/menus/{id} [get]
func (h *Handler) Detail(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	node, err := h.svc.Get(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, node)
}

// Create 新增菜单
// @Summary 新增菜单
// @Tags    菜单管理
// @Router  /system/menus [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增菜单")
	id, err := h.svc.Create(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}

// Update 修改菜单
// @Summary 修改菜单
// @Tags    菜单管理
// @Router  /system/menus/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req UpdateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改菜单")
	if err := h.svc.Update(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// Delete 删除菜单
// @Summary 删除菜单
// @Tags    菜单管理
// @Router  /system/menus/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除菜单")
	if err := h.svc.Delete(id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

func pathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errcode.New(errcode.ErrBadRequest, "路径参数 id 非法")
	}
	return id, nil
}
