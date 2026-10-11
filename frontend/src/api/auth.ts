import { http } from '@/utils/request'
import type { LoginParams, LoginResult, ProfileInfo, RouteRecord } from '@/types/api'

/** 认证相关接口 */
export const authApi = {
  /** 登录 */
  login(data: LoginParams) {
    return http.post<LoginResult>('/auth/login', data)
  },
  /** 退出登录 */
  logout() {
    return http.post<string>('/auth/logout')
  },
}

/** 个人中心接口 */
export const profileApi = {
  /** 当前用户信息（含角色与权限） */
  info() {
    return http.get<ProfileInfo>('/profile')
  },
  /** 当前用户可访问的动态路由树 */
  routes() {
    return http.get<RouteRecord[]>('/profile/routes')
  },
  /** 修改个人资料 */
  update(data: { nickname: string; email?: string; phone?: string; avatar?: string }) {
    return http.put<string>('/profile', data)
  },
  /** 修改密码 */
  changePassword(data: { oldPassword: string; newPassword: string }) {
    return http.put<string>('/profile/password', data)
  },
}
