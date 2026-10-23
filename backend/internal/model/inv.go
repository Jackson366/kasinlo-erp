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

// 合同状态
//
// 0 待审核是「准入门槛」而不是审批流：合同一保存就会按付款计划生成应收 / 应付，
// 没有这道门槛时，录错一份合同等于当场虚增一笔债权，而终止它又能让债权凭空消失。
// 审核不改变合同内容，只表示「这份合同的钱可以入账了」。
const (
	InvContractPending    int16 = 0 // 待审核（不进应收应付、不能验收、不能收付款）
	InvContractActive     int16 = 1 // 执行中（已审核）
	InvContractFinished   int16 = 2 // 已完成
	InvContractTerminated int16 = 3 // 已终止
)

// 合同付款期次类型。质保金单列一类，因为它天然是「最后一期 + 有到期条件」，
// 与按进度付的款项在报表里要能分开统计（质保金余额是单独的考核口径）。
const (
	InvPlanAdvance   int16 = 1 // 预付款
	InvPlanProgress  int16 = 2 // 进度款
	InvPlanAccept    int16 = 3 // 验收款
	InvPlanRetention int16 = 4 // 质保金
	InvPlanOther     int16 = 5 // 其他
)

// 期次触发节点：按事件而非固定日期进入待收付。
// 空值表示「按约定日期」（due_date）判断。
const (
	InvPlanTriggerSign     = "sign"         // 合同签订即待收付
	InvPlanTriggerAccept   = "accept"       // 验收后待收付
	InvPlanTriggerWarranty = "warranty_end" // 质保到期后待收付
)

// 结算记录来源：单据结算 / 合同期次收付。
// 两者共用 inv_settlements 一张表：收付款是同一件事，拆表只会让
// 「账龄、台账、按往来单位汇总」三处各写两遍。
const (
	InvSettleSourceOrder = "order"
	InvSettleSourcePlan  = "plan"
)

// 附件归属的业务类型。三者共用一张 inv_attachments 表与同一个上传入口，
// 差异只在 biz_type —— 附件本身没有业务语义（就是一堆文件），
// 拆表只会让「上传/下载/删除」这套逻辑抄三遍。
const (
	InvAttachPurchase = "purchase"
	InvAttachSales    = "sales"
	InvAttachContract = "contract"
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
	// 参考进价 / 参考售价：商品级备忘价。具体成交价以规格-单位上的
	// 采购价 / 销售价为准；参考价用于开单前的估价与对比，不参与过账。
	RefPurchasePrice float64 `gorm:"type:numeric(14,2);default:0" json:"refPurchasePrice"`
	RefSalePrice     float64 `gorm:"type:numeric(14,2);default:0" json:"refSalePrice"`
	Status           int16   `gorm:"not null" json:"status"`
	Remark           string  `gorm:"size:255" json:"remark"`
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
	OrderID int64 `gorm:"index;not null" json:"orderId"`
	// PlanID 合同付款期次 ID。合同收付款与单据结算共用这张表：
	// 一笔记账行为的字段完全一致（金额、方式、日期、经办），
	// 分表只会让账龄与台账各算两遍。Source 标明这行挂在谁身上。
	PlanID    int64     `gorm:"index" json:"planId"`
	Source    string    `gorm:"size:16;not null;default:order" json:"source"` // order | plan
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

// InvContract 采购 / 销售合同台账。
//
// 合同是「先谈条件、后执行」的凭据：金额、起止日期、对方单位在这里留档，
// 扫描件 / 盖章 PDF 挂到 inv_attachments（biz_type=contract）。
//
// 刻意**不**把它做成单据的强制上游：一张合同常常对应多张采购 / 销售单，
// 也常出现「先开单、后补合同」。把 contract_id 做成单据必填字段，
// 会让开单多一道前置手续；真正的核对需求（这张合同执行了多少）
// 该由统计查询解决，而不是靠外键。需要时再加关联字段与汇总。
type InvContract struct {
	BaseModel
	ContractNo string `gorm:"size:32;not null" json:"contractNo"`
	Name       string `gorm:"size:128;not null" json:"name"`
	BizType    string `gorm:"size:16;not null;index" json:"bizType"` // purchase | sales
	PartyID    int64  `gorm:"index" json:"partyId"`
	PartyName  string `gorm:"size:128" json:"partyName"` // 冗余名称，列表免联表

	// SignDate 签订日期；StartDate / EndDate 是履约起止，选填。
	//
	// 起止日期用指针的理由同 InvOrder.DueDate：空值要落成 NULL，
	// 不能变成 0001-01-01 那种「两千年前就到期」的语义 —— 到期提醒、
	// 列表筛选、打印纸面都会因此出错。
	SignDate  time.Time  `gorm:"type:date" json:"signDate"`
	StartDate *time.Time `gorm:"type:date" json:"startDate"`
	EndDate   *time.Time `gorm:"type:date" json:"endDate"`

	Amount    float64 `gorm:"type:numeric(14,2);default:0" json:"amount"`
	Status    int16   `gorm:"not null;index" json:"status"` // 1 执行中 2 已完成 3 已终止
	Salesman  string  `gorm:"size:64" json:"salesman"`
	Remark    string  `gorm:"size:255" json:"remark"`
	CreatedBy int64   `json:"createdBy"`

	// ---- 审核 ----
	//
	// 只记「谁在什么时候放行」，不记审批意见：这是一个两态门槛，不是多级审批。
	// 反审核会把这两列清空（否则会留下「已审核但状态是待审核」的矛盾数据）。
	//
	// 时间列不写 type：PostgreSQL 没有 datetime（那是 MySQL 的类型），
	// 写死会让建表直接失败。交给 GORM 按驱动映射成 timestamptz。
	AuditedAt *time.Time `json:"auditedAt"`
	Auditor   string     `gorm:"size:64" json:"auditor"`

	// ---- 验收与质保 ----
	//
	// 四个字段都可有可无：小合同常常「签了、干完、收钱」，不验收入库也不留质保金。
	// 所以不新增状态码（旧数据 status=1 的含义不变），界面上的
	// 「待验收 → 已验收 → 质保中 → 质保到期」全部由这两个日期派生 ——
	// 派生就不会出现「状态字段说质保中、日期说早过期」的自相矛盾。
	AcceptedAt     *time.Time `gorm:"type:date" json:"acceptedAt"`              // 验收日期，空=未验收
	AcceptResult   string     `gorm:"size:16" json:"acceptResult"`              // 合格 / 不合格 / 整改
	WarrantyMonths int        `gorm:"not null;default:0" json:"warrantyMonths"` // 质保期（月）
	// WarrantyEndAt 保存时按「验收日期 + 质保月数」算好落库，而不是每次查询现算：
	// 逾期提醒、质保金到期筛选都要在 SQL 里比较这个日期。
	WarrantyEndAt   *time.Time `gorm:"type:date" json:"warrantyEndAt"`
	RetentionRate   float64    `gorm:"type:numeric(5,2);default:0" json:"retentionRate"`    // 质保金比例 %
	RetentionAmount float64    `gorm:"type:numeric(14,2);default:0" json:"retentionAmount"` // 质保金金额
}

// TableName 指定表名。
func (InvContract) TableName() string { return "inv_contracts" }

// InvContractPlan 合同付款计划（一个期次一行）。
//
// 它同时是「应收 / 应付」的第二类来源：合同签下 100 万、约定 30/40/20/10 四期，
// 这四行就是四笔应收（销售合同）或应付（采购合同）。所以这里冗余了
// BizType / PartyID / PartyName —— 应收应付列表要按往来单位汇总、按合同号搜索，
// 每次联三张表（plans → contracts → customers）得不偿失。
//
// 刻意**不存**状态字段：一期是「未到期 / 待收付 / 部分 / 已结清」，完全由
// amount、settled_amount、due_date、trigger_node 与合同的验收/质保日期推导。
// 存一份状态就要在每个改金额、改日期、登记收付款的地方同步刷新，
// 漏一处就出现「已收满却显示待收付」—— 这类漂移对账时最难解释。
// 这与订单的结算口径一致（见 InvOrder 注释：结算状态由差额推导，不入库）。
type InvContractPlan struct {
	BaseModel
	ContractID int64  `gorm:"not null;index" json:"contractId"`
	BizType    string `gorm:"size:16;index" json:"bizType"` // purchase | sales
	PartyID    int64  `gorm:"index" json:"partyId"`
	PartyName  string `gorm:"size:128" json:"partyName"`

	Seq  int    `gorm:"not null" json:"seq"`            // 期次号，同一合同内唯一（服务层校验，软删下不建唯一索引）
	Name string `gorm:"size:32;not null" json:"name"`   // 如「预付款」「尾款」「质保金」
	Kind int16  `gorm:"not null;default:5" json:"kind"` // 1 预付 2 进度 3 验收 4 质保金 5 其他

	// Rate 与 Amount 二选一填：填比例自动算金额（按合同金额），填金额则比例留空。
	// 两者都存是为了让「30%」这个约定在合同金额变更后还能被看出来。
	Rate   float64 `gorm:"type:numeric(5,2);default:0" json:"rate"`
	Amount float64 `gorm:"type:numeric(14,2);default:0" json:"amount"`

	// DueDate 约定收付日期；TriggerNode 按事件触发（签订 / 验收 / 质保到期）。
	// 两者都留空 = 不限期，只在合同页作为计划行展示，不进逾期统计。
	DueDate     *time.Time `gorm:"type:date" json:"dueDate"`
	TriggerNode string     `gorm:"size:16" json:"triggerNode"`

	SettledAmount float64 `gorm:"type:numeric(14,2);default:0" json:"settledAmount"` // 已收 / 已付累计
	Remark        string  `gorm:"size:255" json:"remark"`
	CreatedBy     int64   `json:"createdBy"`
}

// TableName 指定表名。
func (InvContractPlan) TableName() string { return "inv_contract_plans" }

// InvAttachment 业务附件：采购单 / 销售单 / 合同的图片与文件。
//
// 三类业务共用一张表与一个上传入口，差异只有 biz_type —— 附件本身没有业务语义
// （就是一堆文件），拆表只会把「上传 / 下载 / 删除」这套逻辑抄三遍。
//
// StoredPath 是**相对**存储根目录的路径（如 purchase/12/2026/07/ab12.jpg），
// 不落绝对路径：换机器部署、改存储目录时，库里那条记录还能用。
type InvAttachment struct {
	BaseModel
	BizType  string `gorm:"size:16;not null;index:idx_inv_attachments_biz" json:"bizType"`
	BizID    int64  `gorm:"not null;index:idx_inv_attachments_biz" json:"bizId"`
	FileName string `gorm:"size:255;not null" json:"fileName"` // 原始文件名，下载时用它

	// StoredPath 不出现在接口里：客户端拿 id 走下载接口，
	// 既不需要、也不该知道磁盘结构。
	StoredPath string `gorm:"size:255;not null" json:"-"`

	Size       int64  `gorm:"not null" json:"size"`
	Mime       string `gorm:"size:128" json:"mime"`
	UploadedBy int64  `json:"uploadedBy"`
	Uploader   string `gorm:"size:64" json:"uploader"`
}

// TableName 指定表名。
func (InvAttachment) TableName() string { return "inv_attachments" }
