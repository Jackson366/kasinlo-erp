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
