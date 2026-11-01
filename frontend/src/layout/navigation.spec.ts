import { describe, expect, it } from 'vitest'
import type { RouteRecord } from '@/types/api'
import { routes } from '@/mock/seed'
import { navigationMenus, navigationTrail } from './navigation'

function leaves(items: RouteRecord[]): string[] {
  return items
    .filter((item) => !item.meta.hidden)
    .flatMap((item) => (item.children?.length ? leaves(item.children) : [item.path]))
}

describe('authorized navigation projection', () => {
  it('preserves every authorized destination without generating new pages or mutating the source', () => {
    const before = JSON.stringify(routes)
    expect(leaves(navigationMenus(routes)).sort()).toEqual(leaves(routes).sort())
    expect(JSON.stringify(routes)).toBe(before)
    expect(navigationMenus(routes).some((item) => item.meta.title === '采购')).toBe(true)
  })
  it('places contracts under finance and print templates under the existing system directory', () => {
    const menus = navigationMenus(routes)
    expect(navigationTrail(menus, '/inv/contract').map((item) => item.meta.title)).toEqual([
      '财务',
      '合同管理',
    ])
    expect(navigationTrail(menus, '/inv/print').map((item) => item.meta.title)).toEqual([
      '系统管理',
      '打印模板',
    ])
    expect(menus.some((item) => ['InvContract', 'InvPrint'].includes(item.name))).toBe(false)
  })
  it('keeps print accessible without adding system permissions, and hides unauthorized print entries', () => {
    const printOnly = structuredClone(routes[1])
    printOnly.children = printOnly.children!.filter((item) => item.name === 'InvPrint')
    const menus = navigationMenus([printOnly])
    expect(leaves(menus)).toEqual(['/inv/print'])
    expect(navigationTrail(menus, '/inv/print').map((item) => item.meta.title)).toEqual([
      '系统设置',
      '打印模板',
    ])
    printOnly.children![0].meta.hidden = true
    expect(navigationMenus([printOnly])).toEqual([])
  })
  it('omits empty business groups for restricted accounts', () => {
    const restricted = structuredClone(routes)
    restricted[1].children = restricted[1].children?.filter((item) => item.name === 'InvSales')
    const menus = navigationMenus(restricted)
    expect(menus.some((item) => item.meta.title === '采购')).toBe(false)
    expect(navigationTrail(menus, '/inv/purchase')).toEqual([])
    expect(navigationTrail(menus, '/inv/sales').map((item) => item.meta.title)).toEqual([
      '销售',
      '销售管理',
    ])
  })
  it('keeps hidden leaves and directories out of navigation trails', () => {
    const hidden = structuredClone(routes)
    hidden[1].children!.find((item) => item.name === 'InvStock')!.meta.hidden = true
    expect(leaves(navigationMenus(hidden))).not.toContain('/inv/stock')
    hidden[1].meta.hidden = true
    expect(navigationTrail(navigationMenus(hidden), '/inv/sales')).toEqual([])
  })
  it('preserves custom directories and external links, including nested children', () => {
    const custom = structuredClone(routes[1])
    custom.name = 'CustomInventory'
    expect(navigationMenus([custom])[0]).toBe(custom)
    const external = structuredClone(routes[1].children![0])
    external.meta.isExternal = true
    external.path = 'https://example.com'
    const inventory = structuredClone(routes[1])
    inventory.children = [external]
    expect(navigationMenus([inventory])[0]).toBe(external)
    expect(navigationTrail([external], external.path)).toEqual([])
  })
})
