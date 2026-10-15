/**
 * 打印模板版式配置。
 *
 * 配置以 JSON 字符串存在模板记录的 content 字段里，后端只存取不解释——
 * 「列怎么渲染」是打印视图的事，收在 JSON 里让设计器可以随迭代增删字段
 * 而不用改表结构。
 *
 * 兼容策略：**老模板的 content 里是「一堆 showXxx 布尔 + fontSize」**，
 * 新版把明细列收成了 columns 数组（可改名、可排序、可设对齐）。
 * parseContent 会把旧布尔翻译成默认列集合，所以老模板打开设计器
 * 不会丢配置，保存后才固化成新结构。
 */

/** 明细列的列标识。value 之外的列都由模板配置驱动 */
export type ColumnKey = 'code' | 'name' | 'spec' | 'unit' | 'qty' | 'price' | 'amount'

/** 一列明细的配置 */
export interface PrintColumn {
  key: ColumnKey
  /** 列标题，可改（如把「商品编码」改成「物料编码」） */
  label: string
  visible: boolean
  align: 'left' | 'center' | 'right'
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
}

/** 页面设置 */
export interface PrintPageConfig {
  /** 页边距（mm） */
  margin: number
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
  /** 签收栏（制单 / 审核 / 批准） */
  showSign: boolean
  /** 单据备注 */
  showRemark: boolean
  /** 合计大写金额（仅采购 / 销售单） */
  showAmountCn: boolean
  /** 附加自定义文字，显示在备注之后 */
  customText: string
}

export interface PrintContentConfig {
  /** 单据标题；留空时按单据方向显示「入库单 / 出库单 / 采购单 / 销售单」 */
  title: string
  columns: PrintColumn[]
  header: PrintHeaderConfig
  page: PrintPageConfig
  footer: PrintFooterConfig
}

/** 默认列集合。顺序即纸面顺序，设计器里可上下移动 */
export const DEFAULT_COLUMNS: PrintColumn[] = [
  { key: 'code', label: '商品编码', visible: true, align: 'left' },
  { key: 'name', label: '商品名称', visible: true, align: 'left' },
  { key: 'spec', label: '规格', visible: true, align: 'left' },
  { key: 'unit', label: '单位', visible: true, align: 'center' },
  { key: 'qty', label: '数量', visible: true, align: 'right' },
  { key: 'price', label: '单价', visible: true, align: 'right' },
  { key: 'amount', label: '金额', visible: true, align: 'right' },
]

export const DEFAULT_CONTENT: PrintContentConfig = {
  title: '',
  columns: DEFAULT_COLUMNS.map((c) => ({ ...c })),
  header: {
    companyName: '',
    logoUrl: '',
    address: '',
    phone: '',
    bank: '',
    showTitle: true,
  },
  page: {
    margin: 20,
    rowHeight: 30,
    fontSize: 12,
    titleFontSize: 20,
    headerBg: '#f2f2f2',
  },
  footer: {
    text: '',
    showSign: true,
    showRemark: true,
    showAmountCn: false,
    customText: '',
  },
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
  return DEFAULT_COLUMNS.filter((c) => c.key === 'name' || c.key === 'qty' || on.has(c.key)).map((c) => ({
    ...c,
  }))
}

/**
 * 解析模板 content；空值 / 脏数据一律回退默认配置，打印不能因为配置坏了而失败。
 *
 * 三处兜底：
 *  1. JSON 解析失败 → 全默认；
 *  2. columns 缺失或为空 → 走 legacyColumns（老配置）再兜底默认列；
 *  3. 列集合里缺了「必有的两列」（名称 / 数量）→ 补回来，
 *     否则纸面会出现「没有名称也没有数量」的空表格。
 */
export function parseContent(raw?: string | null): PrintContentConfig {
  const base: PrintContentConfig = {
    title: '',
    columns: DEFAULT_COLUMNS.map((c) => ({ ...c })),
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
  const cols = Array.isArray(parsed.columns) ? (parsed.columns as PrintColumn[]) : null
  if (cols && cols.length > 0) {
    cfg.columns = cols
  } else {
    cfg.columns = legacyColumns(parsed)
  }
  // 必有两列在：名称与数量。缺了就会出现一张没有主体内容的表格
  const keys = new Set(cfg.columns.filter((c) => c.visible).map((c) => c.key))
  if (!keys.has('name') || !keys.has('qty')) {
    const merged = [...cfg.columns]
    for (const k of ['name', 'qty'] as ColumnKey[]) {
      if (!merged.some((c) => c.key === k)) {
        merged.push({ ...DEFAULT_COLUMNS.find((c) => c.key === k)! })
      }
    }
    cfg.columns = merged
  }
  return cfg
}

/** 取某一列是否可见 */
export function colVisible(cfg: PrintContentConfig, key: ColumnKey): boolean {
  return cfg.columns.some((c) => c.key === key && c.visible)
}

/** 可见列（不含名称列，用于表头 colspan 计算） */
export function visibleCols(cfg: PrintContentConfig): PrintColumn[] {
  return cfg.columns.filter((c) => c.visible)
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
