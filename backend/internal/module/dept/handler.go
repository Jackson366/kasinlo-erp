package dept

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/response"
)

// Handler 部门接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建部门处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 部门树
// @Summary      查询部门树
// @Description  userCount 为该部门「含所有子部门」的用户数。
// @Tags         部门管理
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按部门名称 / 负责人模糊搜索"
// @Param        status   query     int     false  "状态：1 启用，0 禁用"  Enums(0, 1)
// @Success      200      {object}  response.Body{data=[]dept.Node}
// @Failure      403      {object}  response.Body  "缺少 system:dept:list / system:user:list 权限"
// @Router       /system/depts [get]
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

// Detail 部门详情
// @Summary      查询部门详情
// @Tags         部门管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "部门 ID"
// @Success      200  {object}  response.Body{data=dept.Node}
// @Failure      404  {object}  response.Body  "部门不存在"
// @Router       /system/depts/{id} [get]
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

// Create 新增部门
// @Summary      新增部门
// @Tags         部门管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateReq  true  "新增部门参数"
// @Success      200   {object}  response.Body{data=response.IDData}
// @Failure      400   {object}  response.Body  "参数校验失败 / 上级部门不存在"
// @Failure      409   {object}  response.Body  "同级下已存在同名部门"
// @Router       /system/depts [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增部门")
	id, err := h.svc.Create(req)
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
// @Failure      404   {object}  response.Body  "部门不存在"
// @Router       /system/depts/{id} [put]
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
	c.Set("opAction", "修改部门")
	if err := h.svc.Update(id, req); err != nil {
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
// @Failure      409  {object}  response.Body  "存在子部门 / 该部门下还有用户"
// @Router       /system/depts/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除部门")
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
