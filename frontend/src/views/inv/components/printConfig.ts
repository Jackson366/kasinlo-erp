/**
 * 打印模板版式配置。
 *
 * 配置以 JSON 字符串存在模板记录的 content 字段里，后端只存取不解释——
 * 「列怎么渲染」是打印视图的事，收在 JSON 里让设计器可以随迭代增删字段
 * 而不用改表结构。新字段必须给默认值：老模板的 content 里没有它，
 * parseContent 会按 DEFAULT_CONTENT 补齐。
 */
export interface PrintContentConfig {
  /** 单据标题；留空时按单据方向显示「入库单 / 出库单」 */
  title: string
  /** 商品编码列 */
  showCode: boolean
  /** 规格列 */
  showSpec: boolean
  /** 单位列 */
  showUnit: boolean
  /** 单价列（采购 / 销售单纸面使用，出入库单忽略） */
  showPrice: boolean
  /** 金额列（采购 / 销售单纸面使用，出入库单忽略） */
  showAmount: boolean
  /** 来源单号（来源采购 / 销售单） */
  showSource: boolean
  /** 制单人 */
  showOperator: boolean
  /** 打印时间 */
  showPrintTime: boolean
  /** 正文字号（pt） */
  fontSize: number
  /** 页脚文字 */
  footer: string
}

export const DEFAULT_CONTENT: PrintContentConfig = {
  title: '',
  showCode: true,
  showSpec: true,
  showUnit: true,
  showPrice: true,
  showAmount: true,
  showSource: true,
  showOperator: true,
  showPrintTime: true,
  fontSize: 12,
  footer: '',
}

/** 解析模板 content；空值 / 脏数据一律回退默认配置，打印不能因为配置坏了而失败 */
export function parseContent(raw?: string | null): PrintContentConfig {
  if (!raw) return { ...DEFAULT_CONTENT }
  try {
    return { ...DEFAULT_CONTENT, ...(JSON.parse(raw) as Partial<PrintContentConfig>) }
  } catch {
    return { ...DEFAULT_CONTENT }
  }
}
