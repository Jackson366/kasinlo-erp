import axios, { type AxiosInstance, type AxiosRequestConfig, type AxiosResponse } from 'axios'
// 值一律从子路径导入：element-plus 根入口引用全部组件，会禁用 tree-shaking
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { ApiResult, LoginResult } from '@/types/api'
import { API_PREFIX } from '@/utils/api'
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

/**
 * 「会话已失效」类业务码：未登录、令牌过期/无效、账号被禁用、账号被锁定。
 *
 * 40102 必须在内。管理员禁用某个账号后，该账号已经打开的页面会持续收到 40102，
 * 若不当成会话失效处理，用户只会不断看到报错弹窗，却始终回不到登录页。
 *
 * 40103（账号锁定）同理：账号被锁定后，任何业务请求都会被 authz 拦下，
 * 不处理的话页面会一直「报错但不跳转」，用户完全不知道发生了什么。
 */
const AUTH_ERROR_CODES = [40100, 40102, 40103, 40104, 40105]

/**
 * 登录 / 刷新接口自身的失败就是「没登进去」，不该再走一遍会话失效流程。
 *
 * `/auth/logout` 必须在里面：登出时令牌可能已经过期，此时接口会返回 401，
 * 而「登出」本来就是用户主动要离开——再弹一个「登录状态已失效，请重新登录」
 * 是纯粹的噪音，且会让登出后残留一个模态框盖在登录页上。
 */
const AUTH_ENDPOINTS = ['/auth/login', '/auth/refresh', '/auth/logout']

/** 判断请求是否发往认证接口本身 */
function isAuthEndpoint(url?: string): boolean {
  if (!url) return false
  return AUTH_ENDPOINTS.some((endpoint) => url.includes(endpoint))
}

const instance: AxiosInstance = axios.create({
  // 版本段（/v1）统一在这里注入，各 api 模块只写相对路径
  baseURL: API_PREFIX,
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
        `${API_PREFIX}/auth/refresh`,
        { refreshToken },
        { timeout: 15000 },
      )
      .then((res) => {
        const body = res.data
        if (body.code !== 0 || !body.data?.accessToken) {
          throw new ApiError(body.code, body.message || '刷新登录状态失败')
        }
        setTokens(body.data.accessToken, body.data.refreshToken)
        // 刷新成功说明会话仍然有效，把「已提示过失效」的闸门复位，
        // 否则用户下次令牌过期时就看不到提示了
        resetSessionExpiredGuard()
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

/**
 * 「登录已失效」的提示与跳转，只做一次。
 *
 * 为什么必须有这个闸门：令牌过期时页面上的请求往往**同时**在飞（列表、统计、字典…），
 * 每一个都会走到这段逻辑。若不加锁，N 个并发 401 就会弹 N 个模态框，
 * 而 `window.location.href` 的跳转还没执行完——用户看到的是屏幕上糊着一堆「登录状态已失效」，
 * 点哪个都跳过去。这不是美观问题，是让人无法判断当前状态的可用性问题。
 */
let sessionExpiredNotified = false

/**
 * 令牌刷新成功后要复位，否则用户下次过期时不会再看到提示。
 *
 * 用函数声明而非 const 箭头函数：refreshAccessToken 定义在它之前，
 * 箭头函数不存在提升，运行时会在首轮 401 时撞上 TDZ。
 */
export function resetSessionExpiredGuard(): void {
  sessionExpiredNotified = false
}

function notifySessionExpired(code: number, message: string, traceId?: string): Promise<never> {
  if (!sessionExpiredNotified) {
    sessionExpiredNotified = true
    ElMessageBox.confirm('登录状态已失效，请重新登录', '提示', {
      confirmButtonText: '重新登录',
      showCancelButton: false,
      type: 'warning',
      // 点遮罩不允许关闭：登录已失效时除了重新登录没有别的出路
      closeOnClickModal: false,
    })
      .then(() => redirectToLogin())
      .catch(() => redirectToLogin())
  }
  return Promise.reject(new ApiError(code, message, traceId))
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

    // 令牌失效：尝试静默刷新并重放请求。
    // 裸 HTTP 401（无业务响应体，如被网关/代理直接拦截）也按会话失效处理：
    // 此时 code 回退为 response.status，不在业务码清单里，若落进通用 toast，
    // 用户会卡在注定失败的页面上反复看到「请求失败（HTTP 401）」却回不到登录页。
    const authFailed = AUTH_ERROR_CODES.includes(code) || response.status === 401
    if (authFailed && !isAuthEndpoint(error.config?.url)) {
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
      return notifySessionExpired(code, message, traceId)
    }

    // 40300（无权限）与兜底分支行为一致：弹轻提示 + reject。
    // 保留弹窗是因为 403 大多来自用户刚点的操作，完全静默会让人以为按钮坏了；
    // 调用方拿到 rejected promise，自行决定后续处理。
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
