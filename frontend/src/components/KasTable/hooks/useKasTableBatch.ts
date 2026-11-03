import { ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { ApiError } from '@/utils/request'
import type { BatchOptions } from '../types'

export function useKasTableBatch<Row>() {
  const batchRunning = ref(false)

  async function runBatch(
    actionLabel: string,
    rows: Row[],
    run: (row: Row) => Promise<unknown>,
    nameOf: (row: Row) => string,
    options?: BatchOptions<Row>,
  ): Promise<boolean> {
    if (!rows.length || batchRunning.value) return false
    batchRunning.value = true
    try {
      return await runBatchAction(actionLabel, rows, run, nameOf, options)
    } finally {
      batchRunning.value = false
    }
  }

  return { batchRunning, runBatch }
}

/** 每条记录调用原接口，统一确认并汇总全部结果；业务后果由页面提供。 */
export async function runBatchAction<Row>(
  actionLabel: string,
  rows: Row[],
  run: (row: Row) => Promise<unknown>,
  nameOf: (row: Row) => string,
  options?: BatchOptions<Row>,
): Promise<boolean> {
  const targets = [...rows]
  if (!targets.length) return false
  const names = targets.slice(0, 5).map(nameOf)
  const more = targets.length > 5 ? ` 等 ${targets.length} 条` : ''
  try {
    await ElMessageBox.confirm(
      `确定对以下 ${targets.length} 条记录执行「${actionLabel}」吗？\n${names.join('、')}${more}${options?.consequences ? `\n${options.consequences}` : ''}`,
      `${actionLabel}确认`,
      { type: 'warning', confirmButtonText: actionLabel, autofocus: false },
    )
  } catch {
    return false
  }

  const results = await Promise.allSettled(targets.map((row) => run(row)))
  const failed = results
    .map((r, i) => ({ r, row: targets[i] }))
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
