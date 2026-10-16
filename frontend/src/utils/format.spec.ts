import dayjs from 'dayjs'
import { describe, expect, it } from 'vitest'
import {
  exportStamp,
  fileStamp,
  flattenTree,
  formatDate,
  formatDateTime,
  formatLatency,
  fromNow,
} from './format'

describe('formatDateTime / formatDate', () => {
  it('空值统一返回占位符', () => {
    expect(formatDateTime()).toBe('-')
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime('')).toBe('-')
    expect(formatDate()).toBe('-')
  })

  it('非法时间返回占位符而不是 Invalid Date', () => {
    expect(formatDateTime('not-a-date')).toBe('-')
    expect(formatDate('not-a-date')).toBe('-')
  })

  it('按默认与自定义模板格式化', () => {
    expect(formatDateTime('2026-03-04 05:06:07')).toBe('2026-03-04 05:06:07')
    expect(formatDateTime('2026-03-04 05:06:07', 'YYYY/MM/DD')).toBe('2026/03/04')
    expect(formatDate('2026-03-04 05:06:07')).toBe('2026-03-04')
  })
})

describe('fromNow', () => {
  it('空值与非法值返回占位符', () => {
    expect(fromNow(null)).toBe('-')
    expect(fromNow('nope')).toBe('-')
  })

  it('按时间差给出相对描述', () => {
    const now = dayjs()
    expect(fromNow(now.toISOString())).toBe('刚刚')
    expect(fromNow(now.subtract(2, 'hour').toISOString())).toBe('2 小时前')
    expect(fromNow(now.subtract(3, 'day').toISOString())).toBe('3 天前')
  })

  it('超过 30 天回退为绝对日期', () => {
    expect(fromNow(dayjs().subtract(40, 'day').toISOString())).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})

describe('formatLatency', () => {
  it('毫秒与秒的切换点稳定', () => {
    expect(formatLatency(0)).toBe('0 ms')
    expect(formatLatency(999)).toBe('999 ms')
    expect(formatLatency(1500)).toBe('1.50 s')
  })

  it('缺少耗时数据时返回占位符', () => {
    expect(formatLatency(undefined as unknown as number)).toBe('-')
  })
})

describe('导出文件名', () => {
  it('fileStamp 为 yyyyMMdd_HHmmss', () => {
    expect(fileStamp(new Date(2026, 2, 4, 5, 6, 7))).toBe('20260304_050607')
  })

  it('exportStamp 保留中文名并带时间戳后缀', () => {
    expect(exportStamp('用户列表')).toMatch(/^用户列表_\d{8}_\d{6}\.xlsx$/)
  })
})

describe('flattenTree', () => {
  type Nd = { id: number; children?: Nd[] }

  it('先根后子地展平嵌套结构', () => {
    const tree: Nd[] = [
      { id: 1, children: [{ id: 2, children: [{ id: 3 }] }, { id: 4 }] },
      { id: 5 },
    ]
    expect(flattenTree(tree).map((n) => n.id)).toEqual([1, 2, 3, 4, 5])
  })

  it('空树返回空数组', () => {
    expect(flattenTree<Nd>([])).toEqual([])
  })
})
