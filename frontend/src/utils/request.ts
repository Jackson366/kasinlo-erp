import axios, { type AxiosInstance, type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { ApiResult, LoginResult } from '@/types/api'
import { clearTokens, getAccessToken, getRefreshToken, setTokens } from '@/utils/auth'

/** 业务错误：携带后端返回的业务码与链路 ID */
export class ApiError extends Error {
  code: number
  traceId?: string

  constructor(code: number, message: string, traceId?: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.traceId = traceId
  }
}

/** 未登录 / 令牌失效的业务码 */
const AUTH_ERROR_CODES = [40100, 40104, 40105]

const instance: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 20000,
  headers: { 'Content-Type': 'application/json' },
})

// ---------------- 请求拦截 ----------------
instance.interceptors.request.use(
  (config) => {
    const token = getAccessToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error),
)

// ---------------- 令牌刷新（并发去重） ----------------
let refreshing: Promise<string> | null = null

/** 用刷新令牌换取新的访问令牌；失败则清理登录态 */
async function refreshAccessToken(): Promise<string> {
  const refreshToken = getRefreshToken()
  if (!refreshToken) {
    throw new ApiError(40100, '登录状态已失效')
  }
  if (!refreshing) {
    refreshing = axios
      .post<ApiResult<LoginResult>>(
        `${import.meta.env.VITE_API_BASE_URL || '/api'}/v1/auth/refresh`,
        { refreshToken },
        { timeout: 15000 },
      )
      .then((res) => {
        const body = res.data
        if (body.code !== 0 || !body.data?.accessToken) {
          throw new ApiError(body.code, body.message || '刷新登录状态失败')
        }
        setTokens(body.data.accessToken, body.data.refreshToken)
        return body.data.accessToken
      })
      .finally(() => {
        refreshing = null
      })
  }
  return refreshing
}

/** 跳转登录页，携带回跳地址 */
function redirectToLogin(): void {
  clearTokens()
  const current = window.location.hash.replace(/^#/, '')
  if (current.startsWith('/login')) return
  const redirect = encodeURIComponent(current || '/')
  window.location.href = `${window.location.pathname}#/login?redirect=${redirect}`
}

/** 把二进制错误响应体还原成业务响应体，解析失败时返回 undefined */
async function readBlobError(blob: Blob): Promise<ApiResult | undefined> {
  try {
    return JSON.parse(await blob.text()) as ApiResult
  } catch {
    return undefined
  }
}

// ---------------- 响应拦截 ----------------
instance.interceptors.response.use(
  (response: AxiosResponse<ApiResult>) => response,
  async (error) => {
    const response = error.response as AxiosResponse<ApiResult> | undefined

    // 网络层错误（后端未启动、超时等）
    if (!response) {
      const message = error.code === 'ECONNABORTED' ? '请求超时，请稍后重试' : '网络异常，请检查后端服务是否已启动'
      ElMessage.error(message)
      return Promise.reject(new ApiError(-1, message))
    }

    const raw = response.data as unknown
    // 文件下载（responseType: 'blob'）出错时，响应体是被包成 Blob 的 JSON 错误体，
    // 必须先解出文本，否则拿不到业务码，用户只能看到「请求失败（HTTP 400）」。
    const body = raw instanceof Blob ? await readBlobError(raw) : (raw as ApiResult | undefined)
    const code = body?.code ?? response.status
    const message = body?.message || `请求失败（HTTP ${response.status}）`
    const traceId = body?.traceId

    // 令牌失效：尝试静默刷新并重放请求
    if (AUTH_ERROR_CODES.includes(code)) {
      const original = error.config as AxiosRequestConfig & { _retry?: boolean }
      if (!original._retry && getRefreshToken()) {
        original._retry = true
        try {
          const token = await refreshAccessToken()
          original.headers = { ...(original.headers || {}), Authorization: `Bearer ${token}` }
          return instance.request(original)
        } catch {
          // 刷新失败，走下面的统一登出逻辑
        }
      }
      ElMessageBox.confirm('登录状态已失效，请重新登录', '提示', {
        confirmButtonText: '重新登录',
        showCancelButton: false,
        type: 'warning',
      })
        .then(() => redirectToLogin())
        .catch(() => redirectToLogin())
      return Promise.reject(new ApiError(code, message, traceId))
    }

    // 无权限：不弹窗打断，交由调用方决定提示方式
    if (code === 40300) {
      ElMessage.error(message)
      return Promise.reject(new ApiError(code, message, traceId))
    }

    ElMessage.error(message)
    return Promise.reject(new ApiError(code, message, traceId))
  },
)

/** 统一解包：成功返回 data，失败抛出 ApiError */
async function unwrap<T>(promise: Promise<AxiosResponse<ApiResult<T>>>): Promise<T> {
  const response = await promise
  const body = response.data
  if (body.code !== 0) {
    throw new ApiError(body.code, body.message, body.traceId)
  }
  return body.data
}

/** 类型安全的请求方法集合 */
export const http = {
  get<T>(url: string, params?: Record<string, unknown>, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(instance.get(url, { params, ...config }))
  },
  post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(instance.post(url, data, config))
  },
  put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(instance.put(url, data, config))
  },
  delete<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(instance.delete(url, { data, ...config }))
  },
}

export default instance
