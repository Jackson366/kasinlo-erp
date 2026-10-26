import axios from 'axios'
import { createMockAdapter } from './adapter'
import instance from '@/utils/request'
import { API_PREFIX } from '@/utils/api'
import { createMockService, MockError } from './service'
import { createSeed, STORAGE_KEY, type MockData } from './seed'

/** 只在显式 mock 模式下动态加载；未知请求直接报错，绝不落到真实后端。 */
export function installMock(): void {
  let initial = createSeed()
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored) {
    try {
      const parsed = JSON.parse(stored) as MockData
      if (
        !Array.isArray(parsed.products) ||
        !Array.isArray(parsed.orders) ||
        !Array.isArray(parsed.contracts) ||
        !parsed.collections ||
        !parsed.profile ||
        !Number.isFinite(parsed.nextId)
      )
        throw new Error('invalid mock data')
      initial = parsed
    } catch {
      console.warn('[Mock] 本地演示数据已失效，已重新初始化')
      localStorage.removeItem(STORAGE_KEY)
    }
  }
  const service = createMockService(initial, (data) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(data))
    } catch {
      throw new MockError(507, 'Mock 数据保存失败，请清理浏览器存储后重试')
    }
  })
  const adapter = createMockAdapter(service, API_PREFIX)
  // 刷新令牌使用裸 axios；文件与业务请求共用 instance。两者必须一起接管。
  instance.defaults.adapter = adapter
  axios.defaults.adapter = adapter
  console.info('[Mock] 独立演示模式；admin / admin123456（可编辑），demo / demo123456（只读）')
}
