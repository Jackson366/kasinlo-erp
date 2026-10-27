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
const out = '/tmp/kasinlo-purchase-handoff-qa'
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
    '16084',
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
      if ((await fetch('http://127.0.0.1:16084')).ok) break
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
  await page.goto('http://127.0.0.1:16084')
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.getByRole('button', { name: '登 录', exact: true }).click()
  await page.waitForURL(/inv\/overview/)
  const navigate = async (location) =>
    page.evaluate(async (location) => {
      const { default: router } = await import('/src/router/index.ts')
      await router.push(location)
    }, location)
  const create = async () =>
    page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      const { id } = await invApi.createOrder({
        bizType: 'purchase',
        partyId: 1,
        orderDate: '2026-10-27',
        items: [{ productId: 1, specId: 1, unitId: 1, qty: 3, price: 10 }],
      })
      return invApi.orderDetail(id)
    })
  const order = await create()
  await navigate({
    path: '/inv/purchase',
    query: { view: 'pending', q: order.orderNo, order: String(order.id) },
  })
  const review = page.locator('.purchase-review')
  const detail = page.getByRole('dialog', {
    name: '采购单据详情',
    exact: true,
  })
  await check(
    '待办只保留一处就近提示，确认窗仍说明审核与付款后果',
    async () => {
      await expect(review).toContainText(order.orderNo)
      await expect(review.locator('.purchase-impact')).toHaveCount(1)
      await expect(review.locator('.detail-stage-note')).toHaveCount(0)
      await expect(review.locator('.purchase-impact.el-alert')).toHaveCount(0)
      await review
        .getByRole('button', { name: '审核并入库', exact: true })
        .click()
      const approve = page.getByRole('dialog', {
        name: '审核并入库',
        exact: true,
      })
      await expect(approve).toContainText('增加库存，并形成应付')
      await expect(approve).toContainText('同时结清全额款项')
      await approve
        .getByRole('button', { name: '确认审核并入库', exact: true })
        .click()
      await expect(review).toContainText('未付款 ¥30.00')
      await expect(
        review.getByRole('button', { name: '处理下一张', exact: true }),
      ).toBeVisible()
    },
  )
  const stockLink = review.getByRole('link', { name: /^查看入库单 / })
  const stockNo = (await stockLink.innerText()).trim()
  await check(
    '入库链接定位当前入库单，刷新仍定位，可返回待办原单',
    async () => {
      await stockLink.click()
      await expect(page.locator('.stock-page')).toBeVisible()
      await expect(
        page.getByRole('tab', { name: '出入库单据', exact: true }),
      ).toHaveAttribute('aria-selected', 'true')
      await expect(
        page.locator('.stock-page .el-table:visible .el-table__body tr'),
      ).toHaveCount(1)
      await expect(page.locator('.stock-page')).toContainText(stockNo)
      await page.reload()
      await expect(
        page.locator('.stock-page .el-table:visible .el-table__body tr'),
      ).toHaveCount(1)
      await page.getByRole('link', { name: '返回采购单', exact: true }).click()
      await expect(review).toContainText(order.orderNo)
      await expect(
        page.getByRole('tab', { name: '待处理', exact: true }),
      ).toHaveAttribute('aria-selected', 'true')
      await expect(
        page.getByRole('textbox', { name: '关键字', exact: true }),
      ).toHaveValue(order.orderNo)
    },
  )
  await check(
    '应付链接选择应付与单据来源，只查当前采购单，付款不会自动执行',
    async () => {
      await review
        .getByRole('link', { name: '查看本单应付', exact: true })
        .click()
      await expect(page.locator('.finance-page')).toBeVisible()
      await expect(
        page.getByRole('tab', { name: '应付明细', exact: true }),
      ).toHaveAttribute('aria-selected', 'true')
      await expect(page.getByPlaceholder('单号 / 往来单位')).toHaveValue(
        order.orderNo,
      )
      await expect(
        page.locator('.finance-page .el-table__body tr'),
      ).toHaveCount(1)
      await expect(page.locator('.finance-page')).toContainText('应付总额')
      await expect(page.locator('.finance-page')).toContainText('¥30.00')
      await expect(page.getByRole('dialog')).toHaveCount(0)
      await page.getByRole('link', { name: '返回采购单', exact: true }).click()
      await expect(review).toContainText(order.orderNo)
    },
  )
  const other = await create()
  await page.evaluate(async (id) => {
    const { invApi } = await import('/src/api/inv.ts')
    await invApi.changeOrderStatus(id, 2, false)
  }, other.id)
  await navigate({
    path: '/inv/purchase',
    query: { view: 'all', q: other.orderNo, order: String(other.id) },
  })
  await check(
    '详情弹窗与工作台共用入口，缓存的应付页不会沿用上一张单据',
    async () => {
      await expect(detail).toContainText(other.orderNo)
      await detail
        .getByRole('link', { name: '查看本单应付', exact: true })
        .click()
      await expect(detail).toBeHidden()
      await expect(page.getByPlaceholder('单号 / 往来单位')).toHaveValue(
        other.orderNo,
      )
      await expect(
        page.locator('.finance-page .el-table__body tr'),
      ).toHaveCount(1)
      await expect(page.locator('.finance-page .el-table__body')).toContainText(
        other.orderNo,
      )
      await expect(
        page.locator('.finance-page .el-table__body'),
      ).not.toContainText(order.orderNo)
      await page.getByRole('link', { name: '返回采购单', exact: true }).click()
      await expect(detail).toContainText(other.orderNo)
    },
  )
  await check('无查看权限时不提供入口，保留财务交接说明', async () => {
    await page.evaluate(async () => {
      const { useUserStore } = await import('/src/store/user.ts')
      const user = useUserStore()
      user.profile = {
        ...user.profile,
        isSuper: false,
        permissions: ['inv:purchase:list'],
      }
    })
    await expect(
      detail.getByRole('link', { name: '查看本单应付', exact: true }),
    ).toHaveCount(0)
    await expect(
      detail.getByRole('link', { name: /^查看入库单 / }),
    ).toHaveCount(0)
    await expect(detail).toContainText('请由财务核查并登记实际付款')
    await page.evaluate(async () => {
      const { useUserStore } = await import('/src/store/user.ts')
      await useUserStore().fetchProfile()
    })
  })
  await check('应付加载失败保留重试入口，重试后仍定位原单', async () => {
    await page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      window.originalPayables = invApi.payables
      invApi.payables = async () => {
        throw new Error('fixture unavailable')
      }
    })
    await detail
      .getByRole('link', { name: '查看本单应付', exact: true })
      .click()
    await expect(page.locator('.finance-page')).toContainText(
      '应收应付加载失败',
    )
    await expect(page.locator('.finance-page .el-table')).toHaveCount(0)
    await page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      invApi.payables = window.originalPayables
    })
    await page
      .locator('.finance-page')
      .getByRole('button', { name: '重新加载', exact: true })
      .click()
    await expect(page.locator('.finance-page .el-table__body')).toContainText(
      other.orderNo,
    )
  })
  await check('无结果与窄屏暗色仍可返回原采购单', async () => {
    const current = new URL(page.url().split('#')[1], 'https://erp.invalid')
    current.searchParams.set('q', 'missing-purchase-404')
    await navigate(current.pathname + current.search)
    await expect(page.locator('.finance-page')).toContainText(
      '当前可见范围内未找到记录',
    )
    await page.setViewportSize({ width: 780, height: 920 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await page.getByRole('link', { name: '返回采购单', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(detail).toContainText(other.orderNo)
    await expect(page.locator('.fade-slide-enter-active')).toHaveCount(0)
    await detail.locator('.order-downstream').scrollIntoViewIfNeeded()
    await page.screenshot({
      path: path.join(out, 'handoff-narrow-dark.png'),
      fullPage: true,
    })
  })
  expect(errors).toEqual([])
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
