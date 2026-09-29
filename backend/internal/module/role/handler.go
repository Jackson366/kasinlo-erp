package role

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/page"
	"nova-admin/internal/pkg/response"
	"nova-admin/internal/pkg/sheet"
)

// Handler 角色接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建角色处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 角色列表
// @Summary 分页查询角色
// @Tags    角色管理
// @Router  /system/roles [get]
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
// @Summary 角色下拉选项
// @Tags    角色管理
// @Router  /system/roles/options [get]
func (h *Handler) Options(c *gin.Context) {
	items, err := h.svc.Options()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Detail 角色详情
// @Summary 查询角色详情
// @Tags    角色管理
// @Router  /system/roles/{id} [get]
func (h *Handler) Detail(c *gin.Context) {
	id, err := pathID(c)
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
// @Summary 新增角色
// @Tags    角色管理
// @Router  /system/roles [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增角色")
	id, err := h.svc.Create(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}

// Update 修改角色
// @Summary 修改角色
// @Tags    角色管理
// @Router  /system/roles/{id} [put]
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
	c.Set("opAction", "修改角色")
	if err := h.svc.Update(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// Delete 删除角色
// @Summary 删除角色
// @Tags    角色管理
// @Router  /system/roles/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除角色")
	if err := h.svc.Delete(id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// AssignMenus 分配菜单权限
// @Summary 为角色分配菜单权限
// @Tags    角色管理
// @Router  /system/roles/{id}/menus [put]
func (h *Handler) AssignMenus(c *gin.Context) {
	id, err := pathID(c)
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
	if err := h.svc.AssignMenus(id, req.MenuIDs); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "权限已保存")
}

func pathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errcode.New(errcode.ErrBadRequest, "路径参数 id 非法")
	}
	return id, nil
}

// Export 导出角色 Excel
// @Summary 按筛选条件导出角色
// @Tags    角色管理
// @Router  /system/roles/export [get]
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
	sheet.Download(c, "角色列表_"+time.Now().Format("20060102_150405")+".xlsx", data)
}
