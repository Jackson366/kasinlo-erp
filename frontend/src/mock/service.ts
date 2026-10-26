import type {
  InvOrderDetail,
  InvOrderPayload,
  InvProductPayload,
  InvContractPayload,
  InvContractRow,
  InvStockOrderPayload,
} from '@/types/api'
import { createSeed, routes, today, timestamp, type MockData, type Row } from './seed'

export class MockError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}
const fail = (message: string, status = 400): never => {
  throw new MockError(status, message)
}
const num = (value: unknown) => Number(value) || 0
const money = (value: number) => Math.round(value * 100) / 100
const sum = <T>(list: T[], fn: (item: T) => number) => money(list.reduce((s, r) => s + fn(r), 0))

export interface MockRequest {
  method: string
  path: string
  params?: Row
  body?: unknown
  token?: string
}
export function createMockService(initial = createSeed(), save: (data: MockData) => void = () => {}) {
  let db = structuredClone(initial)
  const rowById = <T extends { id: number }>(rows: T[], id: number): T =>
    rows.find((r) => r.id === id) ?? fail('Mock：记录不存在', 404)
  const nextId = () => db.nextId++
  function refreshSystemRows(): void {
    for (const user of db.collections['/system/users']) {
      const roleIds = Array.isArray(user.roleIds) ? user.roleIds : []
      Object.assign(user, {
        roleIds,
        roleNames: db.collections['/system/roles'].filter((r) => roleIds.includes(r.id)).map((r) => r.name),
        deptName: db.collections['/system/depts'].find((d) => d.id === user.deptId)?.name || '',
        email: user.email || '',
        phone: user.phone || '',
        avatar: user.avatar || '',
      })
    }
    for (const role of db.collections['/system/roles']) {
      role.menuIds ??= []
      role.dataScope ??= 1
      role.userCount = db.collections['/system/users'].filter((u) =>
        (u.roleIds as number[]).includes(num(role.id)),
      ).length
    }
  }
  function tree(list: Row[], parentId = 0, seen = new Set<number>()): Row[] {
    return list
      .filter((r) => num(r.parentId) === parentId && !seen.has(num(r.id)))
      .map((r) => ({
        ...r,
        children: tree(list, num(r.id), new Set([...seen, num(r.id)])),
      }))
  }
  const inventory = () =>
    db.products.flatMap((p) =>
      p.specs.map((s) => ({
        productId: p.id,
        productCode: p.code,
        productName: p.name,
        categoryName: p.categoryName,
        specId: s.id,
        spec: s.spec,
        unitId: s.units[0].id,
        unit: s.units[0].unit,
        stock: s.stock,
        warnStock: s.warnStock,
        warn: s.stock < s.warnStock,
        purchasePrice: s.units[0].purchasePrice,
      })),
    )
  const refreshProducts = () =>
    db.products.forEach((p) => {
      p.totalStock = sum(p.specs, (s) => s.stock)
      p.warn = p.specs.some((s) => s.stock < s.warnStock)
      p.specCount = p.specs.length
      p.unitCount = sum(p.specs, (s) => s.units.length)
    })
  function filtered<T extends object>(list: T[], q: Row = {}): T[] {
    return list
      .filter((item) => {
        const r = item as Row
        if (
          q.keyword &&
          !Object.values(r).some(
            (v) => typeof v === 'string' && v.toLowerCase().includes(String(q.keyword).toLowerCase()),
          )
        )
          return false
        for (const key of [
          'bizType',
          'status',
          'partyId',
          'categoryId',
          'direction',
          'sourceType',
          'settleStatus',
          'deptId',
          'docType',
          'stage',
          'username',
          'module',
          'method',
          'success',
          'source',
        ]) {
          if (q[key] !== undefined && q[key] !== '' && String(q[key]) !== String(r[key])) return false
        }
        if (q.warnOnly && !r.warn) return false
        if (q.overdueOnly && !r.overdue) return false
        if (q.onlyUnsettled && !(num(r.outstanding) > 0)) return false
        if (q.settle === 'overdue' && !r.overdue) return false
        if (q.settle && q.settle !== 'overdue' && q.settle !== r.settleStatus) return false
        const date = String(r.orderDate || r.signDate || r.createdAt || '')
        if (q.startDate && date.slice(0, 10) < String(q.startDate)) return false
        if (q.endDate && date.slice(0, 10) > String(q.endDate)) return false
        return true
      })
      .sort((a, b) => {
        if (!q.orderBy) return 0
        const key = String(q.orderBy).replace(/_([a-z])/g, (_, ch: string) => ch.toUpperCase())
        const av = (a as Row)[key],
          bv = (b as Row)[key]
        const result =
          typeof av === 'number' && typeof bv === 'number' ? av - bv : String(av).localeCompare(String(bv))
        return q.order === 'asc' ? result : -result
      })
  }
  function page<T extends object>(list: T[], q: Row = {}) {
    const rows = filtered(list, q),
      page = Math.max(1, num(q.page) || 1),
      pageSize = Math.min(200, Math.max(1, num(q.pageSize) || 20))
    return { list: rows.slice((page - 1) * pageSize, page * pageSize), total: rows.length, page, pageSize }
  }
  const finance = (bizType: string) => [
    ...db.orders
      .filter((o) => o.bizType === bizType && o.status === 2)
      .map((o) => ({
        ...o,
        source: 'order',
        orderId: o.id,
        contractId: 0,
        contractNo: '',
        planId: 0,
        planSeq: 0,
        planName: '',
        outstanding: money(o.totalAmount - o.settledAmount),
        overdue: !!o.dueDate && o.dueDate < today() && o.settledAmount < o.totalAmount,
      })),
    ...db.contracts
      .filter((c) => c.bizType === bizType && c.status === 1)
      .flatMap((c) =>
        (c.plans || [])
          .filter((p) => p.status !== 'pending')
          .map((p) => ({
            source: 'plan',
            orderId: 0,
            orderNo: '',
            contractId: c.id,
            contractNo: c.contractNo,
            planId: p.id,
            planSeq: p.seq,
            planName: p.name,
            partyId: c.partyId,
            partyName: c.partyName,
            orderDate: c.signDate,
            dueDate: p.dueDate,
            totalAmount: p.amount,
            settledAmount: p.settledAmount,
            outstanding: p.outstanding,
            settleStatus: p.outstanding === 0 ? 'paid' : p.settledAmount > 0 ? 'partial' : 'unpaid',
            overdue: !!p.dueDate && p.dueDate < today() && p.outstanding > 0,
          })),
      ),
  ]
  function orderItems(payload: InvOrderPayload) {
    if (!payload.items?.length) fail('请添加商品明细')
    return payload.items.map((i) => {
      const p = rowById(db.products, i.productId),
        s = rowById(p.specs, i.specId),
        u = rowById(s.units, i.unitId)
      if (p.status !== 1 || s.status !== 1 || u.status !== 1) fail('商品、规格或单位已停用')
      if (!(i.qty > 0) || i.price < 0) fail('数量必须大于零，价格不能为负数')
      return {
        ...i,
        id: nextId(),
        productCode: p.code,
        productName: p.name,
        spec: s.spec,
        unit: u.unit,
        rate: u.rate,
        qtyBase: money(i.qty * u.rate),
        amount: money(i.qty * i.price),
      }
    })
  }
  function stockChange(order: InvOrderDetail, reverse = false) {
    for (const i of order.items) {
      const s = rowById(rowById(db.products, i.productId).specs, i.specId)
      const delta = (order.bizType === 'purchase' ? 1 : -1) * (reverse ? -1 : 1) * i.qtyBase
      if (s.stock + delta < 0) fail('Mock：库存不足，无法执行此操作')
      s.stock = money(s.stock + delta)
    }
    refreshProducts()
  }
  function settleOrder(id: number, amount: number) {
    const o = rowById(db.orders, id)
    if (o.status !== 2 || amount <= 0 || amount > money(o.totalAmount - o.settledAmount))
      fail('收付款金额必须大于零且不超过已审核单据的未结余额')
    o.settledAmount = money(o.settledAmount + amount)
    o.settleStatus = o.settledAmount === o.totalAmount ? 'paid' : 'partial'
  }
  function dispatch({ method, path, params: q = {}, body, token }: MockRequest): unknown {
    const b = (body || {}) as Row
    const isDemo = token === 'mock-access-demo'
    if (path === '/auth/login' && method === 'POST') {
      if (!['admin', 'demo'].includes(String(b.username)) || b.password !== `${b.username}123456`)
        fail('用户名或密码错误（Mock 账号：admin / demo）', 401)
      return {
        accessToken: `mock-access-${b.username}`,
        refreshToken: `mock-refresh-${b.username}`,
        expiresIn: 86400,
        tokenType: 'Bearer',
        user: {
          userId: b.username === 'admin' ? 1 : 2,
          username: b.username,
          nickname: b.username === 'admin' ? '模拟管理员' : '模拟访客',
          avatar: '',
          deptId: 1,
          roleIds: [1],
          roleCodes: [b.username === 'admin' ? 'super_admin' : 'viewer'],
          dataScope: 1,
          isSuper: b.username === 'admin',
        },
      }
    }
    if (path === '/auth/refresh' && method === 'POST') {
      if (!['mock-refresh-admin', 'mock-refresh-demo'].includes(String(b.refreshToken)))
        fail('Mock 会话已失效', 401)
      return {
        accessToken: String(b.refreshToken).replace('refresh', 'access'),
        refreshToken: b.refreshToken,
      }
    }
    if (!['mock-access-admin', 'mock-access-demo'].includes(token || '')) fail('请先登录 Mock 账号', 401)
    if (isDemo && method !== 'GET' && path !== '/auth/logout')
      fail('Mock 演示账号为只读，请使用 admin 登录操作', 403)
    if (path === '/auth/logout') return '已退出'
    if (path === '/profile/routes') return routes
    if (path === '/profile' && method === 'GET')
      return isDemo
        ? {
            ...db.profile,
            id: 2,
            username: 'demo',
            nickname: '模拟访客',
            isSuper: false,
            roles: ['只读访客'],
            roleCodes: ['viewer'],
            permissions: [
              'inv:overview:view',
              'inv:report:view',
              ...[
                'purchase',
                'sales',
                'contract',
                'stock',
                'finance',
                'print',
                'product',
                'category',
                'customer',
                'supplier',
              ].map((s) => `inv:${s}:list`),
              ...['user', 'role', 'menu', 'dept', 'log'].map((s) => `system:${s}:list`),
              'ops:list',
            ],
          }
        : db.profile
    if (path === '/profile' && method === 'PUT') {
      Object.assign(db.profile, b)
      return '资料已更新'
    }
    if (path === '/profile/password' || /\/system\/users\/\d+\/password$/.test(path))
      fail('Mock 模式不支持修改密码，请使用固定演示密码', 501)
    if (/export|import|\/file$/.test(path) || (path === '/inv/attachments' && method === 'POST'))
      fail('Mock 模式暂不支持文件导入、导出和附件上传下载，请切换真实后端', 501)
    if (path === '/inv/attachments/count') return {}
    if (path === '/inv/attachments') return []
    if (path === '/system/logs/modules') return ['inv', 'system']
    if (path === '/system/roles/options') return db.collections['/system/roles']
    if (path === '/inv/categories/options')
      return db.collections['/inv/categories'].filter((r) => r.status === 1)
    if (path === '/inv/products/pick') {
      const items = filtered(
        db.products.filter((p) => p.status === 1),
        q,
      ).map((p) => ({
        ...p,
        specs: p.specs
          .filter((s) => s.status === 1)
          .map((s) => ({ ...s, units: s.units.filter((u) => u.status === 1) })),
      }))
      return { items, total: items.length, truncated: false }
    }
    if (path === '/inv/products/duplicates')
      return db.products
        .filter((p) => p.name === q.name && p.id !== num(q.excludeId))
        .map((p) => ({ ...p, specSummary: p.specs.map((s) => s.spec).join('、') }))
    if (path === '/inv/inventory') return page(inventory(), q)
    if (path.startsWith('/inv/finance/')) {
      if (method === 'GET') {
        const all = filtered(finance(path.endsWith('payables') ? 'purchase' : 'sales'), q)
        return {
          ...page(all, q),
          summary: {
            total: sum(all, (r) => r.totalAmount),
            settled: sum(all, (r) => r.settledAmount),
            outstanding: sum(all, (r) => r.outstanding),
            overdue: sum(
              all.filter((r) => r.overdue),
              (r) => r.outstanding,
            ),
            orderTotal: sum(
              all.filter((r) => r.source === 'order'),
              (r) => r.totalAmount,
            ),
            planTotal: sum(
              all.filter((r) => r.source === 'plan'),
              (r) => r.totalAmount,
            ),
          },
        }
      }
      if (path === '/inv/finance/settlements') {
        settleOrder(num(b.orderId), num(b.amount))
        return '登记成功'
      }
      if (path.endsWith('/batch')) {
        const ids = [...new Set(b.orderIds as number[])]
        const amounts = b.amounts as Record<number, number> | undefined
        let amount = 0
        for (const id of ids) {
          const o = rowById(db.orders, id)
          const n = amounts?.[id] ?? money(o.totalAmount - o.settledAmount)
          settleOrder(id, n)
          amount += n
        }
        return { count: ids.length, amount: money(amount) }
      }
    }
    const sales = db.orders.filter((o) => o.bizType === 'sales' && o.status === 2),
      purchases = db.orders.filter((o) => o.bizType === 'purchase' && o.status === 2)
    const salesAmount = sum(sales, (o) => o.totalAmount),
      purchaseAmount = sum(purchases, (o) => o.totalAmount)
    if (path === '/inv/overview')
      return {
        customerCount: db.collections['/inv/customers'].length,
        activeCustomer: 2,
        monthNewCustomer: 2,
        receivableTotal: sum(finance('sales'), (r) => r.outstanding),
        payableTotal: sum(finance('purchase'), (r) => r.outstanding),
        monthSales: salesAmount,
        monthPurchase: purchaseAmount,
        pendingOrders: db.orders.filter((o) => o.status === 1).length,
        productCount: db.products.length,
        warnProductCount: db.products.filter((p) => p.warn).length,
      }
    if (path.startsWith('/inv/report/')) {
      const stock = inventory(),
        totalValue = sum(stock, (s) => s.stock * s.purchasePrice)
      switch (path.split('/').pop()) {
        case 'summary':
          return {
            range: q.range || 'month',
            salesAmount,
            costAmount: purchaseAmount,
            grossProfit: salesAmount - purchaseAmount,
            grossRate: salesAmount ? ((salesAmount - purchaseAmount) / salesAmount) * 100 : 0,
            salesOrders: sales.length,
            purchaseOrders: purchases.length,
            purchases: purchaseAmount,
          }
        case 'trend':
          return Array.from({ length: Math.min(90, num(q.days) || 30) }, (_, i) => ({
            date: new Date(Date.now() - i * 86400000).toISOString().slice(0, 10),
            salesAmount: i === 0 ? salesAmount : 0,
            purchaseAmount: i === 0 ? purchaseAmount : 0,
          })).reverse()
        case 'ranking': {
          const orders = filtered(q.biz === 'purchase' ? purchases : sales, q)
          const groups = new Map<
            number,
            { id: number; name: string; amount: number; orderIds: Set<number>; quantity: number }
          >()
          for (const order of orders) {
            for (const item of order.items) {
              const id = q.dimension === 'product' ? item.productId : order.partyId
              const name = q.dimension === 'product' ? item.productName : order.partyName
              const group = groups.get(id) || {
                id,
                name,
                amount: 0,
                orderIds: new Set<number>(),
                quantity: 0,
              }
              group.amount += item.amount
              group.quantity += item.qty
              group.orderIds.add(order.id)
              groups.set(id, group)
            }
          }
          const all = [...groups.values()].sort((a, b) => b.amount - a.amount)
          const total = sum(all, (r) => r.amount)
          return all.slice(0, num(q.limit) || all.length).map((r, i) => ({
            id: r.id,
            rank: i + 1,
            name: r.name,
            partyName: r.name,
            amount: money(r.amount),
            orderCount: r.orderIds.size,
            quantity: r.quantity,
            ratio: total ? Math.round((r.amount / total) * 100) : 0,
          }))
        }
        case 'category':
          return ['工业配件', '电子设备'].map((category) => ({
            category,
            amount: sum(
              sales
                .flatMap((o) => o.items)
                .filter((r) => rowById(db.products, r.productId).categoryName === category),
              (r) => r.amount,
            ),
            ratio: 50,
          }))
        case 'inventory':
          return {
            totalValue,
            specCount: stock.length,
            warnCount: stock.filter((s) => s.warn).length,
            categoryValue: ['工业配件', '电子设备'].map((category) => ({
              category,
              value: sum(
                stock.filter((s) => s.categoryName === category),
                (s) => s.stock * s.purchasePrice,
              ),
              ratio: 50,
            })),
            warnList: stock.filter((s) => s.warn),
          }
        case 'stock-ranking':
          return stock.map((s, i) => ({
            ...s,
            rank: i + 1,
            value: money(s.stock * s.purchasePrice),
            ratio: totalValue ? ((s.stock * s.purchasePrice) / totalValue) * 100 : 0,
          }))
        case 'detail': {
          const rows = filtered(db.orders, q).flatMap((o) =>
            o.items.map((i) => ({
              ...i,
              orderId: o.id,
              orderNo: o.orderNo,
              orderDate: o.orderDate,
              partyName: o.partyName,
              salesman: o.salesman,
            })),
          )
          return {
            rows,
            total: rows.length,
            amount: sum(rows, (r) => r.amount),
            qty: sum(rows, (r) => r.qty),
            truncated: false,
          }
        }
        case 'contract': {
          const rows = filtered(db.contracts, q).map((c) => ({ ...c, contractId: c.id }))
          return {
            rows,
            count: rows.length,
            amount: sum(rows, (r) => r.amount),
            planAmount: sum(rows, (r) => r.planAmount),
            settledAmount: sum(rows, (r) => r.settledAmount),
            outstanding: sum(rows, (r) => r.outstanding),
            retentionAmount: 0,
            retentionOutstanding: 0,
            truncated: false,
          }
        }
        case 'contract-ledger':
          return []
      }
    }
    if (path === '/ops/backups' && method === 'GET') return []
    if (path === '/ops/cleanup/scopes') return []
    if (path === '/ops/cleanup/preview')
      return { dryRun: true, steps: [], vacuumed: [], blocked: 'Mock 模式不执行数据库清理' }
    if (path === '/ops/check')
      return {
        report: {
          database: 'Mock（浏览器本地数据）',
          serverVersion: 'Mock',
          sizeBytes: 0,
          tables: 0,
          schemaVersionDb: 1,
          schemaVersionBinary: 1,
          schemaVersionKnown: true,
          opLogRows: 1,
          opLogOldest: timestamp(),
          loginLogRows: 1,
          idempotencyRows: 0,
          idempotencyExpired: 0,
          latestBackup: '',
          latestBackupAt: '',
          checkedAt: timestamp(),
          keyTables: [],
        },
        warnings: ['Mock 模式，未连接数据库'],
      }
    if (path === '/ops/upgrade' && method === 'GET')
      return {
        release: 'mock',
        schemaDb: 1,
        schemaBinary: 1,
        schemaKnown: true,
        schemaNote: 'Mock',
        latestBackup: '',
        latestBackupAt: '',
        database: 'Mock',
        serverVersion: 'Mock',
        sizeBytes: 0,
        preflight: [],
        records: [],
        warnings: ['Mock 模式不执行升级'],
        restartHint: '',
      }
    if (path === '/ops/upgrade/records') return []
    if (path.startsWith('/ops/')) fail('Mock 模式不执行备份、恢复、清理或系统升级', 501)

    const match =
      /^(\/inv\/(?:products|orders|contracts|stock-orders)|\/system\/users)(?:\/(\d+))?(?:\/(\w+))?$/.exec(
        path,
      )
    if (match) {
      const [, resource, rawId, action] = match,
        id = num(rawId)
      if (resource === '/inv/orders') {
        if (method === 'GET') return id ? rowById(db.orders, id) : page(db.orders, q)
        const old = id ? rowById(db.orders, id) : undefined
        if (action === 'status' || action === 'reverse') {
          const o = old!
          if (action === 'reverse') {
            if (o.status !== 2 || o.settledAmount > 0) fail('仅可反审核未收付款的已完成单据')
            stockChange(o, true)
            o.status = 1
            db.collections['/inv/stock-orders'] = db.collections['/inv/stock-orders'].filter(
              (s) => s.sourceId !== o.id || s.sourceType !== o.bizType,
            )
          } else {
            if (o.status !== 1 || ![2, 3].includes(num(b.status))) fail('仅可审核或取消待处理单据')
            if (num(b.status) === 2) {
              stockChange(o)
              db.collections['/inv/stock-orders'].unshift({
                id: nextId(),
                orderNo: `CK${nextId()}`,
                direction: o.bizType === 'purchase' ? 'in' : 'out',
                sourceType: o.bizType,
                sourceId: o.id,
                sourceOrderNo: o.orderNo,
                orderDate: o.orderDate,
                status: 1,
                itemCount: o.items.length,
                totalQty: sum(o.items, (i) => i.qtyBase),
                remark: o.remark,
                salesman: o.salesman,
                operator: '模拟管理员',
                createdAt: timestamp(),
                items: o.items.map((i) => ({ ...i, unitBase: i.unit })),
              })
            }
            o.status = num(b.status) as 2 | 3
            if (o.status === 2 && b.settleNow) settleOrder(o.id, o.totalAmount)
          }
          return '操作成功'
        }
        if (old && old.status !== 1) fail('仅可修改或删除待处理单据')
        if (method === 'DELETE') {
          db.orders = db.orders.filter((o) => o.id !== id)
          return '已删除'
        }
        const payload = body as InvOrderPayload
        if (!['purchase', 'sales'].includes(payload.bizType)) fail('单据类型无效')
        const party = rowById(
          db.collections[payload.bizType === 'purchase' ? '/inv/suppliers' : '/inv/customers'] as (Row & {
            id: number
          })[],
          payload.partyId,
        )
        const items = orderItems(payload)
        const order: InvOrderDetail = {
          ...(old || {}),
          ...payload,
          id: old?.id || nextId(),
          orderNo:
            payload.orderNo || old?.orderNo || `${payload.bizType === 'purchase' ? 'CG' : 'XS'}${nextId()}`,
          partyName: String(party.name),
          dueDate: payload.dueDate || null,
          status: 1,
          totalAmount: sum(items, (i) => i.amount),
          settledAmount: 0,
          settleStatus: 'unpaid',
          itemCount: items.length,
          items,
          remark: payload.remark || '',
          salesman: payload.salesman || '',
          operator: '模拟管理员',
          createdAt: old?.createdAt || timestamp(),
        }
        if (old) Object.assign(old, order)
        else db.orders.unshift(order)
        return method === 'POST' ? { id: order.id } : '已更新'
      }
      if (resource === '/inv/products') {
        if (method === 'GET') return id ? rowById(db.products, id) : page(db.products, q)
        const old = id ? rowById(db.products, id) : undefined
        if (method === 'DELETE') {
          if (db.orders.some((o) => o.items.some((i) => i.productId === id))) fail('商品已被单据引用')
          db.products = db.products.filter((p) => p.id !== id)
          return '已删除'
        }
        const payload = body as InvProductPayload
        if (!payload.specs?.length || payload.specs.some((s) => !s.units?.length))
          fail('请至少添加一项规格和计量单位')
        const p = {
          ...createSeed().products[0],
          ...old,
          ...payload,
          id: old?.id || nextId(),
          code: payload.code || old?.code || `SP${nextId()}`,
          categoryId: payload.categoryId || 0,
          categoryName: String(
            db.collections['/inv/categories'].find((c) => c.id === payload.categoryId)?.name || '',
          ),
          createdAt: old?.createdAt || timestamp(),
          specs: payload.specs.map((s) => ({
            ...s,
            id: s.id || nextId(),
            stock: old?.specs.find((v) => v.id === s.id)?.stock || 0,
            warnStock: s.warnStock || 0,
            warn: false,
            status: s.status ?? 1,
            units: s.units.map((u) => ({
              ...u,
              id: nextId(),
              status: u.status ?? 1,
              purchasePrice: u.purchasePrice || 0,
              salePrice: u.salePrice || 0,
            })),
          })),
        }
        if (old) Object.assign(old, p)
        else db.products.unshift(p)
        refreshProducts()
        return method === 'POST' ? { id: p.id } : '已更新'
      }
      if (resource === '/inv/contracts') {
        if (action === 'logs') return []
        if (method === 'GET') return id ? rowById(db.contracts, id) : page(db.contracts, q)
        const old = id ? rowById(db.contracts, id) : undefined
        if (action) {
          const c = old!
          if (action === 'audit') {
            if (c.status !== 0) fail('仅可审核待审核合同')
            c.status = 1
            c.stage = 'pending'
            c.stageLabel = '待验收'
            c.auditedAt = timestamp()
            c.auditor = '模拟管理员'
            c.plans?.forEach((p) => {
              if (p.triggerNode === 'sign') {
                p.status = 'due'
                p.statusLabel = '待收付'
              }
            })
          } else if (action === 'unaudit') {
            if (c.status !== 1 || c.settledAmount) fail('仅可反审核未收付款的执行中合同')
            c.status = 0
            c.stage = 'unaudited'
            c.stageLabel = '待审核'
            c.plans?.forEach((p) => {
              p.status = 'pending'
              p.statusLabel = '未到期'
            })
          } else if (action === 'accept') {
            if (c.status !== 1) fail('合同尚未审核')
            c.acceptedAt = String(b.acceptedAt)
            c.acceptResult = String(b.acceptResult)
            c.stage = 'accepted'
            c.stageLabel = '已验收'
          } else fail('Mock 尚未实现此合同操作', 501)
          return '操作成功'
        }
        if (old && old.status !== 0) fail('仅可编辑或删除待审核合同')
        if (method === 'DELETE') {
          db.contracts = db.contracts.filter((c) => c.id !== id)
          return '已删除'
        }
        const p = body as InvContractPayload
        const c: InvContractRow = {
          ...createSeed().contracts[0],
          ...p,
          id: old?.id || nextId(),
          contractNo: p.contractNo || old?.contractNo || `HT${nextId()}`,
          partyName: String(
            db.collections[p.bizType === 'purchase' ? '/inv/suppliers' : '/inv/customers'].find(
              (r) => r.id === p.partyId,
            )?.name || '',
          ),
          amount: p.amount || 0,
          status: 0,
          startDate: p.startDate || null,
          endDate: p.endDate || null,
          createdAt: old?.createdAt || timestamp(),
          plans: (p.plans || []).map((v, i) => ({
            ...v,
            id: v.id || nextId(),
            seq: i + 1,
            kindLabel: '付款计划',
            rate: v.rate || 0,
            amount: v.amount || money(((p.amount || 0) * (v.rate || 0)) / 100),
            dueDate: v.dueDate || null,
            triggerNode: v.triggerNode || '',
            settledAmount: 0,
            outstanding: v.amount || money(((p.amount || 0) * (v.rate || 0)) / 100),
            status: 'pending',
            statusLabel: '未到期',
            remark: v.remark || '',
          })),
        }
        c.planCount = c.plans!.length
        c.planAmount = sum(c.plans!, (r) => r.amount)
        c.outstanding = c.planAmount
        if (old) Object.assign(old, c)
        else db.contracts.unshift(c)
        return method === 'POST' ? { id: c.id } : '已更新'
      }
      if (resource === '/inv/stock-orders') {
        const rows = db.collections[resource]
        if (method === 'GET') return id ? rowById(rows as (Row & { id: number })[], id) : page(rows, q)
        if (method === 'DELETE') {
          const s = rowById(rows as (Row & { id: number })[], id)
          if (s.sourceType !== 'manual') fail('关联业务单据请通过反审核撤销')
          const o = s.mockOrder as InvOrderDetail
          stockChange(o, true)
          db.collections[resource] = rows.filter((r) => r.id !== id)
          return '已作废'
        }
        const payload = body as InvStockOrderPayload
        const mockOrder = {
          bizType: payload.direction === 'in' ? 'purchase' : 'sales',
          items: orderItems({
            ...payload,
            bizType: 'purchase',
            partyId: 1,
            items: payload.items.map((i) => ({ ...i, price: 0 })),
          }),
        } as InvOrderDetail
        stockChange(mockOrder)
        const s = {
          ...payload,
          id: nextId(),
          orderNo: payload.orderNo || `CK${nextId()}`,
          sourceType: 'manual',
          sourceId: 0,
          status: 1,
          itemCount: mockOrder.items.length,
          totalQty: sum(mockOrder.items, (i) => i.qtyBase),
          operator: '模拟管理员',
          createdAt: timestamp(),
          mockOrder,
          items: mockOrder.items.map((i) => ({ ...i, unitBase: i.unit })),
        }
        rows.unshift(s)
        return { id: s.id }
      }
    }
    if (path === '/inv/contract-plans/settle' || /^\/inv\/contract-plans\/\d+\/settles$/.test(path))
      fail('Mock 暂不支持合同期次收付款流水，请连接真实后端', 501)
    for (const [resource, list] of Object.entries(db.collections)) {
      if (path !== resource && !path.startsWith(`${resource}/`)) continue
      const [, rawId, action] = path.slice(resource.length).split('/'),
        id = num(rawId)
      if (rawId && !id) continue
      if (method === 'GET')
        return id
          ? rowById(list as (Row & { id: number })[], id)
          : ['/system/depts', '/system/menus'].includes(resource)
            ? tree(filtered(list, q))
            : page(list, q)
      if (method === 'POST' && !id) {
        const r: Row = {
          status: 1,
          sort: 0,
          remark: '',
          balance: 0,
          orderCount: 0,
          createdAt: timestamp(),
          ...b,
          id: nextId(),
          code: b.code || `MOCK${nextId()}`,
        }
        delete r.password
        list.unshift(r)
        return { id: r.id }
      }
      if (!id) fail('Mock 暂不支持此批量操作', 501)
      const r = rowById(list as (Row & { id: number })[], id)
      if (
        resource.startsWith('/system/') &&
        id === 1 &&
        (method === 'DELETE' || (action === 'status' && b.status === 0))
      )
        fail('内置管理员、角色和部门不可删除或禁用')
      if (method === 'DELETE') {
        db.collections[resource] = list.filter((r) => r.id !== id)
        return '已删除'
      }
      if (action && !['status', 'menus'].includes(action)) fail('Mock 暂不支持此操作', 501)
      Object.assign(r, b)
      delete r.password
      return '已更新'
    }
    fail(`Mock 尚未实现：${method} ${path}`, 501)
  }
  return {
    handle(req: MockRequest): unknown {
      const snapshot = structuredClone(db)
      try {
        refreshSystemRows()
        const result = dispatch(req)
        if (req.method !== 'GET') save(db)
        return structuredClone(result)
      } catch (error) {
        db = snapshot
        throw error
      }
    },
  }
}
