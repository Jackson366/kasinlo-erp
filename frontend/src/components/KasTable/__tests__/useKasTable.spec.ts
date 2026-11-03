import { describe, expect, it, vi } from 'vitest'
import { useKasTable } from '../hooks/useKasTable'
import { useKasTableQuery } from '../hooks/useKasTableQuery'
import { useTableQuery } from '@/composables/useTableQuery'
import { useListTools } from '@/composables/useListTools'
import { downloadFile } from '@/utils/download'

vi.mock('element-plus/es/components/message/index', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
}))
vi.mock('element-plus/es/components/message-box/index', () => ({
  ElMessageBox: { confirm: vi.fn().mockResolvedValue(undefined) },
}))
vi.mock('@/utils/request', () => ({ ApiError: class ApiError extends Error {} }))
vi.mock('@/utils/download', () => ({ downloadFile: vi.fn().mockResolvedValue(undefined) }))

describe('KasTable integration hooks', () => {
  it('clears the rendered selection before search, reset and refresh and loads once per action', async () => {
    const clearSelection = vi.fn()
    const fetch = vi.fn().mockResolvedValue({ list: [{ id: 1 }], total: 1, page: 1, pageSize: 10 })
    const table = useKasTable({
      initial: { keyword: '', status: undefined as number | undefined },
      fetch,
      table: () => ({ clearSelection }),
      export: { exportUrl: '/export', exportParams: () => ({}), fallbackName: 'table.xlsx' },
    })
    table.query.page = 3
    table.query.keyword = 'new filter'
    table.onSelectionChange([{ id: 99 }])
    table.search()
    expect(table.selection.value).toEqual([])
    expect(table.query.page).toBe(1)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(clearSelection).toHaveBeenCalledTimes(1)
    await vi.waitFor(() => expect(table.loading.value).toBe(false))

    table.query.pageSize = 50
    table.query.status = 0
    table.onSelectionChange([{ id: 99 }])
    table.reset()
    expect(table.selection.value).toEqual([])
    expect(table.query).toMatchObject({ keyword: '', status: undefined, page: 1, pageSize: 10 })
    await vi.waitFor(() => expect(table.loading.value).toBe(false))
    table.onSelectionChange([{ id: 99 }])
    await table.load()
    expect(table.selection.value).toEqual([])
    expect(fetch).toHaveBeenCalledTimes(3)
    expect(clearSelection).toHaveBeenCalledTimes(3)
  })

  it('keeps the old query entry point as an alias and the old tools backed by the shared hooks', async () => {
    expect(useTableQuery).toBe(useKasTableQuery)
    const tools = useListTools<{ id: number }>({
      exportUrl: '/export', exportParams: () => ({ status: 0 }), fallbackName: 'table.xlsx',
    })
    tools.onSelectionChange([{ id: 7 }])
    await tools.exportRows(tools.selection.value)
    expect(downloadFile).toHaveBeenLastCalledWith('/export', { status: 0, ids: '7' }, 'table.xlsx')
    tools.clearSelection()
    expect(tools.selection.value).toEqual([])
  })

  it('exports the whole filtered set without pagination and blocks a duplicate export while busy', async () => {
    let finish!: () => void
    vi.mocked(downloadFile).mockReturnValueOnce(new Promise<void>((resolve) => { finish = resolve }))
    const tools = useListTools<{ id: number }>({
      exportUrl: '/export', exportParams: () => ({ keyword: 'A', status: 0 }), fallbackName: 'table.xlsx',
    })
    vi.mocked(downloadFile).mockClear()
    const first = tools.exportRows()
    await tools.exportRows()
    expect(downloadFile).toHaveBeenCalledTimes(1)
    expect(downloadFile).toHaveBeenCalledWith('/export', { keyword: 'A', status: 0 }, 'table.xlsx')
    finish()
    await first
    expect(tools.exporting.value).toBe(false)
  })
})
