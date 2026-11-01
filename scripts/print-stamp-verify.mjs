/** Actual Vue + Mock tests; no real company seal or business records. */
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
  process.env.QA_OUTPUT || path.join(os.tmpdir(), 'kasinlo-print-stamp-qa')
await fs.mkdir(out, { recursive: true })
const server = await createServer({
  root: path.join(root, 'frontend'),
  mode: 'mock',
  server: { host: '127.0.0.1', port: 6097, strictPort: true },
})
await server.listen()
let browser, page
const checks = [],
  errors = []
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
  page = await browser.newPage({
    viewport: { width: 1440, height: 1000 },
    colorScheme: 'light',
    locale: 'zh-CN',
    reducedMotion: 'reduce',
  })
  page.setDefaultTimeout(10000)
  if (process.env.QA_CJK_FONT) {
    const font = (await fs.readFile(process.env.QA_CJK_FONT)).toString('base64')
    await page.addInitScript((font) => {
      document.addEventListener('DOMContentLoaded', () => {
        const style = document.createElement('style')
        style.textContent = `@font-face { font-family: 'QA CJK'; src: url(data:font/ttf;base64,${font}); } :root, html.dark { --el-font-family: 'QA CJK', sans-serif !important; } body, button, input { font-family: 'QA CJK', sans-serif !important; } .sheet { font-family: 'QA CJK', serif !important; }`
        document.head.append(style)
      })
    }, font)
  }
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('http://127.0.0.1:6097')
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.locator('.login__submit').click()
  await page.waitForURL(/inv\/overview/)
  await page.goto('http://127.0.0.1:6097/#/inv/print')
  const card = page.locator('.tpl-card').first()
  await expect(card).toBeVisible()
  // Make the seed's document type match an actual purchase template.
  await card.getByRole('button', { name: '编辑', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: '编辑打印模板', exact: true })
  await dialog
    .locator('.el-form-item')
    .filter({ hasText: '单据类型' })
    .locator('input')
    .fill('采购订单')
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  await expect(dialog).toBeHidden()
  const designer = page.getByRole('dialog', { name: /设计版式/ })
  const stampGroup = () =>
    designer.getByRole('button', { name: '盖章区域', exact: true })
  const input = () => designer.locator('input[type=file]')
  async function openDesigner() {
    await card.getByRole('button', { name: '设计', exact: true }).click()
    await expect(designer).toBeVisible()
  }
  const png = async (opaque = false) =>
    Buffer.from(
      await page.evaluate((opaque) => {
        const canvas = document.createElement('canvas')
        canvas.width = 120
        canvas.height = 90
        const c = canvas.getContext('2d')
        if (opaque) {
          c.fillStyle = 'white'
          c.fillRect(0, 0, 120, 90)
        }
        c.strokeStyle = 'red'
        c.lineWidth = 6
        c.strokeRect(15, 15, 90, 60)
        return canvas.toDataURL('image/png').split(',')[1]
      }, opaque),
      'base64',
    )
  await check(
    'legacy template stays un-stamped; keyboard enables a visible region',
    async () => {
      await openDesigner()
      await expect(designer.locator('.sheet__stamp')).toHaveCount(0)
      await stampGroup().click()
      const toggle = designer.getByRole('switch', { name: '启用盖章区域' })
      await toggle.focus()
      await page.keyboard.press('Space')
      await expect(designer.locator('.sheet__stamp')).toContainText('盖章区域')
      await designer
        .getByRole('button', { name: '保存版式', exact: true })
        .click()
      await expect(designer.getByRole('alert')).toContainText(
        '请选择透明背景 PNG',
      )
    },
  )
  await check(
    'invalid and opaque images show recovery and retain a valid image',
    async () => {
      await input().setInputFiles({
        name: 'invalid.png',
        mimeType: 'image/png',
        buffer: Buffer.from('not png'),
      })
      await expect(designer.getByRole('alert')).toContainText('不是有效的 PNG')
      await input().setInputFiles({
        name: 'opaque.png',
        mimeType: 'image/png',
        buffer: await png(true),
      })
      await expect(designer.getByRole('alert')).toContainText('没有透明背景')
      await input().setInputFiles({
        name: 'sample.png',
        mimeType: 'image/png',
        buffer: await png(),
      })
      await expect(designer.locator('.sheet__stamp img')).toBeVisible()
      await input().setInputFiles({
        name: 'bad.png',
        mimeType: 'image/png',
        buffer: Buffer.from('bad'),
      })
      await expect(designer.getByRole('alert')).toBeVisible()
      await expect(designer.locator('.sheet__stamp img')).toBeVisible()
    },
  )
  await check(
    'millimeter inputs and drag update geometry without distorting the PNG',
    async () => {
      await designer
        .getByRole('spinbutton', { name: '印章左侧距离', exact: true })
        .fill('20')
      await designer
        .getByRole('spinbutton', { name: '印章顶部距离', exact: true })
        .fill('5')
      await designer
        .getByRole('spinbutton', { name: '印章区域宽度', exact: true })
        .fill('40')
      await designer
        .getByRole('spinbutton', { name: '印章区域高度', exact: true })
        .fill('40')
      await page.keyboard.press('Tab')
      const box = await designer.locator('.sheet__stamp').boundingBox()
      await page.mouse.move(box.x + 20, box.y + 20)
      await page.mouse.down()
      await page.mouse.move(box.x + 58, box.y + 20, { steps: 5 })
      await page.mouse.up()
      expect(
        Number(
          await designer
            .getByRole('spinbutton', { name: '印章左侧距离', exact: true })
            .inputValue(),
        ),
      ).toBeGreaterThan(29)
      expect(
        await designer
          .locator('.sheet__stamp img')
          .evaluate((el) => getComputedStyle(el).objectFit),
      ).toBe('contain')
      expect(
        await designer
          .locator('.sheet__stamp')
          .evaluate((el) => getComputedStyle(el).backgroundColor),
      ).toBe('rgba(0, 0, 0, 0)')
    },
  )
  await check(
    'save failure keeps configuration and supports retry; reload restores stamp',
    async () => {
      await page.evaluate(async () => {
        const { default: http } = await import('/src/utils/request.ts')
        window.__stampAdapter = http.defaults.adapter
        http.defaults.adapter = (config) =>
          config.method === 'put' && config.url.includes('print-templates')
            ? Promise.reject(new Error('测试保存失败'))
            : window.__stampAdapter(config)
      })
      await designer
        .getByRole('button', { name: '保存版式', exact: true })
        .click()
      await expect(designer.getByRole('alert').last()).toContainText('网络异常')
      await expect(designer.locator('.sheet__stamp img')).toBeVisible()
      await page.evaluate(async () => {
        const { default: http } = await import('/src/utils/request.ts')
        http.defaults.adapter = window.__stampAdapter
      })
      await designer
        .getByRole('button', { name: '保存版式', exact: true })
        .click()
      await expect(designer).toBeHidden()
      await page.reload()
      await openDesigner()
      await stampGroup().click()
      await expect(designer.locator('.sheet__stamp img')).toBeVisible()
      await expect(
        designer.getByRole('spinbutton', { name: '印章左侧距离', exact: true }),
      ).toBeVisible()
      await expect.poll(() => designer.getByRole('region', { name: '盖章区域', exact: true }).evaluate(el => el.style.height)).toBe('')
      await page.evaluate(() => document.fonts.ready)
      await page.screenshot({
        path: path.join(out, 'designer.png'),
        fullPage: true,
      })
      await designer.getByRole('button', { name: '取消', exact: true }).click()
    },
  )
  await check(
    'metadata edit preserves saved design and temporary stamp switch prints either version',
    async () => {
      await card.getByRole('button', { name: '编辑', exact: true }).click()
      const edit = page.getByRole('dialog', {
        name: '编辑打印模板',
        exact: true,
      })
      await edit
        .locator('.el-form-item')
        .filter({ hasText: '备注' })
        .locator('textarea')
        .fill('保留版式测试')
      await edit.getByRole('button', { name: '确定', exact: true }).click()
      await expect(edit).toBeHidden()
      await page.evaluate(async () => {
        const { default: http } = await import('/src/utils/request.ts')
        const original = http.defaults.adapter
        http.defaults.adapter = async (config) => {
          const response = await original(config)
          if (
            config.method === 'get' &&
            /\/inv\/orders\/\d+$/.test(config.url)
          ) {
            const detail = structuredClone(response.data.data)
            const samples = detail.items
            detail.items = Array.from({ length: 65 }, (_, i) => ({
              ...samples[i % samples.length],
              id: i + 1,
            }))
            detail.totalAmount = detail.items.reduce(
              (sum, item) => sum + item.amount,
              0,
            )
            response.data.data = detail
          }
          return response
        }
      })
      await page.goto('http://127.0.0.1:6097/#/inv/purchase')
      await expect(page.locator('.el-loading-mask:visible')).toHaveCount(0)
      await page
        .getByRole('button', { name: '打印', exact: true })
        .first()
        .click()
      const print = page.getByRole('dialog', {
        name: '打印采购单',
        exact: true,
      })
      await expect(print.locator('.sheet__stamp img')).toBeVisible()
      await expect(print.locator('tbody tr')).toHaveCount(65)
      await print
        .locator('label.el-checkbox')
        .filter({ hasText: '显示印章' })
        .click()
      await expect(print.locator('.sheet__stamp')).toHaveCount(0)
      await print
        .locator('label.el-checkbox')
        .filter({ hasText: '显示印章' })
        .click()
      await page.evaluate(() => {
        window.print = () => {
          window.__stampPrintCalls = (window.__stampPrintCalls || 0) + 1
          window.__stampPrintSnapshot = document
            .querySelector('.print-portal')
            .cloneNode(true)
        }
      })
      await print.getByRole('button', { name: '打印', exact: true }).click()
      await expect
        .poll(() => page.evaluate(() => window.__stampPrintCalls))
        .toBe(1)
      await expect(page.locator('body')).not.toHaveClass(/printing/)
      await page.evaluate(() => {
        window.dispatchEvent(new Event('afterprint'))
        document.body.append(window.__stampPrintSnapshot)
        document.body.classList.add('printing-sheet')
      })
      await page.emulateMedia({ media: 'print' })
      await expect(
        page.locator('.print-portal .sheet__stamp img'),
      ).toBeVisible()
      await expect(page.locator('.navbar')).toBeHidden()
      await page.pdf({
        path: path.join(out, 'stamped.pdf'),
        preferCSSPageSize: true,
        printBackground: true,
      })
      await page.evaluate(() => window.dispatchEvent(new Event('afterprint')))
      await page.evaluate(() => {
        document.body.classList.remove('printing-sheet')
        document.querySelector('.print-portal')?.remove()
      })
      await page.emulateMedia({ media: 'screen' })
      await print.locator('.el-dialog__headerbtn').click()
    },
  )
  await check(
    'A5, narrow dark layout and unsaved cancellation remain usable',
    async () => {
      await page.goto('http://127.0.0.1:6097/#/inv/print')
      await openDesigner()
      await designer
        .locator('.el-radio-button')
        .filter({ hasText: 'A5 竖版' })
        .click()
      await stampGroup().click()
      await expect(designer.locator('.sheet__stamp img')).toBeVisible()
      const bounds = await designer.locator('.sheet__stamp').evaluate((el) => ({
        stamp: el.getBoundingClientRect().right,
        paper: el.closest('.sheet').getBoundingClientRect().right,
      }))
      expect(bounds.stamp).toBeLessThanOrEqual(bounds.paper)
      await page.setViewportSize({ width: 390, height: 844 })
      await page.evaluate(() => document.documentElement.classList.add('dark'))
      await expect(
        designer.getByRole('button', { name: '保存版式', exact: true }),
      ).toBeInViewport()
      await page.screenshot({
        path: path.join(out, 'designer-narrow-dark.png'),
        fullPage: true,
      })
      await designer.getByRole('button', { name: '取消', exact: true }).click()
      await page.getByRole('button', { name: '继续编辑', exact: true }).click()
      await expect(designer).toBeVisible()
      await designer.getByRole('button', { name: '取消', exact: true }).click()
      await page.getByRole('button', { name: '放弃修改', exact: true }).click()
      await expect(designer).toBeHidden()
    },
  )
  if (errors.length) throw new Error(errors.join('\n'))
} catch (error) {
  if (page)
    await page
      .screenshot({ path: path.join(out, 'failure.png'), fullPage: true })
      .catch(() => {})
  throw error
} finally {
  await fs.writeFile(
    path.join(out, 'results.json'),
    JSON.stringify({ checks, errors }, null, 2),
  )
  await browser?.close()
  await server.close()
}
