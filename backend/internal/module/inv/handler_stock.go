package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// StockOrderList 出入库单据列表
// @Summary      出入库单据列表
// @Tags         进销存-库存
// @Produce      json
// @Security     BearerAuth
// @Param        direction  query  string  false  "in 入库 / out 出库"
// @Param        keyword    query  string  false  "单号"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.StockOrderRow}}
// @Router       /inv/stock-orders [get]
func (h *Handler) StockOrderList(c *gin.Context) {
	q := page.FromContext(c)
	direction := c.Query("direction")
	if direction != "" && direction != model.InvStockIn && direction != model.InvStockOut {
		response.Fail(c, errcode.Newf(errcode.ErrBadRequest, "参数 direction 非法：%s", direction))
		return
	}
	list, total, err := h.svc.StockOrderList(q, direction, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// StockOrderCreate 新增出入库单据（即时过账）
// @Summary      新增出入库单据
// @Tags         进销存-库存
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  StockOrderReq  true  "出入库信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/stock-orders [post]
func (h *Handler) StockOrderCreate(c *gin.Context) {
	var req StockOrderReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增出入库单据")
	id, err := h.svc.StockOrderCreate(req, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// StockOrderVoid 作废出入库单据
// @Summary      作废出入库单据（回滚库存）
// @Tags         进销存-库存
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "单据 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/stock-orders/{id} [delete]
func (h *Handler) StockOrderVoid(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "作废出入库单据")
	if err := h.svc.StockOrderVoid(id, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "已作废")
}

// Inventory 库存查询
// @Summary      库存查询（按规格出账，含预警标记）
// @Tags         进销存-库存
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int     false  "页码"
// @Param        pageSize  query  int     false  "每页条数"
// @Param        keyword   query  string  false  "编码 / 名称 / 规格"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.InventoryRow}}
// @Router       /inv/inventory [get]
func (h *Handler) Inventory(c *gin.Context) {
	q := page.FromContext(c)
	list, total, err := h.svc.Inventory(q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// StockOrderDetail 出入库单打印详情
// @Summary      出入库单详情（打印用）
// @Tags         进销存-库存管理
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "出入库单 ID"
// @Success      200  {object}  response.Body{data=inv.StockOrderDetail}
// @Router       /inv/stock-orders/{id} [get]
func (h *Handler) StockOrderDetail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.svc.StockOrderDetail(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}
