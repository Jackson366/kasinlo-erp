import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { authApi, profileApi } from '@/api'
import type { LoginParams, ProfileInfo, UserIdentity } from '@/types/api'
import { clearTokens, hasAccessToken, setTokens } from '@/utils/auth'
import { resetSessionExpiredGuard } from '@/utils/request'

/**
 * 当前登录用户的状态：身份、资料、权限集合。
 * 权限判断统一走 hasPerm / hasAnyPerm，不要在组件里直接读数组。
 */
export const useUserStore = defineStore('user', () => {
  const profile = ref<ProfileInfo | null>(null)
  const identity = ref<UserIdentity | null>(null)

  /** 是否已登录（本地存在访问令牌，随令牌变化而更新） */
  const isLoggedIn = computed(hasAccessToken)
  /** 是否超级管理员：拥有全部权限 */
  const isSuper = computed(() => profile.value?.isSuper ?? false)
  /** 权限码集合 */
  const permissions = computed<string[]>(() => profile.value?.permissions ?? [])
  /** 角色标识集合 */
  const roles = computed<string[]>(() => profile.value?.roleCodes ?? [])
  /** 展示名 */
  const displayName = computed(
    () => profile.value?.nickname || profile.value?.username || identity.value?.username || '未登录',
  )
  /** 头像地址 */
  const avatar = computed(() => profile.value?.avatar || '')

  /** 是否拥有指定权限 */
  function hasPerm(code?: string): boolean {
    if (!code) return true
    if (isSuper.value) return true
    return permissions.value.includes(code)
  }

  /** 是否拥有任一权限 */
  function hasAnyPerm(codes: string[]): boolean {
    if (!codes.length) return true
    if (isSuper.value) return true
    return codes.some((code) => permissions.value.includes(code))
  }

  /** 登录：保存令牌并拉取完整资料 */
  async function login(params: LoginParams): Promise<void> {
    const result = await authApi.login(params)
    setTokens(result.accessToken, result.refreshToken)
    identity.value = result.user
    // 新会话开始，把「已提示过登录失效」的闸门复位
    resetSessionExpiredGuard()
    await fetchProfile()
  }

  /** 拉取当前用户资料与权限 */
  async function fetchProfile(): Promise<ProfileInfo> {
    const info = await profileApi.info()
    profile.value = info
    return info
  }

  /** 退出登录，失败也强制清理本地状态 */
  async function logout(): Promise<void> {
    try {
      await authApi.logout()
    } catch {
      // 令牌可能已过期，忽略服务端错误
    } finally {
      reset()
    }
  }

  /** 清空登录态 */
  function reset(): void {
    profile.value = null
    identity.value = null
    clearTokens()
    // 主动登出后闸门也要复位，否则重新登录时会残留上一次的「不再提示」状态
    resetSessionExpiredGuard()
  }

  return {
    profile,
    identity,
    isLoggedIn,
    isSuper,
    permissions,
    roles,
    displayName,
    avatar,
    hasPerm,
    hasAnyPerm,
    login,
    fetchProfile,
    logout,
    reset,
  }
})
