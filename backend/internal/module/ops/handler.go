package ops

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/response"
)

// Handler 运维接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建运维处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Backups 备份列表
// @Summary      备份列表
// @Tags         系统运维
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=[]BackupItem}
// @Failure      403  {object}  response.Body  "缺少 ops:list 权限"
// @Router       /ops/backups [get]
func (h *Handler) Backups(c *gin.Context) {
	items, err := h.svc.ListBackups()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// RunBackup 执行一次备份
// @Summary      执行备份
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      RunBackupReq  false  "备份参数"
// @Success      200   {object}  response.Body{data=BackupItem}
// @Failure      403   {object}  response.Body  "缺少 ops:backup 权限"
// @Router       /ops/backups [post]
func (h *Handler) RunBackup(c *gin.Context) {
	var req RunBackupReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	item, err := h.svc.RunBackup(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// DeleteBackup 删除一个备份
// @Summary      删除备份
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      DeleteBackupReq  true  "备份文件名"
// @Success      200   {object}  response.Body
// @Failure      403   {object}  response.Body  "缺少 ops:backup 权限"
// @Router       /ops/backups [delete]
func (h *Handler) DeleteBackup(c *gin.Context) {
	var req DeleteBackupReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	if err := h.svc.DeleteBackup(req.File); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// DownloadBackup 下载备份文件
// @Summary      下载备份
// @Tags         系统运维
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        file  query     string  true  "备份文件名"
// @Success      200   {file}    file
// @Failure      403   {object}  response.Body  "缺少 ops:backup 权限"
// @Router       /ops/backups/download [get]
func (h *Handler) DownloadBackup(c *gin.Context) {
	path, err := h.svc.BackupPath(c.Query("file"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}

// Restore 恢复到新库（恢复演练）
// @Summary      恢复演练
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      RestoreReq  true  "备份文件名与目标库"
// @Success      200   {object}  response.Body
// @Failure      403   {object}  response.Body  "缺少 ops:restore 权限"
// @Router       /ops/restore [post]
func (h *Handler) Restore(c *gin.Context) {
	var req RestoreReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	res, err := h.svc.Restore(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// CleanupPreview 清理预演
// @Summary      数据清理预演
// @Tags         系统运维
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=CleanupView}
// @Failure      403  {object}  response.Body  "缺少 ops:list 权限"
// @Router       /ops/cleanup/preview [get]
func (h *Handler) CleanupPreview(c *gin.Context) {
	view, err := h.svc.CleanupPreview()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// CleanupApply 真删（需确认口令）
// @Summary      执行数据清理
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CleanupApplyReq  true  "确认口令与保留天数"
// @Success      200  {object}  response.Body{data=CleanupView}
// @Failure      403  {object}  response.Body  "缺少 ops:cleanup 权限"
// @Router       /ops/cleanup/apply [post]
func (h *Handler) CleanupApply(c *gin.Context) {
	var req CleanupApplyReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	view, err := h.svc.CleanupApply(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// Check 系统体检
// @Summary      系统体检
// @Tags         系统运维
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body
// @Failure      403  {object}  response.Body  "缺少 ops:list 权限"
// @Router       /ops/check [get]
func (h *Handler) Check(c *gin.Context) {
	rep, warns, err := h.svc.Check()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"report": rep, "warnings": warns})
}

// UpgradeStatus 升级页初始数据
// @Summary      升级状态与预检
// @Tags         系统运维
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=UpgradeStatus}
// @Failure      403  {object}  response.Body  "缺少 ops:list 权限"
// @Router       /ops/upgrade [get]
func (h *Handler) UpgradeStatus(c *gin.Context) {
	st, err := h.svc.UpgradeStatus()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, st)
}

// UpgradeRecords 升级记录
// @Summary      升级记录
// @Tags         系统运维
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=[]UpgradeRecord}
// @Failure      403  {object}  response.Body  "缺少 ops:list 权限"
// @Router       /ops/upgrade/records [get]
func (h *Handler) UpgradeRecords(c *gin.Context) {
	records, err := h.svc.UpgradeRecords()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, records)
}

// UploadPackage 上传升级包
// @Summary      上传升级包
// @Tags         系统运维
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file  formData  file  true  "升级包 zip"
// @Success      200   {object}  response.Body
// @Failure      403   {object}  response.Body  "缺少 ops:upgrade 权限"
// @Router       /ops/upgrade/package [post]
func (h *Handler) UploadPackage(c *gin.Context) {
	// 第一道防线：在 gin 解析 multipart **之前**把请求体卡住。
	// c.FormFile 内部会 ParseMultipartForm，超过 32 MB 的部分会先落到临时文件，
	// 也就是说「事后用 LimitReader 统计长度」只能发现超限，
	// 磁盘和内存已经被吃掉一次了 —— 一个声明很小、实际一直发的客户端
	// 足以把临时目录写满。MaxBytesReader 让超限在读的阶段就断掉。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPackageBytes+maxMultipartSlack)
	fh, err := c.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			response.Fail(c, fmt.Errorf("升级包超过 %d MB", maxPackageBytes>>20))
			return
		}
		response.Fail(c, fmt.Errorf("读取上传文件失败：%w", err))
		return
	}
	f, err := fh.Open()
	if err != nil {
		if isBodyTooLarge(err) {
			response.Fail(c, fmt.Errorf("升级包超过 %d MB", maxPackageBytes>>20))
			return
		}
		response.Fail(c, fmt.Errorf("打开上传文件失败：%w", err))
		return
	}
	defer func() { _ = f.Close() }()
	// 第二道防线：请求体里的其它字段（boundary 之外没有，但自定义客户端可能塞）
	// 不被 MaxBytesReader 之外的路径覆盖时，这里再兜一次底。
	data, err := io.ReadAll(io.LimitReader(f, maxPackageBytes+1))
	if err != nil {
		if isBodyTooLarge(err) {
			response.Fail(c, fmt.Errorf("升级包超过 %d MB", maxPackageBytes>>20))
			return
		}
		response.Fail(c, fmt.Errorf("读取上传文件失败：%w", err))
		return
	}
	if int64(len(data)) > maxPackageBytes {
		response.Fail(c, fmt.Errorf("升级包超过 %d MB", maxPackageBytes>>20))
		return
	}
	res, err := h.svc.UploadPackage(fh.Filename, data)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// UpgradeBackup 升级前强制留档备份
// @Summary      升级前备份
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      UpgradeBackupReq  true  "升级计划标识"
// @Success      200   {object}  response.Body{data=BackupItem}
// @Failure      403   {object}  response.Body  "缺少 ops:upgrade 权限"
// @Router       /ops/upgrade/backup [post]
func (h *Handler) UpgradeBackup(c *gin.Context) {
	var req UpgradeBackupReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	item, err := h.svc.UpgradeBackup(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// ApplyUpgrade 执行升级
// @Summary      执行升级
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      ApplyUpgradeReq  true  "计划标识与确认口令"
// @Success      200   {object}  response.Body{data=UpgradeRecord}
// @Failure      403   {object}  response.Body  "缺少 ops:upgrade 权限"
// @Router       /ops/upgrade/apply [post]
func (h *Handler) ApplyUpgrade(c *gin.Context) {
	var req ApplyUpgradeReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	rec, err := h.svc.ApplyUpgrade(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rec)
}

// ── 小工具 ──────────────────────────────────────────────

// maxPackageBytes 升级包大小上限（512 MB），与 service 里的限制保持一致。
const maxPackageBytes = 512 << 20

// maxMultipartSlack 是给 multipart 边界、其它表单字段留的余量。
// MaxBytesReader 卡的是整个请求体，不是文件本身，不留余量会让
// 「刚好 512 MB 的包」因为多了几行 boundary 而被拒。
const maxMultipartSlack = 4 << 20

// isBodyTooLarge 判断错误是否来自请求体超限。
//
// 只看 *http.MaxBytesError 而不做字符串匹配：超限的判定权交给标准库，
// 免得把「磁盘满」「连接中断」这些同样带 too large 字样的错误误判成超限。
func isBodyTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// BizScopes 清理范围与行数预览
// @Summary      业务清理范围预览
// @Tags         系统运维
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=[]BizScopeView}
// @Failure      403  {object}  response.Body  "缺少 ops:list 权限"
// @Router       /ops/cleanup/scopes [get]
func (h *Handler) BizScopes(c *gin.Context) {
	scopes, err := h.svc.BizScopes()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, scopes)
}

// BusinessCleanup 执行业务数据清理
// @Summary      执行业务数据清理
// @Tags         系统运维
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      BizCleanupReq  true  "清理范围、确认口令与库存选项"
// @Success      200   {object}  response.Body{data=ops.BizCleanupReport}
// @Failure      403   {object}  response.Body  "缺少 ops:cleanup 权限"
// @Router       /ops/cleanup/business [post]
func (h *Handler) BusinessCleanup(c *gin.Context) {
	var req BizCleanupReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	rep, err := h.svc.BusinessCleanup(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rep)
}
