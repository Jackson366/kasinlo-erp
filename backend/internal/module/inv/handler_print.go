package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// PrintTemplateList 打印模板列表
// @Summary      打印模板列表
// @Tags         进销存-打印模板
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query  string  false  "编码 / 名称 / 单据类型"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.PrintTemplateRow}}
// @Router       /inv/print-templates [get]
func (h *Handler) PrintTemplateList(c *gin.Context) {
	q := page.FromContext(c)
	q.PageSize = 100
	list, total, err := h.svc.PrintTemplateList(q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// PrintTemplateCreate 新增打印模板
// @Summary      新增打印模板
// @Tags         进销存-打印模板
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  PrintTemplateReq  true  "模板信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/print-templates [post]
func (h *Handler) PrintTemplateCreate(c *gin.Context) {
	var req PrintTemplateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增打印模板")
	id, err := h.svc.PrintTemplateCreate(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// PrintTemplateUpdate 修改打印模板
// @Summary      修改打印模板
// @Tags         进销存-打印模板
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int               true  "模板 ID"
// @Param        body  body  PrintTemplateReq  true  "模板信息"
// @Success      200   {object}  response.Body
// @Router       /inv/print-templates/{id} [put]
func (h *Handler) PrintTemplateUpdate(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req PrintTemplateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改打印模板")
	if err := h.svc.PrintTemplateUpdate(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// PrintTemplateDelete 删除打印模板
// @Summary      删除打印模板
// @Tags         进销存-打印模板
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "模板 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/print-templates/{id} [delete]
func (h *Handler) PrintTemplateDelete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除打印模板")
	if err := h.svc.PrintTemplateDelete(id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}
