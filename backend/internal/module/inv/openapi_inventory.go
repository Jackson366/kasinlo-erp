package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/module"
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

			products := register("product.read", "inv:product:list")
			products.GET("/products", h.platformProducts)
			products.GET("/products/:id", h.platformProduct)

			purchase := register("purchase.read", "inv:purchase:list")
			purchase.GET("/purchase-orders", h.platformOrders(model.InvBizPurchase))
			purchase.GET("/purchase-orders/:id", h.platformOrder(model.InvBizPurchase))

			sales := register("sales.read", "inv:sales:list")
			sales.GET("/sales-orders", h.platformOrders(model.InvBizSales))
			sales.GET("/sales-orders/:id", h.platformOrder(model.InvBizSales))
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
