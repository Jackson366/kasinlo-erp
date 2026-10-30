/** Actual Vue + dev:mock workflow. Backend transaction coverage lives in TradeAuditTest. */
import { createRequire } from 'node:module'
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs/promises'
import path from 'node:path'
const require = createRequire(
  new URL('../frontend/package.json', import.meta.url),
)
const { chromium, expect } = require('@playwright/test')
const root = fileURLToPath(new URL('..', import.meta.url))
const out = '/tmp/kasinlo-purchase-audit-qa'
await fs.mkdir(out, { recursive: true })
const server = spawn(
  process.execPath,
  [
    path.join(root, 'frontend/node_modules/vite/bin/vite.js'),
    '--mode',
    'mock',
    '--host',
    '127.0.0.1',
    '--port',
    '16083',
    '--strictPort',
  ],
  { cwd: path.join(root, 'frontend'), stdio: 'pipe' },
)
let logs = ''
server.stdout.on('data', (b) => {
  logs += b
})
server.stderr.on('data', (b) => {
  logs += b
})
let browser
const checks = []
const errors = []
try {
  for (let n = 0; n < 100; n++) {
    try {
      if ((await fetch('http://127.0.0.1:16083')).ok) break
    } catch {}
    if (server.exitCode !== null) throw new Error(logs)
    await new Promise((r) => setTimeout(r, 100))
  }
  browser = await chromium.launch({
    executablePath: process.env.CHROME_PATH || undefined,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  const context = await browser.newContext({
    viewport: { width: 1600, height: 1100 },
    locale: 'zh-CN',
  })
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const page = await context.newPage()
  page.on('pageerror', (e) => errors.push(e.message))
  const check = async (name, run) => {
    await run()
    checks.push(name)
    console.log('PASS', name)
  }
  await page.goto('http://127.0.0.1:16083')
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.getByRole('button', { name: '登 录', exact: true }).click()
  await page.waitForURL(/inv\/overview/)
  const order = await page.evaluate(async () => {
    const { invApi } = await import('/src/api/inv.ts')
    const { id } = await invApi.createOrder({
      bizType: 'purchase',
      partyId: 1,
      orderDate: '2026-10-27',
      items: [{ productId: 1, specId: 1, unitId: 1, qty: 3, price: 10 }],
    })
    return invApi.orderDetail(id)
  })
  await page.goto(
    `http://127.0.0.1:16083/#/inv/purchase?view=all&order=${order.id}`,
  )
  const detail = page.getByRole('region', { name: '采购单据详情', exact: true })
  const approve = page.getByRole('dialog', { name: '审核并入库', exact: true })
  await check('详情审核后保留本单、显示下游及自审', async () => {
    await expect(detail).toBeVisible()
    await detail
      .getByRole('button', { name: '审核并入库', exact: true })
      .click()
    await expect(approve).toContainText('收货确认依据')
    await expect(approve).toContainText('本次审核人')
    await approve
      .getByRole('button', { name: '确认审核并入库', exact: true })
      .click()
    await expect(approve).toBeHidden()
    await expect(detail.locator('.order-downstream')).toContainText('已过账')
    await expect(detail.locator('.audit-summary')).toContainText('自审')
    await expect(detail.locator('.order-downstream')).toContainText(
      '未付款 ¥30.00',
    )
  })
  await check('详情反审核、历史保留及再次审核', async () => {
    await detail.getByRole('button', { name: '反审核', exact: true }).click()
    const reverse = page.getByRole('dialog', {
      name: '反审核确认',
      exact: true,
    })
    await expect(reverse).toContainText('回滚库存')
    await reverse
      .getByRole('button', { name: '确认反审核', exact: true })
      .click()
    await expect(reverse).toBeHidden()
    await expect(detail.locator('.audit-summary')).toContainText('上次审核')
    await expect(detail.locator('.audit-summary')).toContainText('上次反审核')
    await expect(detail.locator('.order-downstream')).toContainText('已作废')
    await expect(detail.locator('.order-downstream')).toContainText(
      '尚未形成应付',
    )
    await detail
      .getByRole('button', { name: '审核并入库', exact: true })
      .click()
    await approve
      .getByRole('button', { name: '确认审核并入库', exact: true })
      .click()
    await expect(approve).toBeHidden()
    await detail.getByRole('button', { name: '审核历史（3 条）' }).click()
    await expect(
      detail.locator('.order-audit-history .el-table__body tr'),
    ).toHaveCount(3)
    await detail.locator('.order-audit-history .el-table__body tr').last().scrollIntoViewIfNeeded()
    await page.screenshot({
      path: path.join(out, 'audit-history.png'),
      fullPage: true,
    })
  })
  await check('已付款时详情禁用反审核并解释原因', async () => {
    await detail.getByRole('button', { name: '返回单据列表', exact: true }).click()
    await page.evaluate(async (id) => {
      const { invApi } = await import('/src/api/inv.ts')
      await invApi.settle({
        orderId: id,
        amount: 1,
        method: '银行转账',
        settledAt: '2026-10-27',
      })
    }, order.id)
    await page.reload()
    await expect(
      detail.getByRole('button', { name: '反审核', exact: true }),
    ).toBeDisabled()
    await expect(detail).toContainText('请先联系财务撤销付款记录')
  })
  await detail.getByRole('button', { name: '返回单据列表', exact: true }).click()
  await check('关联入库没有直接作废，来源链接能打开缓存的待处理页面', async () => {
    const stockNo = await page.evaluate(async (id) => {
      const { invApi } = await import('/src/api/inv.ts')
      const data = await invApi.orderDetail(id)
      return data.downstream.stockOrders.find((row) => row.status === 1).orderNo
    }, order.id)
    await page.goto('http://127.0.0.1:16083/#/inv/purchase?view=pending')
    await page.reload()
    await expect(page.locator('.purchase-workbench')).toBeVisible()
    await page.goto('http://127.0.0.1:16083/#/inv/stock')
    await page.getByRole('tab', { name: '出入库单据', exact: true }).click()
    const row = page.locator('.stock-page .el-table__body tr').filter({ hasText: stockNo })
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('button', { name: '作废', exact: true })).toHaveCount(0)
    await row.getByRole('button', { name: '原单反审核', exact: true }).click()
    await expect(detail).toBeVisible()
    await expect(detail).toContainText(order.orderNo)
    await detail.getByRole('button', { name: '返回单据列表', exact: true }).click()
  })
  await check('批量十条失败完整可定位、可复制且不结清', async () => {
    await page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      const { ApiError } = await import('/src/utils/request.ts')
      for (let i = 0; i < 10; i++)
        await invApi.createOrder({
          bizType: 'purchase',
          orderNo: `AUDIT-FAIL-${i.toString().padStart(2, '0')}`,
          partyId: 1,
          orderDate: '2026-10-27',
          items: [{ productId: 1, specId: 1, unitId: 1, qty: 1, price: 10 }],
        })
      invApi.changeOrderStatus = async () => {
        throw new ApiError(409, '模拟校验冲突，请刷新核实后重试')
      }
    })
    await page.goto(
      'http://127.0.0.1:16083/#/inv/purchase?view=all&q=AUDIT-FAIL',
    )
    await page.reload()
    // Reload restores module functions; inject failures after the page has mounted.
    await expect(page.locator('.order-page .el-table__body tr')).toHaveCount(10)
    await page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      const { ApiError } = await import('/src/utils/request.ts')
      invApi.changeOrderStatus = async () => {
        throw new ApiError(409, '模拟校验冲突，请刷新核实后重试')
      }
    })
    await page.locator('.order-page .el-table__header .el-checkbox').click()
    await page.getByRole('button', { name: '批量处理' }).click()
    await page
      .getByRole('menuitem', { name: '审核（仅待处理）', exact: true })
      .click()
    const confirmation = page.getByRole('dialog', {
      name: '批量审核确认',
      exact: true,
    })
    await expect(confirmation).toContainText('批量不结清')
    await expect(confirmation).toContainText('增加库存并形成应付')
    await confirmation
      .getByRole('button', { name: '批量审核', exact: true })
      .click()
    const result = page.getByRole('dialog', {
      name: '批量审核结果',
      exact: true,
    })
    await expect(result).toContainText('失败 10 条')
    await expect(result.locator('.el-table__body tr')).toHaveCount(10)
    await result.getByRole('button', { name: '复制全部失败明细' }).click()
    const copied = await page.evaluate(() => navigator.clipboard.readText())
    expect(copied.split('\n')).toHaveLength(10)
    expect(copied).toContain('AUDIT-FAIL-00')
    expect(copied).toContain('AUDIT-FAIL-09')
    await page.screenshot({
      path: path.join(out, 'batch-failures.png'),
      fullPage: true,
    })
    await result.getByRole('button', { name: '关闭', exact: true }).click()
  })
  await check('交接筛选可恢复、窄屏与暗色不丢动作', async () => {
    await page.goto(
      'http://127.0.0.1:16083/#/inv/purchase?view=all&handoff=created',
    )
    await page.reload()
    await expect(page.locator('.order-page')).toContainText('我发起的')
    await page.setViewportSize({ width: 780, height: 920 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await page.screenshot({
      path: path.join(out, 'audit-narrow-dark.png'),
      fullPage: true,
    })
    expect(errors).toEqual([])
  })
  console.log('RESULT', checks.length, 'passed')
  await fs.writeFile(
    path.join(out, 'result.json'),
    JSON.stringify({ checks, errors }, null, 2),
  )
} catch (e) {
  console.error(e)
  process.exitCode = 1
} finally {
  await browser?.close()
  server.kill('SIGTERM')
}
