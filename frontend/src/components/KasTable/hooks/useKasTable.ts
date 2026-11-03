import type { PageQuery } from '@/types/api'
import type { KasTableOptions } from '../types'
import { useKasTableQuery } from './useKasTableQuery'
import { useKasTableSelection } from './useKasTableSelection'
import { useKasTableExport } from './useKasTableExport'
import { useKasTableBatch } from './useKasTableBatch'

/** 当前页勾选的组合入口；筛选、翻页或刷新前清空旧选择。 */
export function useKasTable<F extends PageQuery, Row extends { id: number }>(
  options: KasTableOptions<F, Row>,
) {
  const query = useKasTableQuery(options.initial, options.fetch)
  const selection = useKasTableSelection<Row>()
  const exporting = useKasTableExport<Row>({
    ...options.export,
    exportParams: () => options.export.exportParams(query.query),
  })
  const batch = useKasTableBatch<Row>()

  function clearSelection(table = options.table()): void {
    selection.clearSelection(table)
  }

  async function load(): Promise<void> {
    clearSelection()
    await query.load()
  }

  function search(): void {
    clearSelection()
    query.search()
  }

  function reset(): void {
    clearSelection()
    query.reset()
  }

  // 不另外创建 query/rows/selection；单独使用各 hook 与组合入口共享同一套实现。
  return { ...query, ...selection, ...exporting, ...batch, clearSelection, load, search, reset }
}
