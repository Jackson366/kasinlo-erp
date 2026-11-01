import type { RouteRecord } from '@/types/api'

// Project only the built-in directory; custom menus and authorized leaves stay server-owned.
const businessGroups = [
  {
    name: 'sales',
    title: '销售',
    icon: 'Sell',
    pages: ['InvSales', 'InvCustomer'],
  },
  {
    name: 'purchase',
    title: '采购',
    icon: 'ShoppingCart',
    pages: ['InvPurchase', 'InvSupplier'],
  },
  {
    name: 'inventory',
    title: '库存',
    icon: 'Box',
    pages: ['InvStock', 'InvProduct'],
  },
  { name: 'finance', title: '财务', icon: 'Money', pages: ['InvFinance'] },
  { name: 'reports', title: '报表', icon: 'TrendCharts', pages: ['InvReport'] },
]

export function navigationMenus(records: RouteRecord[]): RouteRecord[] {
  return records
    .filter((item) => !item.meta?.hidden)
    .flatMap((item) => {
      if (item.name !== 'Inv' || item.path !== '/inv' || item.meta?.isExternal) return [item]
      const children = (item.children ?? []).filter((child) => !child.meta?.hidden)
      const groupedNames = new Set<string>()
      const groups = businessGroups.flatMap((group, index) => {
        const leaves = children.filter(
          (child) =>
            group.pages.includes(child.name) && !child.children?.length && !child.meta?.isExternal,
        )
        if (!leaves.length) return []
        leaves.forEach((leaf) => groupedNames.add(leaf.name))
        return [
          {
            id: -(index + 1),
            name: `Navigation-${group.name}`,
            path: `navigation:${group.name}`,
            component: '',
            meta: { ...item.meta, title: group.title, icon: group.icon },
            children: leaves,
          },
        ]
      })
      return [...groups, ...children.filter((child) => !groupedNames.has(child.name))]
    })
}

export function navigationTrail(records: RouteRecord[], path: string): RouteRecord[] {
  for (const item of records) {
    if (item.meta?.hidden || item.meta?.isExternal) continue
    if (!item.children?.length && item.path === path) return [item]
    const trail = navigationTrail(item.children ?? [], path)
    if (trail.length) return [item, ...trail]
  }
  return []
}
