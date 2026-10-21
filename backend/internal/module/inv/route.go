package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册进销存模块路由。全部接口都需要登录，并按下述权限码鉴权。
//
// 权限码一律以 inv: 开头（本模块命名空间），与 module.go 的菜单清单一一对应。
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	auth := middleware.Auth()

	g := rg.Group("/inv", auth)
	{
		// 数据概览与报表：只读
		g.GET("/overview", middleware.RequirePerm("inv:overview:view"), h.Overview)

		// 列表导出：一个入口覆盖商品 / 往来单位 / 单据 / 出入库 / 库存。
		// 这里不挂 RequirePerm —— 具体权限取决于 entity（商品要 product:list、
		// 单据要按 bizType 精确判定），在 handler 里按实体校验，见 ensureExportPerm。
		g.GET("/export", h.Export)

		// 基础资料：客户 / 供应商 / 商品
		g.GET("/customers", middleware.RequirePerm("inv:customer:list", "inv:sales:list"), h.CustomerList)
		g.POST("/customers", middleware.RequirePerm("inv:customer:create"), h.CustomerCreate)
		g.PUT("/customers/:id", middleware.RequirePerm("inv:customer:update"), h.CustomerUpdate)
		g.DELETE("/customers/:id", middleware.RequirePerm("inv:customer:delete"), h.CustomerDelete)

		g.GET("/suppliers", middleware.RequirePerm("inv:supplier:list", "inv:purchase:list"), h.SupplierList)
		g.POST("/suppliers", middleware.RequirePerm("inv:supplier:create"), h.SupplierCreate)
		g.PUT("/suppliers/:id", middleware.RequirePerm("inv:supplier:update"), h.SupplierUpdate)
		g.DELETE("/suppliers/:id", middleware.RequirePerm("inv:supplier:delete"), h.SupplierDelete)

		// 商品分类字典
		g.GET("/categories", middleware.RequirePerm("inv:category:list", "inv:product:list"), h.CategoryList)
		g.GET("/categories/options", middleware.RequirePerm("inv:category:list", "inv:product:list"), h.CategoryOptions)
		g.POST("/categories", middleware.RequirePerm("inv:category:create"), h.CategoryCreate)
		g.PUT("/categories/:id", middleware.RequirePerm("inv:category:update"), h.CategoryUpdate)
		g.DELETE("/categories/:id", middleware.RequirePerm("inv:category:delete"), h.CategoryDelete)

		// 商品：商品管理页 + 采购 / 销售 / 库存页的规格与单位下拉。
		// 静态路径段（pick / duplicates）必须注册在 /:id 之前，否则
		// /products/pick 会先命中 /:id，id 拿到字符串 "pick"（AGENTS.md §2 Step 3）。
		g.GET("/products", middleware.RequirePerm("inv:product:list", "inv:purchase:list", "inv:sales:list", "inv:stock:list"), h.ProductList)
		g.GET("/products/pick", middleware.RequirePerm("inv:product:list", "inv:purchase:list", "inv:sales:list", "inv:stock:list"), h.ProductPick)
		g.GET("/products/duplicates", middleware.RequirePerm("inv:product:list", "inv:product:create", "inv:product:update"), h.ProductDuplicates)
		g.POST("/products", middleware.RequirePerm("inv:product:create"), h.ProductCreate)
		g.GET("/products/:id", middleware.RequirePerm("inv:product:list"), h.ProductDetail)
		g.PUT("/products/:id", middleware.RequirePerm("inv:product:update"), h.ProductUpdate)
		g.DELETE("/products/:id", middleware.RequirePerm("inv:product:delete"), h.ProductDelete)

		// 采购 / 销售单据（同一套接口，用 bizType 区分）
		g.GET("/orders", middleware.RequirePerm("inv:purchase:list", "inv:sales:list"), h.OrderList)
		g.GET("/orders/:id", middleware.RequirePerm("inv:purchase:list", "inv:sales:list"), h.OrderDetail)
		g.POST("/orders", middleware.RequirePerm("inv:purchase:create", "inv:sales:create"), h.OrderCreate)
		g.PUT("/orders/:id", middleware.RequirePerm("inv:purchase:update", "inv:sales:update"), h.OrderUpdate)
		g.DELETE("/orders/:id", middleware.RequirePerm("inv:purchase:delete", "inv:sales:delete"), h.OrderDelete)
		g.PUT("/orders/:id/status", middleware.RequirePerm("inv:purchase:finish", "inv:sales:finish"), h.OrderChangeStatus)
		g.PUT("/orders/:id/reverse", middleware.RequirePerm("inv:purchase:finish", "inv:sales:finish"), h.OrderReverse)

		// 库存：出入库单据与库存查询
		g.GET("/stock-orders", middleware.RequirePerm("inv:stock:list"), h.StockOrderList)
		g.GET("/stock-orders/:id", middleware.RequirePerm("inv:stock:list"), h.StockOrderDetail)
		g.POST("/stock-orders", middleware.RequirePerm("inv:stock:create"), h.StockOrderCreate)
		g.DELETE("/stock-orders/:id", middleware.RequirePerm("inv:stock:delete"), h.StockOrderVoid)
		g.GET("/inventory", middleware.RequirePerm("inv:stock:list"), h.Inventory)

		// 应收应付
		g.GET("/finance/receivables", middleware.RequirePerm("inv:finance:list"), h.Receivables)
		g.GET("/finance/payables", middleware.RequirePerm("inv:finance:list"), h.Payables)
		g.POST("/finance/settlements", middleware.RequirePerm("inv:finance:settle"), h.Settle)
		g.POST("/finance/settlements/batch", middleware.RequirePerm("inv:finance:settle"), h.SettleBatch)

		// 报表中心
		g.GET("/report/summary", middleware.RequirePerm("inv:report:view"), h.ReportSummary)
		g.GET("/report/trend", middleware.RequirePerm("inv:report:view"), h.ReportTrend)
		g.GET("/report/ranking", middleware.RequirePerm("inv:report:view"), h.ReportRanking)
		g.GET("/report/category", middleware.RequirePerm("inv:report:view"), h.ReportCategory)
		g.GET("/report/inventory", middleware.RequirePerm("inv:report:view"), h.ReportInventory)
		g.GET("/report/stock-ranking", middleware.RequirePerm("inv:report:view"), h.ReportStockRanking)
		g.GET("/report/detail", middleware.RequirePerm("inv:report:view"), h.ReportDetail)
		g.GET("/report/export", middleware.RequirePerm("inv:report:view"), h.ReportExport)

		// 打印模板
		g.GET("/print-templates", middleware.RequirePerm("inv:print:list"), h.PrintTemplateList)
		g.POST("/print-templates", middleware.RequirePerm("inv:print:create"), h.PrintTemplateCreate)
		g.PUT("/print-templates/:id", middleware.RequirePerm("inv:print:update"), h.PrintTemplateUpdate)
		g.DELETE("/print-templates/:id", middleware.RequirePerm("inv:print:delete"), h.PrintTemplateDelete)
	}
}
