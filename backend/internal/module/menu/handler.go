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
// @Summary      查询菜单树
// @Tags         菜单管理
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按标题 / 路由名 / 权限标识模糊搜索"
// @Param        status   query     int     false  "状态：1 启用，0 禁用"  Enums(0, 1)
// @Success      200      {object}  response.Body{data=[]menu.Node}
// @Failure      403      {object}  response.Body  "缺少 system:menu:list 权限"
// @Router       /system/menus [get]
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
// @Summary      查询菜单详情
// @Tags         菜单管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "菜单 ID"
// @Success      200  {object}  response.Body{data=menu.Node}
// @Failure      404  {object}  response.Body  "菜单不存在"
// @Router       /system/menus/{id} [get]
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
// @Summary      新增菜单
// @Description  类型必须为 dir / menu / button 之一。新增后会同步授权给超管角色，保证立即可见。
// @Tags         菜单管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateReq  true  "新增菜单参数"
// @Success      200   {object}  response.Body{data=response.IDData}
// @Failure      400   {object}  response.Body  "参数校验失败 / 上级菜单不存在"
// @Failure      409   {object}  response.Body  "路由名称已存在"
// @Router       /system/menus [post]
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
	response.OK(c, response.IDData{ID: id})
}

// Update 修改菜单
// @Summary      修改菜单
// @Description  不能把自己或自己的子孙节点设为上级菜单。
// @Tags         菜单管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int        true  "菜单 ID"
// @Param        body  body      UpdateReq  true  "修改参数"
// @Success      200   {object}  response.Body  "修改成功"
// @Failure      400   {object}  response.Body  "上级菜单不能是自己 / 不能选择自己的子菜单"
// @Failure      404   {object}  response.Body  "菜单不存在"
// @Router       /system/menus/{id} [put]
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
// @Summary      删除菜单
// @Description  存在子菜单时禁止删除。
// @Tags         菜单管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "菜单 ID"
// @Success      200  {object}  response.Body  "删除成功"
// @Failure      409  {object}  response.Body  "存在子菜单"
// @Router       /system/menus/{id} [delete]
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
