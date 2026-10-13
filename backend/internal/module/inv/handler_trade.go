package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// OrderList 单据列表（采购 / 销售共用，凭 bizType 区分）
// @Summary      单据列表
// @Tags         进销存-单据
// @Produce      json
// @Security     BearerAuth
// @Param        bizType  query  string  true   "purchase | sales"
// @Param        keyword  query  string  false  "单号 / 往来单位"
// @Param        status   query  int     false  "1 待处理 2 已审核 3 已取消"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.OrderRow}}
// @Router       /inv/orders [get]
func (h *Handler) OrderList(c *gin.Context) {
	bizType, err := bizTypeOf(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ensureBizPerm(c, bizType); err != nil {
		response.Fail(c, err)
		return
	}
	q := page.FromContext(c)
	status, err := orderStatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, total, err := h.svc.OrderList(bizType, q, status)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// OrderDetail 单据详情
// @Summary      单据详情
// @Tags         进销存-单据
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "单据 ID"
// @Success      200  {object}  response.Body{data=inv.OrderDetail}
// @Router       /inv/orders/{id} [get]
func (h *Handler) OrderDetail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.svc.OrderDetail(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ensureBizPerm(c, detail.BizType); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// OrderCreate 新增单据
// @Summary      新增单据
// @Tags         进销存-单据
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  OrderReq  true  "单据信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/orders [post]
func (h *Handler) OrderCreate(c *gin.Context) {
	var req OrderReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	// 写路径必须精确到动作：路由的 RequirePerm 是「采购或销售任一」，
	// 只校验业务线的话，「采购只读 + 销售新增」的账号也能建采购单。
	if err := ensureBizPermAction(c, req.BizType, "create"); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增单据")
	id, err := h.svc.OrderCreate(req, actorID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// OrderUpdate 修改单据
// @Summary      修改单据
// @Tags         进销存-单据
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int       true  "单据 ID"
// @Param        body  body  OrderReq  true  "单据信息"
// @Success      200   {object}  response.Body
// @Router       /inv/orders/{id} [put]
func (h *Handler) OrderUpdate(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req OrderReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	if err := ensureBizPermAction(c, req.BizType, "update"); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改单据")
	if err := h.svc.OrderUpdate(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// OrderDelete 删除单据
// @Summary      删除单据
// @Tags         进销存-单据
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "单据 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/orders/{id} [delete]
func (h *Handler) OrderDelete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.svc.OrderDetail(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ensureBizPermAction(c, detail.BizType, "delete"); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除单据")
	if err := h.svc.OrderDelete(id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// OrderChangeStatus 流转单据状态
// @Summary      审核单据（审核通过时自动生成出入库单并调整库存）
// @Tags         进销存-单据
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int        true  "单据 ID"
// @Param        body  body  StatusReq  true  "目标状态"
// @Success      200   {object}  response.Body
// @Router       /inv/orders/{id}/status [put]
func (h *Handler) OrderChangeStatus(c *gin.Context) {
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
	detail, err := h.svc.OrderDetail(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ensureBizPermAction(c, detail.BizType, "finish"); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "流转单据状态")
	if err := h.svc.OrderChangeStatus(id, req.Status, actorID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "操作成功")
}
