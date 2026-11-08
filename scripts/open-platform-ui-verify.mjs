/** Browser checks with HTTP fixtures; no ERP or platform database required. Run from frontend. */
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { mkdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
const require = createRequire(new URL('../frontend/package.json', import.meta.url))
const { chromium, expect } = require('@playwright/test')
const { preview } = await import('../frontend/node_modules/vite/dist/node/index.js')
const root = fileURLToPath(new URL('../frontend', import.meta.url))
const out = process.env.QA_OUTPUT || path.join(tmpdir(), 'kasinlo-open-platform-ui')
await mkdir(out, { recursive: true })
const server = await preview({ root, preview: { host: '127.0.0.1', port: 4196, strictPort: true } })
let browser
const errors = []
const requests = []
let grants = [{ id: 'grant-a-123456789012345678901234567890', clientId: 'kas', expiresAt: new Date(Date.now() + 3600000).toISOString() }]
let inspectFail = false, decisionFail = false, grantsFail = false, revokeFail = false, expired = false, deny = false
let revokeCount = 0
async function check(name, fn) { await fn(); console.log(`PASS ${name}`) }
try {
  browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined, headless: true, args: ['--no-sandbox'] })
  const context = await browser.newContext({ viewport: { width: 1440, height: 1100 }, locale: 'zh-CN', colorScheme: 'light', reducedMotion: 'reduce' })
  await context.addInitScript(() => localStorage.setItem('nova_access_token', 'browser-fixture-token'))
  await context.route('**/api/v1/**', async route => {
    const req = route.request(), url = new URL(req.url())
    requests.push(url.pathname)
    const body = req.method() === 'POST' ? req.postDataJSON() : undefined
    let data = null, status = 200, message = 'OK'
    switch (url.pathname.replace('/api/v1', '')) {
      case '/profile': data = { username: 'zhangsan', nickname: '张三', roles: ['销售'], permissions: [], roleCodes: [], isSuper: false }; break
      case '/profile/routes': data = []; break
      case '/open-platform/grants':
        if (grantsFail) { status = 503; message = '服务暂时不可用，请稍后重试' } else data = grants
        break
      case '/open-platform/device/inspect':
        if (inspectFail) { status = 400; message = '验证码不存在或已过期，请在终端重新发起登录' }
        else {
          expect(body.userCode).toBe('A1B2C3D4E5F6')
          data = { clientId: 'kas', scope: 'inventory.read', accessLifetimeSeconds: 28800, expiresAt: new Date(Date.now() + (expired ? -1000 : 600000)).toISOString() }
        }
        break
      case '/open-platform/device/decision':
        expect(body.approve).toBe(!deny)
        if (decisionFail) { status = 503; message = '暂时无法提交授权，请重试' }
        break
      case '/open-platform/grants/grant-a-123456789012345678901234567890/revoke':
        revokeCount++
        if (revokeFail) { status = 503; message = '暂时无法撤销，请稍后重试' } else grants = []
        break
      default: throw new Error(`Unexpected fixture request ${url.pathname}`)
    }
    await route.fulfill({ status, json: { code: status === 200 ? 0 : status, message, data } })
  })
  const page = await context.newPage()
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', msg => { if (msg.type() === 'warning') errors.push(msg.text()) })
  const go = async () => {
    await page.goto('http://127.0.0.1:4196/#/open-platform/authorize')
    await expect(page.getByRole('heading', { name: '连接你的 CLI 与智能体' })).toBeVisible()
    await expect(page.getByText('正在加载有效授权…')).toHaveCount(0)
  }
  const enter = async () => {
    await page.getByLabel('设备验证码', { exact: true }).fill('a1b2 c3d4-e5f6')
    await page.getByRole('button', { name: '查看授权请求' }).click()
  }
  const capture = async name => {
    await expect(page.locator('#nprogress')).toHaveCount(0)
    await page.screenshot({ path: path.join(out, name), fullPage: true, animations: 'disabled' })
  }
  await check('direct authorization link loads current account and existing grants', async () => {
    await go()
    await expect(page.getByRole('link', { name: '查看当前账号资料' })).toHaveText('张三')
    await expect(page.getByRole('button', { name: /^撤销 kas CLI/ })).toBeVisible()
    await capture('code-desktop.png')
  })
  await check('empty code stays inline, focuses input and sends no inspect request', async () => {
    const count = requests.filter(p => p.endsWith('/inspect')).length
    await page.getByRole('button', { name: '查看授权请求' }).click()
    await expect(page.getByRole('alert')).toContainText('12 位验证码')
    await expect(page.getByLabel('设备验证码', { exact: true })).toBeFocused()
    expect(requests.filter(p => p.endsWith('/inspect')).length).toBe(count)
  })
  await check('invalid/expired code can be corrected without leaving page', async () => {
    inspectFail = true
    await enter()
    await expect(page.getByRole('alert')).toContainText('验证码不存在或已过期')
    inspectFail = false
    await enter()
    await expect(page.getByRole('heading', { name: '允许 kas CLI 访问库存？' })).toBeFocused()
    await expect(page.getByText('领取后 8 小时')).toBeVisible()
    await expect(page.locator('.authorize-permission')).toContainText('只读')
    await expect(page.locator('.authorize-description')).toContainText('zhangsan')
    await capture('permission-desktop.png')
  })
  await check('failed decision retains context; successful retry gives terminal next step', async () => {
    decisionFail = true
    await page.getByRole('button', { name: '允许查询库存' }).click()
    await expect(page.getByRole('alert')).toContainText('暂时无法提交授权')
    decisionFail = false
    await page.getByRole('button', { name: '允许查询库存' }).click()
    await expect(page.getByRole('heading', { name: '已同意授权，请返回终端' })).toBeFocused()
    await capture('success-desktop.png')
  })
  await check('rejecting a request reports refusal', async () => {
    await page.getByRole('button', { name: '连接另一个工具' }).click()
    await enter()
    deny = true
    await page.getByRole('button', { name: '拒绝授权' }).click()
    await expect(page.getByRole('heading', { name: '已拒绝这次授权' })).toBeVisible()
    deny = false
  })
  await check('expired request cannot approve and offers a new code', async () => {
    await page.getByRole('button', { name: '连接另一个工具' }).click()
    expired = true
    await enter()
    await expect(page.getByRole('alert')).toContainText('这次授权请求已过期')
    await expect(page.getByRole('button', { name: '允许查询库存' })).toHaveCount(0)
    await page.getByRole('button', { name: '输入新的验证码' }).click()
    expired = false
    await expect(page.getByLabel('设备验证码', { exact: true })).toBeFocused()
  })
  await check('revocation requires confirmation; cancel sends no mutation', async () => {
    await page.getByRole('button', { name: /^撤销 kas CLI/ }).click()
    await expect(page.getByRole('dialog')).toContainText('共用')
    await expect(page.getByRole('dialog')).toBeVisible()
    await capture('revoke-desktop.png')
    await page.setViewportSize({ width: 320, height: 900 })
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    // Element Plus puts role=dialog on the full-screen overlay; measure its content.
    expect((await page.locator('.authorize-revoke-dialog.el-dialog').boundingBox()).width).toBeLessThanOrEqual(288)
    await expect(page.getByRole('button', { name: '确认撤销' })).toBeInViewport()
    await capture('revoke-dark-mobile.png')
    await page.evaluate(() => document.documentElement.classList.remove('dark'))
    await page.setViewportSize({ width: 1440, height: 1100 })
    await page.getByRole('button', { name: '保留授权' }).click()
    expect(revokeCount).toBe(0)
  })
  await check('revocation failure offers refresh and retains server grant', async () => {
    revokeFail = true
    await page.getByRole('button', { name: /^撤销 kas CLI/ }).click()
    await page.getByRole('button', { name: '确认撤销' }).click()
    await expect(page.getByRole('alert')).toContainText('暂时无法撤销')
    await page.getByRole('button', { name: '刷新列表' }).click()
    await expect(page.getByRole('button', { name: /^撤销 kas CLI/ })).toBeVisible()
    revokeFail = false
  })
  await check('successful revocation removes grant and explains shared credential effect', async () => {
    await page.getByRole('button', { name: /^撤销 kas CLI/ }).click()
    await page.getByRole('button', { name: '确认撤销' }).click()
    await expect(page.getByRole('status')).toContainText('所有共用这份凭据')
    await expect(page.getByRole('heading', { name: '暂无有效授权' })).toBeVisible()
  })
  await check('grant loading failure is recoverable and is not rendered as an empty list', async () => {
    grantsFail = true
    await page.getByRole('button', { name: '刷新列表' }).click()
    await expect(page.getByRole('alert')).toContainText('服务暂时不可用')
    await expect(page.getByRole('heading', { name: '暂无有效授权' })).toHaveCount(0)
    grantsFail = false
    await page.getByRole('button', { name: '刷新列表' }).click()
    await expect(page.getByRole('heading', { name: '暂无有效授权' })).toBeVisible()
  })
  await check('light and dark desktop/mobile layouts fit viewport with readable controls', async () => {
    await enter()
    for (const dark of [false, true]) {
      await page.evaluate(value => document.documentElement.classList.toggle('dark', value), dark)
      for (const width of [1440, 390, 320]) {
        await page.setViewportSize({ width, height: 1000 })
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
        const button = page.getByRole('button', { name: '允许查询库存' })
        expect((await button.boundingBox()).height).toBeGreaterThanOrEqual(48)
        // Theme changes animate button colors; assert their settled contrast.
        await expect.poll(() => button.evaluate(el => {
          const luminance = color => {
            const channels = color.match(/[\d.]+/g).slice(0, 3).map(Number).map(v => {
              v /= 255
              return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
            })
            return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
          }
          const style = getComputedStyle(el)
          const a = luminance(style.color), b = luminance(style.backgroundColor)
          return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
        })).toBeGreaterThanOrEqual(4.5)
        await capture(`permission-${dark ? 'dark' : 'light'}-${width}.png`)
      }
    }
  })
  await check('profile has a working authorization management link', async () => {
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.goto('http://127.0.0.1:4196/#/profile')
    await page.getByRole('link', { name: /CLI 与智能体授权/ }).click()
    await expect(page.getByRole('heading', { name: '连接你的 CLI 与智能体' })).toBeVisible()
  })
  expect(errors).toEqual([])
  console.log(`PASS no browser exceptions or Vue warnings; screenshots: ${out}`)
} finally {
  await browser?.close()
  await new Promise(resolve => server.httpServer.close(resolve))
}
