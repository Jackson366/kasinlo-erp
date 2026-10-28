/** Real Vue + dev:mock UI. Missing contract-history APIs are explicit test fixtures.
 * Does not validate backend financial/inventory transactions.
 */
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
const app = 'http://127.0.0.1:16088'
const out = process.env.QA_OUTPUT || '/tmp/kasinlo-task-context-qa'
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
    '16088',
    '--strictPort',
  ],
  { cwd: path.join(root, 'frontend'), stdio: 'pipe' },
)
let serverLog = '',
  browser,
  page
server.stdout.on('data', (b) => {
  serverLog += b
})
server.stderr.on('data', (b) => {
  serverLog += b
})
const checks = [],
  errors = []
const check = async (name, fn) => {
  await fn()
  checks.push(name)
  console.log('PASS', name)
}
try {
  let ready = false
  for (let n = 0; n < 100; n++) {
    try {
      if ((await fetch(app)).ok) {
        ready = true
        break
      }
    } catch {}
    if (server.exitCode !== null) throw new Error(serverLog)
    await new Promise((r) => setTimeout(r, 100))
  }
  if (!ready) throw new Error(serverLog)
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
  await context.addInitScript(() => {
    localStorage.setItem('nova_sidebar_collapsed', '1')
    localStorage.setItem('nova_sidebar_primary_collapsed', '1')
  })
  page = await context.newPage()
  page.setDefaultTimeout(10000)
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('dialog', (dialog) => dialog.accept())
  await page.goto(app)
  await page.getByPlaceholder('用户名', { exact: true }).fill('admin')
  await page.getByPlaceholder('密码', { exact: true }).fill('admin123456')
  await page.getByRole('button', { name: '登 录', exact: true }).click()
  await page.waitForURL(/inv\/overview/)
  const field = (scope, label) =>
    scope
      .locator('.el-form-item')
      .filter({ has: page.getByText(label, { exact: true }) })
      .getByRole('textbox')
  const discard = page.getByRole('dialog', {
    name: '放弃本次修改？',
    exact: true,
  })
  async function keepEditing() {
    await discard.getByRole('button', { name: '继续编辑' }).click()
    await expect(discard).toHaveCount(0)
  }
  async function discardChanges() {
    await discard.getByRole('button', { name: '放弃修改' }).click()
    await expect(discard).toHaveCount(0)
  }
  const editor = page.locator('.purchase-editor')
  await check(
    '采购专用编辑视图、浏览器后退保护、筛选恢复及焦点返回',
    async () => {
      await page.goto(app + '/#/inv/purchase?view=all&q=CG&size=20')
      const create = page.locator('.purchase-header').getByRole('button', {
        name: '新增采购单',
        exact: true,
      })
      await expect(page).toHaveURL(/q=CG/)
      await create.click()
      await expect(editor).toBeVisible()
      await expect(page.getByRole('dialog')).toHaveCount(0)
      await expect(page).toHaveURL(/action=create/)
      await expect(editor.locator('.purchase-editor__heading')).toBeFocused()
      await page.screenshot({
        path: out + '/purchase-editor-desktop.png',
        fullPage: true,
      })
      await expect(editor.locator('.purchase-entry-help')).toHaveCount(0)
      await expect(editor.locator('.editor-save-note')).toContainText('不会增加库存或登记付款')
      await expect(editor.locator('.item-editor__total')).toHaveCount(0)
      await editor.getByRole('button', { name: '自定义编号', exact: true }).click()
      await expect(editor.getByRole('textbox', { name: '自定义单据编号' })).toBeFocused()
      await editor.getByRole('textbox', { name: '自定义单据编号' }).fill('CG-CUSTOM-001')
      await field(editor, '备注').fill('采购未保存')
      await page.locator('.navbar__right > button').first().click()
      await keepEditing()
      const currentTag = page.locator('.tags-view__item.is-active')
      await currentTag.locator('.tags-view__close').click()
      await keepEditing()
      await expect(currentTag).toBeVisible()

      await page.evaluate(() => history.back())
      await expect(discard).toBeVisible()
      await keepEditing()
      await expect(field(editor, '备注')).toHaveValue('采购未保存')
      await editor.getByRole('button', { name: '返回采购工作台' }).click()
      await discardChanges()
      await expect(editor).toHaveCount(0)
      await expect(page).toHaveURL(/q=CG/)
      await expect(page).toHaveURL(/size=20/)
      await expect(create).toBeFocused()
    },
  )
  await check('采购编辑地址刷新、加载失败重试及错误目标不可保存', async () => {
    await page.goto(app + '/#/inv/purchase?action=edit&editId=999999')
    await expect(editor).toContainText('采购单加载失败')
    await expect(
      editor.getByRole('button', { name: '保存采购单' }),
    ).toBeDisabled()
    await editor.getByRole('button', { name: '返回采购工作台' }).click()
    await page.goto(app + '/#/inv/purchase?action=edit&editId=1')
    await expect(
      editor.getByRole('heading', { name: '编辑采购单' }),
    ).toBeVisible()
    await expect(field(editor, '备注')).not.toHaveValue('')
    const original = await field(editor, '备注').inputValue()
    await field(editor, '备注').fill('刷新前的临时输入')
    await page.reload()
    await expect(field(editor, '备注')).toHaveValue(original)
    await expect(page).toHaveURL(/action=edit/)
    await editor.getByRole('button', { name: '取消', exact: true }).click()
    await expect(editor).toHaveCount(0)
  })
  await check('采购编辑深链仍检查创建与编辑权限', async () => {
    await page.evaluate(async () => {
      const { useUserStore } = await import('/src/store/user.ts')
      const store = useUserStore()
      window.originalProfile = store.profile
      store.profile = {
        ...store.profile,
        isSuper: false,
        permissions: ['inv:purchase:list'],
      }
    })
    for (const query of ['action=create', 'action=edit&editId=1']) {
      await page.evaluate((hash) => {
        location.hash = hash
      }, '#/inv/purchase?' + query)
      await expect(editor).toContainText('当前账号没有办理此操作的权限')
      await expect(
        editor.getByRole('button', { name: '保存采购单' }),
      ).toBeDisabled()
      await expect(editor.locator('.el-form')).toHaveCount(0)
      await editor.getByRole('button', { name: '返回采购工作台' }).click()
    }
    await page.evaluate(async () => {
      const { useUserStore } = await import('/src/store/user.ts')
      useUserStore().profile = window.originalProfile
    })
  })
  await check('窄屏长采购单自然滚动、固定操作区及暗色', async () => {
    await page.goto(app + '/#/inv/purchase?action=create')
    await expect(editor.getByRole('button', { name: '添加明细' })).toBeVisible()
    await page.setViewportSize({ width: 600, height: 700 })
    await page.getByRole('button', { name: '切换到暗色主题' }).click()
    for (let n = 0; n < 12; n++)
      await editor.getByRole('button', { name: '添加明细' }).click()
    const save = editor.getByRole('button', { name: '保存采购单' })
    await expect(save).toBeInViewport()
    const box = await editor.boundingBox()
    expect(box.x + box.width).toBeLessThanOrEqual(601)
    await field(editor, '备注').fill('底部可访问')
    await page.screenshot({
      path: out + '/purchase-editor-narrow.png',
      fullPage: true,
    })
    await editor.getByRole('button', { name: '取消', exact: true }).click()
    await discardChanges()
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.getByRole('button', { name: '切换到亮色主题' }).click()
  })
  await check('商品关闭、Esc 与导航保护未保存修改', async () => {
    await page.goto(app + '/#/inv/product')
    await page.getByRole('button', { name: '新增商品', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '新增商品', exact: true })
    await field(dialog, '商品名称').fill('保护商品输入')
    await dialog.getByRole('button', { name: '关闭此对话框' }).click()
    await keepEditing()
    await expect(field(dialog, '商品名称')).toHaveValue('保护商品输入')
    await page.keyboard.press('Escape')
    await keepEditing()
    await page.evaluate(() => {
      location.hash = '#/inv/contract'
    })
    await keepEditing()
    await expect(page).toHaveURL(/inv\/product/)
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await discardChanges()
    await expect(dialog).toBeHidden()
    await page.getByRole('button', { name: '新增商品', exact: true }).click()
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await expect(discard).toHaveCount(0)
  })
  await check('合同计划关闭保护及保存失败保留输入', async () => {
    await page.goto(app + '/#/inv/contract')
    await page
      .getByRole('row')
      .filter({ hasText: '设备销售合同' })
      .getByRole('button', { name: '编辑', exact: true })
      .click()
    const dialog = page.getByRole('dialog', { name: '编辑合同', exact: true })
    await dialog.getByRole('button', { name: '添加一期' }).click()
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await expect(discard).toContainText('合同尚未保存')
    await keepEditing()
    await expect(dialog.locator('.el-table__body tr')).toHaveCount(2)
    await dialog.getByPlaceholder('预付款 / 尾款').last().fill('补充尾款')
    await page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      window.originalContractUpdate = invApi.updateContract
      invApi.updateContract = async () => {
        throw new Error('模拟保存失败')
      }
    })
    await dialog.getByRole('button', { name: '保存合同' }).click()
    await expect(dialog).toContainText('保存失败，本次输入已保留')
    await expect(dialog.getByPlaceholder('预付款 / 尾款').last()).toHaveValue(
      '补充尾款',
    )
    await page.evaluate(async () => {
      const { invApi } = await import('/src/api/inv.ts')
      invApi.updateContract = window.originalContractUpdate
    })
    // Back out to keep the original contract unchanged.
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await discardChanges()
  })
  await page.evaluate(async () => {
    const { invApi } = await import('/src/api/inv.ts')
    const original = invApi.contractDetail
    invApi.contractDetail = async (id) => {
      const data = await original(id)
      if (id !== 2) return data
      return {
        ...data,
        settledAmount: 1000,
        outstanding: 19000,
        plans: data.plans.map((plan) => ({
          ...plan,
          settledAmount: 1000,
          outstanding: 19000,
        })),
      }
    }
    window.historyFixture = { fail: true, delay: 0 }
    invApi.contractPlanSettles = async (id) => {
      const { fail, delay } = window.historyFixture
      if (delay) await new Promise((r) => setTimeout(r, delay))
      if (fail) throw new Error('Fixture unavailable')
      return [
        {
          id: 1,
          planId: id,
          amount: 1000,
          method: '银行转账',
          settledAt: '2026-10-28',
          remark: `期次 ${id} 的核对流水`,
        },
      ]
    }
    invApi.contractLogs = async (id) => [
      {
        id: 1,
        username: '核对员',
        action: `查看合同 ${id}`,
        createdAt: '2026-10-28T08:00:00Z',
        ip: '127.0.0.1',
        status: 1,
      },
    ]
  })
  await check(
    '合同流水页内查看、失败重试与操作记录切换，不新增弹窗',
    async () => {
      await page
        .getByRole('row')
        .filter({ hasText: '设备销售合同' })
        .getByRole('button', { name: '详情', exact: true })
        .click()
      const drawer = page.getByRole('dialog', { name: /合同 HT/ })
      await expect(drawer.locator('.contract-money-summary')).toContainText('¥19,000.00')
      await expect(drawer.locator('.detail-desc')).toBeHidden()
      await drawer.locator('.contract-info .el-collapse-item__header').click()
      await expect(drawer.locator('.detail-desc')).toBeVisible()
      await expect(drawer.locator('.detail-desc')).toContainText('质保到期')
      await drawer.locator('.contract-info .el-collapse-item__header').click()
      await expect(drawer.locator('.detail-desc')).toBeHidden()
      await expect(drawer.locator('.contract-plans thead')).toContainText('期次 / 名称')
      await expect(drawer.locator('.contract-plans thead')).not.toContainText('比例')
      await drawer.locator('.contract-plans .el-table__expand-icon').first().click()
      await expect(drawer.locator('.contract-plans .record-details')).toContainText('签订即收付')
      await drawer.locator('.contract-plans .el-table__expand-icon').first().click()
      await drawer.getByRole('button', { name: '流水', exact: true }).click()
      const panel = drawer.getByRole('region', { name: '合同核对记录' })
      await expect(panel).toContainText('流水加载失败')
      await expect(page.getByRole('dialog')).toHaveCount(1)
      await page.evaluate(() => {
        window.historyFixture.fail = false
      })
      await panel.getByRole('button', { name: '重新加载' }).click()
      await expect(panel).toContainText('核对流水')
      await expect(
        drawer.getByText('付款计划与收付', { exact: true }),
      ).toBeVisible()
      await panel.getByRole('tab', { name: '操作记录' }).click()
      await expect(panel).toContainText('核对员')
      await expect(panel).toContainText('最近 50 条')
      await expect(panel.locator('thead')).not.toContainText('IP')
      await panel.locator('.el-table__expand-icon').first().click()
      await expect(panel.locator('.record-details')).toContainText('127.0.0.1')
      await panel.getByRole('tab', { name: '期次流水' }).click()
      await expect(panel).toContainText('核对流水')
      await page.screenshot({
        path: out + '/contract-inline-history.png',
        fullPage: true,
      })
      await panel.getByRole('button', { name: '收起记录' }).click()
      await expect(panel).toHaveCount(0)
      await drawer.getByRole('button', { name: '关闭此对话框' }).click()
    },
  )
  await check('关掉合同后迟到流水不会出现在另一份合同', async () => {
    await page.evaluate(() => {
      window.historyFixture.delay = 800
    })
    await page
      .getByRole('row')
      .filter({ hasText: '设备销售合同' })
      .getByRole('button', { name: '详情', exact: true })
      .click()
    const drawer = page.getByRole('dialog', { name: /合同 HT/ })
    await drawer.getByRole('button', { name: '流水', exact: true }).click()
    await drawer.getByRole('button', { name: '关闭此对话框' }).click()
    await page
      .getByRole('row')
      .filter({ hasText: '年度配件采购合同' })
      .getByRole('button', { name: '详情', exact: true })
      .click()
    await page.waitForTimeout(900)
    await expect(
      drawer.getByRole('region', { name: '合同核对记录' }),
    ).toHaveCount(0)
    await drawer.getByRole('button', { name: '记录', exact: true }).click()
    await expect(drawer).toContainText('查看合同 1')
    await page.setViewportSize({ width: 600, height: 700 })
    await expect(async () => {
      const box = await drawer.boundingBox()
      expect(box.x).toBeGreaterThanOrEqual(0)
      expect(box.x + box.width).toBeLessThanOrEqual(601)
    }).toPass({ timeout: 3000 })
    await page.screenshot({
      path: out + '/contract-records-narrow.png',
      fullPage: true,
    })
    await drawer.locator('.contract-info .el-collapse-item__header').click()
    await expect(drawer.locator('.detail-desc')).toBeVisible()
    const detailWidth = (await drawer.boundingBox()).width
    const widths = await drawer.locator('.detail-desc .el-descriptions__cell').evaluateAll(nodes => nodes.map(n => n.getBoundingClientRect().width))
    expect(widths.every(width => width <= detailWidth)).toBe(true)
    await page.screenshot({ path: out + '/contract-details-narrow.png', fullPage: true })
  })
  expect(errors).toEqual([])
  await fs.writeFile(
    out + '/result.json',
    JSON.stringify({ checks, errors }, null, 2),
  )
  console.log(`PASS ${checks.length} task-context scenarios; no page errors`)
} catch (error) {
  if (page) {
    await page
      .screenshot({ path: out + '/failure.png', fullPage: true })
      .catch(() => {})
    await fs.writeFile(
      out + '/failure.txt',
      `${error.stack}\n${await page.locator('body').innerText()}`,
    )
  }
  console.error(error, errors, serverLog)
  process.exitCode = 1
} finally {
  await browser?.close()
  server.kill('SIGTERM')
}
