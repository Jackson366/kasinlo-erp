package inv

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

type platformProductRow struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Barcode      string `json:"barcode"`
	CategoryID   int64  `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	Status       int16  `json:"status"`
}

type platformProductUnit struct {
	ID     int64   `json:"id"`
	Unit   string  `json:"unit"`
	Rate   float64 `json:"rate"`
	IsBase bool    `json:"isBase"`
	Status int16   `json:"status"`
}

type platformProductSpec struct {
	ID     int64                 `json:"id"`
	Spec   string                `json:"spec"`
	Status int16                 `json:"status"`
	Units  []platformProductUnit `json:"units"`
}

type platformProductDetail struct {
	platformProductRow
	Specs []platformProductSpec `json:"specs"`
}

func toPlatformProduct(row ProductRow) platformProductRow {
	return platformProductRow{
		ID: row.ID, Code: row.Code, Name: row.Name, Barcode: row.Barcode,
		CategoryID: row.CategoryID, CategoryName: row.CategoryName, Status: row.Status,
	}
}

// platformProducts 查询商品档案。
//
// @Summary 查询商品档案
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "编码、名称、条码或规格"
// @Param status query int false "商品状态"
// @Param categoryId query int false "分类 ID"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=response.PageData{list=[]inv.platformProductRow}}
// @Router /internal/open-platform/products [get]
func (h *Handler) platformProducts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	q := page.FromContext(c)
	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	categoryID, err := bind.IntQuery(c, "categoryId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, total, err := h.svc.ProductList(q, status, categoryID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformProductRow, 0, len(list))
	for _, row := range list {
		rows = append(rows, toPlatformProduct(row))
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// platformProduct 查询商品规格与计量单位。
//
// @Summary 查询商品详情
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "商品 ID"
// @Success 200 {object} response.Body{data=inv.platformProductDetail}
// @Router /internal/open-platform/products/{id} [get]
func (h *Handler) platformProduct(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	row, err := h.svc.ProductDetail(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail := platformProductDetail{
		platformProductRow: toPlatformProduct(*row),
		Specs:              make([]platformProductSpec, 0, len(row.Specs)),
	}
	for _, spec := range row.Specs {
		units := make([]platformProductUnit, 0, len(spec.Units))
		for _, unit := range spec.Units {
			units = append(units, platformProductUnit{
				ID: unit.ID, Unit: unit.Unit, Rate: unit.Rate, IsBase: unit.IsBase, Status: unit.Status,
			})
		}
		detail.Specs = append(detail.Specs, platformProductSpec{ID: spec.ID, Spec: spec.Spec, Status: spec.Status, Units: units})
	}
	response.OK(c, detail)
}

type platformOrderRow struct {
	ID            int64      `json:"id"`
	OrderNo       string     `json:"orderNo"`
	BizType       string     `json:"bizType"`
	PartyName     string     `json:"partyName"`
	OrderDate     time.Time  `json:"orderDate"`
	DueDate       *time.Time `json:"dueDate"`
	Status        int16      `json:"status"`
	TotalAmount   float64    `json:"totalAmount"`
	SettledAmount float64    `json:"settledAmount"`
	SettleStatus  string     `json:"settleStatus"`
	ItemCount     int64      `json:"itemCount"`
}

type platformOrderDetail struct {
	platformOrderRow
	Items []OrderItemVO `json:"items"`
}

func toPlatformOrder(row OrderRow) platformOrderRow {
	return platformOrderRow{
		ID: row.ID, OrderNo: row.OrderNo, BizType: row.BizType, PartyName: row.PartyName,
		OrderDate: row.OrderDate, DueDate: row.DueDate, Status: row.Status, TotalAmount: row.TotalAmount,
		SettledAmount: row.SettledAmount, SettleStatus: row.SettleStatus, ItemCount: row.ItemCount,
	}
}

// platformOrders 查询采购或销售单据。
//
// @Summary 查询采购或销售单据
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "单号、往来单位或业务员"
// @Param status query int false "单据状态"
// @Param settleStatus query string false "结算状态"
// @Param startDate query string false "起始单据日期"
// @Param endDate query string false "结束单据日期"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=response.PageData{list=[]inv.platformOrderRow}}
// @Router /internal/open-platform/purchase-orders [get]
// @Router /internal/open-platform/sales-orders [get]
func (h *Handler) platformOrders(bizType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		q := page.FromContext(c)
		filter, err := readOrderFilter(c, bizType)
		if err != nil {
			response.Fail(c, err)
			return
		}
		list, total, err := h.svc.OrderList(filter, q, middleware.CurrentIdentity(c))
		if err != nil {
			response.Fail(c, err)
			return
		}
		rows := make([]platformOrderRow, 0, len(list))
		for _, row := range list {
			rows = append(rows, toPlatformOrder(row))
		}
		response.OKPage(c, rows, total, q.Page, q.PageSize)
	}
}

// platformOrder 查询指定业务线的单据与明细。
//
// @Summary 查询采购或销售单据详情
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "单据 ID"
// @Success 200 {object} response.Body{data=inv.platformOrderDetail}
// @Router /internal/open-platform/purchase-orders/{id} [get]
// @Router /internal/open-platform/sales-orders/{id} [get]
func (h *Handler) platformOrder(bizType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		id, err := bind.PathID(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		order, err := h.svc.orderForDetail(id, middleware.CurrentIdentity(c), bizType)
		if err != nil {
			response.Fail(c, err)
			return
		}
		items, err := h.svc.orderItems(id)
		if err != nil {
			response.Fail(c, err)
			return
		}
		row := toOrderRow(order, int64(len(items)), "")
		response.OK(c, platformOrderDetail{platformOrderRow: toPlatformOrder(row), Items: items})
	}
}
