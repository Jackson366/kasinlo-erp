// ============================================================
// 与后端约定的通用类型定义
// 后端统一响应结构：{ code, message, data, traceId }
// ============================================================

/** 统一响应体 */
export interface ApiResult<T = unknown> {
  code: number
  message: string
  data: T
  traceId?: string
}

/** 分页数据结构 */
export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  pageSize: number
}

/** 分页查询参数 */
export interface PageQuery {
  page?: number
  pageSize?: number
  keyword?: string
  orderBy?: string
  order?: 'asc' | 'desc'
}

/** 通用启用状态：1 启用 0 禁用 */
export type Status = 0 | 1

/** 数据范围：1 全部 2 本部门 3 本部门及以下 4 仅本人 */
export type DataScope = 1 | 2 | 3 | 4

// ---------------- 认证相关 ----------------

export interface LoginParams {
  username: string
  password: string
}

export interface LoginResult {
  accessToken: string
  refreshToken: string
  expiresIn: number
  tokenType: string
  user: UserIdentity
}

export interface UserIdentity {
  userId: number
  username: string
  nickname: string
  avatar: string
  deptId: number | null
  roleIds: number[]
  roleCodes: string[]
  dataScope: DataScope
  isSuper: boolean
}

export interface ProfileInfo {
  id: number
  username: string
  nickname: string
  email: string
  phone: string
  avatar: string
  deptId: number | null
  deptName: string
  status: Status
  roles: string[]
  roleCodes: string[]
  isSuper: boolean
  permissions: string[]
}

// ---------------- 动态路由 ----------------

export interface RouteMeta {
  title: string
  icon: string
  hidden: boolean
  keepAlive: boolean
  isExternal: boolean
}

export interface RouteRecord {
  id: number
  name: string
  path: string
  component: string
  redirect?: string
  meta: RouteMeta
  children?: RouteRecord[]
}

// ---------------- 用户 ----------------

export interface UserItem {
  id: number
  username: string
  nickname: string
  email: string
  phone: string
  avatar: string
  deptId: number | null
  deptName: string
  status: Status
  remark: string
  roleIds: number[]
  roleNames: string[]
  lastLoginAt: string | null
  lastLoginIp: string
  createdAt: string
}

export interface UserQuery extends PageQuery {
  deptId?: number
  status?: Status
  roleId?: number
}

/** 用户导出条件：在列表筛选条件之上附加「按选中行导出」 */
export interface UserExportQuery extends UserQuery {
  /** 选中项 ID，逗号分隔；为空表示按筛选条件导出全部 */
  ids?: string
}

/** 单行导入校验结果 */
export interface ImportRow {
  /** Excel 中的实际行号（含表头，从 1 开始），便于用户直接定位 */
  row: number
  username: string
  nickname: string
  /** 为空数组表示该行校验通过 */
  errors: string[]
}

/** 导入（或导入预览）结果 */
export interface ImportResult {
  /** 数据行总数（不含表头） */
  total: number
  valid: number
  invalid: number
  /** 实际写入的行数，预览阶段恒为 0 */
  created: number
  /** 逐行明细 */
  rows: ImportRow[]
}

export interface UserPayload {
  username?: string
  password?: string
  nickname: string
  email?: string
  phone?: string
  deptId?: number | null
  status?: Status
  roleIds?: number[]
  remark?: string
}

// ---------------- 角色 ----------------

export interface RoleItem {
  id: number
  name: string
  code: string
  sort: number
  status: Status
  dataScope: DataScope
  isBuiltin: boolean
  remark: string
  userCount: number
  menuIds: number[]
  createdAt: string
}

export interface RolePayload {
  name: string
  code?: string
  sort?: number
  status?: Status
  dataScope?: DataScope
  remark?: string
  menuIds?: number[]
}

export interface RoleOption {
  id: number
  name: string
  code: string
}

// ---------------- 菜单 ----------------

export type MenuType = 'dir' | 'menu' | 'button'

export interface MenuNode {
  id: number
  parentId: number
  name: string
  path: string
  component: string
  redirect: string
  title: string
  icon: string
  type: MenuType
  permission: string
  sort: number
  hidden: boolean
  keepAlive: boolean
  isExternal: boolean
  status: Status
  children?: MenuNode[]
}

export interface MenuPayload {
  parentId: number
  name: string
  path?: string
  component?: string
  redirect?: string
  title: string
  icon?: string
  type: MenuType
  permission?: string
  sort?: number
  hidden?: boolean
  keepAlive?: boolean
  isExternal?: boolean
  status?: Status
}

// ---------------- 部门 ----------------

export interface DeptNode {
  id: number
  parentId: number
  name: string
  leader: string
  phone: string
  email: string
  sort: number
  status: Status
  userCount: number
  createdAt: string
  children?: DeptNode[]
}

export interface DeptPayload {
  parentId: number
  name: string
  leader?: string
  phone?: string
  email?: string
  sort?: number
  status?: Status
}

// ---------------- 日志 ----------------

export interface OperationLogItem {
  id: number
  userId: number
  username: string
  module: string
  action: string
  method: string
  path: string
  ip: string
  userAgent: string
  request: string
  status: number
  latencyMs: number
  errorMsg: string
  createdAt: string
}

export interface LoginLogItem {
  id: number
  userId: number
  username: string
  ip: string
  userAgent: string
  success: boolean
  message: string
  createdAt: string
}

// ---------------- 仪表盘 ----------------

export interface DashboardOverview {
  userCount: number
  roleCount: number
  deptCount: number
  menuCount: number
  todayLogins: number
  todayOps: number
  todayFailures: number
  activeUserWeek: number
}

/** 仪表盘异常信号 */
export interface DashboardAlert {
  level: 'warning' | 'danger'
  signal: string
  message: string
}

export interface TrendPoint {
  date: string
  logins: number
  opers: number
  failCount: number
}

export interface ModuleStat {
  module: string
  total: number
}

// ============================================================
// 进销存（inv）
// ============================================================

/** 单据业务类型 */
export type InvBizType = 'purchase' | 'sales'
/** 出入库方向 */
export type InvStockDirection = 'in' | 'out'
/** 单据状态：1 待处理 2 已完成 3 已取消 */
export type InvOrderStatus = 1 | 2 | 3
/** 结算状态 */
export type InvSettleStatus = 'unpaid' | 'partial' | 'paid'

/** 客户 / 供应商 / 商品 通用价格格式说明：金额均为元，保留两位小数 */

/** 客户 / 供应商列表行 */
export interface InvPartyRow {
  id: number
  code: string
  name: string
  contact: string
  phone: string
  address: string
  taxNo: string
  status: Status
  remark: string
  /** 客户为应收余额，供应商为应付余额 */
  balance: number
  orderCount: number
  createdAt: string
}

/** 客户 / 供应商载荷。code 留空由后端自动生成（KH#### / GYS####） */
export interface InvPartyPayload {
  code?: string
  name: string
  contact?: string
  phone?: string
  address?: string
  taxNo?: string
  status?: Status
  remark?: string
}

/** 商品分类列表行 */
export interface InvCategoryRow {
  id: number
  /** 上级分类，0 = 根 */
  parentId: number
  name: string
  sort: number
  status: Status
  remark: string
  /** 引用该分类的商品数；>0 时不允许删除 */
  productCount: number
  /** 直接子分类数；>0 时不允许删除（否则子分类会集体变成根） */
  childCount: number
  createdAt: string
}

/** 商品分类载荷 */
export interface InvCategoryPayload {
  /** 上级分类，0 = 根 */
  parentId: number
  name: string
  sort?: number
  status?: Status
  remark?: string
}

/** 商品分类下拉项。parentId 让前端能渲染成树形下拉 */
export interface InvCategoryOption {
  id: number
  parentId: number
  name: string
}

/** 计量单位（商品档案与单据明细共用） */
export interface InvProductUnit {
  id: number
  unit: string
  /** 1 个该单位 = rate 个基本单位（基本单位自身恒为 1） */
  rate: number
  isBase: boolean
  purchasePrice: number
  salePrice: number
  status: Status
}

/** 规格（库存的归属层） */
export interface InvProductSpec {
  id: number
  spec: string
  /** 当前库存，以基本单位计 */
  stock: number
  warnStock: number
  warn: boolean
  status: Status
  units: InvProductUnit[]
}

/** 商品列表行（商品管理页） */
export interface InvProductRow {
  id: number
  code: string
  name: string
  /** 商品条码（选填，扫描/粘贴均可） */
  barcode: string
  categoryId: number
  categoryName: string
  /** 参考进价 / 参考售价：商品级备忘价（成交价以规格-单位为准） */
  refPurchasePrice: number
  refSalePrice: number
  specCount: number
  unitCount: number
  totalStock: number
  warn: boolean
  status: Status
  remark: string
  createdAt: string
  specs: InvProductSpec[]
}

/** 计量单位（提交用，不含 ID：单位行按「删除 + 重建」整体覆盖） */
export interface InvProductUnitPayload {
  unit: string
  rate: number
  isBase: boolean
  purchasePrice?: number
  salePrice?: number
  status?: Status
}

/** 规格（提交用）。id 由编辑态回传，用来把「改名」与「删旧建新」区分开 */
export interface InvProductSpecPayload {
  id?: number
  spec: string
  warnStock?: number
  status?: Status
  units: InvProductUnitPayload[]
}

/** 商品新增 / 修改载荷。库存不在表单里 —— 它由出入库单据驱动。
 *  code 留空由后端自动生成（SP####）；参考进 / 售价为商品级备忘价 */
export interface InvProductPayload {
  code?: string
  name: string
  /** 商品条码（选填）；空串表示清空 */
  barcode?: string
  categoryId?: number
  refPurchasePrice?: number
  refSalePrice?: number
  status?: Status
  remark?: string
  specs: InvProductSpecPayload[]
}

/** 单据明细可选的计量单位 */
export interface InvPickUnit {
  id: number
  unit: string
  rate: number
  isBase: boolean
  purchasePrice: number
  salePrice: number
}

/** 单据明细可选的规格 */
export interface InvPickSpec {
  id: number
  spec: string
  stock: number
  units: InvPickUnit[]
}

/** 单据明细的商品 → 规格 → 单位 级联选项 */
export interface InvProductPick {
  id: number
  code: string
  name: string
  /** 商品条码：开单时可以按条码模糊匹配，扫码枪输入即等于输入条码 */
  barcode: string
  /** 参考进价 / 参考售价：>0 时作为开单默认单价（优先于单位成交价） */
  refPurchasePrice: number
  refSalePrice: number
  specs: InvPickSpec[]
}

/**
 * 商品级联选项的返回体。
 * truncated 为 true 表示商品数超过单次上限、下拉里只是一部分 ——
 * 前端必须提示「请用关键字搜索」，否则用户会以为系统没录这个商品。
 */
export interface InvProductPickResult {
  items: InvProductPick[]
  total: number
  truncated: boolean
}

/** 同名商品提示行（录商品时防重复） */
export interface InvProductDupRow {
  id: number
  code: string
  name: string
  categoryName: string
  specSummary: string
  status: Status
}

/** 库存查询行。粒度是规格：库存记在规格上，按基本单位计数 */
export interface InvInventoryRow {
  productId: number
  productCode: string
  productName: string
  categoryName: string
  specId: number
  spec: string
  unitId: number
  /** 基本单位名（库存的计数单位） */
  unit: string
  stock: number
  warnStock: number
  warn: boolean
  purchasePrice: number
}

/** 单据明细行（请求） */
export interface InvOrderItemPayload {
  productId: number
  specId: number
  unitId: number
  qty: number
  price: number
}

/** 单据新增 / 修改载荷。orderNo 留空由后端自动生成（CG/XS + 日期 + 流水） */
export interface InvOrderPayload {
  bizType: InvBizType
  orderNo?: string
  partyId: number
  orderDate: string
  dueDate?: string
  remark?: string
  /** 业务员（自由文本，可不填） */
  salesman?: string
  items: InvOrderItemPayload[]
}

/** 单据明细行（响应） */
export interface InvOrderItem {
  id: number
  productId: number
  /** 商品编码，来自商品档案补齐（商品已删则留空） */
  productCode: string
  productName: string
  specId: number
  spec: string
  unitId: number
  unit: string
  /** 成交单位相对基本单位的换算率（下单时的快照） */
  rate: number
  qty: number
  /** 折算成基本单位的数量 = qty × rate */
  qtyBase: number
  price: number
  amount: number
}

/** 单据列表行 */
export interface InvOrderRow {
  id: number
  orderNo: string
  bizType: InvBizType
  partyId: number
  partyName: string
  orderDate: string
  /** 结算到期日（选填）。未填写时为 null，不参与逾期判定，也不打印 */
  dueDate: string | null
  status: InvOrderStatus
  totalAmount: number
  settledAmount: number
  settleStatus: InvSettleStatus
  itemCount: number
  remark: string
  /** 业务员 */
  salesman: string
  /** 制单人显示名（昵称优先，回落用户名） */
  operator: string
  createdAt: string
}

/** 单据详情 */
export interface InvOrderDetail extends InvOrderRow {
  items: InvOrderItem[]
}

/** 出入库明细行（请求） */
export interface InvStockItemPayload {
  productId: number
  specId: number
  unitId: number
  qty: number
}

/** 出入库单据载荷。orderNo 留空由后端自动生成（CK + 日期 + 流水） */
export interface InvStockOrderPayload {
  direction: InvStockDirection
  orderNo?: string
  sourceType?: 'purchase' | 'sales' | 'manual'
  orderDate: string
  remark?: string
  /** 业务员（自由文本，可不填） */
  salesman?: string
  items: InvStockItemPayload[]
}

/** 出入库单据列表行 */
export interface InvStockOrderRow {
  id: number
  orderNo: string
  direction: InvStockDirection
  sourceType: 'purchase' | 'sales' | 'manual'
  sourceId: number
  orderDate: string
  status: number
  itemCount: number
  totalQty: number
  remark: string
  /** 业务员 */
  salesman: string
  /** 制单人显示名（昵称优先，回落用户名） */
  operator: string
  createdAt: string
}

/** 应收 / 应付明细行 */
export interface InvFinanceRow {
  /** 来源：order 单据 / plan 合同付款期次 */
  source: 'order' | 'plan'
  orderId: number
  orderNo: string
  contractId: number
  contractNo: string
  planId: number
  planSeq: number
  planName: string
  partyId: number
  partyName: string
  orderDate: string
  /** 结算到期日（选填）。未填写时为 null */
  dueDate: string | null
  totalAmount: number
  settledAmount: number
  outstanding: number
  settleStatus: InvSettleStatus
  overdue: boolean
}

/** 应收 / 应付汇总 */
export interface InvFinanceSummary {
  total: number
  settled: number
  outstanding: number
  overdue: number
  /** 其中：单据类总额 */
  orderTotal: number
  /** 其中：合同期次总额 */
  planTotal: number
}

/** 应收 / 应付分页结果（在分页结构上附加汇总） */
export interface InvFinancePage extends PageResult<InvFinanceRow> {
  summary: InvFinanceSummary
}

/** 结算登记载荷 */
export interface InvSettlePayload {
  orderId: number
  amount: number
  method?: string
  settledAt?: string
  remark?: string
}

/**
 * 批量结算载荷。缺省口径是「每张单据按未结余额结清」；
 * amounts 用于个别单据只结一部分（键为单据 ID）。
 */
export interface InvSettleBatchPayload {
  orderIds: number[]
  amounts?: Record<number, number>
  method?: string
  settledAt?: string
  remark?: string
}

/** 数据概览 */
export interface InvOverview {
  customerCount: number
  activeCustomer: number
  monthNewCustomer: number
  receivableTotal: number
  payableTotal: number
  monthSales: number
  monthPurchase: number
  pendingOrders: number
  productCount: number
  warnProductCount: number
}

/** 报表汇总 */
export interface InvReportSummary {
  range: string
  salesAmount: number
  costAmount: number
  grossProfit: number
  grossRate: number
  salesOrders: number
  purchaseOrders: number
  purchases: number
}

/** 报表趋势点 */
export interface InvReportTrendPoint {
  date: string
  salesAmount: number
  purchaseAmount: number
}

/** 排行行（客户 / 供应商 / 商品通用） */
export interface InvReportRankRow {
  rank: number
  name: string
  id: number
  amount: number
  orderCount: number
  quantity: number
  ratio: number
}

/** 品类占比行 */
export interface InvReportCategoryRow {
  category: string
  amount: number
  ratio: number
}

/** 品类库存金额分布行 */
export interface InvCategoryValueRow {
  category: string
  value: number
  ratio: number
}

/** 低库存预警行 */
export interface InvWarnStockRow {
  productId: number
  productCode: string
  productName: string
  specId: number
  spec: string
  unit: string
  stock: number
  warnStock: number
}

/** 库存分析汇总 */
export interface InvReportInventory {
  totalValue: number
  specCount: number
  warnCount: number
  categoryValue: InvCategoryValueRow[]
  warnList: InvWarnStockRow[]
}

/** 库存排行行（按库存金额降序，全量） */
export interface InvReportStockRankRow {
  rank: number
  productId: number
  productCode: string
  productName: string
  spec: string
  unit: string
  stock: number
  value: number
  ratio: number
}

/** 销售 / 采购明细行（一行一个商品） */
export interface InvReportDetailRow {
  orderId: number
  orderNo: string
  orderDate: string
  partyName: string
  salesman: string
  productName: string
  spec: string
  unit: string
  qty: number
  price: number
  amount: number
}

/** 明细结果集：行 + 合计 */
export interface InvReportDetailResult {
  rows: InvReportDetailRow[]
  total: number
  amount: number
  qty: number
  /** true 表示命中行数超过上限，只展示了前 N 行 */
  truncated: boolean
}

/** 合同执行报表行 */
export interface InvReportContractRow {
  contractId: number
  contractNo: string
  name: string
  bizType: InvContractBizType
  partyName: string
  signDate: string
  amount: number
  planCount: number
  planAmount: number
  settledAmount: number
  outstanding: number
  retentionAmount: number
  retentionOutstanding: number
  stage: InvContractStage
  stageLabel: string
  acceptedAt: string | null
  warrantyEndAt: string | null
}

/** 合同执行报表：行 + 合计 */
export interface InvReportContractResult {
  rows: InvReportContractRow[]
  count: number
  amount: number
  planAmount: number
  settledAmount: number
  outstanding: number
  retentionAmount: number
  retentionOutstanding: number
  /** true 表示超过上限只返回前 N 份 */
  truncated: boolean
}

/** 合同收付款台账行 */
export interface InvReportContractLedgerRow {
  settleId: number
  settledAt: string
  contractNo: string
  name: string
  bizType: InvContractBizType
  partyName: string
  planSeq: number
  planName: string
  amount: number
  method: string
  remark: string
}

/** 打印模板列表行 */
export interface InvPrintTemplateRow {
  id: number
  code: string
  name: string
  docType: string
  paper: string
  /** 版式配置 JSON（列显隐 / 标题 / 页脚等），由模板设计器编辑 */
  content: string
  status: Status
  remark: string
  createdAt: string
}

/** 打印模板载荷 */
export interface InvPrintTemplatePayload {
  code: string
  name: string
  docType?: string
  paper?: 'A4' | 'A5'
  /** 版式配置 JSON 字符串 */
  content?: string
  status?: Status
  remark?: string
}

/** 出入库单打印明细行 */
export interface InvStockPrintItem {
  productCode: string
  productName: string
  spec: string
  unit: string
  /** 成交单位的换算率；基本单位时为 1 */
  rate: number
  qty: number
  /** 折算成基本单位的数量 */
  qtyBase: number
  /** 基本单位名，用于把基本数量写清楚 */
  unitBase: string
}

/** 出入库单打印详情（表头 + 明细 + 关联信息） */
export interface InvStockOrderDetail {
  id: number
  orderNo: string
  direction: InvStockDirection
  sourceType: 'purchase' | 'sales' | 'manual'
  sourceOrderNo: string
  orderDate: string
  remark: string
  /** 业务员 */
  salesman: string
  operator: string
  totalQty: number
  items: InvStockPrintItem[]
}

// ============================================================
// 合同与附件
// ============================================================

/** 附件归属的业务类型 */
export type InvAttachBizType = 'purchase' | 'sales' | 'contract'

/** 业务附件（采购单 / 销售单 / 合同的图片与文件） */
export interface InvAttachmentRow {
  id: number
  bizType: InvAttachBizType
  bizId: number
  /** 原始文件名，下载时用它 */
  fileName: string
  /** 含点的小写扩展名，如 .pdf */
  ext: string
  size: number
  mime: string
  /** 图片可直接预览（后端按扩展名判定，不信客户端的 Content-Type） */
  isImage: boolean
  uploader: string
  createdAt: string
}

/** 合同类型：采购合同挂供应商，销售合同挂客户 */
export type InvContractBizType = 'purchase' | 'sales'

/** 合同状态：0 待审核 / 1 执行中 / 2 已完成 / 3 已终止 */
export type InvContractStatus = 0 | 1 | 2 | 3

/** 履约阶段（后端派生，不入库） */
export type InvContractStage =
  | 'unaudited'
  | 'pending'
  | 'accepted'
  | 'warranty'
  | 'warranty_due'
  | 'finished'
  | 'terminated'

/** 付款期次类型：1 预付 / 2 进度 / 3 验收 / 4 质保金 / 5 其他 */
export type InvPlanKind = 1 | 2 | 3 | 4 | 5

/** 期次触发节点：按事件而非固定日期进入待收付 */
export type InvPlanTrigger = 'sign' | 'accept' | 'warranty_end'

/** 期次状态（后端派生） */
export type InvPlanStatus = 'pending' | 'due' | 'partial' | 'settled'

/** 合同付款计划的一期（返回行） */
export interface InvContractPlanItem {
  id: number
  seq: number
  name: string
  kind: InvPlanKind
  kindLabel: string
  /** 比例 %，与金额二选一（填比例时金额由合同金额折算） */
  rate: number
  amount: number
  dueDate: string | null
  triggerNode: InvPlanTrigger | ''
  settledAmount: number
  outstanding: number
  status: InvPlanStatus
  statusLabel: string
  remark: string
}

/** 合同列表行与详情（结构一致，详情带 plans） */
export interface InvContractRow {
  id: number
  contractNo: string
  name: string
  bizType: InvContractBizType
  partyId: number
  partyName: string
  signDate: string
  startDate: string | null
  endDate: string | null
  amount: number
  status: InvContractStatus
  salesman: string
  operator: string
  remark: string

  // 验收与质保
  acceptedAt: string | null
  acceptResult: string
  warrantyMonths: number
  warrantyEndAt: string | null
  retentionRate: number
  retentionAmount: number

  /** 履约阶段（派生）：待审核 / 待验收 / 已验收 / 质保中 / 质保到期 / 已完成 / 已终止 */
  stage: InvContractStage
  stageLabel: string

  /** 审核：谁在什么时候放行。待审核时为空 */
  auditedAt: string | null
  auditor: string

  // 收付进度（期次汇总）
  planCount: number
  planAmount: number
  settledAmount: number
  outstanding: number
  retentionOutstanding: number

  /** 仅详情返回 */
  plans?: InvContractPlanItem[]

  /** 附件数量（列表按钮角标） */
  attachmentCount: number
  createdAt: string
}

/** 付款期次提交行：id 有值表示既有期次 */
export interface InvContractPlanPayload {
  id?: number
  seq: number
  name: string
  kind: InvPlanKind
  rate?: number
  amount?: number
  dueDate?: string
  triggerNode?: InvPlanTrigger | ''
  remark?: string
}

/** 合同新增 / 修改载荷。contractNo 留空即自动生成，编辑时留空沿用原号 */
export interface InvContractPayload {
  contractNo?: string
  name: string
  bizType: InvContractBizType
  partyId: number
  signDate: string
  startDate?: string
  endDate?: string
  amount?: number
  status?: InvContractStatus
  salesman?: string
  remark?: string

  warrantyMonths?: number
  retentionRate?: number
  retentionAmount?: number
  /** 整份替换：传什么就是什么（已发生收付款的期次不能删、不能改金额） */
  plans?: InvContractPlanPayload[]
}

/** 合同验收载荷 */
export interface InvContractAcceptPayload {
  acceptedAt: string
  acceptResult: '合格' | '不合格' | '整改'
  warrantyMonths?: number
  remark?: string
}

/** 合同期次收付款载荷 */
export interface InvPlanSettlePayload {
  planId: number
  amount: number
  method?: string
  settledAt?: string
  remark?: string
}

/** 期次收付款流水 */
export interface InvPlanSettleRecord {
  id: number
  amount: number
  method: string
  settledAt: string
  remark: string
}

/** 合同操作记录（来自系统操作日志） */
export interface InvContractLogRow {
  id: number
  username: string
  action: string
  method: string
  ip: string
  status: number
  createdAt: string
}
