/** Vue + Mock shell regression. Run from frontend: npm run test:layout. */
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
const out = process.env.QA_OUTPUT || path.join(os.tmpdir(), 'kasinlo-reference-layout-qa')
await fs.mkdir(out, { recursive: true })
const server = await createServer({
  root: path.join(root, 'frontend'),
  mode: 'mock',
  server: { host: '127.0.0.1', port: 6096, strictPort: true },
})
await server.listen()
let browser
let page
const errors = []
const checks = []
async function check(name, fn) {
  await fn()
  checks.push(name)
  console.log(`PASS ${name}`)
}
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
    hasTouch: true,
    reducedMotion: 'reduce',
  })
  // Optional local QA font for hosts without Chinese fonts; never changes product assets.
  if (process.env.QA_CJK_FONT) {
    const bytes = (await fs.readFile(process.env.QA_CJK_FONT)).toString('base64')
    await context.addInitScript((font) => {
      document.addEventListener('DOMContentLoaded', () => {
        const style = document.createElement('style')
        style.textContent = `@font-face { font-family: 'Kasinlo QA Chinese'; src: url(data:font/ttf;base64,${font}); } :root, html.dark { --el-font-family: 'Kasinlo QA Chinese', sans-serif; }`
        document.head.append(style)
      })
    }, bytes)
  }
  page = await context.newPage()
  page.setDefaultTimeout(10000)
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('http://127.0.0.1:6096')
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.locator('.login__submit').click()
  await page.waitForURL(/inv\/overview/)
  const aside = page.locator('.layout__aside')
  const content = page.locator('.layout__content')
  const nav = page.getByRole('navigation', {
    name: '已打开的页面',
    exact: true,
  })
  async function go(route) {
    await page.goto(`http://127.0.0.1:6096/#${route}`)
    await expect(page.locator('.el-loading-mask:visible')).toHaveCount(0)
  }
  await check(
    'desktop single sidebar, shared background and unchanged original brand asset',
    async () => {
      await expect(page.locator('.navbar__brand img')).toHaveAttribute(
        'src',
        '/brand/kasinlo-mark.png',
      )
      expect(await aside.evaluate((el) => el.getBoundingClientRect().width)).toBe(240)
      expect(
        await page.locator('.layout').evaluate((el) => getComputedStyle(el).backgroundColor),
      ).toBe('rgb(247, 248, 252)')
      await expect(aside.locator('.sidebar__secondary')).toHaveCount(0)
      await aside
        .locator('.el-sub-menu__title')
        .filter({ hasText: /^库存$/ })
        .click()
      await aside.getByRole('menuitem', { name: '库存管理', exact: true }).click()
      await expect(page).toHaveURL(/inv\/stock/)
      await expect(page.getByRole('navigation', { name: '面包屑导航' })).toContainText('库存')
      await expect(page.locator('.stock-page .el-tab-pane:visible .el-table__row')).toHaveCount(4)
      await page.evaluate(() => document.fonts.ready)
      await page.screenshot({
        path: path.join(out, 'stock-desktop.png'),
        fullPage: true,
      })
    },
  )
  await check(
    'print templates live under system and contracts under finance with working breadcrumbs',
    async () => {
      await aside
        .locator('.el-sub-menu__title')
        .filter({ hasText: /^系统管理$/ })
        .click()
      await aside.getByRole('menuitem', { name: '打印模板', exact: true }).click()
      await expect(page).toHaveURL(/inv\/print/)
      await expect(page.locator('.breadcrumb')).toContainText('系统管理')
      await expect(page.locator('.breadcrumb')).toContainText('打印模板')
      await aside
        .locator('.el-sub-menu__title')
        .filter({ hasText: /^财务$/ })
        .click()
      await aside.getByRole('menuitem', { name: '合同管理', exact: true }).click()
      await expect(page).toHaveURL(/inv\/contract/)
      await expect(page.locator('.breadcrumb')).toContainText('财务')
      await expect(page.locator('.breadcrumb')).toContainText('合同管理')
      await aside.getByRole('menuitem', { name: '库存管理', exact: true }).click()
      await expect(page.locator('.stock-page .el-tab-pane:visible .el-table__row')).toHaveCount(4)
    },
  )
  await check(
    'inventory search no-results, clear and query retain table and navigation',
    async () => {
      const input = page
        .locator('.stock-page .el-tab-pane:visible')
        .getByPlaceholder('商品编码 / 名称', { exact: true })
      await input.fill('没有这个商品-qa')
      await input.press('Enter')
      await expect(page.locator('.stock-page .el-tab-pane:visible .el-table__row')).toHaveCount(0)
      await expect(
        page.locator('.stock-page .el-tab-pane:visible .el-table__empty-block'),
      ).toBeVisible()
      await input.clear()
      await input.press('Enter')
      await expect(page.locator('.stock-page .el-tab-pane:visible .el-table__row')).toHaveCount(4)
    },
  )
  await check(
    'collapsed rail popup navigates and preserves collapse preference on reload',
    async () => {
      await page.getByRole('button', { name: '收起菜单', exact: true }).click()
      expect(await aside.evaluate((el) => el.getBoundingClientRect().width)).toBe(76)
      await aside
        .locator('.el-sub-menu__title')
        .filter({ hasText: /^采购$/ })
        .hover()
      await page
        .locator('.sidebar-menu-popup:visible')
        .getByRole('menuitem', { name: '采购管理', exact: true })
        .click()
      await expect(page).toHaveURL(/inv\/purchase/)
      await page.reload()
      await expect(aside.locator('.el-menu--collapse')).toBeVisible()
      await page.getByRole('button', { name: '展开菜单', exact: true }).click()
      await expect(aside.getByRole('menuitem', { name: '采购管理', exact: true })).toBeVisible()
    },
  )
  await check('tag navigation restores committed query, close is keyboard accessible', async () => {
    await go('/inv/purchase?view=all&q=CG&size=20')
    await expect(page).toHaveURL(/q=CG/)
    await aside
      .locator('.el-sub-menu__title')
      .filter({ hasText: /^库存$/ })
      .click()
    await aside.getByRole('menuitem', { name: '库存管理', exact: true }).click()
    await nav.getByRole('link', { name: '采购管理', exact: true }).click()
    await expect(page).toHaveURL(/q=CG/)
    const close = nav.getByRole('button', {
      name: '关闭采购管理标签',
      exact: true,
    })
    await close.focus()
    await page.keyboard.press('Enter')
    await expect(nav.getByRole('link', { name: '采购管理', exact: true })).toHaveCount(0)
    await expect(nav.locator('[aria-current="page"]')).toBeFocused()
  })
  async function visit(route) {
    await page.evaluate((value) => {
      location.hash = value
    }, route)
    await expect(page).toHaveURL(`http://127.0.0.1:6096/#${route}`)
    await expect(nav.locator('[aria-current="page"]')).toHaveAttribute(
      'href',
      new RegExp(route.split('?')[0]),
    )
    await expect(page.locator('.el-loading-mask:visible')).toHaveCount(0)
  }
  const tagMenu = page.locator('.el-dropdown-menu:visible')
  await check(
    'right-click targets the clicked inactive tag without switching and replaces More',
    async () => {
      await visit('/inv/purchase?view=all&q=CG')
      await visit('/inv/contract')
      await visit('/inv/stock')
      await expect(nav.getByRole('button', { name: '标签页操作', exact: true })).toHaveCount(0)
      const purchase = nav.getByRole('link', { name: '采购管理', exact: true })
      await purchase.click({ button: 'right' })
      await expect(tagMenu).toBeVisible()
      await expect(tagMenu).toBeFocused()
      await expect(page).toHaveURL(/inv\/stock/)
      await page.screenshot({
        path: path.join(out, 'tag-context-menu.png'),
        fullPage: true,
      })
      await tagMenu.getByRole('menuitem', { name: '关闭当前标签', exact: true }).click()
      await expect(purchase).toHaveCount(0)
      await expect(tagMenu).toHaveCount(0)
      await expect(nav.getByRole('link', { name: '库存管理', exact: true })).toHaveAttribute(
        'aria-current',
        'page',
      )
      await nav.getByRole('link', { name: '合同管理', exact: true }).click({ button: 'right' })
      await tagMenu.getByRole('menuitem', { name: '关闭其它标签', exact: true }).click()
      await expect(page).toHaveURL(/inv\/contract/)
      await expect(tagMenu).toHaveCount(0)
      await expect(nav.getByRole('link', { name: '库存管理', exact: true })).toHaveCount(0)
    },
  )
  await check(
    'keyboard context menu, Escape, outside dismissal and fixed-home disabled actions',
    async () => {
      const contract = nav.getByRole('link', { name: '合同管理', exact: true })
      await contract.focus()
      await page.keyboard.press('Shift+F10')
      await expect(tagMenu).toBeVisible()
      await expect(tagMenu).toBeFocused()
      await expect(
        tagMenu.getByRole('menuitem', { name: '关闭其它标签', exact: true }),
      ).toHaveAttribute('aria-disabled', 'true')
      await page.keyboard.press('Escape')
      await expect(tagMenu).toHaveCount(0)
      await expect(contract).toBeFocused()
      await contract.click({ button: 'right' })
      await page.locator('.breadcrumb').click()
      await expect(tagMenu).toHaveCount(0)
      await visit('/inv/overview')
      const home = nav.getByRole('link', { name: '数据概览', exact: true })
      await home.focus()
      await page.keyboard.press('Shift+F10')
      await expect(
        tagMenu.getByRole('menuitem', { name: '关闭当前标签', exact: true }),
      ).toHaveAttribute('aria-disabled', 'true')
      await page.keyboard.press('Escape')
      await expect(tagMenu).toHaveCount(0)
      await expect(home).toBeFocused()
      await contract.focus()
      await page.keyboard.press('Shift+F10')
      await expect(tagMenu).toBeFocused()
      await page.keyboard.press('ArrowDown')
      await expect(
        tagMenu.getByRole('menuitem', { name: '关闭全部标签', exact: true }),
      ).toBeFocused()
      await page.keyboard.press('Enter')
      await expect(page).toHaveURL(/inv\/overview/)
      await expect(nav.getByRole('link')).toHaveCount(1)
    },
  )
  await check(
    'context-menu bulk closes preserve tags when an unsaved-editor guard is cancelled',
    async () => {
      await visit('/inv/stock')
      await visit('/inv/purchase?action=create')
      const editor = page.locator('.purchase-editor')
      await editor
        .locator('.el-form-item')
        .filter({ has: page.getByText('备注', { exact: true }) })
        .getByRole('textbox')
        .fill('右键菜单保护')
      const discard = page.getByRole('dialog', {
        name: '放弃本次修改？',
        exact: true,
      })
      const stock = nav.getByRole('link', { name: '库存管理', exact: true })
      for (const command of ['关闭其它标签', '关闭全部标签']) {
        await stock.click({ button: 'right' })
        await tagMenu.getByRole('menuitem', { name: command, exact: true }).click()
        await discard.getByRole('button', { name: '继续编辑', exact: true }).click()
        await expect(editor).toBeVisible()
        await expect(stock).toBeVisible()
        await expect(nav.getByRole('link', { name: '采购管理', exact: true })).toHaveAttribute(
          'aria-current',
          'page',
        )
      }
      await stock.click({ button: 'right' })
      await tagMenu.getByRole('menuitem', { name: '关闭其它标签', exact: true }).click()
      await discard.getByRole('button', { name: '放弃修改', exact: true }).click()
      await expect(page).toHaveURL(/inv\/stock/)
      await expect(nav.getByRole('link', { name: '采购管理', exact: true })).toHaveCount(0)
      await expect(nav.getByRole('link', { name: '数据概览', exact: true })).toBeVisible()
    },
  )
  await check('closing an edited tag respects keep-editing and discard guards', async () => {
    await go('/inv/purchase?action=create')
    const editor = page.locator('.purchase-editor')
    await expect(editor).toBeVisible()
    await editor
      .locator('.el-form-item')
      .filter({ has: page.getByText('备注', { exact: true }) })
      .getByRole('textbox')
      .fill('保留未保存输入')
    await nav.getByRole('button', { name: '关闭采购管理标签', exact: true }).click()
    const discard = page.getByRole('dialog', {
      name: '放弃本次修改？',
      exact: true,
    })
    await discard.getByRole('button', { name: '继续编辑', exact: true }).click()
    await expect(editor).toBeVisible()
    await expect(nav.getByRole('link', { name: '采购管理', exact: true })).toBeVisible()
    await nav.getByRole('button', { name: '关闭采购管理标签', exact: true }).click()
    await discard.getByRole('button', { name: '放弃修改', exact: true }).click()
    await expect(editor).toHaveCount(0)
  })
  await check('dark theme inherits surfaces and focusable menu navigation', async () => {
    await page.getByRole('button', { name: '切换到暗色主题', exact: true }).click()
    await expect(page.locator('html')).toHaveClass(/dark/)
    expect(
      await page
        .locator('.layout__navigation')
        .evaluate((el) => getComputedStyle(el).backgroundColor),
    ).toBe('rgb(22, 27, 38)')
    await page.screenshot({
      path: path.join(out, 'dark-desktop.png'),
      fullPage: true,
    })
    await page.getByRole('button', { name: '切换到亮色主题', exact: true }).click()
  })
  await check(
    'mobile drawer opens expanded, traps focus, closes with Escape and restores trigger focus',
    async () => {
      await page.setViewportSize({ width: 390, height: 844 })
      await expect(aside).toBeHidden()
      const open = page.getByRole('button', { name: '打开菜单', exact: true })
      await open.click()
      const drawer = page.getByRole('dialog', { name: '菜单', exact: true })
      await expect(drawer).toBeVisible()
      await expect(drawer.locator('.el-menu--collapse')).toHaveCount(0)
      await page.keyboard.press('Escape')
      await expect(drawer).toBeHidden()
      await expect(open).toBeFocused()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      )
      await page.screenshot({
        path: path.join(out, 'mobile.png'),
        fullPage: true,
      })
      await open.click()
      await drawer
        .locator('.el-sub-menu__title')
        .filter({ hasText: /^采购$/ })
        .click()
      await drawer.getByRole('menuitem', { name: '采购管理', exact: true }).click()
      await expect(page).toHaveURL(/inv\/purchase/)
      await expect(drawer).toBeHidden()
    },
  )
  await check(
    'touch long-press opens an in-viewport menu without navigation and supports tap commands',
    async () => {
      await visit('/inv/stock')
      await visit('/inv/purchase')
      const link = nav.getByRole('link', { name: '采购管理', exact: true })
      await link.scrollIntoViewIfNeeded()
      const box = await link.boundingBox()
      const session = await context.newCDPSession(page)
      await session.send('Input.dispatchTouchEvent', {
        type: 'touchStart',
        touchPoints: [{ x: box.x + box.width / 2, y: box.y + box.height / 2 }],
      })
      await expect(tagMenu).toBeVisible()
      await session.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
      await expect(page).toHaveURL(/inv\/purchase/)
      const menuBox = await tagMenu.boundingBox()
      expect(menuBox.x).toBeGreaterThanOrEqual(0)
      expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(390)
      await page.screenshot({ path: path.join(out, 'tag-context-mobile.png'), fullPage: true })
      await tagMenu.getByRole('menuitem', { name: '关闭全部标签', exact: true }).tap()
      await expect(page).toHaveURL(/inv\/overview/)
      await expect(nav.getByRole('link')).toHaveCount(1)
      await session.detach()
    },
  )
  await check('long form retains shell scrolling at a short viewport', async () => {
    await page.setViewportSize({ width: 1024, height: 640 })
    await go('/inv/purchase?action=create')
    await expect(page.locator('.purchase-editor')).toBeVisible()
    const geometry = await content.evaluate((el) => ({
      scroll: el.scrollHeight,
      height: el.clientHeight,
    }))
    expect(geometry.scroll).toBeGreaterThan(geometry.height)
    await content.evaluate((el) => {
      el.scrollTop = el.scrollHeight
    })
    expect(await content.evaluate((el) => el.scrollTop)).toBeGreaterThan(0)
  })
  expect(errors).toEqual([])
  await fs.writeFile(path.join(out, 'report.json'), JSON.stringify({ checks, errors }, null, 2))
  console.log(`${checks.length} shell checks passed; ${out}`)
} catch (error) {
  if (page)
    await page.screenshot({ path: path.join(out, 'failure.png'), fullPage: true }).catch(() => {})
  await fs.writeFile(
    path.join(out, 'report.json'),
    JSON.stringify({ checks, errors, failure: String(error) }, null, 2),
  )
  throw error
} finally {
  await browser?.close()
  await server.close()
}
