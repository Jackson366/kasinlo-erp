package model

import "time"

// 本文件是进销存（inv）业务的数据模型。
//
// 设计取舍：采购与销售共用一张单据表 inv_orders，用 biz_type 区分；
// 两者的字段、明细、金额与结算语义完全一致，拆成两张表只会让
// 「明细、结算、报表」这三块各写两遍。交易对手（客户/供应商）
// 由 biz_type 推导：purchase -> 供应商，sales -> 客户。

// 单据业务类型
const (
	InvBizPurchase = "purchase" // 采购
	InvBizSales    = "sales"    // 销售
)

// 出入库方向
const (
	InvStockIn  = "in"  // 入库
	InvStockOut = "out" // 出库
)

// 出入库来源。manual 表示手工录入的盘点调拨类单据。
const (
	InvSourcePurchase = "purchase"
	InvSourceSales    = "sales"
	InvSourceManual   = "manual"
)

// 单据状态
const (
	InvStatusPending  int16 = 1 // 待处理（待入库 / 待出库）
	InvStatusDone     int16 = 2 // 已完成（已入库 / 已出库）
	InvStatusCanceled int16 = 3 // 已取消
)

// 结算状态（由「单据金额 - 已结算金额」推导，不入库）
const (
	InvSettleUnpaid  = "unpaid"  // 未结清
	InvSettlePartial = "partial" // 部分结算
	InvSettlePaid    = "paid"    // 已结清
)

// InvCustomer 客户档案。
type InvCustomer struct {
	BaseModel
	Code    string `gorm:"size:32;not null" json:"code"`  // 客户编号
	Name    string `gorm:"size:128;not null" json:"name"` // 客户名称
	Contact string `gorm:"size:64" json:"contact"`        // 联系人
	Phone   string `gorm:"size:32" json:"phone"`          // 联系电话
	Address string `gorm:"size:255" json:"address"`       // 地址
	TaxNo   string `gorm:"size:64" json:"taxNo"`          // 开票信息（统一社会信用代码）
	Status  int16  `gorm:"not null" json:"status"`        // 1 启用 0 禁用
	Remark  string `gorm:"size:255" json:"remark"`        // 备注
}

// TableName 指定表名。
func (InvCustomer) TableName() string { return "inv_customers" }

// InvSupplier 供应商档案。
type InvSupplier struct {
	BaseModel
	Code    string `gorm:"size:32;not null" json:"code"`
	Name    string `gorm:"size:128;not null" json:"name"`
	Contact string `gorm:"size:64" json:"contact"`
	Phone   string `gorm:"size:32" json:"phone"`
	Address string `gorm:"size:255" json:"address"`
	TaxNo   string `gorm:"size:64" json:"taxNo"`
	Status  int16  `gorm:"not null" json:"status"`
	Remark  string `gorm:"size:255" json:"remark"`
}

// TableName 指定表名。
func (InvSupplier) TableName() string { return "inv_suppliers" }

// InvCategory 商品分类字典。
//
// 分类从「商品表上的一列自由文本」收成一张字典表：同一个分类被写成
// 「钢材 / 钢材 」这样的变体时，报表的品类占比会把它拆成两条，而
// 这一列又没有任何地方能改名、排序、停用。收表之后分类才是一个可维护的对象。
//
// 支持多级父子分类（ParentID，0 表示根）。现场的分类经常天然带层级
// （钢材 → 螺纹钢 / 角钢），压成一级会让「螺纹钢」和「角钢」并列在同一个
// 列表里，商品一多就找不到。树由前端按 parent_id 组装，后端只存父子关系。
type InvCategory struct {
	BaseModel
	ParentID int64  `gorm:"not null;default:0;index" json:"parentId"` // 上级分类，0 = 根
	Name     string `gorm:"size:64;not null" json:"name"`             // 分类名称
	Sort     int    `gorm:"not null;default:0" json:"sort"`
	Status   int16  `gorm:"not null" json:"status"` // 1 启用 0 停用
	Remark   string `gorm:"size:255" json:"remark"`
}

// 分类名称的唯一性是「同一父级下不重名」——不同父级下的「标准件」是两个分类，
// 全局唯一会把多级树逼回一级。
func (InvCategory) TableName() string { return "inv_categories" }

// InvProduct 商品档案（主数据头）。
//
// 档案本身只回答「这是什么」：编码、名称、分类。**规格、单位、价格、库存
// 都不在这一层** —— 一个商品可以有多规格，每个规格又能按多个单位计量
// （1 箱 = 10 个），把它们压在一行里，要么只能记一个规格（旧的缺陷），
// 要么库存出现两套账。分层之后职责是唯一的：
//
//	InvProduct     身份：是什么
//	InvProductSpec 库存桶：某个规格现在有多少（按基本单位）
//	InvProductUnit 计量与定价：这个规格可以怎么数、怎么定价
type InvProduct struct {
	BaseModel
	Code       string `gorm:"size:32;not null" json:"code"`  // 商品编码
	Name       string `gorm:"size:128;not null" json:"name"` // 商品名称
	CategoryID int64  `gorm:"index" json:"categoryId"`       // 商品分类
	// Barcode 商品条码（EAN-13 / Code128 等），选填。
	//
	// 不加唯一索引：同一商品常有「箱码 / 单品码」两个条码，供应商换包装也会
	// 改码，历史上还可能出现重复贴错的场景。条码的作用是**扫得快**，
	// 不是做唯一键 —— 拿它当唯一键会让正常录入被 409 挡住，
	// 而库里真正的唯一性早就由 Code 保证了。
	Barcode string `gorm:"size:64;index" json:"barcode"`
	Status  int16  `gorm:"not null" json:"status"`
	Remark  string `gorm:"size:255" json:"remark"`
}

// TableName 指定表名。
func (InvProduct) TableName() string { return "inv_products" }

// InvProductSpec 商品规格。规格是**库存的归属单位**：同一个商品的不同规格
// 各自独立库存、独立预警下限。
//
// Stock 一律以「基本单位」计数（见 InvProductUnit.IsBase）。只有把不同
// 计量单位折算到同一个基准上，「φ20 还剩多少」才是一个确定的数 ——
// 否则「2 根」和「12 米」放在一起既不能相加，也不能比较。
type InvProductSpec struct {
	BaseModel
	ProductID int64   `gorm:"index;not null" json:"productId"`
	Spec      string  `gorm:"size:64;not null" json:"spec"`              // 规格，如 φ20 / 6206
	Stock     float64 `gorm:"type:numeric(14,4);default:0" json:"stock"` // 当前库存（基本单位）
	WarnStock float64 `gorm:"type:numeric(14,4);default:0" json:"warnStock"`
	Sort      int     `gorm:"not null;default:0" json:"sort"`
	Status    int16   `gorm:"not null" json:"status"`
}

// TableName 指定表名。
func (InvProductSpec) TableName() string { return "inv_product_specs" }

// InvProductUnit 规格下的计量单位（含换算率与定价）。
//
// Rate 的含义是「1 个该单位 = Rate 个基本单位」：基本单位自身 Rate = 1。
// 换算率挂在单位上而不是商品上，是因为它跟规格相关 —— 同样叫「箱」，
// φ20 的管一箱装 6 根、φ25 的一箱装 4 根，挂到商品层就没法表达。
type InvProductUnit struct {
	BaseModel
	SpecID        int64   `gorm:"index;not null" json:"specId"`
	Unit          string  `gorm:"size:16;not null" json:"unit"`                      // 单位名，如 根 / 米 / 箱
	Rate          float64 `gorm:"type:numeric(14,4);not null;default:1" json:"rate"` // 1 该单位 = Rate 个基本单位
	IsBase        bool    `gorm:"not null;default:false" json:"isBase"`              // 是否基本单位（每个规格有且仅有一个）
	PurchasePrice float64 `gorm:"type:numeric(14,4);default:0" json:"purchasePrice"` // 按该单位计价的采购价
	SalePrice     float64 `gorm:"type:numeric(14,4);default:0" json:"salePrice"`     // 按该单位计价的销售价
	Sort          int     `gorm:"not null;default:0" json:"sort"`
	Status        int16   `gorm:"not null" json:"status"`
}

// TableName 指定表名。
func (InvProductUnit) TableName() string { return "inv_product_units" }

// InvOrder 采购 / 销售单据主表。
type InvOrder struct {
	BaseModel
	OrderNo   string    `gorm:"size:32;not null" json:"orderNo"`       // 单据编号
	BizType   string    `gorm:"size:16;not null;index" json:"bizType"` // purchase | sales
	PartyID   int64     `gorm:"index" json:"partyId"`                  // 供应商 / 客户 ID
	PartyName string    `gorm:"size:128" json:"partyName"`             // 冗余名称，列表免联表
	OrderDate time.Time `gorm:"type:date" json:"orderDate"`            // 单据日期

	// DueDate 是结算到期日，选填。
	//
	// 必须是指针：它是**可选**字段，未填写时数据层要表达「没有到期日」。
	// 用非指针 time.Time 只能写入 Go 零值（0001-01-01），而这个值落库后
	// 语义上与「已过期两千年」无法区分 —— 应收应付的逾期汇总会把空到期日
	// 的单据一并算进去，打印纸面也会多出一行「到期日：0001-01-01」。
	// 指针 + 可空列让「未填」在数据库里就是 NULL，任何比较都天然为假。
	DueDate       *time.Time `gorm:"type:date" json:"dueDate"`
	Status        int16      `gorm:"not null;index" json:"status"` // 1 待处理 2 已完成 3 已取消
	TotalAmount   float64    `gorm:"type:numeric(14,2);default:0" json:"totalAmount"`
	SettledAmount float64    `gorm:"type:numeric(14,2);default:0" json:"settledAmount"`
	Remark        string     `gorm:"size:255" json:"remark"`

	// Salesman 业务员，自由文本。
	//
	// 不建成外键指向 sys_users：跑业务的常常是没有后台账号的人（门店、外包、
	// 已经离职但历史单据还要能查的人）。挂外键后这类人要么建不出单，要么
	// 离职删账号时连带把历史单据的归属抹掉 —— 那正是最需要留痕的地方。
	Salesman  string `gorm:"size:64" json:"salesman"`
	CreatedBy int64  `json:"createdBy"`
}

// TableName 指定表名。
func (InvOrder) TableName() string { return "inv_orders" }

// InvOrderItem 单据明细行。
//
// 规格与单位在明细里是**快照**（连同换算率一起存），而不是只存外键：
// 历史单据的纸面必须永远打得出来，商品档案后来改了规格名或换算率，
// 都不该让三年前那张单子上的数字跟着变。SpecID / UnitID 保留下来，
// 用于「这个规格被哪些单据引用过」的引用校验。
type InvOrderItem struct {
	BaseModel
	OrderID     int64   `gorm:"index;not null" json:"orderId"`
	ProductID   int64   `gorm:"index" json:"productId"`
	ProductName string  `gorm:"size:128" json:"productName"`
	SpecID      int64   `gorm:"index" json:"specId"`
	Spec        string  `gorm:"size:64" json:"spec"`
	UnitID      int64   `json:"unitId"`
	Unit        string  `gorm:"size:16" json:"unit"`
	Rate        float64 `gorm:"type:numeric(14,4);default:1" json:"rate"`    // 成交单位相对基本单位的换算率（快照）
	Qty         float64 `gorm:"type:numeric(14,2);default:0" json:"qty"`     // 按成交单位计的数量
	QtyBase     float64 `gorm:"type:numeric(14,4);default:0" json:"qtyBase"` // 折算成基本单位的数量 = Qty × Rate
	Price       float64 `gorm:"type:numeric(14,2);default:0" json:"price"`
	Amount      float64 `gorm:"type:numeric(14,2);default:0" json:"amount"`
}

// TableName 指定表名。
func (InvOrderItem) TableName() string { return "inv_order_items" }

// InvSettlement 收付款记录。应收与应付共用，方向由 BizType 推导。
type InvSettlement struct {
	BaseModel
	OrderID   int64     `gorm:"index;not null" json:"orderId"`
	BizType   string    `gorm:"size:16;index" json:"bizType"`
	PartyID   int64     `gorm:"index" json:"partyId"`
	Amount    float64   `gorm:"type:numeric(14,2);default:0" json:"amount"`
	Method    string    `gorm:"size:32" json:"method"` // 现金 / 银行转账 / 承兑
	SettledAt time.Time `gorm:"type:date" json:"settledAt"`
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedBy int64     `json:"createdBy"`
}

// TableName 指定表名。
func (InvSettlement) TableName() string { return "inv_settlements" }

// InvStockOrder 出入库单据主表。过账时会同步调整商品库存。
type InvStockOrder struct {
	BaseModel
	OrderNo    string    `gorm:"size:32;not null" json:"orderNo"`
	Direction  string    `gorm:"size:8;not null;index" json:"direction"` // in | out
	SourceType string    `gorm:"size:16" json:"sourceType"`              // purchase | sales | manual
	SourceID   int64     `json:"sourceId"`
	OrderDate  time.Time `gorm:"type:date" json:"orderDate"`
	Status     int16     `gorm:"not null" json:"status"` // 1 已过账 2 已作废
	Remark     string    `gorm:"size:255" json:"remark"`
	// Salesman 业务员：手工单由填单人选，审核自动生成的单据沿用来单的业务员。
	Salesman  string `gorm:"size:64" json:"salesman"`
	CreatedBy int64  `json:"createdBy"`
}

// TableName 指定表名。
func (InvStockOrder) TableName() string { return "inv_stock_orders" }

// InvStockItem 出入库明细行。
//
// 与单据明细同构：规格 / 单位 / 换算率都是快照，库存按 QtyBase 增减。
// 少了 QtyBase 这一列，出库「2 箱」就被当成 2 个扣掉了。
type InvStockItem struct {
	BaseModel
	StockOrderID int64   `gorm:"index;not null" json:"stockOrderId"`
	ProductID    int64   `gorm:"index" json:"productId"`
	ProductName  string  `gorm:"size:128" json:"productName"`
	SpecID       int64   `gorm:"index" json:"specId"`
	Spec         string  `gorm:"size:64" json:"spec"`
	UnitID       int64   `json:"unitId"`
	Unit         string  `gorm:"size:16" json:"unit"`
	Rate         float64 `gorm:"type:numeric(14,4);default:1" json:"rate"`
	Qty          float64 `gorm:"type:numeric(14,2);default:0" json:"qty"`
	QtyBase      float64 `gorm:"type:numeric(14,4);default:0" json:"qtyBase"`
}

// TableName 指定表名。
func (InvStockItem) TableName() string { return "inv_stock_items" }

// InvPrintTemplate 单据打印模板。
//
// Content 是版式配置的 JSON（打印页的标题、明细列显隐、字号、页脚等），
// 由前端「模板设计」编辑并消费，后端只存取不解释——列怎么渲染是打印视图的事，
// 收在 JSON 里可以随设计器迭代增删字段而不用改表结构。
type InvPrintTemplate struct {
	BaseModel
	Code    string `gorm:"size:32;not null" json:"code"`
	Name    string `gorm:"size:64;not null" json:"name"`
	DocType string `gorm:"size:32" json:"docType"` // 关联单据类型
	Paper   string `gorm:"size:8" json:"paper"`    // A4 | A5
	Content string `gorm:"type:text" json:"content"`
	Status  int16  `gorm:"not null" json:"status"`
	Remark  string `gorm:"size:255" json:"remark"`
}

// TableName 指定表名。
func (InvPrintTemplate) TableName() string { return "inv_print_templates" }
