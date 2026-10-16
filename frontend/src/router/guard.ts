import type { Router } from 'vue-router'
import NProgress from 'nprogress'
import 'nprogress/nprogress.css'
// 从子路径导入：根入口会构建完整安装器、引用全部组件，禁用 tree-shaking
import { ElMessage } from 'element-plus/es/components/message/index'
import { usePermissionStore, useUserStore } from '@/store'
import { WHITE_LIST } from '@/router'

NProgress.configure({ showSpinner: false, trickleSpeed: 120 })

const APP_TITLE = import.meta.env.VITE_APP_TITLE || 'Nova ERP'

/** 安装路由守卫：登录校验 + 动态路由注入 */
export function setupRouterGuard(router: Router): void {
  router.beforeEach(async (to, _from, next) => {
    NProgress.start()
    document.title = to.meta?.title ? `${to.meta.title} · ${APP_TITLE}` : APP_TITLE

    const userStore = useUserStore()
    const permissionStore = usePermissionStore()

    // 白名单路径直接放行；已登录用户访问登录页则回到首页
    if (WHITE_LIST.includes(to.path)) {
      if (to.path === '/login' && userStore.isLoggedIn) {
        next({ path: '/', replace: true })
        return
      }
      next()
      return
    }

    if (!userStore.isLoggedIn) {
      next({ path: '/login', query: { redirect: to.fullPath } })
      return
    }

    // 首次进入：拉取资料 -> 生成路由 -> 重新导航
    if (!permissionStore.loaded) {
      try {
        if (!userStore.profile) {
          await userStore.fetchProfile()
        }
        await permissionStore.generateRoutes(router)
      } catch {
        userStore.reset()
        permissionStore.reset()
        ElMessage.error('获取用户权限失败，请重新登录')
        next({ path: '/login', query: { redirect: to.fullPath } })
        return
      }
      // 必须按 path 重新导航，不能写成 next({ ...to, replace: true })。
      // 此刻 to 命中的是兜底路由（name 为 NotFound），展开后 name 会覆盖 path，
      // 于是又被送回 404 —— 表现为「刷新页面必进 404」。
      next({ path: to.path, query: to.query, hash: to.hash, replace: true })
      return
    }

    // 根路径按首个可访问菜单落地
    if (to.path === '/') {
      next({ path: permissionStore.homePath(), replace: true })
      return
    }

    next()
  })

  router.afterEach(() => {
    NProgress.done()
  })

  router.onError((error: unknown) => {
    NProgress.done()
    // 懒加载 chunk 失败会让 vue-router 静默中止导航（点了菜单没反应），
    // 这里识别错误文案后整页重载一次，并用时间戳防止死循环。
    const message = error instanceof Error ? error.message : String(error)
    if (!/Failed to fetch dynamically imported module|Importing a module script failed/i.test(message)) {
      return
    }
    const KEY = 'nova_chunk_reload_at'
    const last = Number(sessionStorage.getItem(KEY) || 0)
    if (Date.now() - last > 10_000) {
      sessionStorage.setItem(KEY, String(Date.now()))
      window.location.reload()
    }
  })
}
