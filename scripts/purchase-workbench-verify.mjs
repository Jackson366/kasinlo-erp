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
    if (endpoint === '/profile/routes') return ok([{id:90,name:'Inventory',path:'/inv',component:'',meta:{title:'进销存',icon:'Box'},children:[{id:91,name:'Trade',path:'/inv/trade',component:'',meta:{title:'单据管理',icon:'Document'},children:menus}]}])
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
        Object.assign(row, req.postDataJSON())
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
        items: requested.items.map((it) => ({
          ...item,
          ...it,
          qtyBase: it.qty * (it.unitId === 2 ? 1 : 6),
          unit: it.unitId === 2 ? '米' : '根',
          rate: it.unitId === 2 ? 1 : 6,
          amount: it.qty * it.price,
        })),
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
    expect((await page.locator('.order-page > .page-card').boundingBox()).y).toBeLessThan(190)
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
    await expect(review).toContainText('已生成入库单并增加库存')
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
