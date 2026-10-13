package role

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
	"nova-erp/internal/pkg/sheet"
	"nova-erp/internal/pkg/textutil"
)

// Handler 角色接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建角色处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 角色列表
// @Summary      分页查询角色
// @Tags         角色管理
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int     false  "页码"     default(1)
// @Param        pageSize  query     int     false  "每页条数"  default(10)
// @Param        keyword   query     string  false  "按角色名称 / 标识模糊搜索"
// @Param        orderBy   query     string  false  "排序字段：sort / createdAt / name"
// @Param        order     query     string  false  "排序方向：asc / desc"  Enums(asc, desc)
// @Success      200       {object}  response.Body{data=response.PageData{list=[]role.VO}}
// @Failure      403       {object}  response.Body  "缺少 system:role:list 权限"
// @Router       /system/roles [get]
func (h *Handler) List(c *gin.Context) {
	q := page.FromContext(c)
	items, total, err := h.svc.List(q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, items, total, q.Page, q.PageSize)
}

// Options 角色下拉选项
// @Summary      角色下拉选项
// @Description  只返回调用者有权授予的角色：不含超管角色，也不含权限集合超出调用者自身的角色。
// @Tags         角色管理
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=[]role.Option}
// @Failure      403  {object}  response.Body  "缺少 system:role:list / system:user:list 权限"
// @Router       /system/roles/options [get]
func (h *Handler) Options(c *gin.Context) {
	items, err := h.svc.Options(middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Detail 角色详情
// @Summary      查询角色详情
// @Tags         角色管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "角色 ID"
// @Success      200  {object}  response.Body{data=role.VO}
// @Failure      404  {object}  response.Body  "角色不存在"
// @Router       /system/roles/{id} [get]
func (h *Handler) Detail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	vo, err := h.svc.Get(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, vo)
}

// Create 新增角色
// @Summary      新增角色
// @Description  角色标识 super_admin 已被内置超管占用，不可使用。
// @Tags         角色管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateReq  true  "新增角色参数"
// @Success      200   {object}  response.Body{data=response.IDData}
// @Failure      400   {object}  response.Body  "参数校验失败"
// @Failure      409   {object}  response.Body  "角色标识已存在 / 超管标识被占用"
// @Router       /system/roles [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增角色")
	id, err := h.svc.Create(req, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// Update 修改角色
// @Summary      修改角色
// @Description  内置角色不允许禁用；超管角色的权限不可裁剪。
// @Tags         角色管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int        true  "角色 ID"
// @Param        body  body      UpdateReq  true  "修改参数"
// @Success      200   {object}  response.Body  "修改成功"
// @Failure      400   {object}  response.Body  "内置角色不允许禁用"
// @Failure      403   {object}  response.Body  "无权修改超管角色 / 不能把数据范围调得比自身更宽"
// @Failure      404   {object}  response.Body  "角色不存在"
// @Router       /system/roles/{id} [put]
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
	c.Set("opAction", "修改角色")
	if err := h.svc.Update(id, req, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// Delete 删除角色
// @Summary      删除角色
// @Description  内置角色不允许删除；角色下仍有用户时返回 409。
// @Tags         角色管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "角色 ID"
// @Success      200  {object}  response.Body  "删除成功"
// @Failure      400  {object}  response.Body  "内置角色不允许删除"
// @Failure      403  {object}  response.Body  "无权删除超管角色"
// @Failure      409  {object}  response.Body  "该角色下还有用户"
// @Router       /system/roles/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除角色")
	if err := h.svc.Delete(id, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// AssignMenus 分配菜单权限
// @Summary      为角色分配菜单权限
// @Description  覆盖式设置。超管角色默认拥有全部权限，不需要也不允许在此分配。
// @Tags         角色管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int             true  "角色 ID"
// @Param        body  body      AssignMenusReq  true  "菜单 ID 列表"
// @Success      200   {object}  response.Body  "权限已保存"
// @Failure      400   {object}  response.Body  "超管角色无需分配"
// @Failure      403   {object}  response.Body  "无权给该角色授予自己不具备的权限"
// @Failure      404   {object}  response.Body  "角色不存在"
// @Router       /system/roles/{id}/menus [put]
func (h *Handler) AssignMenus(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req AssignMenusReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "分配角色权限")
	if err := h.svc.AssignMenus(id, req.MenuIDs, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "权限已保存")
}

// Export 导出角色 Excel
// @Summary      按筛选条件导出角色
// @Description  返回 xlsx 二进制流。
// @Tags         角色管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按角色名称 / 标识模糊搜索"
// @Success      200      {file}    binary  "xlsx 文件流"
// @Failure      400      {object}  response.Body  "无可导出数据"
// @Router       /system/roles/export [get]
func (h *Handler) Export(c *gin.Context) {
	c.Set("opAction", "导出角色")

	q := page.FromContext(c)
	items, err := h.svc.Export(q.Keyword)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if len(items) == 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "当前条件下没有可导出的数据"))
		return
	}
	data, err := h.svc.BuildExportFile(items)
	if err != nil {
		response.Fail(c, err)
		return
	}
	sheet.Download(c, "角色列表_"+textutil.Timestamp()+".xlsx", data)
}
