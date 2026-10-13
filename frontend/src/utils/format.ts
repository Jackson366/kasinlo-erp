import dayjs from 'dayjs'

/** 格式化日期时间 */
export function formatDateTime(value?: string | null, template = 'YYYY-MM-DD HH:mm:ss'): string {
  if (!value) return '-'
  const date = dayjs(value)
  return date.isValid() ? date.format(template) : '-'
}

/** 格式化为日期 */
export function formatDate(value?: string | null): string {
  return formatDateTime(value, 'YYYY-MM-DD')
}

/**
 * 单据日期（只到天）格式化；空值与「零值日期」一律显示为 -。
 *
 * 为什么单列一个函数而不是继续用 `value?.slice(0, 10) || '-'`：
 *  1. slice 对 `0001-01-01T00:00:00Z` 判不出「这是没有值」——后端历史上把
 *     未填写的到期日写成 Go 零值时间，slice 出来就是「0001-01-01」，
 *     看起来像个正常日期。这里以 1900 年为界把它识别成空值。
 *  2. 各页原先 6 处各写一遍 slice，口径迟早漂移；日期展示统一走这一处。
 */
export function formatDay(value?: string | null): string {
  if (!value) return '-'
  const date = dayjs(value)
  if (!date.isValid() || date.year() < 1900) return '-'
  return date.format('YYYY-MM-DD')
}

/** 相对时间，例如「3 分钟前」 */
export function fromNow(value?: string | null): string {
  if (!value) return '-'
  const target = dayjs(value)
  if (!target.isValid()) return '-'
  const diff = dayjs().diff(target, 'second')
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 2592000) return `${Math.floor(diff / 86400)} 天前`
  return target.format('YYYY-MM-DD')
}

/** 耗时展示 */
export function formatLatency(ms: number): string {
  if (!ms && ms !== 0) return '-'
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(2)} s`
}

/** 生成导出文件名用的时间戳，例如 20260929_203045 */
export function fileStamp(date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const d = `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}`
  const t = `${pad(date.getHours())}${pad(date.getMinutes())}${pad(date.getSeconds())}`
  return `${d}_${t}`
}

/**
 * 导出文件名：业务前缀 + 时间戳 + .xlsx，例如 用户列表_20260929_203045.xlsx。
 *
 * 各列表页原先各自拼 `xxx_${fileStamp()}.xlsx`，四处重复；收敛成一个函数，
 * 后缀与分隔符只有一处定义，改则全改。
 */
export function exportStamp(prefix: string): string {
  return `${prefix}_${fileStamp()}.xlsx`
}

/** 把树形数组拍平成列表，便于查找 */
export function flattenTree<T extends { children?: T[] }>(nodes: T[]): T[] {
  const out: T[] = []
  const walk = (list: T[]) => {
    list.forEach((node) => {
      out.push(node)
      if (node.children?.length) walk(node.children)
    })
  }
  walk(nodes)
  return out
}

/**
 * 金额格式化：千分位 + 两位小数，默认带 ¥ 前缀。
 * 进销存各页的金额展示口径必须一致，因此在工具层收口一份。
 */
export function formatMoney(value?: number | null, withSymbol = true): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  const [intPart, decPart] = Number(value).toFixed(2).split('.')
  const withSep = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return `${withSymbol ? '¥' : ''}${withSep}.${decPart}`
}

/** 数量格式化：整数不带小数，非整数保留最多两位 */
export function formatQty(value?: number | null): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  return Number.isInteger(value) ? String(value) : String(Number(Number(value).toFixed(2)))
}
