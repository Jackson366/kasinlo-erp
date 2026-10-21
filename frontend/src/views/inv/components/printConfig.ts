/**
 * 打印模板版式配置。
 *
 * 配置以 JSON 字符串存在模板记录的 content 字段里，后端只存取不解释——
 * 「列怎么渲染」是打印视图的事，收在 JSON 里让设计器可以随迭代增删字段
 * 而不用改表结构。
 *
 * v3 结构（本版，按纸质送货单重做版式）：
 *   - 纸面从上到下：标题 → 信息条（客户 / 日期 / 单号，一行横向铺开）
 *     → 明细表（细实线、表头无底衬）→ 表尾行（左侧公司信息、右侧合计金额）
 *     → 声明行（左：收货声明，右：签字处）→ 备注 / 页脚；
 *   - 明细列 columns：可改名 / 排序 / 显隐，含序号、基本数量、换算率、备注；
 *   - 信息条 meta：单据编号 / 日期 / 往来单位等可改名、显隐、排序；
 *   - 页边距 page.marginMm：毫米单位，直接驱动打印 @page，所见即所得；
 *   - 声明与签字 footer.declaration / footer.signLabel：一句话收货声明 +
 *     签字处，取代旧版「制单 / 审核 / 批准」三联签收栏（仍可通过 showSign 打开）。
 *
 * 兼容策略：老模板的 content 里是「一堆 showXxx 布尔 + fontSize + margin(px)」
 * 或 v2 的 columns/meta 结构，parseContent 统一回退补默认——老模板打开设计器
 * 不丢配置，保存后固化成新结构。
 */

/** 明细列的列标识。value 之外的列都由模板配置驱动 */
export type ColumnKey =
  | 'index'
  | 'code'
  | 'name'
  | 'spec'
  | 'unit'
  | 'qty'
  | 'qtyBase'
  | 'rate'
  | 'price'
  | 'amount'
  | 'note'

/** 一列明细的配置 */
export interface PrintColumn {
  key: ColumnKey
  /** 列标题，可改（如把「商品编码」改成「物料编码」） */
  label: string
  visible: boolean
  align: 'left' | 'center' | 'right'
}

/** 信息条的一个字段（单据编号 / 日期 / 往来单位……） */
export interface PrintMetaField {
  key: string
  /** 显示文案，可改（如把「业务员」改成「经手人」） */
  label: string
  visible: boolean
}

/** 表头区：公司信息与 Logo */
export interface PrintHeaderConfig {
  companyName: string
  /** Logo 图片地址；留空则不显示 */
  logoUrl: string
  address: string
  phone: string
  bank: string
  /** 单据标题是否显示（关掉后纸面直接从信息条开始） */
  showTitle: boolean
  /**
   * 公司信息位置。纸质送货单的习惯是压在明细表下方（bottom）：
   * 抬头只留单据标题，签收人一眼看到的是货和金额，公司名是落款。
   */
  placement: 'top' | 'bottom'
}

/** 页面设置 */
export interface PrintPageConfig {
  /** 页边距（毫米）。直接注入打印 @page，预览按同一数值模拟——所见即所得 */
  marginMm: number
  /** 明细行高（px） */
  rowHeight: number
  /** 正文字号（pt） */
  fontSize: number
  /** 标题字号（pt） */
  titleFontSize: number
  /** 表头底衬颜色，留空为无（黑白打印友好） */
  headerBg: string
}

/** 底部自定义区域 */
export interface PrintFooterConfig {
  /** 页脚文字 */
  text: string
  /** 签收栏总开关（旧的「制单 / 审核 / 批准」三联行，默认关闭） */
  showSign: boolean
  /**
   * 签收栏的项目（如 制单 / 审核 / 批准）。
   * null = 未自定义，按单据类型取默认（采购销售：制单/审核/批准；出入库：制单/仓管/审核）。
   */
  signItems: string[] | null
  /** 单据备注 */
  showRemark: boolean
  /** 合计大写金额（仅采购 / 销售单） */
  showAmountCn: boolean
  /** 附加自定义文字，显示在备注之后 */
  customText: string
  /**
   * 收货声明。留空则按单据类型取默认话术（销售「款未付」、采购「货款按约定结算」）。
   * 与 signLabel 同行：左边一句话，右边签字处——纸质单据的常规写法。
   */
  declaration: string
  /** 签字处标签，留空按单据类型取默认（客户签字 / 供应商签字 / 仓管签字） */
  signLabel: string
}

export interface PrintContentConfig {
  /** 单据标题；留空时按单据方向显示「入库单 / 出库单 / 采购单 / 销售单」 */
  title: string
  columns: PrintColumn[]
  /** 信息条字段；空数组 = 渲染层按单据类型取默认集合 */
  meta: PrintMetaField[]
  header: PrintHeaderConfig
  page: PrintPageConfig
  footer: PrintFooterConfig
}

/** 全部可用列（设计器列池）。顺序无意义，纸面顺序由模板 columns 决定 */
export const ALL_COLUMNS: PrintColumn[] = [
  { key: 'index', label: '序号', visible: true, align: 'center' },
  { key: 'code', label: '商品编码', visible: true, align: 'left' },
  { key: 'name', label: '商品名称', visible: true, align: 'left' },
  { key: 'spec', label: '规格型号', visible: true, align: 'left' },
  { key: 'unit', label: '计量单位', visible: true, align: 'center' },
  { key: 'price', label: '单价', visible: true, align: 'right' },
  { key: 'qty', label: '数量', visible: true, align: 'right' },
  { key: 'qtyBase', label: '基本数量', visible: true, align: 'right' },
  { key: 'rate', label: '换算率', visible: true, align: 'center' },
  { key: 'amount', label: '金额', visible: true, align: 'right' },
  { key: 'note', label: '备注信息', visible: true, align: 'left' },
]

/**
 * 默认列集合：纸质送货单的七列（名称 / 规格型号 / 计量单位 / 单价 / 数量 / 金额 / 备注）。
 * 序号与商品编码留在列池里按需添加——纸面窄的时候，多两列编号会让商品名挤成两行。
 */
export const DEFAULT_COLUMNS: PrintColumn[] = ['name', 'spec', 'unit', 'price', 'qty', 'amount', 'note'].map(
  (k) => ({ ...ALL_COLUMNS.find((c) => c.key === k)! }),
)

export const DEFAULT_CONTENT: PrintContentConfig = {
  title: '',
  columns: DEFAULT_COLUMNS.map((c) => ({ ...c })),
  meta: [],
  header: {
    companyName: '',
    logoUrl: '',
    address: '',
    phone: '',
    bank: '',
    showTitle: true,
    placement: 'bottom',
  },
  page: {
    marginMm: 12,
    rowHeight: 30,
    fontSize: 11,
    titleFontSize: 18,
    headerBg: '',
  },
  footer: {
    text: '',
    showSign: false,
    signItems: null,
    showRemark: true,
    showAmountCn: false,
    customText: '',
    declaration: '',
    signLabel: '',
  },
}

/** 96dpi 下 1mm 的像素数，预览模拟页边距用 */
export const MM_PX = 96 / 25.4

/** 纸面模式：order = 采购/销售单（带单价金额），stock = 出入库单 */
export type SheetMode = 'order' | 'stock'

/**
 * 信息条默认字段集合。cfg.meta 为空时渲染层按此兜底；
 * 设计器初始化也用它，用户改过一次就固化进模板。
 *
 * 默认只亮三个字段（往来单位 → 日期 → 单号），横向一行铺满、与表格等宽，
 * 这正是纸质送货单的抬头。业务员 / 制单人默认不亮但留在列表里——
 * 纸面窄的时候多两个字段就会折行，需要的人在设计器里勾一下即可。
 */
export function defaultMetaFields(mode: SheetMode, partyLabel: string): PrintMetaField[] {
  if (mode === 'order') {
    return [
      { key: 'party', label: partyLabel || '往来单位', visible: true },
      { key: 'orderDate', label: '单据日期', visible: true },
      { key: 'orderNo', label: '单据编号', visible: true },
      { key: 'salesman', label: '业务员', visible: false },
      { key: 'operator', label: '制单人', visible: false },
    ]
  }
  return [
    { key: 'orderDate', label: '单据日期', visible: true },
    { key: 'orderNo', label: '单据编号', visible: true },
    { key: 'source', label: '来源单号', visible: true },
    { key: 'salesman', label: '业务员', visible: false },
    { key: 'operator', label: '制单人', visible: false },
  ]
}

/** 签收栏默认项目。footer.signItems 为 null 时按单据类型取默认 */
export function defaultSignItems(mode: SheetMode): string[] {
  return mode === 'order' ? ['制单', '审核', '批准'] : ['制单', '仓管', '审核']
}

/** 默认收货声明。bizType / direction 决定「款未付」这类措辞是否成立 */
export function defaultDeclaration(mode: SheetMode, bizType?: string, direction?: string): string {
  if (mode === 'order') {
    return bizType === 'purchase'
      ? '本人确认上述货物数量、型号、外观均无误，货款按约定结算。'
      : '本人确认已收到上述产品，经核对数量、型号、外观均无误，款未付。'
  }
  return direction === 'out'
    ? '本人确认已收到上述产品，经核对数量、型号、外观均无误。'
    : '本人确认上述货物已如数入库，经核对数量、型号、外观均无误。'
}

/** 默认签字处标签 */
export function defaultSignLabel(mode: SheetMode, partyLabel: string, direction?: string): string {
  if (mode === 'order') return `${partyLabel || '往来单位'}签字`
  return direction === 'in' ? '仓管签字' : '客户签字'
}

/**
 * 旧配置 → 新列集合。
 *
 * 老模板只有 showCode / showSpec / showUnit / showPrice / showAmount 五个布尔，
 * 按同样的顺序翻译成 columns，并把不显示的列剔除 ——
 * 老模板保存一次之后才带齐 align / label，这是有意的渐进升级。
 */
function legacyColumns(raw: Record<string, unknown>): PrintColumn[] {
  const flags: Array<[ColumnKey, string]> = [
    ['code', 'showCode'],
    ['spec', 'showSpec'],
    ['unit', 'showUnit'],
    ['price', 'showPrice'],
    ['amount', 'showAmount'],
  ]
  const on = new Set(flags.filter(([, f]) => raw[f] === true).map(([k]) => k))
  // 名称与数量是主体，永远在；其余按老模板的布尔开关取舍
  const kept = DEFAULT_COLUMNS.filter((c) => c.key === 'name' || c.key === 'qty' || on.has(c.key))
  return kept.map((c) => ({ ...c }))
}

/**
 * 解析模板 content；空值 / 脏数据一律回退默认配置，打印不能因为配置坏了而失败。
 *
 * 兜底规则：
 *  1. JSON 解析失败 → 全默认；
 *  2. columns 缺失或为空 → 走 legacyColumns（老配置）再兜底默认列；
 *  3. 列集合里缺了「必有的两列」（名称 / 数量）→ 补回来；
 *  4. marginMm 缺失 → 12mm（老 margin 是 px，语义不同，不再沿用）；
 *  5. meta 缺失 → 空值，渲染层按单据类型取默认集合；
 *  6. footer 里没有 declaration（v2 及更早）→ 视为旧版签收栏，关掉三联签收，
 *     改用一行收货声明 + 签字处：这正是纸质送货单的写法。
 */
export function parseContent(raw?: string | null): PrintContentConfig {
  const base: PrintContentConfig = {
    title: '',
    columns: DEFAULT_COLUMNS.map((c) => ({ ...c })),
    meta: [],
    header: { ...DEFAULT_CONTENT.header },
    page: { ...DEFAULT_CONTENT.page },
    footer: { ...DEFAULT_CONTENT.footer },
  }
  if (!raw) return base
  let parsed: Record<string, unknown>
  try {
    parsed = JSON.parse(raw) as Record<string, unknown>
  } catch {
    return base
  }
  const cfg: PrintContentConfig = {
    ...base,
    title: typeof parsed.title === 'string' ? parsed.title : base.title,
    header: { ...base.header, ...((parsed.header as object) || {}) },
    page: { ...base.page, ...((parsed.page as object) || {}) },
    footer: { ...base.footer, ...((parsed.footer as object) || {}) },
  }
  // 老模板把字号放在顶层（fontSize）、页脚是顶层字符串；新结构收进 page / footer
  if (typeof parsed.fontSize === 'number' && !(parsed.page as object)) {
    cfg.page.fontSize = parsed.fontSize
  }
  if (typeof parsed.footer === 'string') {
    cfg.footer.text = parsed.footer
  }
  // 老结构的 margin 是 px（预览内边距），与 @page 毫米语义不同，缺 marginMm 就用默认
  if (typeof cfg.page.marginMm !== 'number' || !Number.isFinite(cfg.page.marginMm)) {
    cfg.page.marginMm = DEFAULT_CONTENT.page.marginMm
  }
  if (Array.isArray(parsed.meta)) {
    cfg.meta = (parsed.meta as PrintMetaField[]).filter((m) => m && typeof m.key === 'string')
  } else {
    cfg.meta = []
  }
  const cols = Array.isArray(parsed.columns) ? (parsed.columns as PrintColumn[]) : null
  if (cols && cols.length > 0) {
    cfg.columns = cols
  } else {
    cfg.columns = legacyColumns(parsed)
  }
  // 老模板没有序号列：不强行补了（新默认列就没有序号，补了反而和新模板不一致），
  // 但名称与数量必须可见，否则纸面没有主体内容
  const keys = new Set(cfg.columns.filter((c) => c.visible).map((c) => c.key))
  if (!keys.has('name') || !keys.has('qty')) {
    const merged = [...cfg.columns]
    for (const k of ['name', 'qty'] as ColumnKey[]) {
      if (!merged.some((c) => c.key === k)) {
        merged.push({ ...ALL_COLUMNS.find((c) => c.key === k)! })
      }
    }
    cfg.columns = merged
  }
  // v2 及更早的模板没有声明字段：启用声明行，并关掉三联签收栏
  if (typeof cfg.footer.declaration !== 'string' || !('declaration' in (parsed.footer as object || {}))) {
    cfg.footer.showSign = false
  }
  if (typeof cfg.footer.declaration !== 'string') cfg.footer.declaration = ''
  if (typeof cfg.footer.signLabel !== 'string') cfg.footer.signLabel = ''
  return cfg
}

/** 取某一列是否可见 */
export function colVisible(cfg: PrintContentConfig, key: ColumnKey): boolean {
  return cfg.columns.some((c) => c.key === key && c.visible)
}

/**
 * 可见列（用于渲染与 colspan 计算）。
 *
 * 出入库单没有成交价：模板里即便留着单价 / 金额列（老模板、跨单据复用），
 * 打出来也只会是一列 0.00。这里按纸面模式直接剔掉，不留空账。
 */
export function visibleCols(cfg: PrintContentConfig, mode?: SheetMode): PrintColumn[] {
  return cfg.columns.filter(
    (c) => c.visible && !(mode === 'stock' && (c.key === 'price' || c.key === 'amount')),
  )
}

/** 人民币大写（合计金额用）。整数部分逐位映射，角分为零时省略 */
export function amountToCN(amount: number): string {
  if (!Number.isFinite(amount) || amount <= 0) return ''
  const digits = ['零', '壹', '贰', '叁', '肆', '伍', '陆', '柒', '捌', '玖']
  const units = ['', '拾', '佰', '仟']
  const groups = ['', '万', '亿', '万亿']
  const [intPart, decPart] = amount.toFixed(2).split('.')
  const jiao = Number(decPart[0])
  const fen = Number(decPart[1])

  const intToCN = (n: string): string => {
    if (n === '0') return '零'
    let out = ''
    let zero = false
    const len = n.length
    for (let i = 0; i < len; i++) {
      const d = Number(n[i])
      const pos = len - 1 - i
      const g = Math.floor(pos / 4)
      const u = pos % 4
      if (d === 0) {
        zero = true
      } else {
        if (zero && out) out += '零'
        zero = false
        out += digits[d] + units[u]
      }
      if (u === 0 && !zero) out += groups[g]
      if (u === 0) zero = false
    }
    return out.replace(/零+$/, '')
  }

  let s = '人民币' + intToCN(intPart) + '元'
  if (jiao === 0 && fen === 0) return s + '整'
  if (jiao > 0) s += digits[jiao] + '角'
  if (fen > 0) s += digits[fen] + '分'
  else if (jiao > 0) s += '整'
  return s
}

/**
 * 注入打印页设置（A4/A5 竖版 + 页边距）。
 *
 * @page 无法使用 CSS 变量，只能动态生成样式表。由 PrintSheet 挂载与
 * 配置变化时调用——打印弹窗、模板设计器共用一个纸面组件，注入点也就一处。
 * `size: ... portrait` 同时把打印方向钉死为竖版，不再跟随浏览器对话框默认值。
 */
export function applyPrintPage(paper: 'A4' | 'A5', marginMm: number): void {
  const id = 'print-page-style'
  let el = document.getElementById(id) as HTMLStyleElement | null
  if (!el) {
    el = document.createElement('style')
    el.id = id
    document.head.appendChild(el)
  }
  const mm = Number.isFinite(marginMm) && marginMm > 0 ? marginMm : DEFAULT_CONTENT.page.marginMm
  el.textContent = `@page { size: ${paper} portrait; margin: ${mm}mm; }`
}
