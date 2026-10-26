import axios, { AxiosError, type AxiosAdapter } from 'axios'
import { createMockService, MockError } from './service'

export function createMockAdapter(
  service: ReturnType<typeof createMockService>,
  apiPrefix: string,
): AxiosAdapter {
  return async (config) => {
    await new Promise((resolve) => setTimeout(resolve, 100))
    if (config.signal?.aborted) throw new axios.CanceledError()
    const url = new URL(config.url || '/', 'http://mock.local')
    const base = new URL(apiPrefix, 'http://mock.local').pathname.replace(/\/$/, '')
    const path = url.pathname.startsWith(`${base}/`) ? url.pathname.slice(base.length) : url.pathname
    const params = { ...Object.fromEntries(url.searchParams), ...(config.params || {}) }
    let body: unknown = config.data
    if (typeof body === 'string') {
      try {
        body = JSON.parse(body)
      } catch {
        /* 原样交给错误处理 */
      }
    }
    try {
      const data = service.handle({
        method: (config.method || 'get').toUpperCase(),
        path,
        params,
        body,
        token: String(config.headers.Authorization || '').replace(/^Bearer /, ''),
      })
      return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 0, message: 'Mock', data } }
    } catch (error) {
      const status = error instanceof MockError ? error.status : 500
      const message = error instanceof Error ? error.message : 'Mock 请求失败'
      const response = {
        config,
        status,
        statusText: 'Mock Error',
        headers: {},
        data: { code: status === 401 ? 40100 : status, message, data: null },
      }
      throw new AxiosError(message, 'ERR_BAD_RESPONSE', config, undefined, response)
    }
  }
}
