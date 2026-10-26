const { chromium, expect } = require('../frontend/node_modules/@playwright/test')
const fs = require('fs')
const path = require('path')
const os = require('os')
const root = path.resolve(__dirname, '..')
const out = process.env.QA_OUTPUT || path.join(os.tmpdir(), 'kasinlo-purchase-mock-qa')
fs.mkdirSync(out, { recursive: true })
const assert = require('node:assert/strict')
;(async () => {
  const server = require('child_process').spawn(
    process.execPath,
    [
      'node_modules/vite/bin/vite.js',
      '--mode',
      'mock',
      '--host',
      '127.0.0.1',
      '--port',
      '6084',
      '--strictPort',
    ],
    { cwd: path.join(root, 'frontend'), stdio: 'ignore' },
  )
  await new Promise((r) => setTimeout(r, 2000))
  const browser = await chromium.launch({
    executablePath: process.env.CHROME_PATH || undefined,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    locale: 'zh-CN',
  })
  const p = await context.newPage()
  p.on('response', async (r) => {
    if (r.status() === 500) console.log('HTTP500', r.url(), (await r.text()).slice(0, 5000))
  })
  p.on('console', (m) => {
    if (m.type() === 'error' || m.type() === 'warning') console.log('CONSOLE', m.text())
  })
  p.on('framenavigated', (f) => {
    if (f === p.mainFrame()) console.log('NAV', f.url())
  })
  const errors = []
  p.on('pageerror', (e) => errors.push(e.message))
  const data = {}
  const log = (k, v) => {
    data[k] = v
    console.log(k, JSON.stringify(v))
    fs.writeFileSync(path.join(out, 'results.json'), JSON.stringify(data, null, 2))
  }
  const db = () => p.evaluate(() => JSON.parse(localStorage.getItem('kasinlo_mock_data_v1')))
  const pause = () => p.waitForTimeout(500)
  try {
    await p.goto('http://127.0.0.1:6084')
    await p.getByPlaceholder('用户名', { exact: true }).fill('admin')
    await p.getByPlaceholder('密码', { exact: true }).fill('admin123456')
    await p.getByRole('button', { name: '登 录' }).click()
    await p.locator('.purchase-start').waitFor()
    await pause()
    log('home', await p.locator('.purchase-start').innerText())
    await p
      .locator('.purchase-start')
      .getByRole('button', { name: '新增采购单', exact: true })
      .click()
    const d = p.getByRole('dialog', { name: '新增采购单', exact: true })
    await d.waitFor()
    await d.getByPlaceholder('选填', { exact: true }).last().fill('目标评审：收到10件，暂不付款')
    const popupPromise = context.waitForEvent('page')
    await d.getByRole('link', { name: '维护供应商（新标签页）' }).click()
    const s = await popupPromise
    await s.waitForLoadState('domcontentloaded')
    await s.getByRole('button', { name: '新增供应商', exact: true }).click()
    const sd = s.getByRole('dialog', { name: '新增供应商', exact: true })
    await sd.getByPlaceholder('往来单位名称').fill('目标体验供应商')
    log('supplierDialogButtons', await sd.getByRole('button').allTextContents())
    await sd.getByRole('button', { name: '确定', exact: true }).click()
    await sd.waitFor({ state: 'hidden' })
    await s.waitForTimeout(400)
    log(
      'supplierCreatedInOtherTab',
      (await s.locator('body').innerText()).includes('目标体验供应商'),
    )
    log(
      'persistedSupplierBeforeRefresh',
      (await db()).collections['/inv/suppliers'].some((x) => x.name === '目标体验供应商'),
    )
    await p.bringToFront()
    await d.getByRole('button', { name: '刷新可选资料' }).click()
    await pause()
    const sel = d.locator('.el-select').first()
    await sel.click()
    await sel.locator('input').fill('目标体验供应商')
    await pause()
    log('supplierOptionsAfterRefresh', await p.locator('.el-select-dropdown:visible').innerText())
    await p.getByRole('option', { name: '目标体验供应商', exact: true }).click()
    const prod = d.locator('.item-editor__row .el-select').nth(0)
    await prod.click()
    await p.getByRole('option').filter({ hasText: '工业连接器' }).first().click()
    const nums = d.locator('.item-editor__row .el-input-number input')
    await nums.nth(0).fill('10')
    await nums.nth(1).fill('20')
    log('beforeSave', {
      stock: (await db()).products[0].specs[0].stock,
      remark: await d.getByPlaceholder('选填', { exact: true }).last().inputValue(),
    })
    await d.getByRole('button', { name: '保存采购单', exact: true }).click()
    await d.waitFor({ state: 'hidden' })
    await pause()
    log('saveReview', await p.locator('.purchase-review').innerText())
    let state = await db()
    let order = state.orders.find((x) => x.remark === '目标评审：收到10件，暂不付款')
    assert(state.collections['/inv/suppliers'].some((x) => x.name === '目标体验供应商'))
    assert.equal(state.products[0].specs[0].stock, 100)
    log('savedState', {
      id: order.id,
      orderNo: order.orderNo,
      total: order.totalAmount,
      status: order.status,
      stock: state.products[0].specs[0].stock,
      supplierStillExists: state.collections['/inv/suppliers'].some(
        (x) => x.name === '目标体验供应商',
      ),
    })
    await p
      .locator('.purchase-review')
      .getByRole('button', { name: '审核并入库', exact: true })
      .click()
    const ap = p.getByRole('dialog', { name: '审核并入库', exact: true })
    log('approveDialog', await ap.innerText())
    await ap.getByRole('button', { name: '确认审核并入库', exact: true }).click()
    await ap.waitFor({ state: 'hidden' })
    await pause()
    log('afterApprovalReview', await p.locator('.purchase-review').innerText())
    state = await db()
    order = state.orders.find((x) => x.id === order.id)
    log('approvedState', {
      status: order.status,
      settled: order.settledAmount,
      settleStatus: order.settleStatus,
      stock: state.products[0].specs[0].stock,
      stockOrders: state.collections['/inv/stock-orders'].filter((x) => x.sourceId === order.id),
    })
    await p.screenshot({ path: path.join(out, 'purchase-approved.png'), fullPage: true })
    await p
      .locator('.purchase-review')
      .getByRole('button', { name: '查看入库记录', exact: true })
      .click()
    await pause()
    console.log('AFTER_STOCK_CLICK', p.url())
    assert.equal(
      await p.getByRole('tab', { name: '出入库单据', exact: true }).getAttribute('aria-selected'),
      'true',
    )
    await expect(p.locator('.stock-page .el-table__body-wrapper tbody tr:visible')).toHaveCount(1)
    await p.screenshot({ path: path.join(out, 'repair-stock-related.png'), fullPage: true })
    await p.getByRole('link', { name: '返回采购单', exact: true }).click()
    await pause()
    assert((await p.locator('.purchase-review').innerText()).includes(order.orderNo))
    await p
      .locator('.purchase-review')
      .getByRole('button', { name: '查看本单应付', exact: true })
      .click()
    await pause()
    assert.equal(
      await p.getByRole('tab', { name: '应付明细', exact: true }).getAttribute('aria-selected'),
      'true',
    )
    assert((await p.locator('.summary-row').innerText()).includes('应付总额'))
    await expect(p.locator('.finance-page .el-table__body-wrapper tbody tr')).toHaveCount(1)
    log('relatedPayable', await p.locator('.finance-page').innerText())
    await p.screenshot({ path: path.join(out, 'repair-finance-related.png'), fullPage: true })
    await p.locator('.finance-page').getByRole('button', { name: '登记付款', exact: true }).click()
    const pay = p.getByRole('dialog')
    await pay.locator('.el-input-number input').fill('50')
    log('paymentButtons', await pay.getByRole('button').allTextContents())
    await pay.getByRole('button', { name: '确定', exact: true }).click()
    await pay.waitFor({ state: 'hidden' })
    await pause()
    await p.getByRole('link', { name: '返回采购单', exact: true }).click()
    await pause()
    assert((await p.locator('.purchase-review').innerText()).includes('已结算 ¥50.00'))
    assert(!(await p.locator('.purchase-review').innerText()).includes('尚未登记付款'))
    await p.screenshot({ path: path.join(out, 'repair-purchase-return.png'), fullPage: true })
    log(
      'PASS',
      'supplier cross-tab refresh and retention; stock link; payable link; partial payment; return refreshed',
    )
    await p.setViewportSize({ width: 600, height: 900 })
    await expect(
      p.locator('.purchase-review').getByRole('button', { name: '查看本单应付', exact: true }),
    ).toBeVisible()
    await p.screenshot({ path: path.join(out, 'purchase-narrow.png'), fullPage: true })
    await p.evaluate(() => document.documentElement.classList.add('dark'))
    await p
      .locator('.purchase-review')
      .getByRole('button', { name: '查看本单应付', exact: true })
      .click()
    await expect(p.locator('.finance-page .el-table__body-wrapper tbody tr')).toHaveCount(1)
    await p.screenshot({ path: path.join(out, 'finance-narrow-dark.png'), fullPage: true })
    await p.getByRole('tab', { name: '应收明细', exact: true }).click()
    await expect(p.locator('.purchase-context')).toHaveCount(0)
    await expect(p.locator('.summary-row')).toContainText('应收总额')
    await p.goto(`http://127.0.0.1:6084/#/inv/finance?view=payable&purchaseId=${order.id}&purchaseNo=${order.orderNo}`)
    await expect(p.locator('.purchase-context')).toBeVisible()
    await expect(p.locator('.summary-row')).toContainText('应付总额')
    await p.getByRole('link', { name: '查看全部记录', exact: true }).click()
    await expect(p.locator('.purchase-context')).toHaveCount(0)
    await expect(p.locator('.finance-page .el-table__body-wrapper tbody tr')).toHaveCount(10)
    const t = await context.newPage()
    await t.goto('http://127.0.0.1:6084/#/inv/supplier')
    async function prepareSupplier(tab, name) {
      await tab.getByRole('button', { name: '新增供应商', exact: true }).click()
      const form = tab.getByRole('dialog', { name: '新增供应商', exact: true })
      await form.getByPlaceholder('往来单位名称').fill(name)
      return form
    }
    const [f1, f2] = await Promise.all([
      prepareSupplier(s, '并发供应商甲'),
      prepareSupplier(t, '并发供应商乙'),
    ])
    await Promise.all([
      f1.getByRole('button', { name: '确定', exact: true }).click(),
      f2.getByRole('button', { name: '确定', exact: true }).click(),
    ])
    await Promise.all([f1.waitFor({ state: 'hidden' }), f2.waitFor({ state: 'hidden' })])
    state = await db()
    const added = state.collections['/inv/suppliers'].filter((x) =>
      ['并发供应商甲', '并发供应商乙'].includes(x.name),
    )
    assert.equal(added.length, 2)
    assert.equal(new Set(added.map((x) => x.id)).size, 2)
    assert.equal(state.orders.find((x) => x.id === order.id).settledAmount, 50)
    assert.equal(errors.length, 0)
    log(
      'concurrency',
      'PASS: simultaneous tab writes retain both suppliers, unique IDs, original order and payment',
    )
    log('errors', errors)
  } catch (e) {
    log('failure', e.stack)
    log('screen', await p.locator('body').innerText())
    await p.screenshot({ path: path.join(out, 'failure.png'), fullPage: true })
    process.exitCode = 1
  }
  await browser.close()
  server.kill()
})()
