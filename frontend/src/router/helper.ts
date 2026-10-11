import type { RouteRecordRaw } from 'vue-router'
import type { RouteRecord } from '@/types/api'

/**
 * 预加载全部页面组件。
 * 使用相对路径是因为 import.meta.glob 只支持字面量路径，不能用 @ 别名。
 */
const viewModules = import.meta.glob('../views/**/*.vue')

/** 把后端返回的组件路径（如 system/user/index）解析为异步组件 */
function resolveComponent(path: string): (() => Promise<unknown>) | undefined {
  if (!path) return undefined
  const key = `../views/${path}.vue`
  const loader = viewModules[key]
  if (!loader) {
    console.warn(`[router] 未找到组件 src/views/${path}.vue，请检查菜单配置`)
    return undefined
  }
  return loader
}

/**
 * 将后端菜单树转换为 vue-router 路由。
 *
 * 目录节点只用于侧边栏分组，不注册为路由；叶子节点拍平后统一挂在 Layout 之下，
 * 因此所有页面路径都是绝对路径（以 / 开头），侧边栏层级与路由层级解耦。
 */
export function buildRoutes(records: RouteRecord[]): RouteRecordRaw[] {
  const result: RouteRecordRaw[] = []

  const walk = (list: RouteRecord[]) => {
    list.forEach((record) => {
      if (record.meta?.isExternal) return
      const children = record.children ?? []
      if (children.length > 0) {
        walk(children)
        return
      }
      const component = resolveComponent(record.component)
      if (!component) return
      result.push({
        path: record.path,
        name: record.name,
        component,
        meta: {
          title: record.meta?.title ?? record.name,
          icon: record.meta?.icon ?? '',
          keepAlive: record.meta?.keepAlive ?? false,
          hidden: record.meta?.hidden ?? false,
          permission: record.meta?.permission ?? '',
        },
      })
    })
  }

  walk(records)
  return result
}

/** 找到第一个可访问的叶子路径，作为登录后的落地页 */
export function firstLeafPath(records: RouteRecord[]): string {
  for (const record of records) {
    const children = record.children ?? []
    if (children.length > 0) {
      const found = firstLeafPath(children)
      if (found) return found
      continue
    }
    if (!record.meta?.hidden && record.path) return record.path
  }
  return ''
}

