import { expect, it, vi } from 'vitest'
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
  ElMessageBox: { confirm: vi.fn().mockResolvedValue(undefined) },
}))
vi.mock('@/utils/request', () => ({ ApiError: class ApiError extends Error {} }))
vi.mock('@/utils/download', () => ({ downloadFile: vi.fn() }))
import { runBatchAction, type BatchOutcome } from './useListTools'
import { ElMessageBox } from 'element-plus'
it('批量保存全部失败单号与原因，允许部分成功，确认展示业务后果', async () => {
  const rows = Array.from({ length: 12 }, (_, id) => ({
    id,
    orderNo: `CG-LONG-NUMBER-${id}`,
  }))
  let result: BatchOutcome<(typeof rows)[number]> | undefined
  const outcome = await runBatchAction(
    '批量审核',
    rows,
    async (row) => {
      if (row.id < 10) throw new Error(`失败原因 ${row.id}`)
    },
    (row) => row.orderNo,
    {
      consequences: '生成入库单、增加库存、形成应付；批量不结清',
      onResult: (value) => {
        result = value
      },
    },
  )
  expect(outcome).toBe(true)
  expect(result?.succeeded).toBe(2)
  expect(result?.failures).toHaveLength(10)
  expect(result?.failures[9]).toMatchObject({
    name: 'CG-LONG-NUMBER-9',
    reason: '失败原因 9',
  })
  expect(ElMessageBox.confirm).toHaveBeenCalledWith(
    expect.stringContaining('批量不结清'),
    '批量审核确认',
    expect.any(Object),
  )
})
