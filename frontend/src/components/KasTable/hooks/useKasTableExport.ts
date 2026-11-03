import { ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { downloadFile } from '@/utils/download'
import { ApiError } from '@/utils/request'
import type { KasTableExportOptions } from '../types'

export function useKasTableExport<Row extends { id: number }>(options: KasTableExportOptions) {
  const exporting = ref(false)

  /** 未传记录时导出筛选全集；传入记录时按 ID 导出选中项。 */
  async function exportRows(rows?: Row[]): Promise<void> {
    if (exporting.value) return
    exporting.value = true
    try {
      const params = { ...options.exportParams() }
      if (rows?.length) params.ids = rows.map((row) => row.id).join(',')
      await downloadFile(options.exportUrl, params, options.fallbackName)
    } catch (error) {
      if (error instanceof ApiError) ElMessage.warning(error.message)
      else ElMessage.error('导出失败，请稍后重试')
    } finally {
      exporting.value = false
    }
  }

  return { exporting, exportRows }
}
