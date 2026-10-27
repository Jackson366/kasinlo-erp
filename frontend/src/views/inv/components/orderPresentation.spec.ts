import { describe, it, expect } from 'vitest'
import { pendingAge } from './orderPresentation'
describe('采购等待时长', () => {
  it('不推断缺失历史，按进入待处理的时间计算并在 24 小时提示', () => {
    expect(pendingAge(null)).toEqual({ text: '历史数据未记录', warning: false })
    const now = Date.parse('2026-10-27T12:00:00Z')
    expect(pendingAge('2026-10-27T11:30:00Z', now)).toEqual({
      text: '等待不足 1 小时',
      warning: false,
    })
    expect(pendingAge('2026-10-26T12:00:00Z', now)).toEqual({
      text: '已等待 1 天',
      warning: true,
    })
  })
})
