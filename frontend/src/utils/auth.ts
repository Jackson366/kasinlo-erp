import { ref } from 'vue'

const ACCESS_TOKEN_KEY = 'nova_access_token'
const REFRESH_TOKEN_KEY = 'nova_refresh_token'

/**
 * 令牌既持久化在 localStorage（跨会话保留），又同时保存在模块级 ref 里。
 *
 * 为什么要多一层 ref：localStorage **不是响应式的**。
 * 如果 getAccessToken() 每次都直接读 localStorage，那么
 * `computed(() => !!getAccessToken())` 会被永久缓存成首次求值的结果，
 * 典型症状是「登录成功后路由守卫仍认为未登录，被立刻弹回登录页」。
 * 这里把令牌作为唯一数据源，读取方（store / 请求拦截器）自然都是响应式的。
 */
const accessToken = ref(localStorage.getItem(ACCESS_TOKEN_KEY) || '')
const refreshToken = ref(localStorage.getItem(REFRESH_TOKEN_KEY) || '')

/** 读取访问令牌 */
export function getAccessToken(): string {
  return accessToken.value
}

/** 读取刷新令牌 */
export function getRefreshToken(): string {
  return refreshToken.value
}

/** 是否持有访问令牌 */
export function hasAccessToken(): boolean {
  return !!accessToken.value
}

/** 保存令牌对 */
export function setTokens(token: string, refresh?: string): void {
  accessToken.value = token
  localStorage.setItem(ACCESS_TOKEN_KEY, token)
  if (refresh) {
    refreshToken.value = refresh
    localStorage.setItem(REFRESH_TOKEN_KEY, refresh)
  }
}

/** 清除全部令牌 */
export function clearTokens(): void {
  accessToken.value = ''
  refreshToken.value = ''
  localStorage.removeItem(ACCESS_TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
}
