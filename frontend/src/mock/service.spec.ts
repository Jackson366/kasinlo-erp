import { describe, expect, it } from 'vitest'
import { readFileSync, existsSync } from 'node:fs'
import { createMockService, MockError } from './service'
import { createSeed, routes } from './seed'
import type { InvOrderDetail, PageResult, InvInventoryRow } from '@/types/api'

function setup() {
  const service = createMockService()
  const request = (
    method: string,
    path: string,
    body?: unknown,
    params?: Record<string, unknown>,
    token = 'mock-access-admin',
  ) => service.handle({ method, path, body, params, token })
  return { request }
}
const orderPayload = {
  bizType: 'purchase',
  partyId: 1,
  orderDate: '2026-10-26',
  items: [{ productId: 1, specId: 1, unitId: 1, qty: 3, price: 50 }],
}

describe('独立 Mock 模式', () => {
  it('校验账号密码、要求登录并执行 demo 只读限制', () => {
    const { request } = setup()
    expect(() => request('POST', '/auth/login', { username: 'admin', password: 'wrong' })).toThrow(
      '用户名或密码错误',
    )
    expect(request('POST', '/auth/login', { username: 'admin', password: 'admin123456' })).toMatchObject({
      accessToken: 'mock-access-admin',
    })
    expect(() => request('GET', '/profile', undefined, undefined, '')).toThrow('请先登录')
    expect(request('GET', '/profile', undefined, undefined, 'mock-access-demo')).toMatchObject({
      isSuper: false,
      username: 'demo',
    })
    expect(() => request('POST', '/inv/orders', orderPayload, undefined, 'mock-access-demo')).toThrow('只读')
  })
  it('菜单覆盖后端声明的每个页面且组件都存在', () => {
    const leaves = routes.flatMap((r) => r.children || [r])
    for (const module of ['inv', 'user', 'role', 'menu', 'dept', 'log', 'ops']) {
      const source = readFileSync(`../backend/internal/module/${module}/module.go`, 'utf8')
      for (const [, component] of source.matchAll(/Component: "([^"]+)"/g)) {
        expect(
          leaves.some((r) => r.component === component),
          component,
        ).toBe(true)
      }
    }
    for (const r of leaves) expect(existsSync(`src/views/${r.component}.vue`), r.path).toBe(true)
  })
  it('各页首屏接口具备正确的数据容器', () => {
    const { request } = setup()
    for (const path of [
      '/inv/customers',
      '/inv/suppliers',
      '/inv/categories',
      '/inv/products',
      '/inv/orders',
      '/inv/contracts',
      '/inv/stock-orders',
      '/inv/inventory',
      '/inv/print-templates',
      '/system/users',
      '/system/roles',
      '/system/logs/operations',
      '/system/logs/logins',
    ]) {
      expect(request('GET', path), path).toMatchObject({ list: expect.any(Array), total: expect.any(Number) })
    }
    for (const path of [
      '/inv/report/trend',
      '/inv/report/ranking',
      '/inv/report/category',
      '/inv/report/stock-ranking',
      '/inv/report/contract-ledger',
      '/system/depts',
      '/system/menus',
      '/system/roles/options',
      '/ops/backups',
    ])
      expect(request('GET', path), path).toBeInstanceOf(Array)
    expect(request('GET', '/inv/products/pick')).toMatchObject({ items: expect.any(Array), truncated: false })
    expect(request('GET', '/inv/finance/payables')).toMatchObject({
      summary: { outstanding: expect.any(Number) },
    })
    expect(request('GET', '/inv/report/detail')).toHaveProperty('rows')
    expect(request('GET', '/inv/report/contract')).toHaveProperty('rows')
    expect(request('GET', '/inv/report/inventory')).toHaveProperty('warnList')
    expect(request('GET', '/ops/check')).toHaveProperty('report')
    expect(request('GET', '/ops/upgrade')).toHaveProperty('preflight')
  })
  it('分页、查询、审核、反审核、结算和删除形成闭环', () => {
    const { request } = setup()
    expect(
      request('GET', '/inv/orders', undefined, { bizType: 'purchase', page: 2, pageSize: 5 }),
    ).toMatchObject({ total: 16, page: 2, pageSize: 5, list: expect.any(Array) })
    const { id } = request('POST', '/inv/orders', orderPayload) as { id: number }
    request('PUT', `/inv/orders/${id}`, { ...orderPayload, remark: '已编辑' })
    expect(request('GET', `/inv/orders/${id}`)).toMatchObject({ totalAmount: 150, remark: '已编辑' })
    const stock = () =>
      (request('GET', '/inv/inventory') as PageResult<InvInventoryRow>).list.find((r) => r.specId === 1)!
        .stock
    const before = stock()
    request('PUT', `/inv/orders/${id}/status`, { status: 2 })
    expect(stock()).toBe(before + 3)
    expect(() => request('PUT', `/inv/orders/${id}/status`, { status: 2 })).toThrow('待处理')
    request('PUT', `/inv/orders/${id}/reverse`)
    expect(stock()).toBe(before)
    request('PUT', `/inv/orders/${id}/status`, { status: 2 })
    request('POST', '/inv/finance/settlements', { orderId: id, amount: 150 })
    expect(request('GET', `/inv/orders/${id}`)).toMatchObject({ settledAmount: 150, settleStatus: 'paid' })
    expect(() => request('PUT', `/inv/orders/${id}/reverse`)).toThrow('未收付款')
    const draft = request('POST', '/inv/orders', orderPayload) as { id: number }
    request('DELETE', `/inv/orders/${draft.id}`)
    expect(() => request('GET', `/inv/orders/${draft.id}`)).toThrow('不存在')
  })
  it('库存不足或批量结算中途失败时整体回滚', () => {
    const { request } = setup()
    const { id } = request('POST', '/inv/orders', {
      ...orderPayload,
      bizType: 'sales',
      items: [{ ...orderPayload.items[0], qty: 101 }],
    }) as { id: number }
    expect(() => request('PUT', `/inv/orders/${id}/status`, { status: 2 })).toThrow('库存不足')
    expect(request('GET', `/inv/orders/${id}`)).toMatchObject({ status: 1 })
    const o = request('GET', '/inv/orders/4') as InvOrderDetail
    expect(() => request('POST', '/inv/finance/settlements/batch', { orderIds: [4, 999999] })).toThrow(
      '不存在',
    )
    expect(request('GET', '/inv/orders/4')).toMatchObject({ settledAmount: o.settledAmount })
  })
  it('保存后的数据可重新加载，返回值不会直接修改数据库', () => {
    let saved = createSeed()
    const service = createMockService(saved, (db) => {
      saved = structuredClone(db)
    })
    const req = { method: 'POST', path: '/inv/orders', body: orderPayload, token: 'mock-access-admin' }
    const { id } = service.handle(req) as { id: number }
    const reloaded = createMockService(saved)
    const read = { method: 'GET', path: `/inv/orders/${id}`, token: req.token }
    const result = reloaded.handle(read) as InvOrderDetail
    result.remark = '不应污染'
    expect((reloaded.handle(read) as InvOrderDetail).remark).not.toBe('不应污染')
  })
  it('新增系统用户的角色、部门和默认字段可被列表及编辑弹窗读取', () => {
    const { request } = setup()
    const { id } = request('POST', '/system/users', {
      username: 'mock-user',
      nickname: '新用户',
      roleIds: [1],
      deptId: 1,
    }) as { id: number }
    expect(request('GET', `/system/users/${id}`)).toMatchObject({
      roleNames: ['管理员'],
      deptName: '总部',
      email: '',
      avatar: '',
    })
    request('PUT', `/system/users/${id}`, { roleIds: [] })
    expect(request('GET', `/system/users/${id}`)).toMatchObject({ roleNames: [] })
  })
  it('排行按客户或商品聚合，金额与实际已审核单据一致', () => {
    const { request } = setup()
    const byCustomer = request('GET', '/inv/report/ranking') as { name: string; amount: number }[]
    expect(byCustomer.map((r) => r.name).sort()).toEqual(['上海远航制造', '苏州星河工业'])
    const sales = request('GET', '/inv/orders', undefined, {
      bizType: 'sales',
      status: 2,
      pageSize: 200,
    }) as PageResult<InvOrderDetail>
    expect(byCustomer.reduce((sum, r) => sum + r.amount, 0)).toBe(
      sales.list.reduce((sum, r) => sum + r.totalAmount, 0),
    )
  })
  it('未知请求与未支持的文件/运维操作明确报错', () => {
    const { request } = setup()
    for (const [method, path] of [
      ['GET', '/unknown'],
      ['GET', '/inv/export'],
      ['POST', '/ops/backups'],
      ['POST', '/system/users/import'],
    ]) {
      expect(() => request(method, path)).toThrow(MockError)
    }
  })
})
