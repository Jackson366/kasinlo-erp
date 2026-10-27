import { ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { downloadFile } from '@/utils/download'
import { ApiError } from '@/utils/request'

export interface BatchOutcome<Row> {
  succeeded: number
  failures: { row: Row; name: string; reason: string }[]
}
export interface BatchOptions<Row> {
  consequences?: string
  onResult?: (result: BatchOutcome<Row>) => void
}

/** 表格实例上用到的最小接口（el-table 的实例方法） */
interface TableLike {
  clearSelection: () => void
}

/**
 * 列表页的「导出 / 打印 / 批量」三件套。
 *
 * 抽出来的理由很实际：这些逻辑在 10 个列表页里长得一模一样，
 * 而其中两处最容易写错 ——
 *  1. 批量操作要**逐条**调用（后端的删除/作废接口是单条的），
 *     且失败项要用一条汇总消息说清楚，不能 10 条失败弹 10 次 toast；
 *  2. 导出要带**当前筛选条件**，不是「导出当前页那 10 行」——
 *     用户点了导出，期望的是「我筛出来的这一批」。
 */
export function useListTools<Row extends { id: number }>(options: {
  /** 导出接口地址（后端返回 xlsx 流） */
  exportUrl: string
  /** 当前筛选条件；导出与之完全一致 */
  exportParams: () => Record<string, unknown>
  /** 兜底文件名（后端没给 Content-Disposition 时用） */
  fallbackName: string
}) {
  const selection = ref<Row[]>([]) as Ref<Row[]>
  const exporting = ref(false)
  const batchRunning = ref(false)

  function onSelectionChange(rows: Row[]): void {
    selection.value = rows
  }

  function clearSelection(table?: InstanceType<never> | null): void {
    ;(table as unknown as TableLike | null)?.clearSelection()
    selection.value = []
  }

  /** 导出：不传 rows 时导出「当前筛选的全部」，传 rows 时只导选中的记录 */
  async function exportRows(rows?: Row[]): Promise<void> {
    if (exporting.value) return
    exporting.value = true
    try {
      const params: Record<string, unknown> = { ...options.exportParams() }
      if (rows?.length) {
        // 选中态导出：把 id 列表带给后端，后端按 id 过滤（保持与列表同一套查询）
        params.ids = rows.map((r) => r.id).join(',')
      }
      await downloadFile(options.exportUrl, params, options.fallbackName)
    } catch (error) {
      if (error instanceof ApiError) ElMessage.warning(error.message)
      else ElMessage.error('导出失败，请稍后重试')
    } finally {
      exporting.value = false
    }
  }

  async function runBatch(
    actionLabel: string,
    rows: Row[],
    run: (row: Row) => Promise<unknown>,
    nameOf: (row: Row) => string,
    batchOptions?: BatchOptions<Row>,
  ): Promise<boolean> {
    if (!rows.length || batchRunning.value) return false
    batchRunning.value = true
    try {
      return await runBatchAction(actionLabel, rows, run, nameOf, batchOptions)
    } finally {
      batchRunning.value = false
    }
  }

  return {
    selection,
    exporting,
    batchRunning,
    onSelectionChange,
    clearSelection,
    exportRows,
    runBatch,
  }
}

/**
 * 批量执行（可独立使用）。逐条调用并汇总结果，只弹**一条**提示。
 *
 * 逐条而不是新加批量接口：后端的删除/作废各自带业务校验（引用检查、
 * 状态检查、库存回滚），批量接口要在一个事务里复刻这些判断，
 * 反而更容易让「部分成功」变成一笔糊涂账。逐条至少能精确说出
 * 「哪 2 条没成功、为什么」。
 */
export async function runBatchAction<Row>(
  actionLabel: string,
  rows: Row[],
  run: (row: Row) => Promise<unknown>,
  nameOf: (row: Row) => string,
  options?: BatchOptions<Row>,
): Promise<boolean> {
  if (!rows.length) return false
  const names = rows.slice(0, 5).map(nameOf)
  const more = rows.length > 5 ? ` 等 ${rows.length} 条` : ''
  try {
    await ElMessageBox.confirm(
      `确定对以下 ${rows.length} 条记录执行「${actionLabel}」吗？\n${names.join('、')}${more}${options?.consequences ? `\n${options.consequences}` : ''}`,
      `${actionLabel}确认`,
      { type: 'warning', confirmButtonText: actionLabel, autofocus: false },
    )
  } catch {
    return false
  }

  const results = await Promise.allSettled(rows.map((row) => run(row)))
  const failed = results
    .map((r, i) => ({ r, row: rows[i] }))
    .filter((x) => x.r.status === 'rejected')
  const okCount = results.length - failed.length

  options?.onResult?.({
    succeeded: okCount,
    failures: failed.map(({ row, r }) => ({
      row,
      name: nameOf(row),
      reason:
        r.status === 'rejected' && r.reason instanceof Error
          ? r.reason.message
          : '请求失败，请刷新核实后重试',
    })),
  })
  if (failed.length === 0) {
    ElMessage.success(`${actionLabel}成功 ${okCount} 条`)
  } else {
    const first = failed[0].r as PromiseRejectedResult
    const reason =
      first.reason instanceof ApiError ? first.reason.message : '未知原因'
    ElMessage.warning(
      `${actionLabel}完成：成功 ${okCount} 条，失败 ${failed.length} 条（${nameOf(failed[0].row)}：${reason}）`,
    )
  }
  return okCount > 0
}
