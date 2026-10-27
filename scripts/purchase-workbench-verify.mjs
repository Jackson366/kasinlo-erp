/** Real Vue UI with deterministic mock API; does not test backend transactions.
 * cd frontend && npm run test:purchase
 * Install Chromium with `npx playwright install chromium`, or set CHROME_PATH.
 * The runner starts Vite in its own process and always closes the browser/server.
 */
import { createRequire } from 'node:module'
import { spawn } from 'node:child_process'
import fs from 'node:fs/promises'
import path from 'node:path'
import os from 'node:os'
import { fileURLToPath } from 'node:url'
const require = createRequire(
  new URL('../frontend/package.json', import.meta.url),
)
const { chromium, expect } = require('@playwright/test')
const root = fileURLToPath(new URL('..', import.meta.url))
const out =
  process.env.QA_OUTPUT || path.join(os.tmpdir(), 'kasinlo-purchase-qa')
await fs.mkdir(out, { recursive: true })
const app = process.env.APP_URL || 'http://127.0.0.1:6083'
const server = process.env.APP_URL
  ? null
  : spawn(
      process.execPath,
      [
        path.join(root, 'frontend/node_modules/vite/bin/vite.js'),
        '--host',
        '127.0.0.1',
        '--port',
        '6083',
        '--strictPort',
      ],
      { cwd: path.join(root, 'frontend'), stdio: 'pipe' },
    )
let serverLog = ''
server?.stdout.on('data', (b) => {
  serverLog += b
})
server?.stderr.on('data', (b) => {
  serverLog += b
})
let browser
let debugPage
const checks = []
const pageErrors = []
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
try {
  let ready = false
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(app)).ok) {
        ready = true
        break
      }
    } catch {}
    if (server?.exitCode !== null && server?.exitCode !== undefined)
      throw new Error(serverLog)
    await sleep(100)
  }
  if (!ready) throw new Error('Vite did not start: ' + serverLog)
  browser = await chromium.launch({
    executablePath: process.env.CHROME_PATH || undefined,
    headless: true,
    args: [
      '--no-sandbox',
      '--disable-dev-shm-usage',
      '--use-gl=angle',
      '--use-angle=swiftshader',
    ],
  })
  const context = await browser.newContext({
    viewport: { width: 1600, height: 1100 },
    locale: 'zh-CN',
  })
  await context.addInitScript(() => {
    localStorage.setItem('nova_access_token', 'fixture-token')
    localStorage.setItem('nova_sidebar_collapsed', '1')
  })
  const page = await context.newPage()
  debugPage = page
  page.on('pageerror', (e) => pageErrors.push(e.message))
  const item = {
    id: 1,
    productId: 1,
    productCode: 'SP001',
    productName: '304 不锈钢管',
    specId: 1,
    spec: 'φ20 × 2mm',
    unitId: 1,
    unit: '根',
    rate: 6,
    qty: 10,
    qtyBase: 60,
    price: 120,
    amount: 1200,
  }
  // The real API returns hydrated product snapshots after both create and edit.
  const hydrateItem = (it) => ({
    ...item,
    ...it,
    qtyBase: it.qty * (it.unitId === 2 ? 1 : 6),
    unit: it.unitId === 2 ? '米' : '根',
    rate: it.unitId === 2 ? 1 : 6,
    amount: it.qty * it.price,
  })
  const seed = (i) => ({
    id: i,
    orderNo: `CG20261025${String(i).padStart(3, '0')}`,
    bizType: 'purchase',
    partyId: 1,
    partyName:
      i % 2 ? '杭州恒信金属材料有限公司' : '宁波远航工业供应链有限公司',
    orderDate: '2026-10-25',
    dueDate: i === 1 ? null : '2026-11-25',
    status: 1,
    totalAmount: 1200,
    settledAmount: 0,
    settleStatus: 'unpaid',
    itemCount: 1,
    salesman: '陈敏',
    operator: '采购经办',
    remark: '到货后核对规格与数量。',
    createdAt: '2026-10-25T08:00:00Z',
    items: [{ ...item }],
  })
  const state = {
    orders: Array.from({ length: 12 }, (_, i) => seed(i + 1)),
    failList: false,
    failDetail: null,
    failSave: false,
    failAudit: false,
    readOnly: false,
    delayed: null,
    listCalls: 0,
    auditCalls: 0,
    updateCalls: 0,
    fullNavigation: false,
  }
  state.orders.push({
    ...seed(13),
    status: 2,
    settledAmount: 600,
    settleStatus: 'partial',
  })
  state.orders.push({ ...seed(14), status: 3 })
  const menus = [
    ['InvPurchase', '/inv/purchase', 'inv/purchase/index', '采购管理'],
    ['InvSales', '/inv/sales', 'inv/sales/index', '销售管理'],
  ].map(([name, p, component, title], i) => ({
    id: i + 100,
    name,
    path: p,
    component,
    meta: {
      title,
      icon: 'ShoppingCart',
      keepAlive: true,
      hidden: false,
      isExternal: false,
    },
  }))
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      endpoint = url.pathname.replace('/api/v1', '')
    const ok = (data) =>
      route.fulfill({ json: { code: 0, message: 'ok', data } })
    const fail = (message) =>
      route.fulfill({ status: 500, json: { code: 50100, message, data: null } })
    if (endpoint === '/profile')
      return ok({
        id: 1,
        username: 'buyer',
        nickname: '采购经办',
        avatar: '',
        status: 1,
        isSuper: !state.readOnly,
        permissions: ['inv:purchase:list', 'inv:sales:list'],
        roleCodes: ['buyer'],
        roles: [],
      })
    if (endpoint === '/profile/routes') {
      const inventory = {
        id: 90,
        name: 'Inventory',
        path: '/inv',
        component: '',
        meta: { title: '进销存', icon: 'Box' },
        children: [
          {
            id: 91,
            name: 'Trade',
            path: '/inv/trade',
            component: '',
            meta: { title: '单据管理', icon: 'Document' },
            children: menus,
          },
        ],
      }
      if (!state.fullNavigation) return ok([inventory])
      const leaves = [
        ['InvContract', 'contract', '合同管理', 'Document'],
        ['InvStock', 'stock', '库存管理', 'Box'],
        ['InvFinance', 'finance', '应收应付', 'Money'],
        ['InvReport', 'report', '报表中心', 'TrendCharts'],
        ['InvPrint', 'print', '打印模板', 'Printer'],
        ['InvProduct', 'product', '商品档案', 'Goods'],
        ['InvCustomer', 'customer', '客户管理', 'User'],
        ['InvSupplier', 'supplier', '供应商管理', 'Van'],
      ].map(([name, slug, title, icon], i) => ({
        id: 200 + i,
        name,
        path: '/inv/' + slug,
        component: 'inv/' + slug + '/index',
        meta: { title, icon },
      }))
      return ok([
        {
          id: 80,
          name: 'InvOverview',
          path: '/inv/overview',
          component: 'inv/overview/index',
          meta: { title: '数据概览', icon: 'Odometer' },
        },
        { ...inventory, children: [...menus, ...leaves] },
        {
          id: 300,
          name: 'System',
          path: '/system',
          component: '',
          meta: { title: '系统管理', icon: 'Setting' },
          children: [
            {
              id: 301,
              name: 'SystemUser',
              path: '/system/user',
              component: 'system/user/index',
              meta: { title: '用户管理', icon: 'User' },
            },
          ],
        },
        {
          id: 400,
          name: 'Hidden',
          path: '/hidden',
          meta: { title: '隐藏分组', hidden: true },
        },
      ])
    }
    if (endpoint === '/inv/overview') return ok({customerCount:0,productCount:0,pendingOrders:0})
    if (
      [
        '/inv/report/trend',
        '/inv/report/ranking',
        '/inv/report/category',
      ].includes(endpoint)
    )
      return ok([])
    if (state.failOptions && ['/inv/suppliers', '/inv/products/pick'].includes(endpoint))
      return fail('模拟资料加载失败')
    if (endpoint === '/inv/suppliers' || endpoint === '/inv/customers')
      return ok({
        list: [{ id: 1, name: '杭州恒信金属材料有限公司', status: 1 }],
        total: 1,
      })
    if (endpoint === '/inv/products/pick')
      return ok({
        items: [
          {
            id: 1,
            name: item.productName,
            code: 'SP001',
            refPurchasePrice: 120,
            specs: [
              {
                id: 1,
                spec: item.spec,
                units: [
                  {
                    id: 1,
                    unit: '根',
                    rate: 6,
                    isBase: false,
                    purchasePrice: 120,
                    salePrice: 150,
                  },
                  {
                    id: 2,
                    unit: '米',
                    rate: 1,
                    isBase: true,
                    purchasePrice: 20,
                    salePrice: 25,
                  },
                ],
              },
            ],
          },
        ],
        truncated: false,
      })
    if (endpoint === '/inv/attachments/count') return ok({ 1: 2 })
    if (endpoint === '/inv/attachments') return ok([])
    if (endpoint === '/inv/orders' && req.method() === 'GET') {
      state.listCalls++
      if (state.failList) return fail('模拟列表加载失败')
      const sales = url.searchParams.get('bizType') === 'sales'
      let list = sales
        ? [
            {
              ...seed(91),
              bizType: 'sales',
              orderNo: 'XS20261025001',
              partyName: '示例客户',
            },
          ]
        : state.orders
      const q = url.searchParams.get('keyword') || ''
      list = list.filter((r) =>
        `${r.orderNo}${r.partyName}${r.salesman}`.includes(q),
      )
      const status = Number(url.searchParams.get('status'))
      if (status) list = list.filter((r) => r.status === status)
      const settle = url.searchParams.get('settleStatus')
      if (settle) list = list.filter((r) => r.settleStatus === settle)
      const pageNum = Number(url.searchParams.get('page')) || 1,
        pageSize = Number(url.searchParams.get('pageSize')) || 10
      return ok({
        list: list.slice((pageNum - 1) * pageSize, pageNum * pageSize),
        total: list.length,
        page: pageNum,
        pageSize,
      })
    }
    const match = endpoint.match(
      /^\/inv\/orders\/(\d+)(?:\/(status|reverse))?$/,
    )
    if (match) {
      const id = Number(match[1]),
        row = state.orders.find((r) => r.id === id) || {
          ...seed(91),
          bizType: 'sales',
          orderNo: 'XS20261025001',
          partyName: '示例客户',
        }
      if (req.method() === 'GET') {
        if (state.delayed === id) await sleep(500)
        if (state.failDetail === id) return fail('模拟详情加载失败')
        return ok(row)
      }
      if (match[2] === 'status') {
        state.auditCalls++
        if (state.failAudit) return fail('模拟审核失败')
        row.status = 2
        return ok('已审核')
      }
      if (req.method() === 'PUT') {
        state.updateCalls++
        if (state.failSave) return fail('模拟保存失败')
        const requested = req.postDataJSON()
        Object.assign(row, requested, {items: requested.items.map(hydrateItem)})
        return ok('已保存')
      }
    }
    if (endpoint === '/inv/orders' && req.method() === 'POST') {
      if (state.failSave) return fail('模拟保存失败')
      const requested = req.postDataJSON()
      const row = {
        ...seed(100),
        ...requested,
        partyName: '杭州恒信金属材料有限公司',
        items: requested.items.map(hydrateItem),
      }
      row.totalAmount = row.items.reduce((sum, it) => sum + it.amount, 0)
      state.orders.unshift(row)
      return ok({ id: 100 })
    }
    throw new Error(`Unmocked API ${req.method()} ${endpoint}`)
  })
  const check = async (name, fn) => {
    await fn()
    checks.push(name)
    console.log('PASS ' + name)
  }
  const assertSidebarContrast = async () => {
    const ratios = await page.locator('.sidebar__primary-item.is-active, .sidebar__menu .el-menu-item.is-active').evaluateAll(elements => {
      const luminance = color => color.match(/[\d.]+/g).slice(0,3).map(Number).map(v => {
        v /= 255
        return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
      }).reduce((sum, v, i) => sum + v * [0.2126, 0.7152, 0.0722][i], 0)
      return elements.map(el => {
        const style = getComputedStyle(el)
        const [a,b] = [luminance(style.color), luminance(style.backgroundColor)].sort((a,b) => a-b)
        return (b + 0.05) / (a + 0.05)
      })
    })
    expect(ratios.length).toBe(2)
    for (const ratio of ratios) expect(ratio).toBeGreaterThanOrEqual(4.5)
  }
  const review = page.locator('.purchase-review')
  const queue = page.locator('.purchase-queue__item')
  await page.goto(app + '/#/inv/purchase')
  await check('默认待处理及真实明细/单位换算', async () => {
    await expect(queue).toHaveCount(10)
    await expect(review).toContainText(item.productName)
    await expect(review).toContainText('60')
    await expect(review).toContainText('未设付款期限')
    await expect(review).not.toContainText('未结清')
    await expect(page).toHaveURL(/view=pending/)
  })
  await check('紧凑标题与单条待办卡片不拉伸', async () => {
    const header = page.locator('.purchase-header')
    expect((await header.boundingBox()).height).toBeLessThanOrEqual(48)
    await expect(header.locator('p')).toHaveCount(0)
    // The optional guide adds its own measured height; keep the underlying workspace compact.
    const guide = page.locator('.order-page > .purchase-guide')
    const guideSpace = await guide.evaluate(el => el.getBoundingClientRect().height + parseFloat(getComputedStyle(el).marginBottom))
    expect((await page.locator('.order-page > .page-card').boundingBox()).y - guideSpace).toBeLessThan(190)
    expect((await guide.boundingBox()).height).toBeLessThan(110)
    const input = page.getByPlaceholder('单号 / 往来单位 / 业务员')
    await input.fill(state.orders[0].orderNo)
    await expect(queue).toHaveCount(1)
    expect((await queue.first().boundingBox()).height).toBeLessThan(160)
    await page.screenshot({path: out + '/purchase-single.png', fullPage: true})
    await input.fill('')
    await expect(queue).toHaveCount(10)
  })
  await page.screenshot({ path: out + '/purchase-desktop.png', fullPage: true })
  await check('选择与慢响应竞态', async () => {
    state.delayed = 2
    await queue.nth(1).click()
    await queue.nth(2).click()
    await expect(review.locator('.detail-head__no')).toHaveText(
      state.orders[2].orderNo,
    )
    await sleep(550)
    await expect(review.locator('.detail-head__no')).toHaveText(
      state.orders[2].orderNo,
    )
    state.delayed = null
  })
  await check('详情失败及重试', async () => {
    state.failDetail = 2
    await queue.nth(1).click()
    await expect(review).toContainText('采购单加载失败')
    state.failDetail = null
    await review.getByRole('button', { name: '重新加载', exact: true }).click()
    await expect(review.locator('.detail-head__no')).toHaveText(
      state.orders[1].orderNo,
    )
  })
  await check('供应商下拉与中文日历打开', async () => {
    await review.getByRole('button', { name: '编辑采购单' }).click()
    const dialog = page.getByRole('dialog', { name: '编辑单据', exact: true })
    await dialog.locator('.el-select').first().click()
    await page
      .getByRole('option', { name: '杭州恒信金属材料有限公司', exact: true })
      .click()
    await dialog.locator('.el-date-editor').first().click()
    await expect(
      page.locator('.el-date-picker:visible .el-date-picker__header'),
    ).toContainText('年')
    await page.keyboard.press('Escape')
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
  })
  await check('编辑失败保留输入与未保存关闭保护', async () => {
    await review.getByRole('button', { name: '编辑采购单' }).click()
    const dialog = page.getByRole('dialog', { name: '编辑单据', exact: true })
    const remark = dialog.getByPlaceholder('选填', { exact: true }).last()
    await remark.fill('保留这条修改')
    state.failSave = true
    await dialog.getByRole('button', { name: '保存采购单' }).click()
    await expect(dialog).toContainText('本次输入已保留')
    await expect(remark).toHaveValue('保留这条修改')
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    const confirm = page.getByRole('dialog', { name: '放弃本次修改？' })
    await confirm.getByRole('button', { name: '继续编辑' }).click()
    await expect(remark).toHaveValue('保留这条修改')
    state.failSave = false
    await dialog.getByRole('button', { name: '保存采购单' }).click()
    await expect(dialog).not.toBeVisible()
    await expect(review).toContainText('保留这条修改')
  })
  await check('审核失败恢复、成功结果与连续处理', async () => {
    await review
      .getByRole('button', { name: '审核并入库', exact: true })
      .click()
    const dialog = page.getByRole('dialog', { name: '审核并入库', exact: true })
    await expect(dialog).toContainText('请确认已收到货物')
    state.failAudit = true
    await dialog.getByRole('button', { name: '确认审核并入库' }).click()
    await expect(dialog).toBeVisible()
    state.failAudit = false
    await dialog.getByRole('button', { name: '确认审核并入库' }).click()
    await expect(review).toContainText('已生成入库单、增加库存')
    await expect(
      review.getByRole('button', { name: '审核并入库', exact: true }),
    ).toHaveCount(0)
    await review.getByRole('button', { name: '处理下一张' }).click()
    await expect(
      review.getByRole('button', { name: '审核并入库', exact: true }),
    ).toBeVisible()
  })
  await check('搜索无结果、即时清除与分页一次请求', async () => {
    const input = page.getByPlaceholder('单号 / 往来单位 / 业务员')
    await input.fill('不存在的供应商')
    await expect(page.locator('.purchase-queue')).toContainText(
      '没有符合条件的采购单',
    )
    await page
      .locator('.purchase-queue')
      .getByRole('button', { name: '清除筛选' })
      .click()
    await expect(queue).toHaveCount(10)
    const before = state.listCalls
    await page.locator('.el-pagination .btn-next').click()
    await expect(queue).toHaveCount(1)
    expect(state.listCalls - before).toBe(1)
    await expect(page).toHaveURL(/page=2/)
    await page.reload()
    await expect(queue).toHaveCount(1)
  })
  await check('列表失败独立于空队列并可恢复', async () => {
    state.failList = true
    await page.getByRole('button', { name: '刷新单据', exact: true }).click()
    await expect(page.locator('.purchase-list-error')).toContainText(
      '单据列表加载失败',
    )
    state.failList = false
    await page
      .locator('.purchase-list-error')
      .getByRole('button', { name: '重新加载' })
      .click()
    await expect(queue).toHaveCount(1)
  })
  await check('历史查账、付款语义与更多操作', async () => {
    await page.getByRole('tab', { name: '全部单据', exact: true }).click()
    await expect(page.locator('.purchase-workbench')).toHaveCount(0)
    await expect(page.locator('.el-table__header')).toContainText('付款到期日')
    await expect(page.locator('.el-table__body')).toContainText(
      '审核后形成应付',
    )
    const table = page.locator('.order-page .page-card > .el-table')
    const dateCell = table.locator('.el-table__body tr').first().locator('td .cell').nth(3)
    expect((await dateCell.boundingBox()).height).toBeLessThan(30)
    const detailHeader = table.locator('.el-table__header th .cell').nth(5)
    expect((await detailHeader.boundingBox()).height).toBeLessThan(30)
    const actionGroup = table.locator('.purchase-row-actions').first()
    const buttons = await actionGroup.locator('button').all()
    const boxes = await Promise.all(buttons.map(button => button.boundingBox()))
    const centers = boxes.map(box => box.y + box.height / 2)
    expect(Math.max(...centers) - Math.min(...centers)).toBeLessThan(2)
    const otherGroup = table.locator('.purchase-row-actions').nth(1)
    expect(Math.abs((await actionGroup.boundingBox()).x - (await otherGroup.boundingBox()).x)).toBeLessThan(2)
    await page.screenshot({path: out + '/purchase-history.png', fullPage: true})

    await page
      .locator('.el-table__body')
      .getByRole('button', { name: state.orders[0].orderNo, exact: true })
      .first()
      .click()
    await expect(
      page.getByRole('dialog', { name: '采购单据详情' }),
    ).toContainText(item.productName)
    await page
      .getByRole('dialog', { name: '采购单据详情' })
      .getByRole('button', { name: '关闭', exact: true })
      .click()
  })
  await check('窄屏队列/详情往返及键盘焦点', async () => {
    await page.setViewportSize({ width: 640, height: 900 })
    await page.getByRole('tab', { name: '待处理', exact: true }).click()
    await expect(queue.first()).toBeVisible()
    await queue.first().focus()
    await page.keyboard.press('Enter')
    await expect(review).toBeVisible()
    await expect(page.locator('.purchase-queue')).not.toBeVisible()
    await review.getByRole('button', { name: '返回待办' }).click()
    await expect(queue.first()).toBeFocused()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true)
  })
  await page.locator('.layout__content').evaluate((el) => {
    el.scrollTop = 0
  })
  await page
    .locator('.el-message')
    .last()
    .waitFor({ state: 'hidden' })
    .catch(() => {})
  await page.screenshot({ path: out + '/purchase-mobile.png', fullPage: true })
  await check('暗色模式与销售列表回归', async () => {
    await page.setViewportSize({ width: 1600, height: 1100 })
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await page.screenshot({ path: out + '/purchase-dark.png', fullPage: true })
    await page.goto(app + '/#/inv/sales')
    await expect(page.locator('.purchase-workbench')).toHaveCount(0)
    await expect(page.locator('.list-head__title')).toContainText('销售单据')
    await expect(
      page.locator('.order-page .page-card > .el-table .el-table__body'),
    ).toContainText('示例客户')
    await expect(
      page.getByRole('button', { name: '新增销售单', exact: true }),
    ).toBeVisible()
  })
  await check('只读权限不出现可执行审核或编辑', async () => {
    state.readOnly = true
    await page.goto(app + '/#/inv/purchase')
    await page.reload()
    await expect(review).toContainText(item.productName)
    await expect(
      review.getByRole('button', { name: '审核并入库', exact: true }),
    ).not.toBeVisible()
    await expect(
      page.getByRole('button', { name: '新增采购单', exact: true }),
    ).not.toBeVisible()
  })
  await check('新建采购单保存后回到待办核对', async () => {
    state.readOnly = false
    await page.reload()
    await page.getByRole('button', { name: '新增采购单', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '新增采购单', exact: true })
    await dialog.locator('.el-select').nth(0).click()
    await page
      .getByRole('option', { name: '杭州恒信金属材料有限公司', exact: true })
      .click()
    await dialog.locator('.item-editor__row .el-select.col-product').click()
    await page.getByRole('option').filter({ hasText: '304 不锈钢管' }).click()
    await dialog.getByRole('button', { name: '保存采购单' }).click()
    await expect(dialog).not.toBeVisible()
    await expect(review.locator('.detail-head__no')).toHaveText('CG20261025100')
  })
  await check('侧栏展开分组与保留文字的收起态', async () => {
    const rail = page.locator('.sidebar__rail')
    await expect(rail.getByRole('button', { name: '进销存', exact: true })).toBeVisible()
    expect((await rail.boundingBox()).width).toBe(132)
    const handle = page.getByRole('button', {name:'展开二级菜单',exact:true})
    await expect(handle).toBeVisible()
    const sideBox = await page.locator('.layout__aside').boundingBox()
    const handleBox = await handle.boundingBox()
    expect(Math.abs(handleBox.x + handleBox.width / 2 - sideBox.x - sideBox.width)).toBeLessThan(2)
    await rail.getByRole('button', { name: '进销存', exact: true }).click()
    const secondary = page.locator('.sidebar__secondary')
    await expect(secondary).toBeVisible()
    await expect(secondary.getByRole('menuitem', {name:'采购管理'})).toHaveClass(/is-active/)
    await page.waitForTimeout(250)
    expect((await page.locator('.layout__aside').boundingBox()).width).toBe(280)
    await page.screenshot({path:out+'/sidebar-expanded.png',fullPage:true})
    await secondary.locator('.el-sub-menu__title').filter({hasText:'单据管理'}).click()
    await expect(secondary.getByRole('menuitem', {name:'采购管理',exact:true})).not.toBeVisible()
    await page.getByRole('button', {name:'收起二级菜单',exact:true}).click()
    await page.getByRole('button', {name:'展开二级菜单',exact:true}).click()
    await expect(secondary.getByRole('menuitem', {name:'采购管理',exact:true})).not.toBeVisible()
    await secondary.locator('.el-sub-menu__title').filter({hasText:'单据管理'}).click()
    await expect(secondary.getByRole('menuitem', {name:'采购管理',exact:true})).toBeVisible()

    await page.getByRole('button', { name: '收起二级菜单', exact: true }).click()
    await expect(secondary).not.toBeVisible()
    await expect(rail.getByRole('button', { name: '进销存', exact: true })).toBeVisible()
    await page.waitForTimeout(250)
    expect((await page.locator('.layout__aside').boundingBox()).width).toBe(132)
    await page.screenshot({path:out+'/sidebar-collapsed.png',fullPage:true})
    await page.reload()
    await expect(page.locator('.sidebar__secondary')).not.toBeVisible()
    await expect(rail.getByRole('button', { name: '进销存', exact: true })).toBeVisible()
  })
  await check('标签页菜单由 ElDropdown 正常触发', async () => {
    await page.locator('.tags-view__more-btn').click()
    await expect(
      page.getByRole('menuitem', { name: '关闭其它标签' }),
    ).toBeVisible()
    await page.keyboard.press('Escape')
  })
  await check('完整侧栏：文字单行、产品图标、当前页面与路由切换', async () => {
    state.fullNavigation = true
    await page.reload()
    await expect(review).toContainText(item.productName)
    await page
      .locator('.sidebar__rail')
      .getByRole('button', { name: '进销存', exact: true })
      .click()
    await page.evaluate(() => document.documentElement.classList.remove('dark'))
    const primary = page.locator('.sidebar__primary')
    await expect(primary.locator('.sidebar__primary-item')).toHaveCount(3)
    const logo = page.locator('.sidebar__logo')
    expect(
      await logo.evaluate((img) => img.complete && img.naturalWidth > 0),
    ).toBe(true)
    expect(await page.locator('link[rel="icon"]').getAttribute('href')).toBe(
      '/brand/kasinlo-mark.png',
    )
    for (const label of await primary
      .locator('.sidebar__primary-item > span')
      .all()) {
      const size = await label.boundingBox()
      expect(size.height).toBeLessThanOrEqual(20)
      expect(
        await label.evaluate((el) => el.scrollWidth <= el.clientWidth),
      ).toBe(true)
    }
    await expect(primary.locator('.is-active')).toHaveCount(1)
    await expect(
      page.locator('.sidebar__secondary .el-menu-item.is-active'),
    ).toHaveText('采购管理')
    await page.waitForTimeout(250)
    await page.screenshot({
      path: out + '/sidebar-full-light.png',
      fullPage: true,
    })
    await expect(assertSidebarContrast).toPass({timeout:2000})
    await page
      .locator('.sidebar')
      .screenshot({ path: out + '/sidebar-detail-light.png' })
    await page
      .locator('.sidebar__secondary')
      .getByRole('menuitem', { name: '销售管理', exact: true })
      .click()
    await expect(page).toHaveURL(/\/inv\/sales/)
    await expect(
      page.locator('.sidebar__secondary .el-menu-item.is-active'),
    ).toHaveText('销售管理')
    await primary.getByRole('button', { name: '系统管理', exact: true }).click()
    await expect(page.locator('.sidebar__heading')).toHaveText('系统管理')
    await expect(page).toHaveURL(/\/inv\/sales/)
    await primary.getByRole('button', { name: '进销存', exact: true }).click()
    await page
      .locator('.sidebar__secondary')
      .getByRole('menuitem', { name: '采购管理', exact: true })
      .click()
    await expect(review).toContainText(item.productName)
  })
  await check('侧栏主题、键盘焦点、窄屏与减少动态效果', async () => {
    const handle = page.getByRole('button', {
      name: '收起二级菜单',
      exact: true,
    })
    await page.keyboard.press('Tab')
    await handle.focus()
    expect(
      await handle.evaluate((el) => getComputedStyle(el).outlineStyle),
    ).toBe('solid')
    await page.keyboard.press('Enter')
    await expect(page.locator('.sidebar__secondary')).not.toBeVisible()
    await expect(
      page.getByRole('button', { name: '展开二级菜单', exact: true }),
    ).toBeFocused()
    await page.waitForTimeout(250)
    await page
      .locator('.sidebar')
      .screenshot({ path: out + '/sidebar-detail-collapsed.png' })
    await page.keyboard.press('Enter')
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await page.waitForTimeout(250)
    await page
      .locator('.sidebar')
      .screenshot({ path: out + '/sidebar-detail-dark.png' })
    await expect(assertSidebarContrast).toPass({timeout:2000})
    await page.setViewportSize({ width: 640, height: 900 })
    await expect(page.locator('.sidebar__secondary')).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true)
    await page.emulateMedia({ reducedMotion: 'reduce' })
    expect(
      await page
        .locator('.layout__aside')
        .evaluate((el) => getComputedStyle(el).transitionDuration),
    ).toBe('0s')
    await expect(page.locator('.sidebar__rail')).toHaveCSS('transition-duration','0s')
    await expect(page.locator('.sidebar__primary-label').first()).toHaveCSS('transition-duration','0s')
    await page
      .getByRole('button', { name: '收起二级菜单', exact: true })
      .click()
    await expect(page.locator('.layout__aside')).toHaveCSS('width', '132px')
    await page
      .getByRole('button', { name: '展开二级菜单', exact: true })
      .click()
    await page
      .locator('.sidebar__rail')
      .getByRole('link', { name: '数据概览', exact: true })
      .click()
    await expect(page.locator('.overview-page')).toBeVisible()
    await expect(page.locator('.sidebar__toggle')).toHaveCount(0)
    await expect(page.locator('.sidebar__rail')).toHaveCSS('width', '132px')
    await expect(page.locator('.sidebar__primary .is-active')).toHaveCount(1)
    await page.setViewportSize({ width: 1600, height: 1100 })
  })
  await check('菜单层级与缩放：一级 20/18px、二级 16px、键盘与偏好恢复', async () => {
    await page.emulateMedia({reducedMotion:'no-preference'})
    await page.evaluate(() => document.documentElement.classList.remove('dark'))
    const sidebar = page.locator('.sidebar')
    const rail = page.locator('.sidebar__rail')
    const inventory = rail.getByRole('button', {name:'进销存',exact:true})
    await inventory.click()
    await page.locator('.sidebar__secondary').getByRole('menuitem',{name:'采购管理',exact:true}).click()
    await expect(page.locator('.layout__aside')).toHaveCSS('width','280px')
    const metrics = () => inventory.evaluate(el => {
      const icon = el.querySelector('.el-icon')
      const label = el.querySelector('span')
      const box = icon.getBoundingClientRect()
      return {width:box.width,height:box.height,x:box.x,font:getComputedStyle(label).fontSize,transform:getComputedStyle(icon).transform}
    })
    const baseline = await metrics()
    expect(baseline.width).toBe(20)
    expect(baseline.font).toBe('14px')
    const secondaryItem = page.locator('.sidebar__secondary .el-menu-item.is-active')
    await expect(secondaryItem).toHaveCSS('font-size','13px')
    await expect(secondaryItem).toHaveCSS('font-weight','500')
    expect((await secondaryItem.locator('.el-icon').boundingBox()).width).toBe(16)
    expect((await inventory.boundingBox()).height).toBeGreaterThan((await secondaryItem.boundingBox()).height)
    expect(await inventory.evaluate(el => getComputedStyle(el).backgroundColor)).not.toBe(await secondaryItem.evaluate(el => getComputedStyle(el).backgroundColor))
    // Measure during the actual width transition, not just the endpoint.
    const sampleTransition = async (trigger, width, {icon=20,font=14,resize=false} = {}) => {
      const sampling = inventory.evaluate(async el => {
        const samples = []
        const until = performance.now() + 350
        while (performance.now() < until) {
          await new Promise(requestAnimationFrame)
          const icon = el.querySelector('.el-icon'), label = el.querySelector('span')
          const box = icon.getBoundingClientRect()
          samples.push({width:box.width,height:box.height,x:box.x,font:getComputedStyle(label).fontSize,transform:getComputedStyle(icon).transform})
        }
        return samples
      })
      if (typeof trigger === 'function') await trigger()
      else await trigger.click()
      await expect(page.locator('.layout__aside')).toHaveCSS('width',width)
      const samples = await sampling
      for (const sample of samples) {
        expect(sample.width).toBeGreaterThanOrEqual(17.99)
        expect(sample.width).toBeLessThanOrEqual(20.01)
        expect(Number.parseFloat(sample.font)).toBeGreaterThanOrEqual(11.99)
        expect(Number.parseFloat(sample.font)).toBeLessThanOrEqual(14.01)
        expect(sample.x + sample.width / 2).toBeCloseTo(baseline.x + baseline.width / 2, 1)
      }
      const settled = await metrics()
      expect(settled.width).toBeCloseTo(icon,1)
      expect(settled.font).toBe(font+'px')
      if (resize) {
        expect(samples.some(sample => sample.width > 18.05 && sample.width < 19.95)).toBe(true)
        expect(samples.some(sample => Number.parseFloat(sample.font) > 12.05 && Number.parseFloat(sample.font) < 13.95)).toBe(true)
      } else {
        for (const sample of samples) expect(sample.width).toBeCloseTo(icon,1)
      }
    }
    await sampleTransition(page.getByRole('button',{name:'收起二级菜单',exact:true}), '132px')
    await expect(rail).toHaveCSS('width','132px')
    await sampleTransition(sidebar.getByRole('button',{name:'收起一级菜单',exact:true}), '64px', {icon:18,font:12,resize:true})
    await expect(rail).toHaveCSS('width','64px')
    await expect(inventory.locator('span')).not.toBeVisible()
    await expect(page.locator('.sidebar__wordmark')).not.toBeVisible()
    expect((await page.locator('.sidebar__logo').boundingBox()).width).toBe(30)
    await page.keyboard.press('Tab')
    await inventory.focus()
    await expect(page.getByRole('tooltip',{name:'进销存',exact:true})).toBeVisible()
    await page.keyboard.press('Escape')
    await sampleTransition(() => page.keyboard.press('Enter'),'212px', {icon:18,font:12})
    await expect(inventory.locator('span')).not.toBeVisible()
    await expect(page.locator('.sidebar__secondary')).toBeVisible()
    await page.screenshot({path:out+'/sidebar-primary-icons.png',clip:{x:0,y:0,width:226,height:580}})
    await sampleTransition(page.getByRole('button',{name:'收起二级菜单',exact:true}), '64px', {icon:18,font:12})
    await page.screenshot({path:out+'/sidebar-icons-only.png',clip:{x:0,y:0,width:80,height:580}})
    await page.reload()
    await expect(rail).toHaveCSS('width','64px')
    await expect(inventory.locator('span')).not.toBeVisible()
    await expect(page.locator('.layout__aside')).toHaveCSS('width','64px')
    await page.locator('.navbar').getByRole('button',{name:'展开一级菜单',exact:true}).focus()
    await sampleTransition(() => page.keyboard.press('Enter'), '132px', {icon:20,font:14,resize:true})
    await expect(inventory.locator('span')).toBeVisible()
    await sampleTransition(page.getByRole('button',{name:'展开二级菜单',exact:true}), '280px')
    await expect(rail).toHaveCSS('width','132px')
    await expect(page.locator('.sidebar__secondary .el-menu-item.is-active')).toHaveText('采购管理')
    await page.mouse.move(500,500)
    await page.waitForTimeout(200)
    await page.screenshot({path:out+'/sidebar-horizontal-light.png',clip:{x:0,y:0,width:294,height:580}})
    await expect(assertSidebarContrast).toPass({timeout:2000})
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await page.waitForTimeout(250)
    await expect(assertSidebarContrast).toPass({timeout:2000})
    await page.screenshot({path:out+'/sidebar-horizontal-dark.png',clip:{x:0,y:0,width:294,height:580}})
  })
  await check('引导可跳过、退出和重开，保持输入与业务边界', async () => {
    await page.goto(app + '/#/inv/purchase')
    await page.reload()
    const guide = page.locator('.order-page > .purchase-guide')
    await guide.getByRole('button', {name:'直接办理',exact:true}).click()
    await expect(guide.getByRole('button', {name:'采购操作指南',exact:true})).toBeFocused()
    await page.reload()
    await expect(guide.getByRole('button', {name:'开始引导'})).toHaveCount(0)
    await guide.getByRole('button', {name:'采购操作指南',exact:true}).click()
    await expect(guide.getByRole('list', {name:'采购办理顺序'})).toBeVisible()
    await page.getByRole('button', {name:'新增采购单',exact:true}).click()
    const dialog = page.getByRole('dialog', {name:'新增采购单',exact:true})
    await dialog.getByPlaceholder('选填').last().fill('退出与重开后仍保留')
    const audits = state.auditCalls
    await dialog.getByRole('button', {name:'退出引导',exact:true}).click()
    await expect(dialog.getByRole('button', {name:'采购操作指南',exact:true})).toBeFocused()
    await expect(dialog.locator('.purchase-guide__content')).toHaveCount(0)
    await expect(dialog.getByPlaceholder('选填').last()).toHaveValue('退出与重开后仍保留')
    await expect(dialog.getByRole('button', {name:'刷新可选资料'})).toBeVisible()
    await expect(dialog).toContainText('不会增加库存或登记付款')
    await dialog.getByRole('button', {name:'采购操作指南',exact:true}).click()
    await expect(dialog.getByPlaceholder('选填').last()).toHaveValue('退出与重开后仍保留')
    await expect(dialog.getByRole('list', {name:'采购办理顺序'})).toBeVisible()
    expect(state.auditCalls).toBe(audits)
    await page.setViewportSize({width:600,height:900})
    expect(await dialog.locator('.purchase-guide').evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    await page.screenshot({path:out+'/purchase-guide-narrow.png',fullPage:true})
    await page.setViewportSize({width:1600,height:1100})
    await page.reload()
    await expect(page.locator('.purchase-impact')).toContainText('请确认货物已实际到达')
  })
  await check('办理进度按当前单据变化，不把部分付款或取消计为完成', async () => {
    const saved = state.orders
    const current = {...seed(909), orderNo:'CG-GUIDE-909'}
    state.orders = [current]
    await page.goto(app + '/#/inv/purchase')
    await page.reload()
    const guide = page.locator('.order-page > .purchase-guide')
    await expect(guide).toContainText('已完成 1 / 3 个环节')
    await expect(guide.getByRole('progressbar')).toHaveAttribute('aria-valuenow','33')
    await expect(guide).toContainText('尚未到货时可保留待处理')
    current.status = 2
    current.settleStatus = 'partial'
    current.settledAmount = 300
    await page.getByRole('tab',{name:'全部单据',exact:true}).click()
    await page.getByRole('button',{name:'CG-GUIDE-909',exact:true}).click()
    const dialog = page.getByRole('dialog', {name:'采购单据详情',exact:true})
    const detailGuide = dialog.locator('.purchase-guide')
    await expect(detailGuide).toContainText('已完成 2 / 3 个环节')
    await expect(detailGuide).toContainText('900.00')
    await page.screenshot({path:out+'/purchase-guide-partial.png',fullPage:true})
    current.settleStatus = 'paid'
    current.settledAmount = 1200
    await page.reload()
    await page.getByRole('button',{name:'CG-GUIDE-909',exact:true}).click()
    await expect(detailGuide).toContainText('已完成 3 / 3 个环节')
    await expect(detailGuide.getByRole('progressbar')).toHaveAttribute('aria-valuenow','100')
    current.status = 3
    await page.reload()
    await page.getByRole('button',{name:'CG-GUIDE-909',exact:true}).click()
    await expect(detailGuide).toContainText('这张采购单已取消')
    await expect(detailGuide.getByRole('progressbar')).toHaveCount(0)
    state.orders = saved
    await page.goto(app + '/#/inv/overview')
    await page.reload()
    await expect(page.locator('.purchase-start .purchase-guide').getByRole('progressbar')).toHaveCount(0)
  })
  await check('首页真实采购待办、快捷新建与刷新保留输入', async () => {
    state.fullNavigation = true
    state.readOnly = false
    state.failList = false
    await page.goto(app + '/#/inv/overview')
    await page.reload()
    const start = page.locator('.purchase-start')
    await expect(start).toContainText('张采购单待核对')
    await start.getByRole('button', {name:'新增采购单',exact:true}).focus()
    await page.keyboard.press('Enter')
    const dialog = page.getByRole('dialog', {name:'新增采购单',exact:true})
    await expect(dialog).toBeVisible()
    await expect(page).not.toHaveURL(/action=create/)
    await expect(dialog).toContainText('不会增加库存或登记付款')
    await expect(dialog.getByRole('link', {name:'维护商品（新标签页）'})).toHaveAttribute('target','_blank')
    await dialog.getByPlaceholder('选填').last().fill('刷新仍保留的采购备注')
    state.failOptions = true
    await dialog.getByRole('button', {name:'刷新可选资料'}).click()
    await expect(dialog).toContainText('资料加载失败，请刷新后再选择')
    await expect(dialog.getByPlaceholder('选填').last()).toHaveValue('刷新仍保留的采购备注')
    state.failOptions = false
    await dialog.getByRole('button', {name:'刷新可选资料'}).click()
    await expect(dialog).not.toContainText('资料加载失败，请刷新后再选择')
    await expect(dialog.getByPlaceholder('选填').last()).toHaveValue('刷新仍保留的采购备注')
    await page.screenshot({path:out+'/purchase-entry.png',fullPage:true})
    await page.reload()
    await expect(dialog).not.toBeVisible()
  })
  await check('首页待办加载失败、重试与零待办方向', async () => {
    state.failList = true
    await page.goto(app + '/#/inv/overview')
    await page.reload()
    const start = page.locator('.purchase-start')
    await expect(start).toContainText('数量加载失败')
    state.failList = false
    const saved = state.orders
    state.orders = []
    await start.getByRole('button', {name:'重新加载'}).click()
    await expect(start).toContainText('从一张采购单开始')
    await expect(start.getByRole('button',{name:'新增采购单'})).toBeVisible()
    await page.setViewportSize({width:600,height:900})
    await page.screenshot({path:out+'/purchase-start-narrow.png',fullPage:true})
    expect(await start.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    await page.evaluate(() => document.documentElement.classList.remove('dark'))
    await page.setViewportSize({width:1600,height:1100})
    await page.screenshot({path:out+'/purchase-start-light.png',fullPage:true})
    state.orders = saved
  })
  await check('无创建权限时隐藏首页新增入口', async () => {
    state.readOnly = true
    await page.reload()
    const start = page.locator('.purchase-start')
    await expect(start).toBeVisible()
    await expect(start.getByRole('button',{name:'新增采购单'})).toHaveCount(0)
    state.readOnly = false
  })
  expect(pageErrors).toEqual([])
  console.log(`PASS ${checks.length} scenarios; no uncaught browser errors`)
  await fs.writeFile(
    out + '/results.json',
    JSON.stringify(
      { checks, pageErrors, boundary: 'mock API; real Vue UI' },
      null,
      2,
    ),
  )
} catch (e) {
  console.error(e)
  console.error('pageErrors', pageErrors)
  if (debugPage) {
    console.error('URL', debugPage.url())
    console.error((await debugPage.locator('body').innerText()).slice(-3500))
    await debugPage.screenshot({ path: out + '/failure.png', fullPage: true })
  }
  process.exitCode = 1
} finally {
  await browser?.close()
  server?.kill()
}
