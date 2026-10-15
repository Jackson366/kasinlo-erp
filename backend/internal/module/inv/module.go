package inv

import "nova-erp/internal/module"

// Module 返回进销存模块的自描述清单。
//
// 导航结构与画布设计一致，共 11 个页面。其中「数据概览」是**顶级菜单**而不是
// 「进销存」目录的子项：登录后的落地页由前端按菜单树第一个叶子推导
// （AGENTS.md §5.62），顶级叶子才能稳定当首页；放在目录下的话，
// 落地页会变成目录的 Redirect，语义上「首页 = 一个跳转」也不对。
// 其余 10 个页面归「进销存」目录：先业务流程（采购 → 销售 → 库存 → 应收应付
// → 报表 → 打印模板），再基础资料（商品 → 商品分类 → 客户 → 供应商）。
//
// 权限命名空间统一为 inv。菜单权限码与 route.go 里 RequirePerm 用的是同一批字符串，
// 避免「菜单看得见、接口 403」——这正是把声明收进模块的价值。
func Module() module.Module {
	return module.Module{
		Namespace: "inv",
		Register:  Register,
		Menus: []module.MenuNode{
			{
				Name: "InvOverview", Title: "数据概览", Icon: "Odometer",
				Path: "/inv/overview", Component: "inv/overview/index",
				Permission: "inv:overview:view", Sort: 1, KeepAlive: true,
			},
			{
				Name: "Inv", Title: "进销存", Icon: "Goods",
				Path: "/inv", Redirect: "/inv/overview", Sort: 2, Type: module.TypeDir,
				Children: []module.MenuNode{
					{
						Name: "InvPurchase", Title: "采购管理", Icon: "ShoppingCart",
						Path: "/inv/purchase", Component: "inv/purchase/index",
						Permission: "inv:purchase:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvPurchaseCreate", Title: "新增采购单", Type: module.TypeButton, Permission: "inv:purchase:create"},
							{Name: "InvPurchaseUpdate", Title: "修改采购单", Type: module.TypeButton, Permission: "inv:purchase:update"},
							{Name: "InvPurchaseDelete", Title: "删除采购单", Type: module.TypeButton, Permission: "inv:purchase:delete"},
							{Name: "InvPurchaseFinish", Title: "采购审核", Type: module.TypeButton, Permission: "inv:purchase:finish"},
						},
					},
					{
						Name: "InvSales", Title: "销售管理", Icon: "Sell",
						Path: "/inv/sales", Component: "inv/sales/index",
						Permission: "inv:sales:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvSalesCreate", Title: "新增销售单", Type: module.TypeButton, Permission: "inv:sales:create"},
							{Name: "InvSalesUpdate", Title: "修改销售单", Type: module.TypeButton, Permission: "inv:sales:update"},
							{Name: "InvSalesDelete", Title: "删除销售单", Type: module.TypeButton, Permission: "inv:sales:delete"},
							{Name: "InvSalesFinish", Title: "销售审核", Type: module.TypeButton, Permission: "inv:sales:finish"},
						},
					},
					{
						Name: "InvFinance", Title: "应收应付", Icon: "Money",
						Path: "/inv/finance", Component: "inv/finance/index",
						Permission: "inv:finance:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvFinanceSettle", Title: "登记收付款", Type: module.TypeButton, Permission: "inv:finance:settle"},
						},
					},
					{
						Name: "InvReport", Title: "报表中心", Icon: "TrendCharts",
						Path: "/inv/report", Component: "inv/report/index",
						Permission: "inv:report:view", KeepAlive: true,
					},
					{
						Name: "InvPrint", Title: "打印模板", Icon: "Printer",
						Path: "/inv/print", Component: "inv/print/index",
						Permission: "inv:print:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvPrintCreate", Title: "新增模板", Type: module.TypeButton, Permission: "inv:print:create"},
							{Name: "InvPrintUpdate", Title: "修改模板", Type: module.TypeButton, Permission: "inv:print:update"},
							{Name: "InvPrintDelete", Title: "删除模板", Type: module.TypeButton, Permission: "inv:print:delete"},
						},
					},
					{
						// 商品与客户 / 供应商同属「基础资料」：单据要用的一切主数据都在这一簇里，
						// 放在业务流程（采购→销售→库存→应收应付→报表→打印）之后，
						// 语义上就是「先把档案备齐，再走业务」，与侧边栏从上到下的阅读顺序一致。
						//
						// 分类不再是独立页面，而是这页左侧的树：维护分类时本来就要盯着
						// 它下面的商品，分成两页等于为同一个动作开两个入口、来回切换。
						// 分类的按钮权限点因此挂在本节点下（授权关系不变，只是少一个菜单）。
						Name: "InvProduct", Title: "商品档案", Icon: "GoodsFilled",
						Path: "/inv/product", Component: "inv/product/index",
						Permission: "inv:product:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvProductCreate", Title: "新增商品", Type: module.TypeButton, Permission: "inv:product:create"},
							{Name: "InvProductUpdate", Title: "修改商品", Type: module.TypeButton, Permission: "inv:product:update"},
							{Name: "InvProductDelete", Title: "删除商品", Type: module.TypeButton, Permission: "inv:product:delete"},
							{Name: "InvCategoryCreate", Title: "新增分类", Type: module.TypeButton, Permission: "inv:category:create"},
							{Name: "InvCategoryUpdate", Title: "修改分类", Type: module.TypeButton, Permission: "inv:category:update"},
							{Name: "InvCategoryDelete", Title: "删除分类", Type: module.TypeButton, Permission: "inv:category:delete"},
						},
					},
					{
						Name: "InvCustomer", Title: "客户管理", Icon: "User",
						Path: "/inv/customer", Component: "inv/customer/index",
						Permission: "inv:customer:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvCustomerCreate", Title: "新增客户", Type: module.TypeButton, Permission: "inv:customer:create"},
							{Name: "InvCustomerUpdate", Title: "修改客户", Type: module.TypeButton, Permission: "inv:customer:update"},
							{Name: "InvCustomerDelete", Title: "删除客户", Type: module.TypeButton, Permission: "inv:customer:delete"},
						},
					},
					{
						Name: "InvSupplier", Title: "供应商管理", Icon: "Van",
						Path: "/inv/supplier", Component: "inv/supplier/index",
						Permission: "inv:supplier:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "InvSupplierCreate", Title: "新增供应商", Type: module.TypeButton, Permission: "inv:supplier:create"},
							{Name: "InvSupplierUpdate", Title: "修改供应商", Type: module.TypeButton, Permission: "inv:supplier:update"},
							{Name: "InvSupplierDelete", Title: "删除供应商", Type: module.TypeButton, Permission: "inv:supplier:delete"},
						},
					},
				},
			},
		},
	}
}
