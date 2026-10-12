/**
 * 退出登录（含「改密后强制重登」这类等价场景）的完整会话清理。
 *
 * 三件事必须一起做，少任何一件都会留下「半截会话」：
 *
 *  1. userStore.logout()     服务端登出 + 清令牌与身份；
 *  2. permissionStore.reset() 卸载动态路由。少了它，下一个账号登录后
 *     上一个账号注册的路由还挂在路由表上，generateRoutes 虽会先卸载，
 *     但「登录页 -> 新账号首页」这段窗口里旧菜单仍可渲染；
 *  3. appStore.clearTags()    清空标签页。少了它最直观：新账号的标签栏
 *     挂着上一个账号的页面，而对应路由已被卸载，点哪个都 404。
 *
 * 历史上 Navbar 的「退出登录」和个人中心的「改密后强制重登」是两份实现，
 * 后者漏了 2 和 3 —— 改密码后重新登录，标签栏还留着旧账号的页面。
 * 收敛到这一个 composable，两个入口共用同一套清理。
 *
 * 用法：
 *   const { logout } = useLogout()
 *   await logout()   // 内部已跳转 /login，提示文案由调用方自定
 */

import { useRouter } from 'vue-router'
import { useAppStore, usePermissionStore, useUserStore } from '@/store'

export function useLogout() {
  const router = useRouter()
  const userStore = useUserStore()
  const permissionStore = usePermissionStore()
  const appStore = useAppStore()

  /** 退出登录并清理会话，最后跳转登录页。消息提示由调用方负责。 */
  async function logout(): Promise<void> {
    await userStore.logout()
    permissionStore.reset()
    appStore.clearTags()
    await router.push('/login')
  }

  return { logout }
}
