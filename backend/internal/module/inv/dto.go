package inv

import "time"

// ============================================================
// 基础资料：客户 / 供应商（共用一套请求结构）
// ============================================================

// PartyReq 客户 / 供应商新增、修改请求。
type PartyReq struct {
	// Code 留空时由服务端自动生成（客户 KH#### / 供应商 GYS####）；填了就用手工值（唯一性照常校验）。
	Code    string `json:"code" binding:"omitempty,max=32" label:"编号"`
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
	// Code 留空时由服务端自动生成（SP####）；填了就用手工值（唯一性照常校验）。
	Code       string `json:"code" binding:"omitempty,max=32" label:"商品编码"`
	Name       string `json:"name" binding:"required,max=128" label:"商品名称"`
	Barcode    string `json:"barcode" binding:"omitempty,max=64" label:"商品条码"`
	CategoryID int64  `json:"categoryId" label:"商品分类"`
	// 参考进价 / 参考售价：商品级备忘价。具体成交价仍以规格-单位上的
	// 采购价 / 销售价为准，参考价用于开单前的估价与对比。
	RefPurchasePrice float64          `json:"refPurchasePrice" binding:"omitempty,gte=0" label:"参考进价"`
	RefSalePrice     float64          `json:"refSalePrice" binding:"omitempty,gte=0" label:"参考销售价"`
	Status           *int16           `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Remark           string           `json:"remark" binding:"omitempty,max=255" label:"备注"`
	Specs            []ProductSpecReq `json:"specs" binding:"dive" label:"规格"`
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
	ID               int64           `json:"id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Barcode          string          `json:"barcode"`
	CategoryID       int64           `json:"categoryId"`
	CategoryName     string          `json:"categoryName"`
	RefPurchasePrice float64         `json:"refPurchasePrice"`
	RefSalePrice     float64         `json:"refSalePrice"`
	SpecCount        int             `json:"specCount"`
	UnitCount        int             `json:"unitCount"`
	TotalStock       float64         `json:"totalStock"`
	Warn             bool            `json:"warn"`
	Status           int16           `json:"status"`
	Remark           string          `json:"remark"`
	CreatedAt        time.Time       `json:"createdAt"`
	Specs            []ProductSpecVO `json:"specs"`
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
	ID      int64  `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Barcode string `json:"barcode"`
	// 参考进价 / 参考售价：开单选中商品时作为默认单价（>0 时优先于单位成交价）
	RefPurchasePrice float64      `json:"refPurchasePrice"`
	RefSalePrice     float64      `json:"refSalePrice"`
	Specs            []PickSpecVO `json:"specs"`
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
	BizType string `json:"bizType" binding:"required,oneof=purchase sales" label:"单据类型"`
	// OrderNo 单据编号。留空自动生成（CG/XS + 日期 + 流水）；填了用手工值（重复会 409）。
	OrderNo   string         `json:"orderNo" binding:"omitempty,max=32" label:"单据编号"`
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
	// Source 应收应付的来源："" 全部 / order 单据 / plan 合同期次。
	// 只在 finance 类查询里用，单据列表忽略它。
	Source string
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
	// SettleNow 仅审核（status=2）时有效：审核通过后立即按全额登记一笔
	// 收 / 付款（采购 → 付款、销售 → 收款），适合「一手交钱一手交货」。
	SettleNow bool `json:"settleNow" label:"同时结清"`
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
	Direction string `json:"direction" binding:"required,oneof=in out" label:"方向"`
	// OrderNo 单据编号。留空自动生成（CK + 日期 + 流水）；填了用手工值（重复会 409）。
	OrderNo    string         `json:"orderNo" binding:"omitempty,max=32" label:"单据编号"`
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
	// Source 决定这行怎么来的：order（已审核单据）或 plan（合同付款期次）。
	// 两类钱都进应收应付，靠这个字段分别筛选 —— 混在一起而不标来源，
	// 对账时分不清「这笔是发货开的单」还是「合同约定的第三期」。
	Source        string     `json:"source"`
	OrderID       int64      `json:"orderId"` // Source=order 时有值
	OrderNo       string     `json:"orderNo"`
	ContractID    int64      `json:"contractId"` // Source=plan 时有值
	ContractNo    string     `json:"contractNo"`
	PlanID        int64      `json:"planId"`
	PlanSeq       int        `json:"planSeq"`
	PlanName      string     `json:"planName"`
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
//
// OrderTotal / PlanTotal 是「其中」口径，不是并列的两块：
// Total = OrderTotal + PlanTotal，前端在汇总条上标「单据 / 合同」两个小字，
// 让用户一眼看出这个月要收的钱里有多少是合同分期、多少是发货单。
type FinanceSummary struct {
	Total       float64 `json:"total"`
	Settled     float64 `json:"settled"`
	Outstanding float64 `json:"outstanding"`
	Overdue     float64 `json:"overdue"`
	OrderTotal  float64 `json:"orderTotal"`
	PlanTotal   float64 `json:"planTotal"`
}

// SettleReq 登记收付款请求。
type SettleReq struct {
	OrderID   int64   `json:"orderId" binding:"required,gt=0" label:"单据"`
	Amount    float64 `json:"amount" binding:"required,gt=0" label:"金额"`
	Method    string  `json:"method" binding:"omitempty,max=32" label:"结算方式"`
	SettledAt string  `json:"settledAt" binding:"omitempty" label:"结算日期"`
	Remark    string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// SettleBatchReq 批量收付款。
//
// 默认口径是「按每张单据的未结余额全额结清」——一批单子一起付款时，
// 最常做的就是照着余额付干净。Amounts 用于个别单据只付一部分的场景：
// 传了就按该单据的指定金额，没传的仍按未结余额结清。
type SettleBatchReq struct {
	OrderIDs  []int64           `json:"orderIds" binding:"required,min=1,max=200,dive,gt=0" label:"单据"`
	Amounts   map[int64]float64 `json:"amounts" binding:"omitempty" label:"金额"`
	Method    string            `json:"method" binding:"omitempty,max=32" label:"结算方式"`
	SettledAt string            `json:"settledAt" binding:"omitempty" label:"结算日期"`
	Remark    string            `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// SettleBatchResult 批量结果：实际结算的单据数与金额合计。
type SettleBatchResult struct {
	Count  int     `json:"count"`
	Amount float64 `json:"amount"`
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

// ReportStockRankRow 库存排行行（按库存金额降序，全量）。
//
// 计价口径与库存分析一致：库存数量（基本单位）× 基本单位采购价。
// 没有采购价的规格金额为 0，仍会出现在榜上（数量有价值无，正好提示去补价）。
type ReportStockRankRow struct {
	Rank        int     `json:"rank"`
	ProductID   int64   `json:"productId"`
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	Spec        string  `json:"spec"`
	Unit        string  `json:"unit"`
	Stock       float64 `json:"stock"`
	Value       float64 `json:"value"`
	Ratio       float64 `json:"ratio"`
}

// ReportDetailRow 销售 / 采购明细行（单据明细粒度，一行一个商品）。
type ReportDetailRow struct {
	OrderID     int64   `json:"orderId"`
	OrderNo     string  `json:"orderNo"`
	OrderDate   string  `json:"orderDate"`
	PartyName   string  `json:"partyName"`
	Salesman    string  `json:"salesman"`
	ProductName string  `json:"productName"`
	Spec        string  `json:"spec"`
	Unit        string  `json:"unit"`
	Qty         float64 `json:"qty"`
	Price       float64 `json:"price"`
	Amount      float64 `json:"amount"`
}

// ReportDetailResult 明细结果集：行 + 合计，前端不必自己再算一遍。
type ReportDetailResult struct {
	Rows   []ReportDetailRow `json:"rows"`
	Total  int64             `json:"total"`
	Amount float64           `json:"amount"`
	Qty    float64           `json:"qty"`
	// Truncated 为 true 表示命中行数超过上限被截断（明细只展示前 N 行）
	Truncated bool `json:"truncated"`
}

// ReportContractRow 合同执行报表行：一份合同一行，带收付进度与质保金余额。
type ReportContractRow struct {
	ContractID           int64      `json:"contractId"`
	ContractNo           string     `json:"contractNo"`
	Name                 string     `json:"name"`
	BizType              string     `json:"bizType"`
	PartyName            string     `json:"partyName"`
	SignDate             time.Time  `json:"signDate"`
	Amount               float64    `json:"amount"`
	PlanCount            int        `json:"planCount"`
	PlanAmount           float64    `json:"planAmount"`
	SettledAmount        float64    `json:"settledAmount"`
	Outstanding          float64    `json:"outstanding"`
	RetentionAmount      float64    `json:"retentionAmount"`
	RetentionOutstanding float64    `json:"retentionOutstanding"`
	Stage                string     `json:"stage"`
	StageLabel           string     `json:"stageLabel"`
	AcceptedAt           *time.Time `json:"acceptedAt"`
	WarrantyEndAt        *time.Time `json:"warrantyEndAt"`
}

// ReportContractResult 合同执行报表：行 + 合计。
//
// PlanAmount 与 Amount 常常不等（合同可能只约定了几期、或含不收款条款），
// 两个数都返回，报表里并列显示 —— 只给一个「合计」会让人误以为合同金额就是应收。
type ReportContractResult struct {
	Rows                 []ReportContractRow `json:"rows"`
	Count                int                 `json:"count"`
	Amount               float64             `json:"amount"`
	PlanAmount           float64             `json:"planAmount"`
	SettledAmount        float64             `json:"settledAmount"`
	Outstanding          float64             `json:"outstanding"`
	RetentionAmount      float64             `json:"retentionAmount"`
	RetentionOutstanding float64             `json:"retentionOutstanding"`
	Truncated            bool                `json:"truncated"`
}

// ReportContractLedgerRow 合同收付款台账：一行一笔实际发生的收付款。
type ReportContractLedgerRow struct {
	SettleID   int64     `json:"settleId"`
	SettledAt  time.Time `json:"settledAt"`
	ContractNo string    `json:"contractNo"`
	Name       string    `json:"name"`
	BizType    string    `json:"bizType"`
	PartyName  string    `json:"partyName"`
	PlanSeq    int       `json:"planSeq"`
	PlanName   string    `json:"planName"`
	Amount     float64   `json:"amount"`
	Method     string    `json:"method"`
	Remark     string    `json:"remark"`
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

// ============================================================
// 合同
// ============================================================

// ContractFilter 合同列表筛选条件。
type ContractFilter struct {
	BizType string     // "" 全部 / purchase / sales
	Status  *int16     // 1 执行中 2 已完成 3 已终止
	Start   *time.Time // 按**签订日期**过滤，闭区间
	End     *time.Time
	PartyID int64 // 往来单位
	// Stage 按派生的履约阶段筛选：
	//   unaudited     待审核（status=0）
	//   pending       未验收（accepted_at 为空）
	//   accepting     已验收（accepted_at 非空）
	//   warranty      质保期内（warranty_end_at 未到）
	//   warranty_due  质保已到期（warranty_end_at 已过）
	Stage string
}

// ContractPlanReq 合同付款计划的一期。
//
// ID 有值表示这一期已经存在（编辑时原样回传），没有则是新增。
// 已有收付款的期次只允许改名称与备注，金额与比例必须保持不变 ——
// 改了会让「已收付累计」与合同金额对不上（见 service 层的校验）。
type ContractPlanReq struct {
	ID          int64   `json:"id" binding:"omitempty" label:"期次"`
	Seq         int     `json:"seq" binding:"required,min=1,max=99" label:"期次号"`
	Name        string  `json:"name" binding:"required,max=32" label:"期次名称"`
	Kind        int16   `json:"kind" binding:"omitempty,oneof=1 2 3 4 5" label:"期次类型"`
	Rate        float64 `json:"rate" binding:"gte=0,lte=100" label:"比例"`
	Amount      float64 `json:"amount" binding:"gte=0" label:"金额"`
	DueDate     string  `json:"dueDate" binding:"omitempty" label:"约定日期"`
	TriggerNode string  `json:"triggerNode" binding:"omitempty,oneof=sign accept warranty_end" label:"触发节点"`
	Remark      string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// ContractReq 合同新增 / 修改请求。
type ContractReq struct {
	// ContractNo 留空即自动生成 HT + 流水号；填了就用填的（重号会被拒）。
	ContractNo string `json:"contractNo" binding:"omitempty,max=32" label:"合同编号"`
	Name       string `json:"name" binding:"required,max=128" label:"合同名称"`
	BizType    string `json:"bizType" binding:"required,oneof=purchase sales" label:"合同类型"`
	PartyID    int64  `json:"partyId" binding:"required,gt=0" label:"往来单位"`
	SignDate   string `json:"signDate" binding:"required" label:"签订日期"`
	// StartDate / EndDate 履约起止，选填；两个都填时结束不能早于开始。
	StartDate string  `json:"startDate" binding:"omitempty" label:"开始日期"`
	EndDate   string  `json:"endDate" binding:"omitempty" label:"结束日期"`
	Amount    float64 `json:"amount" binding:"gte=0" label:"合同金额"`
	Status    *int16  `json:"status" binding:"omitempty,oneof=1 2 3" label:"状态"`
	Salesman  string  `json:"salesman" binding:"omitempty,max=64" label:"业务员"`
	Remark    string  `json:"remark" binding:"omitempty,max=255" label:"备注"`

	// 质保条款：质保期按月填，质保金可按比例折算成金额。
	// 两个都填时以金额为准（有的合同直接写「质保金 5 万元」而不是百分比）。
	WarrantyMonths  int     `json:"warrantyMonths" binding:"omitempty,min=0,max=120" label:"质保期(月)"`
	RetentionRate   float64 `json:"retentionRate" binding:"omitempty,gte=0,lte=100" label:"质保金比例"`
	RetentionAmount float64 `json:"retentionAmount" binding:"omitempty,gte=0" label:"质保金金额"`

	// Plans 付款计划：整份替换语义 —— 传什么就是什么，
	// 没出现的既有期次在「未发生收付款」时被删除，已发生收付款则拒绝。
	Plans []ContractPlanReq `json:"plans" binding:"omitempty,max=20,dive" label:"付款计划"`
}

// ContractAcceptReq 合同验收请求。验收会同时把质保到期日算出来。
type ContractAcceptReq struct {
	AcceptedAt     string `json:"acceptedAt" binding:"required" label:"验收日期"`
	AcceptResult   string `json:"acceptResult" binding:"required,oneof=合格 不合格 整改" label:"验收结论"`
	WarrantyMonths int    `json:"warrantyMonths" binding:"omitempty,min=0,max=120" label:"质保期(月)"`
	Remark         string `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// PlanSettleReq 合同付款期次的收付款请求。
type PlanSettleReq struct {
	PlanID    int64   `json:"planId" binding:"required,gt=0" label:"付款期次"`
	Amount    float64 `json:"amount" binding:"required,gt=0" label:"金额"`
	Method    string  `json:"method" binding:"omitempty,max=32" label:"结算方式"`
	SettledAt string  `json:"settledAt" binding:"omitempty" label:"收付日期"`
	Remark    string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// ContractLogRow 合同的操作记录。
//
// 直接复用系统操作日志（按请求路径归集），不再单独建合同变更表：
// 中间件已经把「谁、什么动作、什么请求体、什么结果」都记下了，
// 再存一份只会出现「两处记录不一致」的麻烦。
type ContractLogRow struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// SettleRecord 一条收付款流水（合同期次详情展开用）。
type SettleRecord struct {
	ID        int64     `json:"id"`
	Amount    float64   `json:"amount"`
	Method    string    `json:"method"`
	SettledAt time.Time `json:"settledAt"`
	Remark    string    `json:"remark"`
}

// ContractPlanItem 合同付款期次的返回行。
//
// Status 与 StatusLabel 都是派生的（未到期 / 待收付 / 部分 / 已结清），
// 库里不存 —— 改金额、改日期、登记收付款各处都不需要再去同步一个状态列。
type ContractPlanItem struct {
	ID            int64      `json:"id"`
	Seq           int        `json:"seq"`
	Name          string     `json:"name"`
	Kind          int16      `json:"kind"`
	KindLabel     string     `json:"kindLabel"`
	Rate          float64    `json:"rate"`
	Amount        float64    `json:"amount"`
	DueDate       *time.Time `json:"dueDate"`
	TriggerNode   string     `json:"triggerNode"`
	SettledAmount float64    `json:"settledAmount"`
	Outstanding   float64    `json:"outstanding"`
	Status        string     `json:"status"`
	StatusLabel   string     `json:"statusLabel"`
	Remark        string     `json:"remark"`
}

// ContractRow 合同列表行与详情（结构一致，详情只是单条）。
type ContractRow struct {
	ID         int64      `json:"id"`
	ContractNo string     `json:"contractNo"`
	Name       string     `json:"name"`
	BizType    string     `json:"bizType"`
	PartyID    int64      `json:"partyId"`
	PartyName  string     `json:"partyName"`
	SignDate   time.Time  `json:"signDate"`
	StartDate  *time.Time `json:"startDate"`
	EndDate    *time.Time `json:"endDate"`
	Amount     float64    `json:"amount"`
	Status     int16      `json:"status"`
	Salesman   string     `json:"salesman"`
	Operator   string     `json:"operator"`
	Remark     string     `json:"remark"`

	// 验收与质保
	AcceptedAt      *time.Time `json:"acceptedAt"`
	AcceptResult    string     `json:"acceptResult"`
	WarrantyMonths  int        `json:"warrantyMonths"`
	WarrantyEndAt   *time.Time `json:"warrantyEndAt"`
	RetentionRate   float64    `json:"retentionRate"`
	RetentionAmount float64    `json:"retentionAmount"`

	// 履约阶段（派生）：待审核 / 待验收 / 已验收 / 质保中 / 质保到期 / 已完成 / 已终止
	Stage      string `json:"stage"`
	StageLabel string `json:"stageLabel"`

	// 审核：谁在什么时候放行。待审核时两列为空。
	AuditedAt *time.Time `json:"auditedAt"`
	Auditor   string     `json:"auditor"`

	// 收付进度（合同期次汇总；没有计划时全为 0）
	PlanCount     int     `json:"planCount"`
	PlanAmount    float64 `json:"planAmount"`
	SettledAmount float64 `json:"settledAmount"`
	Outstanding   float64 `json:"outstanding"`
	// RetentionOutstanding 质保金尚未收付的部分，单列因为它是「以后才能要的钱」
	RetentionOutstanding float64 `json:"retentionOutstanding"`

	// Plans 仅详情返回，列表不带（避免一页 20 条合同拉出上百行期次）
	Plans []ContractPlanItem `json:"plans,omitempty"`

	// AttachmentCount 附件数量，列表按钮上显示角标（一次批量查出，不逐行请求）
	AttachmentCount int       `json:"attachmentCount"`
	CreatedAt       time.Time `json:"createdAt"`
}
