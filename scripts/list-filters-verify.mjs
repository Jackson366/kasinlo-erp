/** Real Vue + Mock filter disclosure regression; no backend transaction claims.
 * Run: cd frontend && npm run test:list-filters
 * Install Playwright Chromium, or provide CHROME_PATH.
 */
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs/promises'
import path from 'node:path'
import os from 'node:os'

const root = fileURLToPath(new URL('..', import.meta.url))
const require = createRequire(new URL('../frontend/package.json', import.meta.url))
const { chromium, expect } = require('@playwright/test')
const { createServer } = await import(
  new URL('../frontend/node_modules/vite/dist/node/index.js', import.meta.url)
)
const out = process.env.QA_OUTPUT || path.join(os.tmpdir(), 'kasinlo-list-filters-qa')
await fs.mkdir(out, { recursive: true })
const server = await createServer({
  root: path.join(root, 'frontend'),
  mode: 'mock',
  server: { host: '127.0.0.1', port: 6093, strictPort: true },
})
await server.listen()
const app = 'http://127.0.0.1:6093'
let browser
let page
const errors = []
const checks = []
try {
  browser = await chromium.launch({
    executablePath: process.env.CHROME_PATH || undefined,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    locale: 'zh-CN',
    colorScheme: 'light',
    reducedMotion: 'reduce',
  })
  page = await context.newPage()
  page.setDefaultTimeout(10000)
  page.on('pageerror', (e) => errors.push(e.message))
  await page.goto(app)
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.locator('.login__submit').click()
  await page.waitForURL(/inv\/overview/)
  const filters = () => page.locator('.list-filters:visible')
  const toggle = () => filters().getByRole('button', { name: /^(更多筛选|收起筛选)/ })
  const field = (label) =>
    filters()
      .locator('.el-form-item')
      .filter({ has: page.getByText(label, { exact: true }) })
  async function settled() {
    await expect(page.locator('.el-loading-mask:visible')).toHaveCount(0)
  }
  let navigation = 0
  async function go(route) {
    // 改变文档查询，确保每个默认态是冷启动，而非同文档 hash 切换后的缓存实例。
    await page.goto(`${app}/?filterCheck=${++navigation}#${route}`)
    await filters().waitFor()
    await settled()
    // Observe parameters while preserving the real app API and Mock implementation.
    await page.evaluate(async () => {
      const { default: request } = await import('/src/utils/request.ts')
      window.filterRequests = []
      request.interceptors.request.use((config) => {
        window.filterRequests.push({ url: config.url, params: config.params })
        return config
      })
    })
  }
  async function select(label, value) {
    await field(label).locator('.el-select').click()
    await page.locator('.el-select-dropdown:visible').getByText(value, { exact: true }).click()
  }
  async function check(name, fn) {
    await fn()
    checks.push(name)
    console.log(`PASS ${name}`)
  }
  for (const [route, labels] of [
    ['/inv/purchase', ['关键字', '交接视图']],
    ['/inv/purchase?view=all', ['关键字', '单据状态']],
    ['/inv/sales', ['关键字', '单据状态']],
    ['/inv/contract', ['关键字', '阶段']],
    ['/inv/product', ['关键字', '分类']],
    ['/inv/finance', ['关键字', '结算状态']],
    ['/system/user', ['关键字', '所属部门']],
    ['/system/log', ['关键字', '模块']],
  ]) {
    await check(`${route} 默认仅显示常用条件`, async () => {
      await go(route)
      await expect(filters().locator('.list-filters__fields > .el-form-item:visible')).toHaveCount(
        2,
      )
      for (const label of labels) await expect(field(label)).toBeVisible()
      await expect(filters().locator('.list-filters__advanced')).toBeHidden()
      await expect(toggle()).toHaveAttribute('aria-expanded', 'false')
      await toggle().focus()
      await page.keyboard.press('Enter')
      await expect(toggle()).toHaveAttribute('aria-expanded', 'true')
      await expect(filters().locator('.list-filters__advanced')).toBeVisible()
      await toggle().click()
      await expect(filters().locator('.list-filters__advanced')).toBeHidden()
    })
  }
  await check('财务收起后保留来源，单独清除不改变关键字且仅请求一次', async () => {
    await go('/inv/finance')
    await toggle().click()
    await select('来源', '合同期次')
    await filters().getByRole('button', { name: '查询', exact: true }).click()
    await settled()
    await toggle().click()
    await expect(toggle()).toHaveText('更多筛选（1）')
    await expect(
      filters().getByRole('button', { name: '清除来源：合同期次', exact: true }),
    ).toBeVisible()
    await page.evaluate(() => {
      window.filterRequests = []
    })
    await field('关键字').locator('input').fill('XS')
    await filters().getByRole('button', { name: '清除来源：合同期次', exact: true }).click()
    await settled()
    await expect(field('关键字').locator('input')).toHaveValue('XS')
    await expect(toggle()).toHaveText('更多筛选')
    await expect(toggle()).toBeFocused()
    await page.waitForTimeout(400) // pending keyword debounce must have been cancelled
    const requests = await page.evaluate(() =>
      window.filterRequests.filter((r) => r.url.includes('/receivables')),
    )
    expect(requests).toHaveLength(1)
    expect(requests[0].params).toMatchObject({ keyword: 'XS', page: 1 })
    expect(requests[0].params.source).toBeUndefined()
    await toggle().click()
    await expect(field('来源').locator('.el-select')).toContainText('全部')
  })
  await check('商品停用值 0 仍显示摘要，重置同时清除折叠与常用条件', async () => {
    await go('/inv/product')
    await toggle().click()
    await select('状态', '停用')
    await filters().getByRole('button', { name: '查询', exact: true }).click()
    await settled()
    await toggle().click()
    await expect(
      filters().getByRole('button', { name: '清除状态：停用', exact: true }),
    ).toBeVisible()
    await expect(toggle()).toHaveText('更多筛选（1）')
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            window.filterRequests.filter((r) => r.url.includes('/products')).at(-1)?.params.status,
        ),
      )
      .toBe(0)
    await field('关键字').locator('input').fill('控制模块')
    await filters().getByRole('button', { name: '重置', exact: true }).click()
    await settled()
    await expect(field('关键字').locator('input')).toHaveValue('')
    await expect(filters().locator('.list-filters__chip')).toHaveCount(0)
    await toggle().click()
    await expect(field('状态').locator('.el-select')).toContainText('全部')
  })
  await check('采购深链的三个折叠条件刷新保留，清除日期不丢失其他范围', async () => {
    await go('/inv/purchase?view=all&settle=unpaid&handoff=created&start=2026-10-01&end=2026-11-30')
    await expect(toggle()).toHaveText('更多筛选（3）')
    await page.reload()
    await filters().waitFor()
    await expect(toggle()).toHaveText('更多筛选（3）')
    await filters()
      .getByRole('button', { name: '清除单据日期：2026-10-01 至 2026-11-30', exact: true })
      .click()
    await expect(toggle()).toHaveText('更多筛选（2）')
    await expect.poll(() => new URL(page.url()).hash).not.toContain('start=')
    expect(page.url()).toContain('settle=unpaid')
    expect(page.url()).toContain('handoff=created')
  })
  await check('采购到应付的来源限制收起后仍明确可见', async () => {
    await go(
      '/inv/finance?view=payable&source=order&q=CG&returnTo=' +
        encodeURIComponent('/inv/purchase?view=all'),
    )
    await expect(page.getByRole('tab', { name: '应付明细', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(
      filters().getByRole('button', { name: '清除来源：单据', exact: true }),
    ).toBeVisible()
    const primaryBox = await filters().locator('.list-filters__primary').boundingBox()
    const chipBox = await filters().locator('.list-filters__selection').boundingBox()
    expect(chipBox.y).toBeGreaterThanOrEqual(primaryBox.y + primaryBox.height)
    await page.screenshot({ path: path.join(out, 'finance-collapsed.png') })
    await toggle().click()
    const advancedBox = await filters().locator('.list-filters__advanced').boundingBox()
    expect(advancedBox.y).toBeGreaterThanOrEqual(primaryBox.y + primaryBox.height)
    await page.screenshot({ path: path.join(out, 'finance-expanded.png') })
  })
  await check('合同展开日期控件可用，暗色窄屏筛选不溢出', async () => {
    await go('/inv/contract')
    await toggle().click()
    await field('签订日期').locator('.el-date-editor').click()
    await expect(page.locator('.el-picker-panel:visible')).toBeVisible()
    await page.keyboard.press('Escape')
    await toggle().click()
    await page.evaluate(() => {
      document.documentElement.classList.add('dark')
      localStorage.setItem('nova_sidebar_primary_collapsed', '1')
      localStorage.setItem('nova_sidebar_collapsed', '1')
    })
    await page.reload()
    await filters().waitFor()
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await page.setViewportSize({ width: 760, height: 900 })
    await toggle().click()
    const geometry = await filters().evaluate((el) => ({
      width: el.clientWidth,
      scroll: el.scrollWidth,
    }))
    expect(geometry.scroll).toBeLessThanOrEqual(geometry.width + 1)
    await page.screenshot({ path: path.join(out, 'contract-narrow-dark.png') })
  })
  expect(errors).toEqual([])
  await fs.writeFile(path.join(out, 'results.json'), JSON.stringify({ checks, errors }, null, 2))
  console.log(`PASS ${checks.length} scenarios; no uncaught browser errors`)
} catch (error) {
  await page?.screenshot({ path: path.join(out, 'failure.png') }).catch(() => {})
  throw error
} finally {
  await browser?.close()
  await server.close()
}
