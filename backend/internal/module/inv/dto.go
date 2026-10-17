package inv

import "time"

// ============================================================
// 基础资料：客户 / 供应商（共用一套请求结构）
// ============================================================

// PartyReq 客户 / 供应商新增、修改请求。
type PartyReq struct {
	Code    string `json:"code" binding:"required,max=32" label:"编号"`
	Name    string `json:"name" binding:"required,max=128" label:"名称"`
	Contact string `json:"contact" binding:"omitempty,max=64" label:"联系人"`
	Phone   string `json:"phone" binding:"omitempty,max=32" label:"联系电话"`
	Address string `json:"address" binding:"omitempty,max=255" label:"地址"`
	TaxNo   string `json:"taxNo" binding:"omitempty,max=64" label:"开票信息"`
	Status  *int16 `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Remark  string `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// PartyRow 客户 / 供应商列表行。Balance 为应收（客户）/ 应付（供应商）余额。
type PartyRow struct {
	ID         int64     `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Contact    string    `json:"contact"`
	Phone      string    `json:"phone"`
	Address    string    `json:"address"`
	TaxNo      string    `json:"taxNo"`
	Status     int16     `json:"status"`
	Remark     string    `json:"remark"`
	Balance    float64   `json:"balance"`
	OrderCount int64     `json:"orderCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ============================================================
// 商品分类
// ============================================================

// CategoryReq 商品分类新增 / 修改请求。
type CategoryReq struct {
	ParentID int64  `json:"parentId" label:"上级分类"` // 0 = 根分类
	Name     string `json:"name" binding:"required,max=64" label:"分类名称"`
	Sort     int    `json:"sort" label:"排序"`
	Status   *int16 `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Remark   string `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// CategoryRow 商品分类列表行。ProductCount 为引用该分类的商品数（删除校验要用）。
// 列表返回**平表**（含 parentId），树由前端组装 —— 后端不猜前端要展开到第几层。
type CategoryRow struct {
	ID           int64     `json:"id"`
	ParentID     int64     `json:"parentId"`
	Name         string    `json:"name"`
	Sort         int       `json:"sort"`
	Status       int16     `json:"status"`
	Remark       string    `json:"remark"`
	ProductCount int64     `json:"productCount"`
	ChildCount   int64     `json:"childCount"` // 子分类数，删除校验要用
	CreatedAt    time.Time `json:"createdAt"`
}

// CategoryOption 分类下拉项。ParentID 让前端能渲染成树形下拉，
// 也用来判断「这个选项是不是当前编辑项的后代」（不能挂到自己子孙下面）。
type CategoryOption struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parentId"`
	Name     string `json:"name"`
}

// ============================================================
// 商品（商品 → 规格 → 计量单位）
// ============================================================

// ProductUnitReq 规格下的一个计量单位。
//
// Rate 是「1 个该单位 = Rate 个基本单位」，基本单位自身填 1。
// ProductUnitReq 一个计量单位。
//
// Unit / Rate 都是**可选项**：没填由 normalizeSpecs 兜底成「个」与 1。
// 硬约束只剩长度与符号范围 —— 让用户不必为了录一个简单商品先想清楚计量单位。
type ProductUnitReq struct {
	Unit          string  `json:"unit" binding:"max=16" label:"单位"`
	Rate          float64 `json:"rate" binding:"gte=0" label:"换算率"`
	IsBase        bool    `json:"isBase" label:"基本单位"`
	PurchasePrice float64 `json:"purchasePrice" binding:"gte=0" label:"采购价"`
	SalePrice     float64 `json:"salePrice" binding:"gte=0" label:"销售价"`
	Status        *int16  `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
}

// ProductSpecReq 一个规格及其计量单位。
//
// ID 由前端在编辑时回传，用来把「改名」和「删旧建新」区分开：
// 只按规格名匹配的话，把「φ20」改成「φ20A」会被当成「新增 φ20A + 删除 φ20」，
// 而 φ20 上有历史单据引用、删不掉，只能停用 —— 于是改一次名就多出一条僵尸规格。
//
// 库存刻意不在这里：它由出入库单据驱动，建档与改档都不接受表单传入，
// 否则「档案上的库存」与「单据算出来的库存」会变成两套账。
type ProductSpecReq struct {
	ID        int64            `json:"id" label:"规格 ID"`
	Spec      string           `json:"spec" binding:"max=64" label:"规格"`
	WarnStock float64          `json:"warnStock" binding:"gte=0" label:"预警下限"`
	Status    *int16           `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Units     []ProductUnitReq `json:"units" binding:"dive" label:"计量单位"`
}

// ProductReq 商品新增 / 修改请求。
//
// Specs 可留空：规格与单位都是**可选项**，缺的由 normalizeSpecs 补成
// 「默认 / 个」—— 真正的约束（库存要有归属、单据要能折算）由服务端守住，
// 不转嫁成用户面前的必填项。
type ProductReq struct {
	Code       string           `json:"code" binding:"required,max=32" label:"商品编码"`
	Name       string           `json:"name" binding:"required,max=128" label:"商品名称"`
	Barcode    string           `json:"barcode" binding:"omitempty,max=64" label:"商品条码"`
	CategoryID int64            `json:"categoryId" label:"商品分类"`
	Status     *int16           `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Remark     string           `json:"remark" binding:"omitempty,max=255" label:"备注"`
	Specs      []ProductSpecReq `json:"specs" binding:"dive" label:"规格"`
}

// ProductUnitVO 计量单位（响应）。
type ProductUnitVO struct {
	ID            int64   `json:"id"`
	Unit          string  `json:"unit"`
	Rate          float64 `json:"rate"`
	IsBase        bool    `json:"isBase"`
	PurchasePrice float64 `json:"purchasePrice"`
	SalePrice     float64 `json:"salePrice"`
	Status        int16   `json:"status"`
}

// ProductSpecVO 规格（响应）。Stock 以基本单位计。
type ProductSpecVO struct {
	ID        int64           `json:"id"`
	Spec      string          `json:"spec"`
	Stock     float64         `json:"stock"`
	WarnStock float64         `json:"warnStock"`
	Warn      bool            `json:"warn"`
	Status    int16           `json:"status"`
	Units     []ProductUnitVO `json:"units"`
}

// ProductRow 商品列表行。带全量规格明细，前端列表行可展开直接看规格与单位。
type ProductRow struct {
	ID           int64           `json:"id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Barcode      string          `json:"barcode"`
	CategoryID   int64           `json:"categoryId"`
	CategoryName string          `json:"categoryName"`
	SpecCount    int             `json:"specCount"`
	UnitCount    int             `json:"unitCount"`
	TotalStock   float64         `json:"totalStock"`
	Warn         bool            `json:"warn"`
	Status       int16           `json:"status"`
	Remark       string          `json:"remark"`
	CreatedAt    time.Time       `json:"createdAt"`
	Specs        []ProductSpecVO `json:"specs"`
}

// ProductDupRow 同名商品提示行。用于「录入时把可疑的重复商品先摆出来」。
type ProductDupRow struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	CategoryName string `json:"categoryName"`
	SpecSummary  string `json:"specSummary"`
	Status       int16  `json:"status"`
}

// PickUnitVO 单据明细可选的计量单位。
type PickUnitVO struct {
	ID            int64   `json:"id"`
	Unit          string  `json:"unit"`
	Rate          float64 `json:"rate"`
	IsBase        bool    `json:"isBase"`
	PurchasePrice float64 `json:"purchasePrice"`
	SalePrice     float64 `json:"salePrice"`
}

// PickSpecVO 单据明细可选的规格。
type PickSpecVO struct {
	ID    int64        `json:"id"`
	Spec  string       `json:"spec"`
	Stock float64      `json:"stock"`
	Units []PickUnitVO `json:"units"`
}

// ProductPickVO 单据明细的商品 → 规格 → 单位 级联选项。
type ProductPickVO struct {
	ID      int64        `json:"id"`
	Code    string       `json:"code"`
	Name    string       `json:"name"`
	Barcode string       `json:"barcode"`
	Specs   []PickSpecVO `json:"specs"`
}

// ProductPickResult 商品级联选项的返回体。
//
// Truncated 是**刻意暴露**的：级联选项一次最多给 pickLimit 个商品，
// 超出的部分不返回。若不给这个标记，商品多了以后下拉里就「少了几个」，
// 用户只会以为系统没录进去 —— 静默截断比报错更难排查。
// 前端拿到 truncated=true 要提示「请用关键字搜索」。
type ProductPickResult struct {
	Items     []ProductPickVO `json:"items"`
	Total     int64           `json:"total"`
	Truncated bool            `json:"truncated"`
}

// InventoryRow 库存查询行。粒度是**规格**：库存记在规格上，
// 商品层级只是把同一个商品下的规格归拢在一起看。
type InventoryRow struct {
	ProductID     int64   `json:"productId"`
	ProductCode   string  `json:"productCode"`
	ProductName   string  `json:"productName"`
	CategoryName  string  `json:"categoryName"`
	SpecID        int64   `json:"specId"`
	Spec          string  `json:"spec"`
	UnitID        int64   `json:"unitId"`
	Unit          string  `json:"unit"` // 基本单位（库存的计数单位）
	Stock         float64 `json:"stock"`
	WarnStock     float64 `json:"warnStock"`
	Warn          bool    `json:"warn"`
	PurchasePrice float64 `json:"purchasePrice"` // 基本单位的采购价，用于算库存成本
}

// ============================================================
// 采购 / 销售单据
// ============================================================

// OrderItemReq 单据明细行请求。
//
// 规格与单位必填：同一个商品的不同规格是**不同的库存**，不指定规格就没法过账。
type OrderItemReq struct {
	ProductID int64   `json:"productId" binding:"required,gt=0" label:"商品"`
	SpecID    int64   `json:"specId" binding:"required,gt=0" label:"规格"`
	UnitID    int64   `json:"unitId" binding:"required,gt=0" label:"单位"`
	Qty       float64 `json:"qty" binding:"required,gt=0" label:"数量"`
	Price     float64 `json:"price" binding:"gte=0" label:"单价"`
}

// OrderReq 单据新增 / 修改请求。价格缺省时取商品的采购价 / 销售价。
type OrderReq struct {
	BizType   string         `json:"bizType" binding:"required,oneof=purchase sales" label:"单据类型"`
	PartyID   int64          `json:"partyId" binding:"required,gt=0" label:"往来单位"`
	OrderDate string         `json:"orderDate" binding:"required" label:"单据日期"`
	DueDate   string         `json:"dueDate" binding:"omitempty" label:"到期日"`
	Remark    string         `json:"remark" binding:"omitempty,max=255" label:"备注"`
	Salesman  string         `json:"salesman" binding:"omitempty,max=64" label:"业务员"`
	Items     []OrderItemReq `json:"items" binding:"required,min=1" label:"明细"`
}

// OrderItemVO 单据明细行。编码不在明细快照里，由商品档案补齐（商品已删则留空）。
type OrderItemVO struct {
	ID          int64   `json:"id"`
	ProductID   int64   `json:"productId"`
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	SpecID      int64   `json:"specId"`
	Spec        string  `json:"spec"`
	UnitID      int64   `json:"unitId"`
	Unit        string  `json:"unit"`
	Rate        float64 `json:"rate"`
	Qty         float64 `json:"qty"`
	QtyBase     float64 `json:"qtyBase"`
	Price       float64 `json:"price"`
	Amount      float64 `json:"amount"`
}

// OrderFilter 单据列表筛选条件。
//
// 聚成一个结构体而不是继续加形参：单据类型、状态、结算状态、起止日期
// 已经是四组条件，平铺成参数列表后调用处只剩一串「nil、""、nil」，
// 谁都不愿意读。新增一种筛选只改这里和 service 里的一段 SQL。
type OrderFilter struct {
	BizType string
	Status  *int16
	// Settle 结算状态："" 全部 / unpaid 未结清 / partial 部分结算 / paid 已结清。
	// 与 settleStatus() 的判定必须同一口径，否则筛选出来的行与标签对不上。
	Settle string
	// Start / End 按**单据日期**过滤，闭区间；nil 表示不限。
	Start *time.Time
	End   *time.Time
}

// OrderRow 单据列表行。
type OrderRow struct {
	ID            int64      `json:"id"`
	OrderNo       string     `json:"orderNo"`
	BizType       string     `json:"bizType"`
	PartyID       int64      `json:"partyId"`
	PartyName     string     `json:"partyName"`
	OrderDate     time.Time  `json:"orderDate"`
	DueDate       *time.Time `json:"dueDate"` // 选填：未填时为 null
	Status        int16      `json:"status"`
	TotalAmount   float64    `json:"totalAmount"`
	SettledAmount float64    `json:"settledAmount"`
	SettleStatus  string     `json:"settleStatus"`
	ItemCount     int64      `json:"itemCount"`
	Remark        string     `json:"remark"`
	Salesman      string     `json:"salesman"`
	// Operator 制单人显示名（昵称优先，回落用户名）。列表直接用，省得前端再查一次人。
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrderDetail 单据详情（主表 + 明细）。制单人显示名在 OrderRow.Operator 里。
type OrderDetail struct {
	OrderRow
	Items []OrderItemVO `json:"items"`
}

// StatusReq 单据状态流转请求。status 仅允许 1（恢复待处理）/ 2（审核通过）/ 3（取消）。
type StatusReq struct {
	Status int16 `json:"status" binding:"required,oneof=1 2 3" label:"状态"`
}

// ============================================================
// 库存
// ============================================================

// StockItemReq 出入库明细请求。与单据明细同构，但库存只关心基本数量。
type StockItemReq struct {
	ProductID int64   `json:"productId" binding:"required,gt=0" label:"商品"`
	SpecID    int64   `json:"specId" binding:"required,gt=0" label:"规格"`
	UnitID    int64   `json:"unitId" binding:"required,gt=0" label:"单位"`
	Qty       float64 `json:"qty" binding:"required,gt=0" label:"数量"`
}

// StockOrderReq 出入库单据请求。
type StockOrderReq struct {
	Direction  string         `json:"direction" binding:"required,oneof=in out" label:"方向"`
	SourceType string         `json:"sourceType" binding:"omitempty,oneof=purchase sales manual" label:"来源"`
	OrderDate  string         `json:"orderDate" binding:"required" label:"单据日期"`
	Remark     string         `json:"remark" binding:"omitempty,max=255" label:"备注"`
	Salesman   string         `json:"salesman" binding:"omitempty,max=64" label:"业务员"`
	Items      []StockItemReq `json:"items" binding:"required,min=1" label:"明细"`
}

// StockOrderRow 出入库单据列表行。
type StockOrderRow struct {
	ID         int64     `json:"id"`
	OrderNo    string    `json:"orderNo"`
	Direction  string    `json:"direction"`
	SourceType string    `json:"sourceType"`
	SourceID   int64     `json:"sourceId"`
	OrderDate  time.Time `json:"orderDate"`
	Status     int16     `json:"status"`
	ItemCount  int64     `json:"itemCount"`
	TotalQty   float64   `json:"totalQty"`
	Remark     string    `json:"remark"`
	Salesman   string    `json:"salesman"`
	// Operator 制单人显示名（昵称优先，回落用户名）
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"createdAt"`
}

// StockPrintItem 出入库单打印明细行。编码来自商品档案（打印时要能对上实物）。
// Rate 与 QtyBase 一起出现，是为了让纸面能把「2 箱」与「20 个」对上账。
type StockPrintItem struct {
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	Spec        string  `json:"spec"`
	Unit        string  `json:"unit"`
	Rate        float64 `json:"rate"`
	Qty         float64 `json:"qty"`
	QtyBase     float64 `json:"qtyBase"`
	UnitBase    string  `json:"unitBase"` // 基本单位名，用于把基本数量写清楚
}

// StockOrderDetail 出入库单打印详情：表头 + 明细 + 打印所需的关联信息。
// 与列表行分离——列表不查商品档案与来源单号，打印才值得多花两次查询。
type StockOrderDetail struct {
	ID            int64            `json:"id"`
	OrderNo       string           `json:"orderNo"`
	Direction     string           `json:"direction"`
	SourceType    string           `json:"sourceType"`
	SourceOrderNo string           `json:"sourceOrderNo"` // 来源采购 / 销售单号，手工单为空
	OrderDate     time.Time        `json:"orderDate"`
	Remark        string           `json:"remark"`
	Salesman      string           `json:"salesman"` // 业务员
	Operator      string           `json:"operator"` // 制单人昵称
	TotalQty      float64          `json:"totalQty"`
	Items         []StockPrintItem `json:"items"`
}

// ============================================================
// 应收应付
// ============================================================

// FinanceRow 应收 / 应付明细行。
type FinanceRow struct {
	OrderID       int64      `json:"orderId"`
	OrderNo       string     `json:"orderNo"`
	PartyID       int64      `json:"partyId"`
	PartyName     string     `json:"partyName"`
	OrderDate     time.Time  `json:"orderDate"`
	DueDate       *time.Time `json:"dueDate"` // 选填：未填时为 null，不参与逾期判定
	TotalAmount   float64    `json:"totalAmount"`
	SettledAmount float64    `json:"settledAmount"`
	Outstanding   float64    `json:"outstanding"`
	SettleStatus  string     `json:"settleStatus"`
	Overdue       bool       `json:"overdue"`
}

// FinanceSummary 应收应付汇总。
type FinanceSummary struct {
	Total       float64 `json:"total"`
	Settled     float64 `json:"settled"`
	Outstanding float64 `json:"outstanding"`
	Overdue     float64 `json:"overdue"`
}

// SettleReq 登记收付款请求。
type SettleReq struct {
	OrderID   int64   `json:"orderId" binding:"required,gt=0" label:"单据"`
	Amount    float64 `json:"amount" binding:"required,gt=0" label:"金额"`
	Method    string  `json:"method" binding:"omitempty,max=32" label:"结算方式"`
	SettledAt string  `json:"settledAt" binding:"omitempty" label:"结算日期"`
	Remark    string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// ============================================================
// 报表 / 概览
// ============================================================

// OverviewVO 数据概览。
type OverviewVO struct {
	CustomerCount    int64   `json:"customerCount"`
	ActiveCustomer   int64   `json:"activeCustomer"`
	MonthNewCustomer int64   `json:"monthNewCustomer"`
	ReceivableTotal  float64 `json:"receivableTotal"`
	PayableTotal     float64 `json:"payableTotal"`
	MonthSales       float64 `json:"monthSales"`
	MonthPurchase    float64 `json:"monthPurchase"`
	PendingOrders    int64   `json:"pendingOrders"`
	ProductCount     int64   `json:"productCount"`
	WarnProductCount int64   `json:"warnProductCount"`
}

// ReportSummary 报表汇总。
type ReportSummary struct {
	Range          string  `json:"range"`
	SalesAmount    float64 `json:"salesAmount"`
	CostAmount     float64 `json:"costAmount"`
	GrossProfit    float64 `json:"grossProfit"`
	GrossRate      float64 `json:"grossRate"`
	SalesOrders    int64   `json:"salesOrders"`
	PurchaseOrders int64   `json:"purchaseOrders"`
	Purchases      float64 `json:"purchases"`
}

// ReportTrendPoint 报表趋势点。
type ReportTrendPoint struct {
	Date           string  `json:"date"`
	SalesAmount    float64 `json:"salesAmount"`
	PurchaseAmount float64 `json:"purchaseAmount"`
}

// ReportRankRow 排行行（客户 / 供应商 / 商品三个维度通用）。
//
// 客户与供应商维度按单据主表的 party 聚合，商品维度按明细行的 product 聚合；
// 两者返回同一结构，前端排行表按维度切换列。
type ReportRankRow struct {
	Rank       int     `json:"rank"`
	Name       string  `json:"name"`       // 客户名 / 供应商名 / 商品名
	ID         int64   `json:"id"`         // 对应档案 ID（party_id / product_id）
	Amount     float64 `json:"amount"`     // 销售 / 采购金额
	OrderCount int64   `json:"orderCount"` // 单据数（客户/供应商）或涉及订单数（商品）
	Quantity   float64 `json:"quantity"`   // 商品维度：总销量（基本单位）；客户维度为 0
	Ratio      float64 `json:"ratio"`
}

// ReportCategoryRow 品类占比行。
type ReportCategoryRow struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Ratio    float64 `json:"ratio"`
}

// InvCategoryValueRow 品类库存金额分布行。
type InvCategoryValueRow struct {
	Category string  `json:"category"`
	Value    float64 `json:"value"` // 库存金额（按基本单位采购价计价）
	Ratio    float64 `json:"ratio"`
}

// InvWarnStockRow 低库存预警行。缺货程度按「当前库存 − 预警下限」升序，
// 最缺的排最前。
type InvWarnStockRow struct {
	ProductID   int64   `json:"productId"`
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	SpecID      int64   `json:"specId"`
	Spec        string  `json:"spec"`
	Unit        string  `json:"unit"` // 基本单位（库存的计数单位）
	Stock       float64 `json:"stock"`
	WarnStock   float64 `json:"warnStock"`
}

// ReportInventory 库存分析汇总。
type ReportInventory struct {
	TotalValue    float64               `json:"totalValue"`    // 库存总金额（成本价）
	SpecCount     int64                 `json:"specCount"`     // 启用规格数
	WarnCount     int64                 `json:"warnCount"`     // 低库存预警规格数
	CategoryValue []InvCategoryValueRow `json:"categoryValue"` // 品类库存金额分布
	WarnList      []InvWarnStockRow     `json:"warnList"`      // 低库存预警清单（前 20）
}

// ============================================================
// 打印模板
// ============================================================

// PrintTemplateReq 打印模板新增 / 修改请求。
type PrintTemplateReq struct {
	Code    string `json:"code" binding:"required,max=32" label:"模板编码"`
	Name    string `json:"name" binding:"required,max=64" label:"模板名称"`
	DocType string `json:"docType" binding:"omitempty,max=32" label:"单据类型"`
	Paper   string `json:"paper" binding:"omitempty,oneof=A4 A5" label:"纸张"`
	Content string `json:"content" binding:"omitempty,max=8000" label:"版式配置"`
	Status  *int16 `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Remark  string `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// PrintTemplateRow 打印模板列表行。
type PrintTemplateRow struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	DocType   string    `json:"docType"`
	Paper     string    `json:"paper"`
	Content   string    `json:"content"`
	Status    int16     `json:"status"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
}
