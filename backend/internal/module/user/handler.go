package user

import (
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/authz"
	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
	"nova-erp/internal/pkg/sheet"
	"nova-erp/internal/pkg/textutil"
)

// Handler 用户接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建用户处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List 用户列表
// @Summary      分页查询用户
// @Description  结果会叠加当前登录用户的数据范围（全部 / 本部门 / 本部门及以下 / 仅本人）。
// @Tags         用户管理
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int     false  "页码，从 1 开始"      default(1)
// @Param        pageSize  query     int     false  "每页条数，上限 200"   default(10)
// @Param        keyword   query     string  false  "按用户名 / 昵称 / 手机号模糊搜索"
// @Param        deptId    query     int     false  "部门 ID（含子部门）"
// @Param        status    query     int     false  "状态：1 启用，0 禁用"  Enums(0, 1)
// @Param        roleId    query     int     false  "角色 ID"
// @Param        orderBy   query     string  false  "排序字段：createdAt / updatedAt / username / lastLoginAt"
// @Param        order     query     string  false  "排序方向：asc / desc"  Enums(asc, desc)
// @Success      200       {object}  response.Body{data=response.PageData{list=[]user.VO}}
// @Failure      401       {object}  response.Body  "未登录或令牌失效"
// @Failure      403       {object}  response.Body  "缺少 system:user:list 权限"
// @Router       /system/users [get]
func (h *Handler) List(c *gin.Context) {
	q := page.FromContext(c)
	var f ListQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, err := h.svc.List(q, f, actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, items, total, q.Page, q.PageSize)
}

// Options 用户下拉选项
// @Summary      用户下拉选项
// @Description  仅返回启用状态、且在调用者数据范围内的用户，最多 100 条。
// @Tags         用户管理
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按用户名 / 昵称模糊搜索"
// @Success      200      {object}  response.Body{data=[]user.Option}
// @Failure      403      {object}  response.Body  "缺少 system:user:list / system:role:list / system:dept:list 权限"
// @Router       /system/users/options [get]
func (h *Handler) Options(c *gin.Context) {
	items, err := h.svc.Options(c.Query("keyword"), actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Detail 用户详情
// @Summary      查询用户详情
// @Description  超出调用者数据范围时按「不存在」返回 404，不泄露该用户是否存在。
// @Tags         用户管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "用户 ID"
// @Success      200  {object}  response.Body{data=user.VO}
// @Failure      404  {object}  response.Body  "用户不存在"
// @Router       /system/users/{id} [get]
func (h *Handler) Detail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	vo, err := h.svc.Get(id, actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, vo)
}

// Create 新增用户
// @Summary      新增用户
// @Description  只能把用户建在自己数据范围内的部门。roleIds 会被校验：不允许授予超出调用者自身权限的角色（如超管）。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateReq  true  "新增用户参数"
// @Success      200   {object}  response.Body{data=response.IDData}
// @Failure      400   {object}  response.Body  "参数校验失败 / 部门超出数据范围"
// @Failure      403   {object}  response.Body  "无权授予所选角色"
// @Failure      409   {object}  response.Body  "用户名已存在"
// @Router       /system/users [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增用户")
	id, err := h.svc.Create(req, actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// Update 修改用户
// @Summary      修改用户
// @Description  roleIds 传 null 表示不调整角色；传数组则为覆盖式设置，并校验调用者有权授予这些角色。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int        true  "用户 ID"
// @Param        body  body      UpdateReq  true  "修改参数"
// @Success      200   {object}  response.Body  "修改成功"
// @Failure      400   {object}  response.Body  "参数校验失败"
// @Failure      403   {object}  response.Body  "无权授予所选角色 / 无权操作超管账号"
// @Failure      404   {object}  response.Body  "用户不存在"
// @Router       /system/users/{id} [put]
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
	c.Set("opAction", "修改用户")
	if err := h.svc.Update(id, req, actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// Delete 删除用户
// @Summary      删除用户
// @Description  软删除。内置管理员、当前登录用户、持有超管角色的用户不可删除。
// @Tags         用户管理
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "用户 ID"
// @Success      200  {object}  response.Body  "删除成功"
// @Failure      400  {object}  response.Body  "内置管理员或当前登录用户不允许删除"
// @Failure      404  {object}  response.Body  "用户不存在"
// @Router       /system/users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除用户")
	if err := h.svc.Delete(id, middleware.CurrentUserID(c), actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// UpdateStatus 启用/禁用用户
// @Summary      启用或禁用用户
// @Description  禁用会把 token_epoch 加一，使该用户已签发的令牌立即失效。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int        true  "用户 ID"
// @Param        body  body      StatusReq  true  "目标状态"
// @Success      200   {object}  response.Body  "状态已更新"
// @Failure      400   {object}  response.Body  "内置管理员或当前登录用户不允许禁用"
// @Failure      404   {object}  response.Body  "用户不存在"
// @Router       /system/users/{id}/status [put]
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := bind.PathID(c)
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
	if err := h.svc.UpdateStatus(id, req.Status, middleware.CurrentUserID(c), actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "状态已更新")
}

// ResetPassword 重置用户密码
// @Summary      重置用户密码
// @Description  重置后该用户所有已签发令牌立即失效，需重新登录。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int               true  "用户 ID"
// @Param        body  body      ResetPasswordReq  true  "新密码"
// @Success      200   {object}  response.Body  "密码已重置"
// @Failure      400   {object}  response.Body  "密码长度不能少于 6 位"
// @Failure      404   {object}  response.Body  "用户不存在"
// @Router       /system/users/{id}/password [put]
func (h *Handler) ResetPassword(c *gin.Context) {
	id, err := bind.PathID(c)
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
	if err := h.svc.ResetPassword(id, req.Password, actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "密码已重置")
}

// actor 返回当前请求的身份，供数据范围过滤使用。
// 本模块的路由都挂在 Auth 之后，这里拿不到身份只可能是配置错误；
// 服务层的 scopeQuery 对 nil 会退化成「不受限」，因此不需要在调用点判空。
func actor(c *gin.Context) *authz.Identity { return middleware.CurrentIdentity(c) }

// ---------------- Excel 导入导出 ----------------

// maxImportFileSize 上传文件大小上限（5MB 足以容纳上千行数据）
const maxImportFileSize = 5 << 20

// Export 导出用户 Excel
// @Summary      按筛选条件导出用户
// @Description  返回 xlsx 二进制流。导出的行集与列表完全一致（含数据范围约束）；单次上限见 MaxExportRows。
// @Tags         用户管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        ids       query     string  false  "选中导出的用户 ID，逗号分隔；提供时忽略其他筛选条件"
// @Param        keyword   query     string  false  "按用户名 / 昵称 / 手机号模糊搜索"
// @Param        deptId    query     int     false  "部门 ID（含子部门）"
// @Param        status    query     int     false  "状态：1 启用，0 禁用"
// @Param        roleId    query     int     false  "角色 ID"
// @Success      200       {file}    binary  "xlsx 文件流"
// @Failure      400       {object}  response.Body  "无可导出数据 / 超出单次导出上限"
// @Router       /system/users/export [get]
func (h *Handler) Export(c *gin.Context) {
	c.Set("opAction", "导出用户")

	q := page.FromContext(c)
	var f ExportQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.svc.Export(q.Keyword, f, actor(c))
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
	sheet.Download(c, "用户列表_"+textutil.Timestamp()+".xlsx", data)
}

// ImportTemplate 下载用户导入模板
// @Summary      下载用户导入模板
// @Description  返回 xlsx 二进制流。模板数据区为空，填写说明在第二个工作表中。
// @Tags         用户管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Success      200  {file}  binary  "xlsx 文件流"
// @Router       /system/users/import-template [get]
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
// @Summary      批量导入用户
// @Description  上传 xlsx（仅支持 .xlsx，≤5MB，≤1000 行）。dryRun=1 时只校验并逐行返回结果，不写库。
// @Description  导入同样受数据范围约束，且角色授予会被校验。
// @Tags         用户管理
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        dryRun  query     string  false  "为 1/true 时只校验不写库"  Enums(1, true)
// @Param        file    formData  file    true   "导入文件（.xlsx）"
// @Success      200     {object}  response.Body{data=user.ImportResult}
// @Failure      400     {object}  response.Body  "文件为空 / 格式不支持 / 表头不匹配 / 超出行数上限"
// @Failure      403     {object}  response.Body  "文件中的角色超出可授予范围（权限或数据范围）"
// @Failure      409     {object}  response.Body  "导入时出现唯一性冲突"
// @Router       /system/users/import [post]
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

	result, err := h.svc.Import(data, dryRun, actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}
