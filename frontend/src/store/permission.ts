import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { RouteRecordRaw, Router } from 'vue-router'
import { profileApi } from '@/api'
import type { RouteRecord } from '@/types/api'
import { ROOT_ROUTE_NAME } from '@/router'
import { buildRoutes, firstLeafPath } from '@/router/helper'

/**
 * 动态路由与侧边栏菜单。
 *
 * 设计取舍：侧边栏菜单使用后端返回的「树形结构」（保留目录层级），
 * 而 vue-router 只注册「拍平后的叶子路由」，避免多层 router-view 嵌套带来的复杂度。
 * 两者共用同一份数据源，不会出现菜单与路由不一致的问题。
 *
 * 路由的挂载与卸载都由本 store 负责：router.addRoute 返回的卸载函数会保存下来，
 * 在切换账号或退出登录时统一调用，避免上一个账号的路由残留在路由表里。
 */
export const usePermissionStore = defineStore('permission', () => {
  /** 后端返回的原始菜单树，供侧边栏渲染 */
  const menus = ref<RouteRecord[]>([])
  /** 注册到 vue-router 的路由（已拍平） */
  const routes = ref<RouteRecordRaw[]>([])
  /** 是否已加载完成 */
  const loaded = ref(false)

  /** addRoute 返回的卸载函数，需在重置时调用 */
  let disposers: Array<() => void> = []

  /** 卸载已注册的动态路由 */
  function unmountRoutes(): void {
    disposers.forEach((dispose) => dispose())
    disposers = []
  }

  /** 拉取当前用户的动态路由、转换并挂载到 Layout 之下 */
  async function generateRoutes(router: Router): Promise<RouteRecordRaw[]> {
    const records = await profileApi.routes()
    menus.value = records
    routes.value = buildRoutes(records)
    unmountRoutes()
    routes.value.forEach((route) => {
      disposers.push(router.addRoute(ROOT_ROUTE_NAME, route))
    })
    loaded.value = true
    return routes.value
  }

  /**
   * 首页路径：第一个可访问的叶子路由。
   * 兜底用 /profile —— 它是静态路由，任何登录用户都一定存在，
   * 不会出现「没有菜单的用户被送到 404」的情况。
   */
  function homePath(): string {
    return firstLeafPath(menus.value) || '/profile'
  }

  /** 重置（退出登录或切换账号时调用） */
  function reset(): void {
    unmountRoutes()
    menus.value = []
    routes.value = []
    loaded.value = false
  }

  return { menus, routes, loaded, generateRoutes, homePath, reset }
})
