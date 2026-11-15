package inv

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
}

type platformStockRow struct {
	ID         int64     `json:"id"`
	OrderNo    string    `json:"orderNo"`
	Direction  string    `json:"direction"`
	OrderDate  time.Time `json:"orderDate"`
	Status     int16     `json:"status"`
	IsReversal bool      `json:"isReversal"`
	SourceType string    `json:"sourceType,omitempty"`
	SourceID   int64     `json:"sourceId,omitempty"`
}

type platformStockDetail struct {
	platformStockRow
	Items []StockPrintItem `json:"items"`
}

// platformStockOrders 查询出入库单据。
//
// @Summary 查询出入库单据
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param direction query string false "in 或 out"
// @Param sourceType query string false "purchase 或 sales，与 sourceId 搭配"
// @Param sourceId query int false "来源单据 ID"
// @Param includeVoided query bool false "包含作废单据"
// @Param keyword query string false "出入库单号"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=response.PageData{list=[]inv.platformStockRow}}
// @Router /internal/open-platform/stock-orders [get]
func (h *Handler) platformStockOrders(c *gin.Context) {
	noStore(c)
	q := page.FromContext(c)
	direction, sourceType := c.Query("direction"), c.Query("sourceType")
	if direction != "" && direction != model.InvStockIn && direction != model.InvStockOut {
		response.FailWith(c, errcode.ErrBadRequest, "参数 direction 非法")
		return
	}
	includeVoided, err := stockHistoryQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	sourceID, err := bind.IntQuery(c, "sourceId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	if sourceType != "" || c.Query("sourceId") != "" {
		if sourceID <= 0 || sourceType != model.InvBizPurchase && sourceType != model.InvBizSales {
			response.FailWith(c, errcode.ErrBadRequest, "请同时提供 purchase 或 sales 与有效的 sourceId")
			return
		}
		if err := ensureBizPermAction(c, sourceType, "list"); err != nil {
			response.Fail(c, err)
			return
		}
	}
	list, total, err := h.svc.stockOrderList(q, direction, includeVoided, middleware.CurrentIdentity(c), sourceType, sourceID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformStockRow, 0, len(list))
	for _, row := range list {
		rows = append(rows, platformStockRow{
			ID: row.ID, OrderNo: row.OrderNo, Direction: row.Direction, OrderDate: row.OrderDate,
			Status: row.Status, IsReversal: row.IsReversal, SourceType: sourceType, SourceID: sourceID,
		})
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// platformStockOrder 查询出入库明细。
//
// @Summary 查询出入库明细
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "出入库单 ID"
// @Success 200 {object} response.Body{data=inv.platformStockDetail}
// @Router /internal/open-platform/stock-orders/{id} [get]
func (h *Handler) platformStockOrder(c *gin.Context) {
	noStore(c)
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.svc.stockOrderDetail(id, middleware.CurrentIdentity(c), true)
	if err != nil {
		response.Fail(c, err)
		return
	}
	row := platformStockDetail{
		platformStockRow: platformStockRow{
			ID: detail.ID, OrderNo: detail.OrderNo, Direction: detail.Direction,
			OrderDate: detail.OrderDate, Status: detail.Status, IsReversal: detail.IsReversal,
		},
		Items: detail.Items,
	}
	response.OK(c, row)
}

type platformFinancePage struct {
	List     []FinanceRow   `json:"list"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Summary  FinanceSummary `json:"summary"`
}

// platformFinanceList 查询应收或应付及同口径汇总。
//
// @Summary 查询应收或应付
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param settle query string false "unpaid、partial、paid 或 overdue"
// @Param source query string false "order 或 plan"
// @Param keyword query string false "单号、往来单位、合同号或期次名"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=inv.platformFinancePage}
// @Router /internal/open-platform/receivables [get]
// @Router /internal/open-platform/payables [get]
func (h *Handler) platformFinanceList(receivable bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		noStore(c)
		switch c.Query("settle") {
		case "", model.InvSettleUnpaid, model.InvSettlePartial, model.InvSettlePaid, "overdue":
		default:
			response.FailWith(c, errcode.ErrBadRequest, "参数 settle 非法")
			return
		}
		switch c.Query("source") {
		case "", "order", "plan":
		default:
			response.FailWith(c, errcode.ErrBadRequest, "参数 source 非法")
			return
		}
		h.financeList(c, receivable)
	}
}

type platformSettlementRow struct {
	ID        int64     `json:"id"`
	Amount    float64   `json:"amount"`
	Kind      string    `json:"kind"`
	Method    string    `json:"method"`
	SettledAt time.Time `json:"settledAt"`
	AdjustsID *int64    `json:"adjustsId"`
}

type platformSettlements struct {
	OrderID     int64                   `json:"orderId"`
	Settled     float64                 `json:"settled"`
	Outstanding float64                 `json:"outstanding"`
	Rows        []platformSettlementRow `json:"rows"`
}

// platformOrderSettlements 查询单据收付款流水。
//
// @Summary 查询单据收付款流水
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "采购或销售单据 ID"
// @Success 200 {object} response.Body{data=inv.platformSettlements}
// @Router /internal/open-platform/order-settlements/{id} [get]
func (h *Handler) platformOrderSettlements(c *gin.Context) {
	noStore(c)
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	history, err := h.svc.OrderSettlementHistory(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformSettlementRow, 0, len(history.Rows))
	for _, row := range history.Rows {
		rows = append(rows, platformSettlementRow{row.ID, row.Amount, row.Kind, row.Method, row.SettledAt, row.AdjustsID})
	}
	response.OK(c, platformSettlements{OrderID: id, Settled: history.Settled, Outstanding: history.Outstanding, Rows: rows})
}
