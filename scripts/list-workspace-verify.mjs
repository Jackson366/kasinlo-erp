/** Real Vue + Mock regression for viewport lists, disclosure and footer ownership. */
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs/promises'
import path from 'node:path'
import os from 'node:os'
const root = fileURLToPath(new URL('..', import.meta.url))
const require = createRequire(
  new URL('../frontend/package.json', import.meta.url),
)
const { chromium, expect } = require('@playwright/test')
const { createServer } = await import(
  new URL('../frontend/node_modules/vite/dist/node/index.js', import.meta.url)
)
const out =
  process.env.QA_OUTPUT || path.join(os.tmpdir(), 'kasinlo-list-workspace-qa')
await fs.mkdir(out, { recursive: true })
const server = await createServer({
  root: path.join(root, 'frontend'),
  mode: 'mock',
  server: { host: '127.0.0.1', port: 6098, strictPort: true },
})
await server.listen()
let browser
const errors = []
let checks = 0
async function check(name, fn) {
  await fn()
  checks++
  console.log(`PASS ${name}`)
}
try {
  browser = await chromium.launch({
    executablePath: process.env.CHROME_PATH || undefined,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  const page = await browser.newPage({
    viewport: { width: 1440, height: 900 },
    locale: 'zh-CN',
    colorScheme: 'light',
    reducedMotion: 'reduce',
  })
  page.setDefaultTimeout(10000)
  if (process.env.QA_CJK_FONT) {
    const font = (await fs.readFile(process.env.QA_CJK_FONT)).toString('base64')
    await page.addInitScript((font) => {
      document.addEventListener('DOMContentLoaded', () => {
        const style = document.createElement('style')
        style.textContent = `@font-face { font-family: 'QA CJK'; src: url(data:font/ttf;base64,${font}); } :root, html.dark { --el-font-family: 'QA CJK', sans-serif; }`
        document.head.append(style)
      })
    }, font)
  }
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('console', (m) => {
    if (m.type() === 'warning' && m.text().includes('resolve component'))
      errors.push(m.text())
  })
  await page.goto('http://127.0.0.1:6098')
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.locator('.login__submit').click()
  await page.waitForURL(/inv\/overview/)
  async function go(route) {
    await page.evaluate(async (target) => {
      const { default: router } = await import('/src/router/index.ts')
      await router.push(target)
      const { nextTick } = await import('/node_modules/.vite/deps/vue.js')
      await nextTick()
    }, route)
    await expect(
      page.locator('.fade-slide-enter-active, .fade-slide-leave-active'),
    ).toHaveCount(0)
    await expect(page.locator('.el-loading-mask:visible')).toHaveCount(0)
  }
  async function geometry() {
    const panel = page
      .locator('.layout__content .list-panel--fit:visible')
      .first()
    await expect
      .poll(() => panel.evaluate((el) => el.style.height))
      .not.toBe('auto')
    return panel.evaluate((el) => {
      const main = el.closest('.layout__content')
      return {
        panel: el.getBoundingClientRect().toJSON(),
        table: el
          .querySelector(':scope > .el-table')
          ?.getBoundingClientRect()
          .toJSON(),
        pager: el
          .querySelector(':scope > .table-pager')
          ?.getBoundingClientRect()
          .toJSON(),
        main: main.getBoundingClientRect().toJSON(),
        scrollHeight: main.scrollHeight,
        clientHeight: main.clientHeight,
      }
    })
  }
  await go('/system/user')
  await check(
    'expanded user fields use bounded widths and tools wrap below when needed',
    async () => {
      const filters = page.locator('.list-filters')
      const toggle = page.getByRole('button', { name: '更多筛选', exact: true })
      await toggle.click()
      await expect(page.locator('.list-filters__advanced')).toBeVisible()
      const fields = await filters
        .locator('.el-form-item:visible')
        .evaluateAll((items) =>
          items.map((el) => el.getBoundingClientRect().toJSON()),
        )
      expect(fields).toHaveLength(3)
      expect(fields[0].width).toBeLessThanOrEqual(301)
      expect(fields[1].width).toBeLessThanOrEqual(221)
      expect(fields[2].width).toBeLessThanOrEqual(221)
      expect(Math.abs(fields[0].top - fields[2].top)).toBeLessThan(2)
      expect(
        await filters.evaluate((el) => el.getBoundingClientRect().height),
      ).toBeLessThanOrEqual(80)
      await page.screenshot({
        path: path.join(out, 'user-compact-filters.png'),
        fullPage: true,
      })
      await page
        .getByRole('button', { name: '收起筛选', exact: true })
        .press('Escape')
      await expect(
        page.getByRole('button', { name: '更多筛选', exact: true }),
      ).toBeFocused()
    },
  )
  await go('/inv/sales')
  await check(
    'business actions follow More filters and reflow when expanded',
    async () => {
      await page.setViewportSize({ width: 1600, height: 900 })
      const tools = page.locator('.list-filters__tools')
      const toggle = page.locator('.list-filters__toggle')
      await expect(
        tools.getByRole('button', { name: '新增销售单', exact: true }),
      ).toBeVisible()
      await expect(
        page.locator('.list-head > .list-head__actions'),
      ).toHaveCount(0)
      await expect
        .poll(async () => {
          const a = await tools.boundingBox()
          const b = await toggle.boundingBox()
          return Math.abs(a.y - b.y)
        })
        .toBeLessThan(2)
      const closed = await tools.boundingBox()
      const more = await toggle.boundingBox()
      expect(closed.x).toBeGreaterThanOrEqual(more.x + more.width)
      await toggle.click()
      await expect
        .poll(async () => (await tools.boundingBox()).y)
        .toBeGreaterThan(closed.y + 20)
      const open = await tools.boundingBox()
      expect(open.x + open.width).toBeLessThanOrEqual(1600)
      await page.screenshot({
        path: path.join(out, 'sales-actions-expanded.png'),
        fullPage: true,
      })
      await toggle.click()
      await expect
        .poll(async () => (await tools.boundingBox()).y)
        .toBe(closed.y)
      await page.setViewportSize({ width: 1440, height: 900 })
    },
  )
  await check('56px top bar, 44px tags and breadcrumb in header', async () => {
    expect(
      await page
        .locator('.navbar')
        .evaluate((el) => el.getBoundingClientRect().height),
    ).toBe(56)
    expect(
      await page
        .locator('.layout__navigation')
        .evaluate((el) => el.getBoundingClientRect().height),
    ).toBeLessThanOrEqual(48)
    await expect(page.locator('.navbar .breadcrumb')).toBeVisible()
  })
  await check(
    'table fills remainder, footer stays in content viewport',
    async () => {
      const g = await geometry()
      console.log(JSON.stringify(g))
      expect(g.table.height).toBeGreaterThan(450)
      expect(g.pager.bottom).toBeLessThanOrEqual(g.main.bottom + 1)
      expect(g.scrollHeight).toBeLessThanOrEqual(g.clientHeight + 2)
    },
  )
  await page.screenshot({
    path: path.join(out, 'sales-desktop.png'),
    fullPage: true,
  })
  await check(
    'disclosure keeps chosen values, Escape restores toggle focus',
    async () => {
      await expect(page.locator('.list-filters__advanced')).toBeHidden()
      await page.getByRole('button', { name: '更多筛选', exact: true }).click()
      await page.locator('.list-filters__advanced .el-select').first().click()
      await page.getByRole('option', { name: '已审核', exact: true }).click()
      const open = page.getByRole('button', { name: '收起筛选（1）' })
      await expect(open).toHaveAttribute('aria-expanded', 'true')
      await open.press('Escape')
      const closed = page.getByRole('button', { name: '更多筛选（1）' })
      await expect(closed).toBeFocused()
      await closed.click()
      await expect(
        page.locator('.list-filters__advanced .el-select').first(),
      ).toContainText('已审核')
      await page.getByRole('button', { name: '重置', exact: true }).click()
      await expect(
        page.getByRole('button', { name: '收起筛选', exact: true }),
      ).toBeVisible()
    },
  )
  await check('page size and empty results keep footer stable', async () => {
    const before = await geometry()
    await page.locator('.pagination .el-select').click()
    await page.getByRole('option', { name: '50条/页', exact: true }).click()
    const after = await geometry()
    expect(Math.abs(before.pager.bottom - after.pager.bottom)).toBeLessThan(2)
    await page
      .getByPlaceholder('单号 / 往来单位 / 业务员', { exact: true })
      .fill('no-match-workspace-qa')
    await page.getByRole('button', { name: '查询', exact: true }).click()
    await expect(page.locator('.el-table__empty-block')).toBeVisible()
    expect(
      Math.abs(after.pager.bottom - (await geometry()).pager.bottom),
    ).toBeLessThan(2)
    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(
      page.locator('.el-table__body-wrapper tbody tr').first(),
    ).toBeVisible()
  })
  await check('business/system sibling tables fill viewport', async () => {
    for (const route of [
      '/inv/purchase?view=all',
      '/inv/customer',
      '/inv/supplier',
      '/inv/product',
      '/inv/contract',
      '/inv/finance',
      '/system/user',
      '/system/role',
      '/system/menu',
      '/system/dept',
      '/system/log',
      '/inv/stock',
    ]) {
      console.log(`ROUTE ${route}`)
      await go(route)
      await expect(
        page.locator('.layout__content .list-panel--fit:visible').first(),
      ).toBeVisible()
      const g = await geometry()
      expect(g.table.height, route).toBeGreaterThanOrEqual(199)
      expect(g.panel.bottom, route).toBeLessThanOrEqual(g.main.bottom + 2)
    }
  })
  await check('stock tabs own independent table scroll regions', async () => {
    await page.getByRole('tab', { name: '出入库单据', exact: true }).click()
    expect((await geometry()).pager.bottom).toBeLessThanOrEqual(
      (await geometry()).main.bottom + 1,
    )
    await page.getByRole('tab', { name: '库存查询', exact: true }).click()
    await expect(page.locator('.list-panel--fit:visible')).toHaveCount(1)
  })
  await check(
    'purchase editor preserves natural content scrolling',
    async () => {
      await go('/inv/purchase?action=create')
      await expect(page.locator('.purchase-editor')).toBeVisible()
      await page
        .getByRole('button', { name: /保存/ })
        .last()
        .scrollIntoViewIfNeeded()
      expect(
        await page
          .locator('.layout__content')
          .evaluate((el) => getComputedStyle(el).overflowY),
      ).toBe('auto')
      await page
        .getByRole('button', { name: '取消', exact: true })
        .last()
        .click()
    },
  )
  await go('/inv/sales')
  await check(
    '390px dark layout keeps filters, breadcrumb and footer usable',
    async () => {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.getByRole('button', { name: '切换到暗色主题' }).click()
      const disclosure = page.getByRole('button', { name: /(更多|收起)筛选/ })
      if ((await disclosure.getAttribute('aria-expanded')) === 'false')
        await disclosure.click()
      await expect(page.locator('.list-filters__advanced')).toBeVisible()
      await expect(page.locator('.navbar .breadcrumb')).toBeHidden()
      await expect(
        page.locator('.layout__navigation .breadcrumb'),
      ).toBeVisible()
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(390)
      await page.locator('.pagination').scrollIntoViewIfNeeded()
      await expect(page.locator('.pagination .btn-next')).toBeVisible()
      await page.screenshot({
        path: path.join(out, 'sales-narrow-dark.png'),
        fullPage: true,
      })
    },
  )
  expect(errors).toEqual([])
  await fs.writeFile(
    path.join(out, 'result.json'),
    JSON.stringify({ checks, errors }, null, 2),
  )
  console.log(`PASS ${checks} checks; ${out}`)
} finally {
  await browser?.close()
  await server.close()
}
