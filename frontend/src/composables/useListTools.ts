import { useKasTableSelection } from '@/components/KasTable/hooks/useKasTableSelection'
import { useKasTableExport } from '@/components/KasTable/hooks/useKasTableExport'
import { useKasTableBatch } from '@/components/KasTable/hooks/useKasTableBatch'
import type { KasTableExportOptions } from '@/components/KasTable/types'

export type { BatchOutcome, BatchOptions } from '@/components/KasTable/types'
export { runBatchAction } from '@/components/KasTable/hooks/useKasTableBatch'

/** 兼容尚未迁移的列表，所有能力只保留 KasTable 中的一份实现。 */
export function useListTools<Row extends { id: number }>(options: KasTableExportOptions) {
  return {
    ...useKasTableSelection<Row>(),
    ...useKasTableExport<Row>(options),
    ...useKasTableBatch<Row>(),
  }
}
