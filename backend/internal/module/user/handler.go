package user

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/middleware"
	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/page"
	"nova-admin/internal/pkg/response"
	"nova-admin/internal/pkg/sheet"
)

// Handler 用户接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建用户处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 用户列表
// @Summary 分页查询用户
// @Tags    用户管理
// @Router  /system/users [get]
func (h *Handler) List(c *gin.Context) {
	q := page.FromContext(c)
	var f ListQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, err := h.svc.List(q, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, items, total, q.Page, q.PageSize)
}

// Options 用户下拉选项
// @Summary 用户下拉选项
// @Tags    用户管理
// @Router  /system/users/options [get]
func (h *Handler) Options(c *gin.Context) {
	items, err := h.svc.Options(c.Query("keyword"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Detail 用户详情
// @Summary 查询用户详情
// @Tags    用户管理
// @Router  /system/users/{id} [get]
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

// Create 新增用户
// @Summary 新增用户
// @Tags    用户管理
// @Router  /system/users [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增用户")
	id, err := h.svc.Create(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}

// Update 修改用户
// @Summary 修改用户
// @Tags    用户管理
// @Router  /system/users/{id} [put]
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
	c.Set("opAction", "修改用户")
	if err := h.svc.Update(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// Delete 删除用户
// @Summary 删除用户
// @Tags    用户管理
// @Router  /system/users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除用户")
	if err := h.svc.Delete(id, middleware.CurrentUserID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// UpdateStatus 启用/禁用用户
// @Summary 启用或禁用用户
// @Tags    用户管理
// @Router  /system/users/{id}/status [put]
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req StatusReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改用户状态")
	if err := h.svc.UpdateStatus(id, req.Status, middleware.CurrentUserID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "状态已更新")
}

// ResetPassword 重置用户密码
// @Summary 重置用户密码
// @Tags    用户管理
// @Router  /system/users/{id}/password [put]
func (h *Handler) ResetPassword(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req ResetPasswordReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "重置用户密码")
	if err := h.svc.ResetPassword(id, req.Password); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "密码已重置")
}

func pathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errcode.New(errcode.ErrBadRequest, "路径参数 id 非法")
	}
	return id, nil
}

// ---------------- Excel 导入导出 ----------------

// maxImportFileSize 上传文件大小上限（5MB 足以容纳上千行数据）
const maxImportFileSize = 5 << 20

// Export 导出用户 Excel
// @Summary 按筛选条件导出用户
// @Tags    用户管理
// @Router  /system/users/export [get]
func (h *Handler) Export(c *gin.Context) {
	c.Set("opAction", "导出用户")

	q := page.FromContext(c)
	var f ExportQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.svc.Export(q.Keyword, f)
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
	sheet.Download(c, "用户列表_"+timestamp()+".xlsx", data)
}

// ImportTemplate 下载用户导入模板
// @Summary 下载用户导入模板
// @Tags    用户管理
// @Router  /system/users/import-template [get]
func (h *Handler) ImportTemplate(c *gin.Context) {
	c.Set("opAction", "下载用户导入模板")

	data, err := buildImportTemplate()
	if err != nil {
		response.Fail(c, err)
		return
	}
	sheet.Download(c, "用户导入模板.xlsx", data)
}

// Import 批量导入用户
// @Summary 批量导入用户（dryRun=1 时仅校验不写库）
// @Tags    用户管理
// @Router  /system/users/import [post]
func (h *Handler) Import(c *gin.Context) {
	dryRun := c.Query("dryRun") == "1" || c.Query("dryRun") == "true"
	// 预校验与真正入库都显式命名：若只给后者命名，
	// 预校验会落到「POST → 新增」的兜底动作名上，审计记录读起来像真的建了用户。
	if dryRun {
		c.Set("opAction", "校验导入文件")
	} else {
		c.Set("opAction", "导入用户")
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "请选择要上传的 Excel 文件"))
		return
	}
	if file.Size <= 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "上传的文件为空"))
		return
	}
	if file.Size > maxImportFileSize {
		response.Fail(c, errcode.Newf(errcode.ErrBadRequest,
			"文件大小不能超过 %d MB", maxImportFileSize>>20))
		return
	}
	// excelize 只支持 Office 2007+ 的 xlsx，遇到旧的 .xls 要给出明确提示
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".xlsx") {
		response.Fail(c, errcode.New(errcode.ErrBadRequest,
			"仅支持 .xlsx 格式，请用 Excel 另存为「Excel 工作簿(*.xlsx)」后重试"))
		return
	}

	src, err := file.Open()
	if err != nil {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "读取上传文件失败"))
		return
	}
	defer func() { _ = src.Close() }()

	// 多读 1 字节用于判断是否超限，避免无界读入
	data, err := io.ReadAll(io.LimitReader(src, maxImportFileSize+1))
	if err != nil {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "读取上传文件失败"))
		return
	}
	if len(data) > maxImportFileSize {
		response.Fail(c, errcode.Newf(errcode.ErrBadRequest,
			"文件大小不能超过 %d MB", maxImportFileSize>>20))
		return
	}

	result, err := h.svc.Import(data, dryRun)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// timestamp 生成导出文件名用的时间戳。
func timestamp() string { return time.Now().Format("20060102_150405") }
