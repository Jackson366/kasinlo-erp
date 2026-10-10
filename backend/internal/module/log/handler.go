package log

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/pkg/bind"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/page"
	"nova-admin/internal/pkg/response"
	"nova-admin/internal/pkg/sheet"
)

// ClearReq 清空日志请求，before 为空表示清空全部。
type ClearReq struct {
	Before string `json:"before" label:"清空该时间之前的数据"`
}

// Handler 日志接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建日志处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Operations 操作日志列表
// @Summary      分页查询操作日志
// @Tags         日志管理
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int     false  "页码"     default(1)
// @Param        pageSize  query     int     false  "每页条数"  default(10)
// @Param        username  query     string  false  "操作人用户名"
// @Param        module    query     string  false  "业务模块"
// @Param        method    query     string  false  "HTTP 方法"
// @Param        status    query     int     false  "HTTP 状态码"
// @Param        startAt   query     string  false  "开始时间，支持 RFC3339 或 2006-01-02 15:04:05"
// @Param        endAt     query     string  false  "结束时间，支持 RFC3339 或 2006-01-02 15:04:05"
// @Param        orderBy   query     string  false  "排序字段：createdAt / latencyMs / status"
// @Param        order     query     string  false  "排序方向：asc / desc"  Enums(asc, desc)
// @Success      200       {object}  response.Body{data=response.PageData{list=[]model.OperationLog}}
// @Failure      403       {object}  response.Body  "缺少 system:log:list 权限"
// @Router       /system/logs/operations [get]
func (h *Handler) Operations(c *gin.Context) {
	q := page.FromContext(c)
	var f OperationQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	rows, total, err := h.svc.Operations(q, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// Logins 登录日志列表
// @Summary      分页查询登录日志
// @Tags         日志管理
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int     false  "页码"     default(1)
// @Param        pageSize  query     int     false  "每页条数"  default(10)
// @Param        username  query     string  false  "用户名"
// @Param        success   query     bool    false  "是否登录成功"
// @Param        startAt   query     string  false  "开始时间"
// @Param        endAt     query     string  false  "结束时间"
// @Param        order     query     string  false  "排序方向：asc / desc"  Enums(asc, desc)
// @Success      200       {object}  response.Body{data=response.PageData{list=[]model.LoginLog}}
// @Failure      403       {object}  response.Body  "缺少 system:log:list 权限"
// @Router       /system/logs/logins [get]
func (h *Handler) Logins(c *gin.Context) {
	q := page.FromContext(c)
	var f LoginQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	rows, total, err := h.svc.Logins(q, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// Modules 模块下拉
// @Summary      操作日志模块列表
// @Description  返回操作日志中出现过的业务模块名，用于筛选下拉。
// @Tags         日志管理
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=[]string}
// @Failure      403  {object}  response.Body  "缺少 system:log:list 权限"
// @Router       /system/logs/modules [get]
func (h *Handler) Modules(c *gin.Context) {
	items, err := h.svc.Modules()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// ClearOperations 清空操作日志
// @Summary      清空操作日志
// @Description  请求体可省略。before 为空表示清空全部，否则只删除该时间之前的记录。
// @Tags         日志管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      ClearReq  false  "清空条件"
// @Success      200   {object}  response.Body{data=response.DeletedData}
// @Failure      403   {object}  response.Body  "缺少 system:log:delete 权限"
// @Router       /system/logs/operations [delete]
func (h *Handler) ClearOperations(c *gin.Context) {
	var req ClearReq
	if err := bind.JSON(c, &req); err != nil {
		// 允许不带请求体，视为清空全部
		req = ClearReq{}
	}
	c.Set("opAction", "清空操作日志")
	n, err := h.svc.ClearOperations(parseTime(req.Before))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.DeletedData{Deleted: n})
}

// ClearLogins 清空登录日志
// @Summary      清空登录日志
// @Description  请求体可省略。before 为空表示清空全部，否则只删除该时间之前的记录。
// @Tags         日志管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      ClearReq  false  "清空条件"
// @Success      200   {object}  response.Body{data=response.DeletedData}
// @Failure      403   {object}  response.Body  "缺少 system:log:delete 权限"
// @Router       /system/logs/logins [delete]
func (h *Handler) ClearLogins(c *gin.Context) {
	var req ClearReq
	if err := bind.JSON(c, &req); err != nil {
		req = ClearReq{}
	}
	c.Set("opAction", "清空登录日志")
	n, err := h.svc.ClearLogins(parseTime(req.Before))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.DeletedData{Deleted: n})
}

// ExportOperations 导出操作日志
// @Summary      按筛选条件导出操作日志
// @Description  返回 xlsx 二进制流，筛选参数同列表接口。
// @Tags         日志管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        username  query     string  false  "操作人用户名"
// @Param        module    query     string  false  "业务模块"
// @Param        method    query     string  false  "HTTP 方法"
// @Param        status    query     int     false  "HTTP 状态码"
// @Param        startAt   query     string  false  "开始时间"
// @Param        endAt     query     string  false  "结束时间"
// @Success      200       {file}    binary  "xlsx 文件流"
// @Failure      400       {object}  response.Body  "无可导出数据"
// @Router       /system/logs/operations/export [get]
func (h *Handler) ExportOperations(c *gin.Context) {
	c.Set("opAction", "导出操作日志")

	q := page.FromContext(c)
	var f OperationQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	rows, err := h.svc.ExportOperations(q, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if len(rows) == 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "当前条件下没有可导出的数据"))
		return
	}
	data, err := h.svc.BuildOperationsFile(rows)
	if err != nil {
		response.Fail(c, err)
		return
	}
	sheet.Download(c, "操作日志_"+time.Now().Format("20060102_150405")+".xlsx", data)
}

// ExportLogins 导出登录日志
// @Summary      按筛选条件导出登录日志
// @Description  返回 xlsx 二进制流，筛选参数同列表接口。
// @Tags         日志管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        username  query     string  false  "用户名"
// @Param        success   query     bool    false  "是否登录成功"
// @Param        startAt   query     string  false  "开始时间"
// @Param        endAt     query     string  false  "结束时间"
// @Success      200       {file}    binary  "xlsx 文件流"
// @Failure      400       {object}  response.Body  "无可导出数据"
// @Router       /system/logs/logins/export [get]
func (h *Handler) ExportLogins(c *gin.Context) {
	c.Set("opAction", "导出登录日志")

	q := page.FromContext(c)
	var f LoginQuery
	if err := bind.Query(c, &f); err != nil {
		response.Fail(c, err)
		return
	}
	rows, err := h.svc.ExportLogins(q, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if len(rows) == 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "当前条件下没有可导出的数据"))
		return
	}
	data, err := h.svc.BuildLoginsFile(rows)
	if err != nil {
		response.Fail(c, err)
		return
	}
	sheet.Download(c, "登录日志_"+time.Now().Format("20060102_150405")+".xlsx", data)
}
