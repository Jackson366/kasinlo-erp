import { describe, expect, it, vi } from 'vitest'
import { useTableQuery } from './useTableQuery'
import type { PageResult } from '@/types/api'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}
const page = (id: number): PageResult<{ id: number }> => ({
  list: [{ id }],
  total: 1,
  page: 1,
  pageSize: 10,
})

describe('useTableQuery failure recovery', () => {
  it('keeps readable rows while distinguishing a failed refresh from an empty result', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(page(1))
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(page(2))
    const table = useTableQuery({}, fetch)
    await table.load()
    await table.load()
    expect(table.error.value).toBe(true)
    expect(table.loading.value).toBe(false)
    expect(table.rows.value).toEqual([{ id: 1 }])
    await table.load()
    expect(table.error.value).toBe(false)
    expect(table.rows.value).toEqual([{ id: 2 }])
  })

  it('a stale failure neither replaces a newer result nor clears its busy state', async () => {
    const old = deferred<PageResult<{ id: number }>>()
    const current = deferred<PageResult<{ id: number }>>()
    const fetch = vi
      .fn()
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(current.promise)
    const table = useTableQuery({}, fetch)
    const a = table.load(),
      b = table.load()
    old.reject(new Error('old timeout'))
    await a
    expect(table.loading.value).toBe(true)
    expect(table.error.value).toBe(false)
    current.resolve(page(2))
    await b
    expect(table.rows.value).toEqual([{ id: 2 }])
    expect(table.loading.value).toBe(false)
  })
})
