import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RouteMeta, RouteRecord } from '@/types/api'
import { availableViewPaths, buildRoutes, firstLeafPath } from './helper'

/** 构造一条菜单记录，只写用例关心的字段，其余补默认值 */
type RecInput = Partial<Omit<RouteRecord, 'meta'>> & { path: string; meta?: Partial<RouteMeta> }

function rec(input: RecInput): RouteRecord {
  // path 是必填的定位字段，name 默认与之一致
  if (!input.name) input.name = input.path
  return {
    id: 1,
    component: '',
    ...input,
    name: input.name,
    meta: {
      title: input.meta?.title ?? input.name,
      icon: input.meta?.icon ?? '',
      hidden: input.meta?.hidden ?? false,
      keepAlive: input.meta?.keepAlive ?? false,
      isExternal: input.meta?.isExternal ?? false,
    },
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('availableViewPaths', () => {
  it('返回真实存在的页面组件路径（去掉 ../views/ 前缀与 .vue 后缀）', () => {
    const paths = availableViewPaths()
    expect(paths).toContain('system/user/index')
    expect(paths).toContain('inv/overview/index')
    expect(paths.every((p) => !p.startsWith('../') && !p.endsWith('.vue'))).toBe(true)
  })
})

describe('buildRoutes', () => {
  it('目录节点不注册为路由，叶子拍平挂到顶层', () => {
    const routes = buildRoutes([
      rec({
        path: '/system',
        children: [
          rec({ path: '/system/user', component: 'system/user/index' }),
          rec({ path: '/system/role', component: 'system/role/index' }),
        ],
      }),
    ])
    expect(routes.map((r) => r.path)).toEqual(['/system/user', '/system/role'])
    // 目录本身不应出现：它只用于侧边栏分组
    expect(routes.some((r) => r.path === '/system')).toBe(false)
  })

  it('叶子节点补齐 meta 默认值', () => {
    const [route] = buildRoutes([
      rec({ path: '/overview', name: 'Overview', component: 'inv/overview/index' }),
    ])
    expect(route.meta?.title).toBe('Overview')
    expect(route.meta?.icon).toBe('')
    expect(route.meta?.keepAlive).toBe(false)
    expect(route.meta?.hidden).toBe(false)
  })

  it('外链节点不注册内部路由', () => {
    const routes = buildRoutes([
      rec({ path: '/ext', component: 'inv/overview/index', meta: { isExternal: true } }),
    ])
    expect(routes).toEqual([])
  })

  it('组件路径不存在时跳过该节点并告警，而不是产出会白屏的路由', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const routes = buildRoutes([rec({ path: '/ghost', component: 'no/such/view' })])
    expect(routes).toEqual([])
    expect(warn).toHaveBeenCalled()
  })

  it('组件路径为空时跳过', () => {
    expect(buildRoutes([rec({ path: '/empty' })])).toEqual([])
  })
})

describe('firstLeafPath', () => {
  it('返回第一个可访问的叶子路径（深度优先）', () => {
    const records = [
      rec({
        path: '/system',
        children: [rec({ path: '/system/user', component: 'system/user/index' })],
      }),
      rec({ path: '/overview', component: 'inv/overview/index' }),
    ]
    expect(firstLeafPath(records)).toBe('/system/user')
  })

  it('隐藏节点不能作为落地页', () => {
    const records = [
      rec({ path: '/hidden', meta: { hidden: true } }),
      rec({ path: '/visible', component: 'inv/overview/index' }),
    ]
    expect(firstLeafPath(records)).toBe('/visible')
  })

  it('外链节点不能作为落地页（它没有对应的内部路由）', () => {
    const records = [
      rec({ path: '/ext', meta: { isExternal: true } }),
      rec({ path: '/visible', component: 'inv/overview/index' }),
    ]
    expect(firstLeafPath(records)).toBe('/visible')
  })

  it('没有页面组件的节点不能作为落地页', () => {
    // 这类节点会被 buildRoutes 跳过，选它做落地页只会落到 404
    const records = [rec({ path: '/no-component' }), rec({ path: '/visible', component: 'inv/overview/index' })]
    expect(firstLeafPath(records)).toBe('/visible')
  })

  it('组件路径写错的节点不能作为落地页', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    const records = [
      rec({ path: '/typo', component: 'system/user/indx' }),
      rec({ path: '/visible', component: 'inv/overview/index' }),
    ]
    expect(firstLeafPath(records)).toBe('/visible')
  })

  it('父目录下的子节点全部不可用时，继续在后续兄弟节点里找', () => {
    const records = [
      rec({ path: '/empty-dir', children: [] }),
      rec({ path: '/a', children: [rec({ path: '/a/hidden', meta: { hidden: true } })] }),
      rec({ path: '/b', component: 'inv/overview/index' }),
    ]
    expect(firstLeafPath(records)).toBe('/b')
  })

  it('没有任何可访问节点时返回空串', () => {
    expect(firstLeafPath([rec({ path: '/only', meta: { hidden: true } })])).toBe('')
    expect(firstLeafPath([])).toBe('')
  })
})
