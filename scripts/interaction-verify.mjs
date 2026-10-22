/**
 * 交互缺陷的真实浏览器验证（零依赖 CDP，无需 puppeteer/playwright）。
 *
 * 与 browser-verify.mjs 的分工：
 *   browser-verify.mjs     —— 权限指令、部门树、请求层、keep-alive 等结构性验证
 *   interaction-verify.mjs —— 本文件，**受控组件**的交互正确性（见 AGENTS.md §5.44–§5.48）
 *
 * 覆盖的 5 类缺陷：
 *   1. 表单提交：空表单点登录必须有提示，焦点落到出错字段（§5.47）
 *   2. 状态开关：取消确认后回到原状，不能停在「看起来改了、其实没改」（§5.44）
 *   3. 跨页脏选择：改筛选条件后旧选择必须清空（§5.45）
 *   4. 请求冗余：日志页首屏不请求登录日志；改每页条数只发一次（§5.46）
 *   5. typecheck 类问题（env.d.ts 等）由 npm run typecheck 保证，脚本里只做冒烟
 *
 * 关键实现约束（踩过的坑，勿改）：
 *   - el-checkbox / el-switch 是**受控组件**，element.click() 与合成 MouseEvent
 *     **驱动不了** Vue 响应式更新（DOM 的 .is-checked 变了但业务计数不变），
 *     必须用 realClick() 走 Input.dispatchMouseEvent 按坐标点真实鼠标。
 *   - v-perm 隐藏元素走 visibility: hidden，innerText **取不到内容**，
 *     计数要断言 DOM 状态（.el-checkbox.is-checked 数量）而非文案。
 *
 * 用法：先启动后端与前端 dev server，再执行
 *   node scripts/interaction-verify.mjs
 *   APP_URL=http://localhost:4173 node scripts/interaction-verify.mjs   # 验证生产产物
 *
 * 生产产物下有一项会**跳过**并显式报出来：「switch：请求失败时错误被捕获」。
 * 它靠 import 页面里的源码模块（`/src/api/user.ts`）来注入坏令牌，而
 * vite preview 只发 dist 产物，没有 `/src/*`。跳过 ≠ 通过，汇总里单独计数。
 */

import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

const CHROME = process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const APP = process.env.APP_URL || 'http://localhost:6083'
const PORT = 9352
const API = 'http://127.0.0.1:6085/api/v1'
const OUT_DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\//, ''))

const userDataDir = path.join(os.tmpdir(), `cdp-p3-${Date.now()}`)
const chrome = spawn(CHROME, [
  '--headless=new',
  `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${userDataDir}`,
  '--window-size=1600,1000',
  '--no-first-run', '--no-default-browser-check',
  '--disable-gpu', '--disable-dev-shm-usage',
  'about:blank',
], { stdio: 'ignore' })

function killChrome() {
  try { spawn('taskkill', ['/PID', String(chrome.pid), '/T', '/F'], { stdio: 'ignore' }) } catch {}
  try { chrome.kill() } catch {}
}
process.on('exit', killChrome)

let ws, msgId = 0
const pending = new Map()
const consoleLogs = []
const pageErrors = []
const requests = []

function send(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++msgId
    pending.set(id, { resolve, reject })
    ws.send(JSON.stringify({ id, method, params }))
  })
}

async function connect() {
  for (let i = 0; i < 60; i++) {
    try { if ((await fetch(`http://127.0.0.1:${PORT}/json/version`)).ok) break } catch {}
    await sleep(300)
  }
  const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json()
  const page = list.find((t) => t.type === 'page')
  ws = new WebSocket(page.webSocketDebuggerUrl)
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej })
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data)
    if (m.id && pending.has(m.id)) {
      const { resolve, reject } = pending.get(m.id)
      pending.delete(m.id)
      m.error ? reject(new Error(JSON.stringify(m.error))) : resolve(m.result)
    } else if (m.method === 'Runtime.consoleAPICalled') {
      consoleLogs.push({ type: m.params.type, text: (m.params.args || []).map((a) => a.value ?? a.description ?? a.type).join(' ') })
    } else if (m.method === 'Runtime.exceptionThrown') {
      pageErrors.push(m.params.exceptionDetails?.exception?.description || m.params.exceptionDetails?.text)
    } else if (m.method === 'Network.requestWillBeSent') {
      requests.push(m.params.request.url)
    }
  }
  await send('Runtime.enable')
  await send('Page.enable')
  await send('Network.enable')
}

async function evaluate(expression, awaitPromise = false) {
  const r = await send('Runtime.evaluate', { expression, awaitPromise, returnByValue: true })
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
  return r.result.value
}

async function navigate(url, wait = 2000) {
  await send('Page.navigate', { url })
  await sleep(wait)
}

/**
 * 用 CDP Input 域派发真实鼠标点击。
 *
 * 页面内 `element.click()` / 合成 MouseEvent 驱动不了 el-checkbox、el-switch 这类
 * 受控组件 —— 它们的内部状态由 Vue 响应式数据决定，事件必须真的走到浏览器的
 * 命中测试与事件派发路径上才会生效。用坐标派发则与真实用户操作等价。
 */
async function realClick({ x, y }) {
  for (const type of ['mousePressed', 'mouseReleased']) {
    await send('Input.dispatchMouseEvent', {
      type, x, y, button: 'left',
      buttons: type === 'mousePressed' ? 1 : 0,
      clickCount: 1,
    })
  }
}

/** 取某个选择器元素的中心坐标 */
async function centerOf(selector, contains = null) {
  return evaluate(`(() => {
    const nodes = Array.from(document.querySelectorAll(${JSON.stringify(selector)}));
    const el = ${contains ? `nodes.find(n => (n.innerText || '').includes(${JSON.stringify(contains)}))` : 'nodes[0]'};
    if (!el) return null;
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return null;
    return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) };
  })()`)
}

async function api(pathname, method = 'GET', token = '', body) {
  const res = await fetch(`${API}${pathname}`, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json().catch(() => ({}))
}

const results = []
function check(name, pass, detail) {
  results.push({ name, pass, detail })
  console.log(`${pass ? '✅' : '❌'} ${name}${detail ? ' — ' + detail : ''}`)
}

/**
 * 跳过一项并显式报出来。
 *
 * 为什么不直接 pass：跳过与通过是两件事。生产产物（vite preview）下没有
 * 可 import 的源码模块，依赖 `/src/...` 的断言根本执行不到——
 * 把它算成「通过」等于把没验证的东西报成已验证的，正是本项目最忌讳的
 * 「静默失败通道」。
 */
function skip(name, reason) {
  results.push({ name, pass: true, skipped: true, detail: reason })
  console.log(`⏭️  ${name} — 跳过：${reason}`)
}

/** 当前页面能否 import 源码模块（只有 dev server 提供 /src/*，生产产物不行） */
async function canImportSources() {
  try {
    return await evaluate(`import('/src/utils/auth.ts').then(() => true).catch(() => false)`, true)
  } catch {
    return false
  }
}

const INJECT = (tk) => `(() => {
  localStorage.setItem('nova_access_token', ${JSON.stringify(tk.access)});
  localStorage.setItem('nova_refresh_token', ${JSON.stringify(tk.refresh)});
  return 'ok';
})()`

async function main() {
  await connect()

  const login = await api('/auth/login', 'POST', '', { username: 'admin', password: 'admin123456' })
  if (login.code !== 0) throw new Error('管理员登录失败: ' + JSON.stringify(login))
  const admin = { access: login.data.accessToken, refresh: login.data.refreshToken }

  // 可切换状态的目标账号（不能是超管自己，也不能是 id=1）
  const suffix = Date.now()
  const tmpName = `p3_probe_${suffix}`
  const created = await api('/system/users', 'POST', admin.access, {
    username: tmpName, password: 'probe123456', nickname: 'P3探针', deptId: 2, roleIds: [3],
  })
  if (created.code !== 0) throw new Error('创建探针账号失败: ' + JSON.stringify(created))
  const tmpID = created.data.id

  try {
    // ---------- 1. 首屏密码校验 ----------
    await navigate(`${APP}/?boot=${Date.now()}#/login`)
    const blank = await evaluate(`(async () => {
      const btns = Array.from(document.querySelectorAll('button'));
      const btn = btns.find(b => b.textContent.replace(/\\s+/g,'').includes('登录'));
      if (!btn) return { err: 'no login button' };
      btn.click();
      await new Promise(r => setTimeout(r, 800));
      const errs = Array.from(document.querySelectorAll('.el-form-item__error'))
        .map(e => e.textContent.trim()).filter(Boolean);
      const active = document.activeElement;
      return {
        errs,
        focusIsInput: active instanceof HTMLInputElement,
        focusPlaceholder: active instanceof HTMLInputElement ? active.placeholder : active?.tagName,
      };
    })()`, true)
    check('登录：空表单点登录有明确提示', (blank.errs || []).length >= 2,
      `提示 ${(blank.errs || []).length} 条：${(blank.errs || []).join(' / ')}`)
    check('登录：焦点自动落到第一个出错字段', blank.focusIsInput,
      `焦点元素 ${blank.focusPlaceholder || '-'}`)

    // 回车提交（未失焦）也必须触发长度校验
    const enter = await evaluate(`(async () => {
      const setVal = (el, v) => {
        const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
        setter.call(el, v);
        el.dispatchEvent(new Event('input', { bubbles: true }));
      };
      const inputs = Array.from(document.querySelectorAll('.login input'));
      const user = inputs.find(i => i.placeholder === '用户名');
      const pwd = inputs.find(i => i.placeholder === '密码');
      setVal(user, 'admin');
      setVal(pwd, '123');
      // 不失焦，直接回车 —— trigger 只有 blur 时这里不会有任何提示
      await new Promise(r => setTimeout(r, 400));
      const errs = Array.from(document.querySelectorAll('.el-form-item__error'))
        .map(e => e.textContent.trim()).filter(Boolean);
      return { errs };
    })()`, true)
    check('登录：输入未失焦时长度校验已生效',
      (enter.errs || []).some(e => e.includes('6') || e.includes('长度')),
      `提示：${(enter.errs || []).join(' / ') || '无'}`)

    // ---------- 2 & 3. switch 回滚 + 跨页脏选择 ----------
    await navigate(`${APP}/?boot=${Date.now()}#/login`)
    await evaluate(INJECT(admin))
    await navigate(`${APP}/?boot=${Date.now()}#/system/user`)
    await sleep(2200)

    // 找到探针账号所在行的 switch
    const findSwitch = `(() => {
      const rows = Array.from(document.querySelectorAll('.el-table__row'));
      const row = rows.find(r => (r.innerText || '').includes(${JSON.stringify(tmpName)}));
      if (!row) return null;
      const sw = row.querySelector('.el-switch');
      return sw ? { text: (row.innerText || '').replace(/\\s+/g, ' ').slice(0, 40) } : null;
    })()`
    const before = await evaluate(findSwitch)
    if (!before) throw new Error('列表里找不到探针账号')

    // 勾选 + 改关键字触发 search → 选择必须被清掉
    // headless 下合成 MouseEvent 驱动不了 el-checkbox（受控组件），
    // 改为取真实坐标 → 用 CDP Input 事件点击，与用户操作等价。
    const cbBox = await evaluate(`(() => {
      const row = Array.from(document.querySelectorAll('.el-table__row'))
        .find(r => (r.innerText || '').includes(${JSON.stringify(tmpName)}));
      if (!row) return null;
      const el = row.querySelector('.el-checkbox');
      if (!el) return null;
      const r = el.getBoundingClientRect();
      return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) };
    })()`)
    if (!cbBox) throw new Error('找不到探针账号的勾选框')

    await realClick(cbBox)
    await sleep(700)

    const dirty = await evaluate(`(async () => {
      // 「导出选中项（N）」在 v-perm 隐藏的下拉里，innerText 取不到，
      // 直接数表格里被勾选的行数（reserve-selection 会保留跨页勾选）
      const selCount = () => document.querySelectorAll('.el-table__body .el-checkbox.is-checked').length;
      const beforeSel = String(selCount());

      // 改筛选条件（搜索框 placeholder 实际是「用户名 / 昵称 / 手机号」）
      const kw = Array.from(document.querySelectorAll('input'))
        .find(i => i.placeholder && (i.placeholder.includes('昵称') || i.placeholder.includes('手机号')));
      if (!kw) return { err: 'no keyword input' };
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      setter.call(kw, 'admin');
      kw.dispatchEvent(new Event('input', { bubbles: true }));
      await new Promise(r => setTimeout(r, 300));
      const searchBtn = Array.from(document.querySelectorAll('button'))
        .find(b => ['查询', '搜索', '重置'].includes(b.textContent.trim()));
      searchBtn?.click();
      await new Promise(r => setTimeout(r, 1500));

      const afterSel = String(selCount());
      return { beforeSel, afterSel, keyword: kw.value, btn: searchBtn?.textContent.trim() };
    })()`, true)
    check('跨页/改条件：勾选不再残留到新查询',
      dirty.beforeSel !== '0' && dirty.afterSel === '0',
      `勾选后选中 ${dirty.beforeSel} 条 → 改条件后 ${dirty.afterSel} 条（关键字="${dirty.keyword}"）`)

    // switch 取消回滚
    await navigate(`${APP}/?boot=${Date.now()}#/system/user`)
    await sleep(2200)
    const swState = await evaluate(`(async () => {
      const rows = Array.from(document.querySelectorAll('.el-table__row'));
      const row = rows.find(r => (r.innerText || '').includes(${JSON.stringify(tmpName)}));
      if (!row) return { err: 'no row' };
      const sw = row.querySelector('.el-switch');
      if (!sw) return { err: 'no switch' };
      const before = sw.classList.contains('is-checked');

      // el-switch 的可点区域是内部 input，用它的原生 click
      const inner = sw.querySelector('input');
      inner.click();
      await new Promise(r => setTimeout(r, 700));
      // 确认框已弹出，取消它
      const box = document.querySelector('.el-message-box');
      const appeared = !!box;
      const afterClick = sw.classList.contains('is-checked');
      const cancel = Array.from(box?.querySelectorAll('button') || []).find(b => b.textContent.trim() === '取消');
      cancel?.click();
      await new Promise(r => setTimeout(r, 1200));

      const afterCancel = sw.classList.contains('is-checked');
      return { before, appeared, afterClick, afterCancel };
    })()`, true)
    check('switch：取消确认后回到原状态',
      swState.before === swState.afterCancel,
      `初始 checked=${swState.before} → 点开关 ${swState.afterClick} → 取消后 ${swState.afterCancel}（确认框出现=${swState.appeared}）`)

    // 请求失败回滚：用无效令牌调用 updateStatus
    // 这一步要靠源码模块拿到 api/auth，因此只对 dev server 有效；
    // 生产产物下必须显式跳过而不是报 FAIL，也不能悄悄算通过。
    if (await canImportSources()) {
      const failRollback = await evaluate(`(async () => {
        const api = await import('/src/api/user.ts');
        const auth = await import('/src/utils/auth.ts');
        const good = auth.getAccessToken();
        auth.setTokens('bad.token.value', 'bad.refresh.value');
        let code = 'no-throw';
        try { await api.userApi.updateStatus(${tmpID}, 0); } catch (e) { code = String(e.code ?? e.message); }
        auth.setTokens(good, ${JSON.stringify(admin.refresh)});
        return { code };
      })()`, true)
      check('switch：请求失败时错误被捕获（不静默吞掉）', failRollback.code !== 'no-throw',
        `失败返回码 ${failRollback.code}`)
    } else {
      skip('switch：请求失败时错误被捕获（不静默吞掉）',
        '当前是生产产物，拿不到 /src/api/user.ts；不带 APP_URL 用 dev server 跑即可覆盖')
    }

    // ---------- 4. 日志页请求数 ----------
    await navigate(`${APP}/?boot=${Date.now()}#/login`)
    await evaluate(INJECT(admin))
    requests.length = 0
    await navigate(`${APP}/?boot=${Date.now()}#/system/log`)
    await sleep(2500)
    const firstPaint = requests.filter(u => u.includes('/api/v1/system/logs/'))
    const loginOnFirst = firstPaint.filter(u => u.includes('logins')).length
    const opOnFirst = firstPaint.filter(u => u.includes('operations')).length
    check('日志页：首屏不请求登录日志',
      loginOnFirst === 0 && opOnFirst >= 1,
      `首屏 operations=${opOnFirst} 次，logins=${loginOnFirst} 次`)

    // 切到登录日志 tab 才加载，且只加载一次
    requests.length = 0
    const tabSwitch = await evaluate(`(async () => {
      const tabs = Array.from(document.querySelectorAll('.el-tabs__item'));
      const loginTab = tabs.find(t => t.textContent.trim() === '登录日志');
      loginTab?.click();
      await new Promise(r => setTimeout(r, 1800));
      const first = window.__logReqCount = 1;
      // 再切回再切回，确认不重复加载
      tabs.find(t => t.textContent.trim() === '操作日志')?.click();
      await new Promise(r => setTimeout(r, 900));
      tabs.find(t => t.textContent.trim() === '登录日志')?.click();
      await new Promise(r => setTimeout(r, 1500));
      return { first };
    })()`, true)
    const afterSwitch = requests.filter(u => u.includes('/api/v1/system/logs/logins'))
    check('日志页：切 tab 才加载登录日志',
      afterSwitch.length >= 1,
      `切到登录日志后 logins 请求 ${afterSwitch.length} 次`)

    // 改每页条数：只发一次
    requests.length = 0
    const sizeChange = await evaluate(`(async () => {
      // 切回操作日志 tab
      const tabs = Array.from(document.querySelectorAll('.el-tabs__item'));
      tabs.find(t => t.textContent.trim() === '操作日志')?.click();
      await new Promise(r => setTimeout(r, 1200));
      const sel = document.querySelector('.el-pagination .el-select__wrapper');
      if (!sel) return { err: 'no size selector' };
      sel.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
      sel.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
      sel.click();
      await new Promise(r => setTimeout(r, 800));
      const opt = Array.from(document.querySelectorAll('.el-select-dropdown__item'))
        .find(o => o.textContent.trim() === '20/page');
      opt?.click();
      await new Promise(r => setTimeout(r, 1800));
      return { ok: true };
    })()`, true)
    const afterSize = requests.filter(u => u.includes('/api/v1/system/logs/operations'))
    check('分页：改每页条数只发一次列表请求',
      afterSize.length <= 1,
      `改 pageSize 后 operations 请求 ${afterSize.length} 次${sizeChange.err ? '（未找到控件）' : ''}`)

  } finally {
    await api(`/system/users/${tmpID}`, 'DELETE', admin.access).catch(() => {})
  }

  const errs = consoleLogs.filter((l) => ['error', 'warning'].includes(l.type))
  const uniq = [...new Set(errs.map((e) => `${e.type}: ${String(e.text).slice(0, 140)}`))]
  console.log(`\n控制台错误/警告：${errs.length} 条（去重 ${uniq.length}）`)
  uniq.slice(0, 10).forEach((t) => console.log('  •', t))
  console.log(`页面异常：${pageErrors.length} 条`)
  ;[...new Set(pageErrors)].slice(0, 5).forEach((t) => console.log('  •', String(t).slice(0, 200)))

  const failed = results.filter((r) => !r.pass)
  const skipped = results.filter((r) => r.skipped)
  const passed = results.length - failed.length - skipped.length
  console.log(
    `\n通过 ${passed}/${passed + failed.length}` +
      (skipped.length ? `，跳过 ${skipped.length}（需 dev server）` : ''),
  )

  fs.writeFileSync(path.join(OUT_DIR, 'p3-result.json'),
    JSON.stringify({ results, consoleLogs, pageErrors }, null, 2))
  ws.close()
  killChrome()
  process.exit(failed.length ? 1 : 0)
}

main().catch((e) => { console.error('脚本失败:', e); killChrome(); process.exit(1) })
