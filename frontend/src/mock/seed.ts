import routeFixtures from './routes.json'
import type { InvProductRow, InvOrderDetail, InvContractRow, ProfileInfo, RouteRecord } from '@/types/api'

export type Row = Record<string, unknown>
export const STORAGE_KEY = 'kasinlo_mock_data_v1'
export const today = () => new Date().toISOString().slice(0, 10)
export const timestamp = () => new Date().toISOString()
export const routes: RouteRecord[] = [
  routeFixtures[0],
  ...(['inv', 'system'] as const).map((group, i) => ({
    id: 100 + i,
    name: group === 'inv' ? 'Inv' : 'System',
    path: `/${group}`,
    component: '',
    meta: {
      title: group === 'inv' ? '进销存' : '系统管理',
      icon: group === 'inv' ? 'Goods' : 'Setting',
      hidden: false,
      keepAlive: false,
      isExternal: false,
    },
    children: routeFixtures.filter((r) => r.path.startsWith(`/${group}/`) && r !== routeFixtures[0]),
  })),
]

export interface MockData {
  products: InvProductRow[]
  orders: InvOrderDetail[]
  contracts: InvContractRow[]
  profile: ProfileInfo
  collections: Record<string, Row[]>
  nextId: number
}

export function createSeed(): MockData {
  const createdAt = timestamp()
  const party = (id: number, name: string, code: string): Row => ({
    id,
    code,
    name,
    contact: '王经理',
    phone: '13800138000',
    address: '杭州市滨江区',
    taxNo: '',
    status: 1,
    remark: 'Mock 演示资料',
    balance: 0,
    orderCount: 0,
    createdAt,
  })
  const products: InvProductRow[] = ['工业连接器', '控制模块', '电源适配器', '传感器'].map((name, i) => ({
    id: i + 1,
    code: `SP000${i + 1}`,
    name,
    barcode: `690000000000${i}`,
    categoryId: (i % 2) + 1,
    categoryName: i % 2 ? '电子设备' : '工业配件',
    refPurchasePrice: 50 + i * 20,
    refSalePrice: 80 + i * 30,
    specCount: 1,
    unitCount: 1,
    totalStock: i === 2 ? 5 : 100,
    warn: i === 2,
    status: 1,
    remark: 'Mock 演示商品',
    createdAt,
    specs: [
      {
        id: i + 1,
        spec: '标准型',
        stock: i === 2 ? 5 : 100,
        warnStock: 10,
        warn: i === 2,
        status: 1,
        units: [
          {
            id: i + 1,
            unit: '件',
            rate: 1,
            isBase: true,
            purchasePrice: 50 + i * 20,
            salePrice: 80 + i * 30,
            status: 1,
          },
        ],
      },
    ],
  }))
  const orders: InvOrderDetail[] = Array.from({ length: 32 }, (_, i) => {
    const p = products[i % products.length]
    const bizType = i < 16 ? 'purchase' : 'sales'
    const price = bizType === 'purchase' ? p.refPurchasePrice : p.refSalePrice
    const qty = ((i % 4) + 1) * 5
    const totalAmount = price * qty
    return {
      id: i + 1,
      orderNo: `${bizType === 'purchase' ? 'CG' : 'XS'}${today().replaceAll('-', '')}${String(i + 1).padStart(4, '0')}`,
      bizType,
      partyId: (i % 2) + 1,
      partyName:
        bizType === 'purchase'
          ? ['杭州精工供应链', '宁波联创科技'][i % 2]
          : ['上海远航制造', '苏州星河工业'][i % 2],
      orderDate: today(),
      dueDate: new Date(Date.now() + ((i % 3) - 1) * 86400000 * 7).toISOString().slice(0, 10),
      status: i % 4 === 0 ? 1 : 2,
      totalAmount,
      settledAmount: i % 4 === 1 ? totalAmount : i % 4 === 2 ? totalAmount / 2 : 0,
      settleStatus: i % 4 === 1 ? 'paid' : i % 4 === 2 ? 'partial' : 'unpaid',
      itemCount: 1,
      remark: 'Mock 演示单据',
      salesman: '张晓',
      operator: '模拟管理员',
      createdAt,
      items: [
        {
          id: i + 1,
          productId: p.id,
          productCode: p.code,
          productName: p.name,
          specId: p.specs[0].id,
          spec: '标准型',
          unitId: p.specs[0].units[0].id,
          unit: '件',
          rate: 1,
          qty,
          qtyBase: qty,
          price,
          amount: totalAmount,
        },
      ],
    }
  })
  const contracts: InvContractRow[] = [0, 1].map((i) => ({
    id: i + 1,
    contractNo: `HT202600${i + 1}`,
    name: ['年度配件采购合同', '设备销售合同'][i],
    bizType: i ? 'sales' : 'purchase',
    partyId: 1,
    partyName: i ? '上海远航制造' : '杭州精工供应链',
    signDate: today(),
    startDate: today(),
    endDate: null,
    amount: 20000,
    status: i ? 1 : 0,
    salesman: '张晓',
    operator: '模拟管理员',
    remark: 'Mock 演示合同',
    acceptedAt: null,
    acceptResult: '',
    warrantyMonths: 12,
    warrantyEndAt: null,
    retentionRate: 0,
    retentionAmount: 0,
    stage: i ? 'pending' : 'unaudited',
    stageLabel: i ? '待验收' : '待审核',
    auditedAt: i ? createdAt : null,
    auditor: i ? '模拟管理员' : '',
    planCount: 1,
    planAmount: 20000,
    settledAmount: 0,
    outstanding: 20000,
    retentionOutstanding: 0,
    attachmentCount: 0,
    createdAt,
    plans: [
      {
        id: i + 1,
        seq: 1,
        name: '首付款',
        kind: 1,
        kindLabel: '预付',
        rate: 100,
        amount: 20000,
        dueDate: today(),
        triggerNode: 'sign',
        settledAmount: 0,
        outstanding: 20000,
        status: i ? 'due' : 'pending',
        statusLabel: i ? '待收付' : '未到期',
        remark: '',
      },
    ],
  }))
  const profile: ProfileInfo = {
    id: 1,
    username: 'admin',
    nickname: '模拟管理员',
    email: 'mock@example.com',
    phone: '',
    avatar: '',
    deptId: 1,
    deptName: '总部',
    status: 1,
    roles: ['管理员'],
    roleCodes: ['super_admin'],
    isSuper: true,
    permissions: [],
  }
  const menus = routeFixtures.map((r) => ({
    ...r,
    ...r.meta,
    parentId: 0,
    type: 'menu',
    permission: '',
    sort: r.id,
    status: 1,
    redirect: '',
  }))
  return {
    products,
    orders,
    contracts,
    profile,
    nextId: 1000,
    collections: {
      '/inv/customers': [party(1, '上海远航制造', 'KH0001'), party(2, '苏州星河工业', 'KH0002')],
      '/inv/suppliers': [party(1, '杭州精工供应链', 'GYS0001'), party(2, '宁波联创科技', 'GYS0002')],
      '/inv/categories': ['工业配件', '电子设备'].map((name, i) => ({
        id: i + 1,
        parentId: 0,
        name,
        sort: i,
        status: 1,
        remark: '',
        productCount: 2,
        childCount: 0,
        createdAt,
      })),
      '/inv/print-templates': [
        {
          id: 1,
          code: 'purchase-default',
          name: '标准采购单',
          docType: 'purchase',
          paper: 'A4',
          content: '{}',
          status: 1,
          remark: '',
          createdAt,
        },
      ],
      '/inv/stock-orders': orders
        .filter((o) => o.status === 2)
        .map((o) => ({
          id: o.id,
          orderNo: `CK${o.id.toString().padStart(6, '0')}`,
          direction: o.bizType === 'purchase' ? 'in' : 'out',
          sourceType: o.bizType,
          sourceId: o.id,
          sourceOrderNo: o.orderNo,
          orderDate: o.orderDate,
          status: 1,
          itemCount: o.items.length,
          totalQty: o.items.reduce((s, i) => s + i.qtyBase, 0),
          remark: o.remark,
          salesman: o.salesman,
          operator: o.operator,
          createdAt,
          items: o.items.map((i) => ({ ...i, unitBase: i.unit })),
        })),
      '/inv/attachments': [],
      '/system/users': [
        {
          ...profile,
          roleIds: [1],
          roleNames: ['管理员'],
          remark: '',
          createdAt,
          lastLoginAt: createdAt,
          lastLoginIp: '127.0.0.1',
        },
      ],
      '/system/roles': [
        {
          id: 1,
          name: '管理员',
          code: 'super_admin',
          sort: 1,
          status: 1,
          dataScope: 1,
          isBuiltin: true,
          remark: '',
          userCount: 1,
          menuIds: menus.map((m) => m.id),
          createdAt,
        },
      ],
      '/system/depts': [
        {
          id: 1,
          parentId: 0,
          name: '总部',
          leader: '王经理',
          phone: '',
          email: '',
          sort: 1,
          status: 1,
          userCount: 1,
          children: [],
          createdAt,
        },
      ],
      '/system/menus': menus,
      '/system/logs/operations': [
        {
          id: 1,
          userId: 1,
          username: 'admin',
          module: 'inv',
          action: '查看采购单',
          method: 'GET',
          path: '/inv/orders',
          ip: '127.0.0.1',
          userAgent: 'Mock',
          request: '{}',
          status: 200,
          latencyMs: 12,
          errorMsg: '',
          createdAt,
        },
      ],
      '/system/logs/logins': [
        {
          id: 1,
          userId: 1,
          username: 'admin',
          ip: '127.0.0.1',
          userAgent: 'Mock',
          success: true,
          message: 'Mock 登录成功',
          createdAt,
        },
      ],
    },
  }
}
