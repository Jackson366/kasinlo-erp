package inv

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/response"
)

// AttachmentUpload 上传附件（采购单 / 销售单 / 合同）
// @Summary      上传业务附件
// @Description  支持图片、PDF、Office 文档与压缩包，单文件 20 MB 以内。业务对象必须存在且在数据范围内。
// @Tags         进销存-附件
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        bizType  formData  string  true  "业务类型：purchase / sales / contract"
// @Param        bizId    formData  int     true  "业务对象 ID"
// @Param        file     formData  file    true  "附件文件"
// @Success      200      {object}  response.Body{data=AttachmentVO}
// @Failure      400      {object}  response.Body
// @Router       /inv/attachments [post]
func (h *Handler) AttachmentUpload(c *gin.Context) {
	// 与升级包上传同一套思路：在解析 multipart **之前**卡住请求体。
	// 否则超限的那部分会先落到临时文件，磁盘已经被写满了才发现。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAttachmentBytes+maxMultipartSlack)

	bizType := strings.TrimSpace(c.PostForm("bizType"))
	bizID, err := strconv.ParseInt(strings.TrimSpace(c.PostForm("bizId")), 10, 64)
	if err != nil || bizID <= 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "缺少或非法的业务对象 ID"))
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			response.Fail(c, errcode.Newf(errcode.ErrBadRequest, "附件超过 %d MB", maxAttachmentBytes>>20))
			return
		}
		response.Fail(c, errcode.Newf(errcode.ErrBadRequest, "读取上传文件失败：%v", err))
		return
	}

	c.Set("opAction", "上传附件")
	vo, err := h.svc.UploadAttachment(bizType, bizID, fh, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, vo)
}

// AttachmentList 列出某个业务对象的附件
// @Summary      附件列表
// @Tags         进销存-附件
// @Produce      json
// @Security     BearerAuth
// @Param        bizType  query  string  true  "业务类型"
// @Param        bizId    query  int     true  "业务对象 ID"
// @Success      200      {object}  response.Body{data=[]AttachmentVO}
// @Router       /inv/attachments [get]
func (h *Handler) AttachmentList(c *gin.Context) {
	bizType := strings.TrimSpace(c.Query("bizType"))
	bizID, err := bind.IntQuery(c, "bizId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, err := h.svc.ListAttachments(bizType, bizID, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// AttachmentCount 批量统计附件数量
// @Summary      附件数量（列表标记用）
// @Description  一次查完一批单据的附件数，避免列表逐行请求。
// @Tags         进销存-附件
// @Produce      json
// @Security     BearerAuth
// @Param        bizType  query  string  true   "业务类型"
// @Param        ids      query  string  true   "业务对象 ID，逗号分隔"
// @Success      200      {object}  response.Body{data=map[string]int}
// @Router       /inv/attachments/count [get]
func (h *Handler) AttachmentCount(c *gin.Context) {
	bizType := strings.TrimSpace(c.Query("bizType"))
	ids := parseAttachmentIDs(c.Query("ids"))
	out, err := h.svc.AttachmentCounts(bizType, ids)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, out)
}

// AttachmentFile 下载 / 预览附件
// @Summary      下载或预览附件
// @Description  默认 inline（图片、PDF 在浏览器里直接看），加 ?download=1 强制下载。
// @Tags         进销存-附件
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        id        path   int     true   "附件 ID"
// @Param        download  query  string  false  "1 = 下载（默认 inline 预览）"
// @Success      200       {file}  binary
// @Router       /inv/attachments/{id}/file [get]
func (h *Handler) AttachmentFile(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	att, abs, err := h.svc.AttachmentFile(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "查看附件")
	// nosniff：别让浏览器按内容猜类型 —— 一个被改名成 .txt 的 html
	// 在 inline 渲染下就是一次 XSS，而扩展名是我们校验过的。
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", att.Mime)
	if c.Query("download") == "1" {
		c.FileAttachment(abs, att.FileName)
		return
	}
	// inline 预览：文件名走 header，中文需要 RFC 5987 编码，
	// 否则浏览器拿到的可能是乱码（附件的原始名常常是中文）。
	c.Header("Content-Disposition", "inline; filename*=UTF-8''"+escapeFileName(att.FileName))
	c.File(abs)
}

// AttachmentDelete 删除附件
// @Summary      删除附件
// @Description  软删记录（文件保留在附件目录，可恢复）。
// @Tags         进销存-附件
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  int  true  "附件 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/attachments/{id} [delete]
func (h *Handler) AttachmentDelete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除附件")
	if err := h.svc.DeleteAttachment(id, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// isBodyTooLarge 判断是否是「请求体超限」导致的读错误。
func isBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	return strings.Contains(err.Error(), "request body too large")
}

// escapeFileName 按 RFC 5987 编码文件名（用于 Content-Disposition）。
func escapeFileName(name string) string {
	var b strings.Builder
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString(fmt.Sprintf("%%%02X", c))
	}
	return b.String()
}

// parseAttachmentIDs 解析逗号分隔的 ID 串，非法片段直接丢弃。
//
// 不报错是有意的：这是列表页的装饰性请求（算附件角标），
// 一个脏 id 不该让整页列表的角标全都不出来。
func parseAttachmentIDs(raw string) []int64 {
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if v, err := strconv.ParseInt(p, 10, 64); err == nil && v > 0 {
			out = append(out, v)
		}
	}
	return out
}
