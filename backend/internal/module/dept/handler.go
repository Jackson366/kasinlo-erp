package dept

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/response"
)

// Handler 部门接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建部门处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 部门树
// @Summary      查询部门树
// @Description  userCount 为该部门「含所有子部门」的用户数。返回结果受调用者的数据范围约束：
// @Description  非超管只能看到本部门（本部门及以下）范围内的部门，范围外的部门按「不存在」处理。
// @Tags         部门管理
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按部门名称 / 负责人模糊搜索"
// @Param        status   query     int     false  "状态：1 启用，0 禁用"  Enums(0, 1)
// @Success      200      {object}  response.Body{data=[]dept.Node}
// @Failure      403      {object}  response.Body  "缺少 system:dept:list / system:user:list 权限"
// @Router       /system/depts [get]
func (h *Handler) List(c *gin.Context) {
	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	tree, err := h.svc.List(c.Query("keyword"), status, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, tree)
}

// Detail 部门详情
// @Summary      查询部门详情
// @Tags         部门管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "部门 ID"
// @Success      200  {object}  response.Body{data=dept.Node}
// @Failure      404  {object}  response.Body  "部门不存在，或超出你的数据范围"
// @Router       /system/depts/{id} [get]
func (h *Handler) Detail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	node, err := h.svc.Get(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, node)
}

// Create 新增部门
// @Summary      新增部门
// @Tags         部门管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateReq  true  "新增部门参数"
// @Success      200   {object}  response.Body{data=response.IDData}
// @Failure      400   {object}  response.Body  "参数校验失败 / 上级部门不存在"
// @Failure      403   {object}  response.Body  "上级部门超出你的数据范围"
// @Failure      409   {object}  response.Body  "同级下已存在同名部门"
// @Router       /system/depts [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增部门")
	id, err := h.svc.Create(req, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// Update 修改部门
// @Summary      修改部门
// @Description  不能把自己或自己的子孙部门设为上级部门。
// @Tags         部门管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int        true  "部门 ID"
// @Param        body  body      UpdateReq  true  "修改参数"
// @Success      200   {object}  response.Body  "修改成功"
// @Failure      400   {object}  response.Body  "上级部门不能是自己 / 不能选择自己的子部门"
// @Failure      403   {object}  response.Body  "目标部门或上级部门超出你的数据范围"
// @Failure      404   {object}  response.Body  "部门不存在"
// @Router       /system/depts/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req UpdateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改部门")
	if err := h.svc.Update(id, req, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// Delete 删除部门
// @Summary      删除部门
// @Description  存在子部门或仍有用户时禁止删除。
// @Tags         部门管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "部门 ID"
// @Success      200  {object}  response.Body  "删除成功"
// @Failure      403  {object}  response.Body  "目标部门超出你的数据范围"
// @Failure      409  {object}  response.Body  "存在子部门 / 该部门下还有用户"
// @Router       /system/depts/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除部门")
	if err := h.svc.Delete(id, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}
