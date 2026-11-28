package inv

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/module"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// PlatformInventoryModule exposes the read-only ERP integration when configured.
func PlatformInventoryModule(cfg config.OpenPlatformConfig) (module.Module, bool) {
	if cfg.Secret == "" {
		return module.Module{}, false
	}
	return module.Module{
		Namespace: "inv",
		Register: func(rg *gin.RouterGroup) {
			h := NewHandler(NewService())
			register := func(scope, permission string) *gin.RouterGroup {
				return rg.Group("/internal/open-platform",
					middleware.PlatformAuth([]byte(cfg.Secret), cfg.Issuer, cfg.Audience, scope),
					middleware.RequirePerm(permission))
			}
			register("inventory.read", "inv:stock:list").GET("/inventory", h.platformInventory)
			register("inventory.report.read", "inv:report:view").GET("/inventory-report", h.platformInventoryReport)

			register("business.report.read", "inv:report:view").GET("/business-summary", h.platformBusinessSummary)
			register("business.trend.read", "inv:report:view").GET("/business-trend", h.platformBusinessTrend)
			register("inventory.ranking.read", "inv:report:view").GET("/stock-ranking", h.platformStockRanking)

			products := register("product.read", "inv:product:list")
			products.GET("/product-categories", h.platformCategories)
			products.GET("/product-categories/:id", h.platformCategory)
			products.GET("/products", h.platformProducts)
			products.GET("/products/:id", h.platformProduct)

			customers := register("customer.read", "inv:customer:list")
			customers.GET("/customers", h.platformParties(model.InvBizSales))
			customers.GET("/customers/:id", h.platformParty(model.InvBizSales))

			suppliers := register("supplier.read", "inv:supplier:list")
			suppliers.GET("/suppliers", h.platformParties(model.InvBizPurchase))
			suppliers.GET("/suppliers/:id", h.platformParty(model.InvBizPurchase))

			purchase := register("purchase.read", "inv:purchase:list")
			purchase.GET("/purchase-orders", h.platformOrders(model.InvBizPurchase))
			purchase.GET("/purchase-orders/:id", h.platformOrder(model.InvBizPurchase))

			sales := register("sales.read", "inv:sales:list")
			sales.GET("/sales-orders", h.platformOrders(model.InvBizSales))
			sales.GET("/sales-orders/:id", h.platformOrder(model.InvBizSales))

			stock := register("stock.read", "inv:stock:list")
			stock.GET("/stock-orders", h.platformStockOrders)
			stock.GET("/stock-orders/:id", h.platformStockOrder)

			contracts := register("contract.read", "inv:contract:list")
			contracts.GET("/contracts", h.platformContracts)
			contracts.GET("/contracts/:id", h.platformContract)

			register("contract.report.read", "inv:report:view").GET("/contract-report", h.platformContractReport)

			register("contract.ledger.read", "inv:report:view").GET("/contract-ledger", h.platformContractLedger)

			contractHistory := register("contract.history.read", "inv:contract:list")
			contractHistory.GET("/contract-activities/:id", h.platformContractActivity)
			contractHistory.GET("/contract-plan-settlements/:id", h.platformContractPlanSettlements)

			finance := register("finance.read", "inv:finance:list")
			finance.GET("/receivables", h.platformFinanceList(true))
			finance.GET("/payables", h.platformFinanceList(false))
			finance.GET("/order-settlements/:id", h.platformOrderSettlements)
		},
	}, true
}

type platformInventoryRow struct {
	ProductID   int64   `json:"productId"`
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	SpecID      int64   `json:"specId"`
	Spec        string  `json:"spec"`
	Unit        string  `json:"unit"`
	Stock       float64 `json:"stock"`
}

func (h *Handler) platformInventory(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	q := page.FromContext(c)
	list, total, err := h.svc.Inventory(q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := make([]platformInventoryRow, 0, len(list))
	for _, row := range list {
		rows = append(rows, platformInventoryRow{
			ProductID:   row.ProductID,
			ProductCode: row.ProductCode,
			ProductName: row.ProductName,
			SpecID:      row.SpecID,
			Spec:        row.Spec,
			Unit:        row.Unit,
			Stock:       row.Stock,
		})
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// platformInventoryReport 查询共享库存的成本与预警。
//
// @Summary 查询库存分析报表
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Body{data=inv.ReportInventory}
// @Failure 403 {object} response.Body
// @Router /internal/open-platform/inventory-report [get]
func (h *Handler) platformInventoryReport(c *gin.Context) {
	noStore(c)
	h.ReportInventory(c)
}

// platformBusinessSummary 查询当前 ERP 身份数据范围内的经营汇总。
//
// @Summary 查询经营报表汇总
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param range query string false "today | week | month | quarter | year，默认 month"
// @Param from query string false "起始日 yyyy-MM-dd，优先于 range"
// @Param to query string false "结束日 yyyy-MM-dd（含当天）"
// @Param categoryId query int false "品类 ID（含子分类），0 = 全部"
// @Success 200 {object} response.Body{data=inv.ReportSummary}
// @Failure 400 {object} response.Body
// @Failure 403 {object} response.Body
// @Router /internal/open-platform/business-summary [get]
func (h *Handler) platformBusinessSummary(c *gin.Context) {
	noStore(c)
	h.ReportSummary(c)
}

type platformBusinessTrendResult struct {
	Rows []ReportTrendPoint `json:"rows"`
}

// platformBusinessTrend 查询当前 ERP 身份数据范围内的日销售与采购趋势。
//
// @Summary 查询日销售与采购趋势
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param days query int false "最近天数（含今天，1–90，默认 30）"
// @Param from query string false "自定义起始日 yyyy-MM-dd，须与 to 同传"
// @Param to query string false "自定义结束日 yyyy-MM-dd（含当天，最多 90 天）"
// @Param categoryId query int false "匹配订单的品类 ID（含子分类），0 = 全部"
// @Success 200 {object} response.Body{data=inv.platformBusinessTrendResult}
// @Failure 400 {object} response.Body
// @Failure 403 {object} response.Body
// @Router /internal/open-platform/business-trend [get]
func (h *Handler) platformBusinessTrend(c *gin.Context) {
	noStore(c)
	filter, err := reportFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := setPlatformTrendDates(&filter, c.Query("days")); err != nil {
		response.Fail(c, err)
		return
	}
	rows, err := h.svc.ReportTrend(0, filter, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, platformBusinessTrendResult{Rows: rows})
}

func setPlatformTrendDates(filter *ReportFilter, rawDays string) error {
	if filter.From != "" || filter.To != "" {
		if rawDays != "" || filter.From == "" || filter.To == "" {
			return errcode.New(errcode.ErrBadRequest, "from 与 to 必须同时提供，且不能与 days 同用")
		}
		from, fromErr := time.Parse(time.DateOnly, filter.From)
		to, toErr := time.Parse(time.DateOnly, filter.To)
		if fromErr != nil || toErr != nil || to.Before(from) || !to.Before(from.AddDate(0, 0, reportTrendMaxDays)) {
			return errcode.New(errcode.ErrBadRequest, "from 和 to 必须为 yyyy-MM-dd，且区间为 1 到 90 天")
		}
		return nil
	}

	days := reportTrendDefaultDays
	if rawDays != "" {
		value, err := strconv.Atoi(rawDays)
		if err != nil || value < 1 || value > reportTrendMaxDays {
			return errcode.New(errcode.ErrBadRequest, "days 必须为 1 到 90 的整数")
		}
		days = value
	}
	today := time.Now().In(database.Location())
	filter.From = today.AddDate(0, 0, 1-days).Format(time.DateOnly)
	filter.To = today.Format(time.DateOnly)
	return nil
}

type platformStockRankingResult struct {
	Rows []ReportStockRankRow `json:"rows"`
}

// platformStockRanking 查询共享库存金额排行。
//
// @Summary 查询库存金额排行
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param limit query int false "最大行数（1–1000，默认 1000）"
// @Success 200 {object} response.Body{data=inv.platformStockRankingResult}
// @Failure 400 {object} response.Body
// @Failure 403 {object} response.Body
// @Router /internal/open-platform/stock-ranking [get]
func (h *Handler) platformStockRanking(c *gin.Context) {
	noStore(c)
	limit := reportRankMaxRows
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > reportRankMaxRows {
			response.FailWith(c, errcode.ErrBadRequest, "limit 必须为 1 到 1000 的整数")
			return
		}
		limit = value
	}
	rows, err := h.svc.ReportStockRanking(limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, platformStockRankingResult{Rows: rows})
}
