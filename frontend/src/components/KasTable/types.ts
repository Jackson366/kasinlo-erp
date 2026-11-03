import type { PageQuery, PageResult } from '@/types/api'

export type KasTableQuery<F extends PageQuery> = F & {
  page: number
  pageSize: number
}

/** 页面只依赖清勾选能力，不绑定 Element Plus 的整个实例。 */
export interface KasTableHandle {
  clearSelection: () => void
}

export interface KasTableExportOptions {
  exportUrl: string
  /** 与列表共用业务筛选转换，不包含分页。 */
  exportParams: () => Record<string, unknown>
  fallbackName: string
}

export interface BatchOutcome<Row> {
  succeeded: number
  failures: { row: Row; name: string; reason: string }[]
}

export interface BatchOptions<Row> {
  consequences?: string
  onResult?: (result: BatchOutcome<Row>) => void
}

export interface KasTableOptions<F extends PageQuery, Row extends { id: number }> {
  initial: F
  fetch: (params: KasTableQuery<F>) => Promise<PageResult<Row>>
  table: () => KasTableHandle | undefined
  export: Omit<KasTableExportOptions, 'exportParams'> & {
    exportParams: (query: KasTableQuery<F>) => Record<string, unknown>
  }
}

export interface KasTableProps<Row> {
  title: string
  subtitle?: string
  selectedCount?: number
  rows: Row[]
  loading: boolean
  error: boolean
  total: number
  page: number
  pageSize: number
}
