/**
 * Nova Admin 前端运行时验证（零依赖，Node 18+）
 *
 * 为什么需要它：编译通过 ≠ 运行时正确。本项目已经出现过多起
 * 「类型检查全绿、页面却打不开」的缺陷（见 AGENTS.md §5），
 * 这类问题只能靠真实浏览器跑一遍才能发现。
 *
 * 原理：用系统已装的 Chrome/Edge 以 `--headless` 起一个调试端口，
 * 通过 CDP（纯 WebSocket 协议，Node 原生支持）驱动页面。
 * 不需要 Playwright / Puppeteer。
 *
 * 用法：
 *   1. 先启动后端与前端（npm run dev 或 npm run preview）
 *   2. node scripts/browser-verify.mjs
 *   3. 指定其他地址：APP_URL=http://localhost:4173 node scripts/browser-verify.mjs
 *
 * 退出码：0 全部通过；1 存在失败项。
 * 明细结果写入 scripts/.browser-verify.json。
 */

import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

// ============ 配置 ============
const CHROME_CANDIDATES = [
  process.env.CHROME_PATH,
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
].filter(Boolean)

const CHROME = CHROME_CANDIDATES.find((p) => fs.existsSync(p))
if (!CHROME) {
  console.error('未找到 Chrome / Edge，请用 CHROME_PATH 环境变量指定可执行文件路径')
  process.exit(1)
}

const APP = process.env.APP_URL || 'http://localhost:6801'
const PORT = Number(process.env.CDP_PORT || 9341)
const ADMIN = { user: 'admin', secret: 'admin123456' }
const VIEWER = { user: 'demo', secret: 'demo123456' }

const OUT_DIR = path.join(path.dirname(new URL(import.meta.url).pathname.replace(/^\//, '')), '.browser-verify.json')
const PROFILE_TAG = 'nova-browser-verify'
// ==============================

const userDataDir = path.join(os.tmpdir(), `${PROFILE_TAG}-${Date.now()}`)
const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${userDataDir}`,
    '--window-size=1600,1000',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-gpu',
    '--disable-dev-shm-usage',
    'about:blank',
  ],
  { stdio: 'ignore', detached: false },
)

function killChrome() {
  try { spawn('taskkill', ['/PID', String(chrome.pid), '/T', '/F'], { stdio: 'ignore' }) } catch { /* 忽略 */ }
  try { chrome.kill() } catch { /* 忽略 */ }
}
process.on('exit', killChrome)

let ws
let msgId = 0
const pending = new Map()
let consoleLogs = []
let pageErrors = []

function send(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++msgId
    pending.set(id, { resolve, reject })
    ws.send(JSON.stringify({ id, method, params }))
  })
}

async function connect() {
  for (let i = 0; i < 60; i++) {
    try { if ((await fetch(`http://127.0.0.1:${PORT}/json/version`)).ok) break } catch { /* 等待启动 */ }
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
      return
    }
    if (m.method === 'Runtime.consoleAPICalled') {
      consoleLogs.push({ type: m.params.type, text: (m.params.args || []).map((a) => a.value ?? a.description ?? a.type).join(' ') })
    } else if (m.method === 'Runtime.exceptionThrown') {
      pageErrors.push(m.params.exceptionDetails?.exception?.description || m.params.exceptionDetails?.text)
    } else if (m.method === 'Log.entryAdded') {
      const e = m.params.entry
      if (e.level === 'error' || e.level === 'warning') consoleLogs.push({ type: `log:${e.level}`, text: e.text })
    }
  }
  await send('Runtime.enable')
  await send('Log.enable')
  await send('Page.enable')
}

/**
 * 固定「系统偏好」为指定值。
 * 注意：无头 Chrome 的 prefers-color-scheme 默认是 **dark**，
 * 若不固定，主题初始值会随浏览器实现漂移，断言就没法稳定。
 */
async function emulateColorScheme(scheme) {
  await send('Emulation.setEmulatedMedia', {
    media: '',
    features: [{ name: 'prefers-color-scheme', value: scheme }],
  })
}

async function evaluate(expression, awaitPromise = false) {
  const r = await send('Runtime.evaluate', { expression, awaitPromise, returnByValue: true })
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
  return r.result.value
}

/**
 * 取「当前打开的」下拉里**真正可见**的选项文本（没有可见 popper 时返回 null）。
 *
 * 两个必须处理的细节：
 *  1. 页面上会同时挂多个未激活的 popper，只按 display 判断不够 ——
 *     用项目里 probeTreeSelect 同一套判据（display / visibility / opacity）。
 *  2. **每条选项本身也要判可见性**：Element Plus 的原生 `filterable` 过滤是用
 *     `v-show` 隐藏不匹配项（DOM 里一条都不少），而「用 v-for 重建列表」的
 *     自定义过滤是 DOM 里真的只剩匹配项。不判可见性的话，前者的过滤结果
 *     会被数成「没过滤」—— 断言假失败，排查时还会怀疑是功能坏了。
 * 刻意写成朴素 for 循环：这段要跨 CDP 求值，嵌套箭头函数 + 模板拼接
 * 曾在真实浏览器里抛 `TypeError: Illegal invocation`，排查成本远高于收益。
 */
const PICK_TEXTS = `(function () {
  var all = document.querySelectorAll('.el-popper');
  var pop = null;
  for (var i = 0; i < all.length; i++) {
    var cs = getComputedStyle(all[i]);
    if (cs.display !== 'none' && cs.visibility !== 'hidden' && Number(cs.opacity) > 0.01) pop = all[i];
  }
  if (!pop) return null;
  var items = pop.querySelectorAll('.el-select-dropdown__item');
  var texts = [];
  for (var j = 0; j < items.length; j++) {
    var ics = getComputedStyle(items[j]);
    if (ics.display === 'none' || ics.visibility === 'hidden') continue;
    if (items[j].classList.contains('is-hidden')) continue;
    texts.push(String(items[j].textContent).trim());
  }
  return texts;
})()`

/** 往 el-select 的搜索框里填值（模拟真实输入，触发 filterable 过滤） */
function typeIntoSelect(selector, value) {
  return (
    '(() => {' +
    `const sel = document.querySelector(${JSON.stringify(selector)});` +
    'const inp = sel && sel.querySelector("input");' +
    'if (!inp) return false;' +
    'const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;' +
    `set.call(inp, ${JSON.stringify(value)});` +
    'inp.dispatchEvent(new Event("input", { bubbles: true }));' +
    'return true;' +
    '})()'
  )
}

/** 导航。注意 hash 变化不会重新加载文档，需要「冷启动」时先跳 about:blank。 */
async function navigate(url, wait = 1600) {
  await send('Page.navigate', { url })
  await sleep(wait)
}

async function coldStart(url, wait = 3600) {
  await navigate('about:blank', 300)
  await navigate(url, wait)
}

/**
 * 等待路由切换完成：目标 hash 已到达 **且** 切换动画已收敛。
 *
 * 只用固定 sleep 会抖：上一页的离场动画没跑完时，`.layout__content` 的第一个子元素
 * 仍是「正在离场」的旧页面，探测结果被误判成「路由跳错了」。
 * 反过来只等「没有动画类」也不行：点击到路由真正生效之间有一段空档，
 * 这时既没动画、hash 也还没变，会立刻误判成已收敛而读到旧页面。
 * 两个条件同时成立才算稳定，并且要求连续命中两次，避开「动画尚未开始」的窗口。
 */
async function waitForRouteSettled(expectedHash, timeout = 4000) {
  const deadline = Date.now() + timeout
  let stable = 0
  while (Date.now() < deadline) {
    const raw = await evaluate(`(() => {
      const content = document.querySelector('.layout__content');
      if (!content) return JSON.stringify({ hash: location.hash, busy: true });
      const busy = Array.from(content.children).some((el) =>
        /-(leave|enter)-(active|from|to)(\\s|$)/.test(String(el.className || '')));
      return JSON.stringify({ hash: location.hash, busy });
    })()`)
    const { hash, busy } = JSON.parse(raw)
    const hashOK = !expectedHash || hash === expectedHash
    stable = !busy && hashOK ? stable + 1 : 0
    if (stable >= 2) return true
    await sleep(80)
  }
  return false
}

/** 探测当前页面状态：先等路由切换完成，再读 DOM，避免读到正在离场/尚未进场的页面。 */
async function inspectPage(expectedHash = null, timeout = 4000) {
  const settled = await waitForRouteSettled(expectedHash, timeout)
  const st = await evaluate(INSPECT_PAGE)
  return { ...st, settled }
}

async function clickByText(selector, text) {
  const box = await evaluate(`(() => {
    const el = Array.from(document.querySelectorAll(${JSON.stringify(selector)}))
      .find(e => e.textContent.trim() === ${JSON.stringify(text)});
    if (!el) return null;
    el.scrollIntoView({ block: 'center' });
    const r = el.getBoundingClientRect();
    return { x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height };
  })()`)
  if (!box) return { ok: false, reason: 'not found' }
  // 折叠/隐藏元素尺寸为 0，坐标点击会落空，退化为 JS click
  if (box.w < 2 || box.h < 2) {
    const ok = await evaluate(`(() => {
      const el = Array.from(document.querySelectorAll(${JSON.stringify(selector)})).find(e => e.textContent.trim() === ${JSON.stringify(text)});
      if (!el) return false; el.click(); return true;
    })()`)
    return { ok, how: 'js' }
  }
  const p = { x: Math.round(box.x), y: Math.round(box.y), button: 'left', clickCount: 1 }
  await send('Input.dispatchMouseEvent', { type: 'mouseMoved', ...p })
  await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...p })
  await send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...p })
  return { ok: true, how: 'mouse' }
}

/**
 * 判定内容区页面是否真的可见。
 * 只看 DOM 存不存在会漏掉「元素在但被进场动画卡成 opacity: 0」这类白屏，
 * 所以同时检查 opacity / display / visibility / 高度 / 残留的过渡类。
 */
const INSPECT_PAGE = `(() => {
  const content = document.querySelector('.layout__content');
  if (!content) return { ok: false, reason: 'no .layout__content' };
  const root = content.firstElementChild;
  if (!root) return { ok: false, reason: 'no page root' };
  const cs = getComputedStyle(root);
  const r = root.getBoundingClientRect();
  const cls = String(root.className || '');
  return {
    ok: true,
    hash: location.hash,
    cls: cls.slice(0, 80),
    w: Math.round(r.width), h: Math.round(r.height),
    opacity: cs.opacity, display: cs.display, visibility: cs.visibility,
    stuckEnter: /-enter-from/.test(cls) && /-enter-active/.test(cls),
    visible: cs.display !== 'none' && cs.visibility !== 'hidden' && Number(cs.opacity) > 0.01 && r.height > 20,
    text: (root.textContent || '').replace(/\\s+/g, ' ').trim().slice(0, 60),
  };
})()`

/** 打开弹窗里的树选择器，返回弹层中真实的树节点数量 */
async function probeTreeSelect(dialogTitle, index = 0) {
  const opened = await evaluate(`(() => {
    const dlgs = Array.from(document.querySelectorAll('.el-dialog')).filter(d => getComputedStyle(d).display !== 'none');
    const dlg = dlgs.find(d => (d.querySelector('.el-dialog__title')?.textContent || '').includes(${JSON.stringify(dialogTitle)}));
    if (!dlg) return 'dialog not found';
    const selects = dlg.querySelectorAll('.el-tree-select, .el-select');
    if (!selects[${index}]) return 'tree-select not found';
    const el = selects[${index}];
    (el.querySelector('.el-select__wrapper') || el).click();
    return 'ok';
  })()`)
  if (opened !== 'ok') return { ok: false, reason: opened }
  await sleep(700)
  return await evaluate(`(() => {
    const pops = Array.from(document.querySelectorAll('.el-popper')).filter(p => {
      const cs = getComputedStyle(p);
      return cs.display !== 'none' && cs.visibility !== 'hidden' && Number(cs.opacity) > 0.01;
    });
    const pop = pops[pops.length - 1];
    if (!pop) return { ok: false, reason: 'no visible popper' };
    const nodes = pop.querySelectorAll('.el-tree-node');
    return {
      ok: true,
      treeNodes: nodes.length,
      items: pop.querySelectorAll('.el-select-dropdown__item').length,
      labels: Array.from(nodes).slice(0, 6).map(n => (n.textContent || '').trim().slice(0, 14)),
    };
  })()`)
}

async function closePopper() {
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await sleep(400)
}

/**
 * 采样页面里「面积不小、背景却很亮」的元素。
 * 暗色主题最容易出的问题就是某个组件没跟着换色，留下一块白底 ——
 * 这类缺陷肉眼看截图能发现，但断言不了，所以这里用亮度阈值扫一遍。
 */
const PROBE_LIGHT_SURFACES = `(() => {
  const lum = (color) => {
    const m = String(color).match(/rgba?\\(([^)]+)\\)/);
    if (!m) return null;
    const p = m[1].split(',').map(Number);
    const a = p.length > 3 ? p[3] : 1;
    if (a < 0.5) return null;
    return (0.2126 * p[0] + 0.7152 * p[1] + 0.0722 * p[2]) / 255;
  };
  const scope = document.querySelector('.layout') || document.body;
  const out = [];
  scope.querySelectorAll('*').forEach((el) => {
    const r = el.getBoundingClientRect();
    if (r.width * r.height < 1200) return;
    const cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden' || Number(cs.opacity) < 0.5) return;
    const l = lum(cs.backgroundColor);
    if (l === null || l < 0.72) return;
    out.push({
      tag: el.tagName.toLowerCase(),
      cls: String(el.className || '').slice(0, 44),
      bg: cs.backgroundColor,
      area: Math.round(r.width * r.height),
    });
  });
  return out;
})()`

/** 计算关键文字与其实际背景的对比度（WCAG 算法），用于确认暗色下文字仍然可读 */
const PROBE_CONTRAST = `(() => {
  const parse = (c) => {
    const m = String(c).match(/rgba?\\(([^)]+)\\)/);
    if (!m) return null;
    const p = m[1].split(',').map(Number);
    return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 };
  };
  const lin = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
  const L = (c) => 0.2126 * lin(c.r) + 0.7152 * lin(c.g) + 0.0722 * lin(c.b);
  const bgOf = (el) => {
    let node = el;
    while (node && node !== document.documentElement) {
      const c = parse(getComputedStyle(node).backgroundColor);
      if (c && c.a > 0.85) return c;
      node = node.parentElement;
    }
    return parse(getComputedStyle(document.body).backgroundColor) || { r: 255, g: 255, b: 255, a: 1 };
  };
  const targets = [
    '.layout__content h3',
    '.stat-card__value',
    '.sidebar__name',
    '.navbar__username',
    '.layout__content .el-table th .cell',
  ];
  return targets.map((sel) => {
    const el = document.querySelector(sel);
    if (!el) return { sel, missing: true };
    const fg = parse(getComputedStyle(el).color);
    const bg = bgOf(el);
    if (!fg) return { sel, missing: true };
    const l1 = L(fg), l2 = L(bg);
    const ratio = (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
    return {
      sel,
      text: (el.textContent || '').replace(/\\s+/g, ' ').trim().slice(0, 16),
      fg: getComputedStyle(el).color,
      bg: 'rgb(' + bg.r + ', ' + bg.g + ', ' + bg.b + ')',
      ratio: Number(ratio.toFixed(2)),
    };
  });
})()`

/** 弹窗底部主按钮：验证「文字色 ≠ 底色」且对比度达标。白底白字这类回归肉眼很容易漏掉。 */
const PROBE_PRIMARY_BUTTON = `(() => {
  const parse = (c) => {
    const m = String(c).match(/rgba?\\(([^)]+)\\)/);
    if (!m) return null;
    const p = m[1].split(',').map(Number);
    return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 };
  };
  const lin = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
  const L = (c) => 0.2126 * lin(c.r) + 0.7152 * lin(c.g) + 0.0722 * lin(c.b);
  const btn = document.querySelector('.el-dialog__footer .el-button--primary');
  if (!btn) return { missing: true, reason: 'no primary button in dialog footer' };
  const cs = getComputedStyle(btn);
  const fg = parse(cs.color);
  const bg = parse(cs.backgroundColor);
  if (!fg || !bg || bg.a < 0.9) return { missing: true, reason: 'unresolved color', bg: cs.backgroundColor, fg: cs.color };
  const l1 = L(fg), l2 = L(bg);
  return {
    text: (btn.textContent || '').trim(),
    cls: String(btn.className),
    fg: cs.color,
    bg: cs.backgroundColor,
    ratio: Number(((Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)).toFixed(2)),
  };
})()`

/** 抓取主题相关的可观测状态 */
const PROBE_THEME = `JSON.stringify({
  dark: document.documentElement.classList.contains('dark'),
  attr: document.documentElement.dataset.theme || null,
  stored: localStorage.getItem('nova_theme'),
  bodyBg: getComputedStyle(document.body).backgroundColor,
  // 数据概览页首张指标卡：亮暗两套主题下底色/渐变应不同（原仪表盘欢迎卡探针的下位替代）
  cardBg: (() => { const e = document.querySelector('.overview-page .stat-card'); return e ? getComputedStyle(e).backgroundImage + '|' + getComputedStyle(e).backgroundColor : null })(),
  contentBg: (() => { const e = document.querySelector('.layout__content'); return e ? getComputedStyle(e).backgroundColor : null })(),
  appBg: getComputedStyle(document.documentElement).getPropertyValue('--app-bg').trim(),
  textColor: getComputedStyle(document.body).color,
})`

/** 通过真实登录页表单登录（不打桩，走完整链路） */
async function formLogin(user, secret) {
  await evaluate(`(() => {
    const inputs = Array.from(document.querySelectorAll('.login input'));
    if (inputs.length < 2) return false;
    const set = (el, v) => {
      const d = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), 'value');
      if (d && d.set) d.set.call(el, v); else el.value = v;
      el.dispatchEvent(new Event('input', { bubbles: true }));
    };
    set(inputs[0], ${JSON.stringify(user)});
    set(inputs[1], ${JSON.stringify(secret)});
    return true;
  })()`)
  await sleep(300)
  await evaluate(`(() => {
    const b = Array.from(document.querySelectorAll('.login button')).find(x => x.textContent.includes('登'));
    if (!b) return false; b.click(); return true;
  })()`)
  await sleep(3200)
  return await evaluate('location.hash')
}

/** 冷启动到登录页并登出，确保每次验证从干净状态开始 */
async function resetSession() {
  try {
    await send('Storage.clearDataForOrigin', { origin: APP, storageTypes: 'local_storage,session_storage,cookies' })
  } catch { /* 忽略 */ }
  await coldStart(`${APP}/#/login`, 2200)
}

// ============ 断言收集 ============
const failures = []
const report = { app: APP, phases: {} }

function check(name, passed, detail) {
  const mark = passed ? '✅' : '❌'
  console.log(`     ${mark} ${name}${detail ? ` — ${detail}` : ''}`)
  if (!passed) failures.push({ name, detail })
}

async function main() {
  await connect()
  // 固定系统偏好为亮色，让「未选择过主题」的基线可复现
  await emulateColorScheme('light')
  console.log(`目标：${APP}`)
  console.log(`浏览器：${CHROME}\n`)

  // ── 阶段 1：冷启动 + 登录 ───────────────────────────
  console.log('[1] 冷启动与登录')
  await resetSession()
  const coldHash = await evaluate('location.hash')
  const coldCls = await evaluate(`document.querySelector('.layout__content')?.firstElementChild?.className || ''`)
  check('冷启动落到登录页', coldHash.startsWith('#/login'), `hash=${coldHash}`)
  void coldCls

  const afterLogin = await formLogin(ADMIN.user, ADMIN.secret)
  // 首页是「数据概览」而不是框架自带的仪表盘：进销存是这套后台的主线功能
  check('登录后进入数据概览', afterLogin === '#/inv/overview', `hash=${afterLogin}`)
  const invOverview = await inspectPage('#/inv/overview')
  check(
    '数据概览可见',
    !!invOverview.visible && invOverview.settled,
    `opacity=${invOverview.opacity} h=${invOverview.h} settled=${invOverview.settled}`,
  )
  report.phases.login = { coldHash, afterLogin, invOverview }

  // ── 阶段 2：动态路由（F5 深链）───────────────────────
  console.log('[2] 动态路由：F5 直接进业务页不应落 404')
  await coldStart(`${APP}/#/system/user`, 3600)
  const deep = await inspectPage('#/system/user')
  check(
    '深链 /system/user 正常渲染',
    deep.hash === '#/system/user' && deep.cls.includes('user-page') && deep.settled,
    `hash=${deep.hash} cls="${deep.cls}" settled=${deep.settled}`,
  )
  report.phases.deepLink = deep

  // 导入 / 导出入口：管理员应当可见，且导入弹窗能正常打开
  const userActions = await evaluate(
    `Array.from(document.querySelectorAll('.user-page .toolbar__actions .el-button')).map(e => e.textContent.trim()).filter(Boolean)`,
  )
  check('管理员可见导入入口', userActions.includes('导入'), `可见按钮=${JSON.stringify(userActions)}`)
  check('管理员可见导出入口', userActions.includes('导出'), `可见按钮=${JSON.stringify(userActions)}`)

  await clickByText('.user-page .toolbar__actions .el-button', '导入')
  await sleep(800)
  const importDlg = JSON.parse(
    await evaluate(`(() => {
      const dlg = Array.from(document.querySelectorAll('.el-dialog')).find(d => d.textContent.includes('导入用户'))
      if (!dlg) return JSON.stringify({ open: false })
      return JSON.stringify({
        open: true,
        h: dlg.offsetHeight,
        hasTemplate: dlg.textContent.includes('下载导入模板'),
        hasUpload: !!dlg.querySelector('.el-upload'),
        text: dlg.textContent.replace(/\\s+/g, ' ').trim().slice(0, 60),
      })
    })()`),
  )
  check('导入弹窗可打开（含模板下载与上传入口）', importDlg.open && importDlg.h > 0 && importDlg.hasTemplate && importDlg.hasUpload, JSON.stringify(importDlg))
  // 关掉弹窗，避免影响后续阶段
  await evaluate(`(() => { const b = Array.from(document.querySelectorAll('.el-dialog button')).find(e => e.textContent.trim() === '取消' || e.textContent.trim() === '关闭'); if (b) b.click(); return true })()`)
  await sleep(600)
  report.phases.exportImport = { userActions, importDlg }

  // ── 阶段 3：菜单遍历 ────────────────────────────────
  console.log('[3] 侧边栏菜单遍历（白屏 / 动画卡死检测）')
  await coldStart(`${APP}/#/inv/overview`, 3200)
  const subTitles = await evaluate(`Array.from(document.querySelectorAll('.layout__aside .el-sub-menu__title')).map(e => e.textContent.trim()).filter(Boolean)`)
  for (const t of subTitles) {
    await clickByText('.layout__aside .el-sub-menu__title', t)
    await sleep(400)
  }
  const labels = await evaluate(`Array.from(document.querySelectorAll('.layout__aside .el-menu-item')).map(e => e.textContent.trim()).filter(Boolean)`)
  const expected = {
    数据概览: { hash: '#/inv/overview', cls: 'overview-page' },
    采购管理: { hash: '#/inv/purchase', cls: 'order-page' },
    销售管理: { hash: '#/inv/sales', cls: 'order-page' },
    库存管理: { hash: '#/inv/stock', cls: 'stock-page' },
    应收应付: { hash: '#/inv/finance', cls: 'finance-page' },
    报表中心: { hash: '#/inv/report', cls: 'report-page' },
    打印模板: { hash: '#/inv/print', cls: 'print-page' },
    // 商品分类已合并进商品档案页（左侧树），独立菜单与页面一并下线，
    // 因此这里不再登记它 —— 留着会点进一个 404 的 hash
    商品管理: { hash: '#/inv/product', cls: 'product-page' },
    客户管理: { hash: '#/inv/customer', cls: 'party-page' },
    供应商管理: { hash: '#/inv/supplier', cls: 'party-page' },
    用户管理: { hash: '#/system/user', cls: 'user-page' },
    角色管理: { hash: '#/system/role', cls: 'role-page' },
    菜单管理: { hash: '#/system/menu', cls: 'menu-page' },
    部门管理: { hash: '#/system/dept', cls: 'dept-page' },
    操作日志: { hash: '#/system/log', cls: 'log-page' },
  }
  const menuResults = []
  for (const label of labels) {
    await clickByText('.layout__aside .el-menu-item', label)
    const exp = expected[label]
    const st = await inspectPage(exp?.hash)
    const matched = !!exp && st.hash === exp.hash && st.cls.includes(exp.cls)
    check(
      `${label} → ${st.hash} 可见且无动画残留`,
      st.visible && st.settled && !st.stuckEnter && matched,
      `cls="${st.cls}" h=${st.h} settled=${st.settled} stuck=${st.stuckEnter} 匹配期望=${matched}`,
    )
    menuResults.push({ label, state: st, matched })
  }
  report.phases.menus = menuResults

  // ── 阶段 4：树选择器 ────────────────────────────────
  console.log('[4] el-tree-select 是否真的渲染成树')
  const treeCases = [
    { url: '#/system/user', btn: '新增用户', dialog: '新增用户', label: '用户管理 / 所属部门' },
    { url: '#/system/dept', btn: '新增部门', dialog: '新增部门', label: '部门管理 / 上级部门' },
    { url: '#/system/menu', btn: '新增菜单', dialog: '新增菜单', label: '菜单管理 / 上级菜单' },
  ]
  const treeResults = []
  for (const t of treeCases) {
    await coldStart(`${APP}/${t.url}`, 2400)
    await evaluate(`(() => { const b = Array.from(document.querySelectorAll('button')).find(e => e.textContent.trim() === ${JSON.stringify(t.btn)}); if (b) b.click(); return !!b })()`)
    await sleep(1000)
    const probe = await probeTreeSelect(t.dialog, 0)
    check(`${t.label} 弹出树节点`, probe.ok && probe.treeNodes > 0, `treeNodes=${probe.treeNodes} labels=${JSON.stringify(probe.labels)}`)
    treeResults.push({ where: t.label, probe })
    await closePopper()
    await evaluate(`(() => { const b = Array.from(document.querySelectorAll('.el-dialog button')).find(e => e.textContent.trim() === '取消'); if (b) b.click(); return true })()`)
    await sleep(600)
  }
  report.phases.treeSelect = treeResults

  // ── 阶段 4.0：分类树操作按钮可见性 + 开单商品模糊查找 ─────
  // 这两个都出过「代码写了、用户看不到」的形态：
  //   1. 分类树的操作按钮做成 hover 才显形，但 el-tree 节点内容区宽度
  //      被 width:100% 撑破容器，三个按钮被裁到可视区之外 —— 按钮存在、
  //      却永远点不到，用户只会认为「没有删除功能」；
  //   2. 商品下拉用 el-select 的 remote，而 remote 模式下 el-select
  //      **不做本地过滤**，原样显示全部 options —— 输入关键字后列表没变化，
  //      看起来就像搜索没生效。
  console.log('[4.0] 分类树操作按钮 + 开单商品模糊查找')
  await coldStart(`${APP}/#/inv/product`, 2800)
  const catOps = await evaluate(`(() => {
    const pane = document.querySelector('.cat-pane__tree');
    const node = document.querySelector('.el-tree .cat-node');
    if (!pane || !node) return { ok: false, reason: 'no pane/node' };
    const ops = node.querySelector('.cat-node__ops');
    if (!ops) return { ok: false, reason: 'no ops container' };
    const btns = Array.from(ops.querySelectorAll('button'));
    const vis = btns.filter((b) => {
      const r = b.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && b.offsetParent !== null;
    });
    const del = btns.find((b) => (b.getAttribute('title') || '').includes('删除'));
    const paneBox = pane.getBoundingClientRect();
    const delBox = del ? del.getBoundingClientRect() : null;
    const nodeBox = node.getBoundingClientRect();
    return {
      ok: true,
      total: btns.length,
      visible: vis.length,
      titles: btns.map((b) => b.getAttribute('title')),
      deleteVisible: !!delBox && delBox.width > 0 && !!del && del.offsetParent !== null,
      deleteInsidePane: !!delBox && delBox.right <= paneBox.right + 1 && delBox.left >= paneBox.left - 1,
      deleteInsideNode: !!delBox && delBox.right <= nodeBox.right + 1,
    };
  })()`)
  check(
    '分类树节点三个操作按钮（含删除）全部可见且在容器内',
    catOps.ok && catOps.total === 3 && catOps.visible === 3 && catOps.deleteVisible && catOps.deleteInsidePane,
    JSON.stringify(catOps),
  )

  await navigate(`${APP}/#/inv/sales`, 2400)
  const pickOpened = await evaluate(`(() => {
    const b = Array.from(document.querySelectorAll('.toolbar__actions button'))
      .find((e) => e.textContent.includes('新增'));
    if (b) b.click();
    return !!b;
  })()`)
  await sleep(1400)
  // 选择器必须精确到 el-select：明细表头里也有一个 <span class="col-product">商品</span>，
  // 用 `.col-product` 会命中那个 span，取 input 得到 null，
  // 报出来的却是 `TypeError: Illegal invocation`（setter 以 null 为 this），
  // 完全指不到真正的原因。
  const PICK_SELECT = '.el-dialog .el-select.col-product'
  const pickOpenedSel = await evaluate(`(() => {
    const el = document.querySelector(${JSON.stringify(PICK_SELECT)});
    if (!el) return false;
    (el.querySelector('.el-select__wrapper') || el).click();
    return true;
  })()`)
  await sleep(800)
  const pickAll = await evaluate(`(function () {
    var texts = ${PICK_TEXTS};
    if (!texts) return { count: -1, reason: 'no visible popper' };
    return { count: texts.length, sample: texts.slice(0, 3) };
  })()`)
  await evaluate(typeIntoSelect('.el-dialog .el-select.col-product', '钢'))
  await sleep(700)
  const pickFiltered = await evaluate(`(function () {
    var texts = ${PICK_TEXTS};
    if (!texts) return { count: -1, reason: 'no visible popper' };
    var allMatch = texts.length > 0;
    for (var i = 0; i < texts.length; i++) {
      if (texts[i].indexOf('钢') === -1) { allMatch = false; break; }
    }
    return { count: texts.length, allMatch: allMatch, sample: texts.slice(0, 3) };
  })()`)
  check(
    '开单选商品支持输入字符模糊查找（列表随输入收敛且命中全部匹配）',
    pickOpened &&
      pickOpenedSel &&
      pickAll.count > 0 &&
      pickFiltered.count > 0 &&
      pickFiltered.count < pickAll.count &&
      pickFiltered.allMatch,
    `建单弹窗=${pickOpened} 下拉打开=${pickOpenedSel} 全量=${pickAll.count} 过滤后=${pickFiltered.count} 全匹配=${pickFiltered.allMatch} 示例=${JSON.stringify(pickFiltered.sample)}`,
  )
  // 商品名称框太小的话，下拉里「名称（编码）· N 个规格」只能看到「热轧钢…」，
  // 用户分不清选的是哪一个 —— 宽度是这页能不能用的硬指标，不能只靠肉眼
  const productColWidth = await evaluate(
    `(() => { const el = document.querySelector('.el-dialog .el-select.col-product'); return el ? Math.round(el.getBoundingClientRect().width) : -1 })()`,
  )
  check(
    '开单明细的商品下拉宽度足够（≥ 260px）',
    productColWidth >= 260,
    `width=${productColWidth}px`,
  )
  // 弹窗尺寸按四档统一（sm 640 / md 800 / lg 1100 / xl 1280）。
  // 此前 19 个弹窗用了 19 个不同宽度，同一系统里点开不同弹窗宽度乱跳。
  // 这条只是把「最大的那一档」钉住 —— 尺寸靠肉眼是发现不了「跑偏了一点」的。
  const orderDialogWidth = await evaluate(`(() => {
    const dlgs = Array.from(document.querySelectorAll('.el-dialog'))
      .filter((d) => getComputedStyle(d).display !== 'none');
    const dlg = dlgs[dlgs.length - 1];
    return dlg ? Math.round(dlg.getBoundingClientRect().width) : -1;
  })()`)
  check(
    '开单弹窗宽度归到 xl 档（1280px）',
    orderDialogWidth === 1280,
    `width=${orderDialogWidth}px`,
  )
  await evaluate(
    `(() => { const b = Array.from(document.querySelectorAll('.el-dialog button')).find((e) => e.textContent.trim() === '取消'); if (b) b.click(); return true })()`,
  )
  await sleep(600)
  report.phases.pickFilter = { pickAll, pickFiltered, catOps }

  // ── 阶段 4.1：商品多规格编辑弹窗 ─────────────────────
  // 这个弹窗里是自绘的「规格卡片 + 单位行」网格，比普通表单更容易出两类问题：
  //   1. 网格横向溢出弹窗（列宽是写死的 px）；
  //   2. 逐字段必填渲染了、但「整数组」的跨行校验没接上 prop，
  //      于是空规格能直接提交（服务端虽然会拦，但用户看到的是「点了确定没反应」）。
  console.log('[4.1] 商品多规格编辑弹窗（结构 + 嵌套校验）')
  await coldStart(`${APP}/#/inv/product`, 2800)
  await evaluate(`(() => { const b = Array.from(document.querySelectorAll('button')).find(e => e.textContent.trim() === '新增商品'); if (b) b.click(); return !!b })()`)
  await sleep(1200)

  const specLayout = await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    if (!dlg) return { ok: false, reason: 'no dialog' };
    const cards = dlg.querySelectorAll('.spec-card');
    const heads = Array.from(dlg.querySelectorAll('.unit-table__row--head .unit-col'))
      .map(e => e.textContent.trim()).filter(Boolean);
    return {
      ok: true,
      cards: cards.length,
      rows: dlg.querySelectorAll('.unit-table__row:not(.unit-table__row--head)').length,
      heads,
      baseChecked: !!dlg.querySelector('.unit-col--base .el-radio.is-checked'),
      overflow: Array.from(cards).some(c => c.scrollWidth > c.clientWidth + 2),
      cols: (dlg.querySelector('.unit-table__row:not(.unit-table__row--head)')?.children || []).length,
    };
  })()`)
  check(
    '新增商品弹窗渲染规格卡片与单位行（默认一个基本单位）',
    specLayout.ok && specLayout.cards === 1 && specLayout.rows === 1 && specLayout.baseChecked,
    `cards=${specLayout.cards} rows=${specLayout.rows} baseChecked=${specLayout.baseChecked}`,
  )
  // 价格列默认收起：简单商品录「编码 + 名称 + 规格」就够，一上来摆五列表格太重。
  // 先断言默认只有三列核心字段，再打开「价格设置」确认价格列会出现。
  check(
    '规格单位默认只露核心列（计量单位 / 换算率 / 基本单位），价格收起',
    ['计量单位', '换算率', '基本单位'].every((h) => specLayout.heads.includes(h)) &&
      !specLayout.heads.includes('采购价') &&
      !specLayout.heads.includes('销售价') &&
      !specLayout.overflow,
    `heads=${JSON.stringify(specLayout.heads)} overflow=${specLayout.overflow}`,
  )

  // 打开「价格设置」→ 价格两列出现且不横向溢出
  await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    const sw = dlg.querySelector('.spec-head .el-switch');
    if (sw) sw.click();
    return true;
  })()`)
  await sleep(500)
  const priceHeads = await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    const heads = Array.from(dlg.querySelectorAll('.unit-table__row--head .unit-col'))
      .map(e => e.textContent.trim()).filter(Boolean);
    const cards = dlg.querySelectorAll('.spec-card');
    return {
      heads,
      overflow: Array.from(cards).some(c => c.scrollWidth > c.clientWidth + 2),
    };
  })()`)
  check(
    '打开「价格设置」后采购价 / 销售价出现且不横向溢出',
    priceHeads.heads.includes('采购价') && priceHeads.heads.includes('销售价') && !priceHeads.overflow,
    `heads=${JSON.stringify(priceHeads.heads)} overflow=${priceHeads.overflow}`,
  )
  // 收起，避免影响后续步骤
  await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    const sw = dlg.querySelector('.spec-head .el-switch');
    if (sw) sw.click();
    return true;
  })()`)
  await sleep(300)

  await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    const pick = (t) => Array.from(dlg.querySelectorAll('button')).find(e => e.textContent.trim() === t);
    pick('添加计量单位')?.click();
    pick('添加规格')?.click();
    return true;
  })()`)
  await sleep(500)
  const grown = await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    return {
      cards: dlg.querySelectorAll('.spec-card').length,
      rows: dlg.querySelectorAll('.unit-table__row:not(.unit-table__row--head)').length,
    };
  })()`)
  check('添加规格 / 添加计量单位按钮生效', grown.cards === 2 && grown.rows === 3, `cards=${grown.cards} rows=${grown.rows}`)

  await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    Array.from(dlg.querySelectorAll('.el-button--primary')).find(e => e.textContent.trim() === '确定')?.click();
    return true;
  })()`)
  await sleep(900)
  const blocked = await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    return {
      open: !!dlg && getComputedStyle(dlg).display !== 'none',
      errors: Array.from(document.querySelectorAll('.el-dialog .el-form-item__error')).map(e => e.textContent.trim()).slice(0, 4),
    };
  })()`)
  check(
    '空规格提交被拦下且弹窗未关闭',
    blocked.open && blocked.errors.length > 0,
    `open=${blocked.open} errors=${JSON.stringify(blocked.errors)}`,
  )
  await evaluate(`(() => {
    const dlg = document.querySelector('.el-dialog');
    Array.from(dlg.querySelectorAll('button')).find(e => e.textContent.trim() === '取消')?.click();
    return true;
  })()`)
  await sleep(600)
  report.phases.productDialog = { specLayout, grown, blocked }

  // ── 阶段 4.2：商品表单的分类下拉必须包含全部层级 ─────
  // 分类多级化之后，直接把树结构（带 children）喂给 el-option 只会渲染**根节点**，
  // 子分类静默消失 —— 下拉看着完全正常，只是少了几个选项，肉眼极难发现，
  // 后果是「钢材 / 螺纹钢」这类子分类在录商品时根本选不到。
  // 这里用「左栏树的节点数 == 下拉选项数」钉住这个不变量：
  // 树是按 parent_id 递归组装的（正确实现），两边数量对不上就是漏了层级。
  console.log('[4.2] 商品表单分类下拉（扁平 + 可搜索 + 含全部层级）')
  await coldStart(`${APP}/#/inv/product`, 2800)
  const treeNodes = await evaluate(`document.querySelectorAll('.cat-pane__tree .el-tree-node').length`)
  const openedProduct = await evaluate(
    `(() => { const b = Array.from(document.querySelectorAll('button')).find(e => e.textContent.trim() === '新增商品'); if (b) b.click(); return !!b })()`,
  )
  await sleep(1200)
  const catSelOpened = await evaluate(`(() => {
    const dlgs = Array.from(document.querySelectorAll('.el-dialog')).filter(d => getComputedStyle(d).display !== 'none');
    const dlg = dlgs[dlgs.length - 1];
    if (!dlg) return 'no dialog';
    const sel = dlg.querySelector('.el-select');
    if (!sel) return 'no select';
    (sel.querySelector('.el-select__wrapper') || sel).click();
    return 'ok';
  })()`)
  await sleep(700)
  const CAT_SELECT = '.el-dialog .el-select'
  const catAll = await evaluate(`(function () {
    var texts = ${PICK_TEXTS};
    if (!texts) return { count: -1, reason: 'no visible popper' };
    return { count: texts.length, sample: texts.slice(0, 5) };
  })()`)
  check(
    '商品表单的分类下拉列出全部分类（含子分类），与左栏树节点数一致',
    openedProduct && catSelOpened === 'ok' && catAll.count > 0 && catAll.count === treeNodes,
    `树节点=${treeNodes} 下拉选项=${catAll.count} 示例=${JSON.stringify(catAll.sample)}`,
  )
  // lg 档：商品档案表单里是 6 列网格（规格卡片 + 单位行），比常规表单宽一档
  const productDialogWidth = await evaluate(`(() => {
    const dlgs = Array.from(document.querySelectorAll('.el-dialog'))
      .filter((d) => getComputedStyle(d).display !== 'none');
    const dlg = dlgs[dlgs.length - 1];
    return dlg ? Math.round(dlg.getBoundingClientRect().width) : -1;
  })()`)
  check(
    '商品档案弹窗宽度归到 lg 档（1100px）',
    productDialogWidth === 1100,
    `width=${productDialogWidth}px`,
  )

  await evaluate(typeIntoSelect(CAT_SELECT, '钢'))
  await sleep(700)
  const catFiltered = await evaluate(`(function () {
    var texts = ${PICK_TEXTS};
    if (!texts) return { count: -1, reason: 'no visible popper' };
    var allMatch = texts.length > 0;
    for (var i = 0; i < texts.length; i++) {
      if (texts[i].indexOf('钢') === -1) { allMatch = false; break; }
    }
    return { count: texts.length, allMatch: allMatch, sample: texts.slice(0, 3) };
  })()`)
  check(
    '商品表单的分类下拉支持输入搜索（与销售单选商品同一套交互）',
    catFiltered.count > 0 && catFiltered.count < catAll.count && catFiltered.allMatch,
    `全量=${catAll.count} 过滤后=${catFiltered.count} 全匹配=${catFiltered.allMatch} 示例=${JSON.stringify(catFiltered.sample)}`,
  )
  await closePopper()
  await evaluate(
    `(() => { const b = Array.from(document.querySelectorAll('.el-dialog button')).find((e) => e.textContent.trim() === '取消'); if (b) b.click(); return true })()`,
  )
  await sleep(600)
  report.phases.productCategorySelect = { treeNodes, catAll, catFiltered }

  // ── 阶段 4.3：图表在 echarts 6 下确实渲染 ─────────────
  // echarts 5.6 → 6.1 是大版本升级，而 EChart.vue 走的是按需注册
  // （echarts/core + echarts.use([...])）。**未注册的图表类型 echarts 不报错**，
  // 只是静默丢掉整个 series —— 页面不白屏、控制台可能也干净，但那块图不出来。
  // 所以这里不查「有没有报错」，直接查 canvas 的**实际像素尺寸**。
  console.log('[4.3] 图表渲染（echarts 6 大版本升级）')
  await coldStart(`${APP}/#/inv/report`, 3000)
  const charts = await evaluate(`(() => {
    const cs = Array.from(document.querySelectorAll('.page-card canvas'));
    const sized = cs.filter((c) => c.width > 0 && c.height > 0);
    return { total: cs.length, sized: sized.length, sample: sized.slice(0, 3).map((c) => c.width + 'x' + c.height) };
  })()`)
  // 报表页只有两张图：趋势（主）+ 品类占比（侧）；排行是表格、KPI 是数字卡片。
  // 断言数按实际来 —— 写大了会得到一条永远失败的假失败。
  check(
    '报表页图表全部渲染出有尺寸的 canvas',
    charts.total >= 2 && charts.sized === charts.total,
    `canvas=${charts.total} sized=${charts.sized} 尺寸=${JSON.stringify(charts.sample)}`,
  )
  // 概览页同样有图，且它是登录落地页 —— 升级后第一眼看到的就是它
  await coldStart(`${APP}/#/inv/overview`, 3000)
  const overviewCharts = await evaluate(`(() => {
    const cs = Array.from(document.querySelectorAll('.page-card canvas'));
    return { total: cs.length, sized: cs.filter((c) => c.width > 0 && c.height > 0).length };
  })()`)
  check(
    '数据概览页图表渲染正常',
    overviewCharts.total >= 1 && overviewCharts.sized === overviewCharts.total,
    `canvas=${overviewCharts.total} sized=${overviewCharts.sized}`,
  )
  report.phases.charts = { report: charts, overview: overviewCharts }

  // ── 阶段 4.4：报表中心重设计（tab 分栏 + 排行维度 + 库存分析）────────
  // 重设计后报表中心从单页堆叠改为四区 tab，排行加「客户 / 产品」维度，
  // 新增采购分析（供应商排行）与库存分析（库存金额分布 + 低库存预警清单）。
  console.log('[4.4] 报表中心 tab 分栏与排行维度')
  await coldStart(`${APP}/#/inv/report`, 3000)
  const tabState = await evaluate(`(() => {
    const tabs = Array.from(document.querySelectorAll('.report-tab'));
    return { count: tabs.length, labels: tabs.map((t) => (t.textContent || '').trim()) };
  })()`)
  check(
    '报表中心有四个报表区导航',
    tabState.count === 4 && tabState.labels.join(',') === '经营总览,销售排行,采购分析,库存分析',
    JSON.stringify(tabState),
  )
  // 切到销售排行，切产品维度，验证「销量」列出现
  await evaluate(`(() => {
    const tabs = Array.from(document.querySelectorAll('.report-tab'));
    const t = tabs.find((e) => (e.textContent || '').trim() === '销售排行');
    if (t) t.click();
    return true;
  })()`)
  await sleep(1200)
  await evaluate(`(() => {
    const btns = Array.from(document.querySelectorAll('.dim-btn'));
    const p = btns.find((e) => (e.textContent || '').trim() === '按产品');
    if (p) p.click();
    return true;
  })()`)
  await sleep(1200)
  const productRank = await evaluate(`(() => {
    const headers = Array.from(document.querySelectorAll('.report-page .el-table th')).map((e) => (e.textContent || '').trim());
    const rows = document.querySelectorAll('.report-page .el-table__body tr').length;
    return { hasSalesQty: headers.some((h) => h.includes('销量')), headers, rows };
  })()`)
  check(
    '销售排行按产品维度显示销量列',
    productRank.hasSalesQty && productRank.rows > 0,
    JSON.stringify(productRank),
  )
  // 切到库存分析，验证 KPI + 预警清单 + 库存分布图
  await evaluate(`(() => {
    const tabs = Array.from(document.querySelectorAll('.report-tab'));
    const t = tabs.find((e) => (e.textContent || '').trim() === '库存分析');
    if (t) t.click();
    return true;
  })()`)
  await sleep(1500)
  const invState = await evaluate(`(() => {
    const kpis = Array.from(document.querySelectorAll('.kpi-card__label')).map((e) => (e.textContent || '').trim());
    const warnItems = document.querySelectorAll('.warn-item').length;
    const cs = Array.from(document.querySelectorAll('.page-card canvas')).filter((c) => c.width > 0 && c.height > 0);
    return { kpis, warnItems, sizedCanvas: cs.length };
  })()`)
  check(
    '库存分析渲染 KPI、预警清单与分布图',
    invState.kpis.length === 3 && invState.sizedCanvas >= 1,
    JSON.stringify(invState),
  )
  // 报表导出入口（导出当前筛选范围）
  const exportEntry = await evaluate(
    `(() => Array.from(document.querySelectorAll('.report-page button')).some((b) => (b.textContent || '').includes('导出')))()`,
  )
  check('报表页提供导出入口', exportEntry === true, `found=${exportEntry}`)

  // 单据反审核入口：已审核的行操作列要有「反审核」
  await coldStart(`${APP}/#/inv/sales`, 3000)
  const reverseEntry = await evaluate(`(() => {
    const rows = Array.from(document.querySelectorAll('.el-table__body tr'));
    for (const r of rows) {
      const t = r.textContent || '';
      if (t.includes('已审核') && t.includes('反审核')) return true;
    }
    return false;
  })()`)
  check('已审核单据提供「反审核」入口', reverseEntry === true, `found=${reverseEntry}`)
  // 反审核入口不应出现在待处理单据上（撤销未审核的单据没有意义）
  const reverseOnPending = await evaluate(`(() => {
    const rows = Array.from(document.querySelectorAll('.el-table__body tr'));
    for (const r of rows) {
      const t = r.textContent || '';
      if (t.includes('待处理') && t.includes('反审核')) return true;
    }
    return false;
  })()`)
  check('待处理单据不显示「反审核」', reverseOnPending === false, `found=${reverseOnPending}`)

  // ── 阶段 5：低权限账号 ──────────────────────────────
  console.log('[5] 低权限账号：按钮级权限与接口鉴权')
  consoleLogs = []
  pageErrors = []
  await resetSession()
  const viewerHash = await formLogin(VIEWER.user, VIEWER.secret)
  check('demo 登录成功', viewerHash === '#/inv/overview', `hash=${viewerHash}`)
  await coldStart(`${APP}/#/system/user`, 2800)
  // §5.34 之后 v-perm 的语义是「CSS 隐藏，绝不摘 DOM」，
  // 所以这里必须检查**可见性**而不是 textContent —— 隐藏的按钮 textContent 仍在。
  // 同时锚定 §5.34 本身：被拒按钮应带 perm-denied 标记且留在 DOM 里。
  const btnState = await evaluate(`(() => {
    const els = Array.from(document.querySelectorAll('.user-page button, .user-page .el-button'))
    const vis = []
    const hiddenDenied = []
    for (const e of els) {
      const t = (e.textContent || '').trim()
      if (!t) continue
      const cs = getComputedStyle(e)
      const visible = cs.visibility !== 'hidden' && cs.display !== 'none' && e.getBoundingClientRect().width > 1
      if (visible) vis.push(t)
      else if (e.classList.contains('perm-denied')) hiddenDenied.push(t)
    }
    return { vis, hiddenDenied }
  })()`)
  const btns = btnState.vis
  check('写操作按钮被 v-perm 隐藏', !btns.includes('新增用户') && !btns.some((t) => t.includes('重置密码')),
    `可见按钮=${JSON.stringify(btns)} 被隐藏=${JSON.stringify(btnState.hiddenDenied)}`)
  // 批量导出等于一次性取走全量数据，风险等级高于「能看列表」，只读账号不应看到入口
  check('只读账号看不到导入/导出入口', !btns.includes('导入') && !btns.includes('导出'),
    `可见按钮=${JSON.stringify(btns)} 被隐藏=${JSON.stringify(btnState.hiddenDenied)}`)
  // §5.34 语义锚点：v-perm 拒绝的按钮应仍在 DOM 里（CSS 隐藏 + perm-denied 标记）
  check('v-perm 隐藏而非摘除 DOM', btnState.hiddenDenied.length >= 4, `隐藏数=${btnState.hiddenDenied.length}`)
  report.phases.viewer = { viewerHash, buttons: btns }

  // ── 阶段 6：退出登录 → 换账号 ───────────────────────
  console.log('[6] 退出登录与账号切换')
  await clickByText('.navbar__user', await evaluate(`document.querySelector('.navbar__user')?.textContent.trim() || ''`))
  await sleep(700)
  await evaluate(`(() => { const el = Array.from(document.querySelectorAll('.el-dropdown-menu__item')).find(e => e.textContent.includes('退出登录')); if (!el) return false; el.click(); return true })()`)
  await sleep(700)
  const confirmed = await evaluate(`(() => { const b = Array.from(document.querySelectorAll('.el-message-box button')).find(x => x.textContent.trim() === '退出'); if (!b) return false; b.click(); return true })()`)
  await sleep(3000)
  const afterLogout = await evaluate(`JSON.stringify({ hash: location.hash, layout: !!document.querySelector('.layout'), keys: Object.keys(localStorage) })`)
  const logoutState = JSON.parse(afterLogout)
  check('确认退出弹窗可点击', confirmed)
  check('退出后回到登录页且令牌已清空', logoutState.hash.startsWith('#/login') && logoutState.keys.length === 0 && !logoutState.layout, afterLogout)

  await formLogin(ADMIN.user, ADMIN.secret)
  const backHash = await evaluate('location.hash')
  check('重新以 admin 登录可正常进入', backHash === '#/inv/overview', `hash=${backHash}`)
  report.phases.switchAccount = { logoutState, backHash }

  // ── 阶段 7：暗色主题 ────────────────────────────────
  console.log('[7] 暗色主题：切换 / 持久化 / 浅底残留 / 对比度')
  await resetSession()
  const theme0 = JSON.parse(await evaluate(PROBE_THEME))
  check('未选择过主题时默认亮色', theme0.dark === false && theme0.stored === null, `dark=${theme0.dark} stored=${theme0.stored} bg=${theme0.bodyBg}`)

  await evaluate(`document.querySelector('.login__theme')?.click()`)
  await sleep(700)
  const theme1 = JSON.parse(await evaluate(PROBE_THEME))
  check('登录页切换按钮生效并落盘', theme1.dark === true && theme1.attr === 'dark' && theme1.stored === 'dark', `dark=${theme1.dark} attr=${theme1.attr} stored=${theme1.stored}`)

  // 带着暗色偏好登录，顺带验证选择在路由跳转后不丢
  const darkLoginHash = await formLogin(ADMIN.user, ADMIN.secret)
  const theme2 = JSON.parse(await evaluate(PROBE_THEME))
  check('登录跳转后仍保持暗色', theme2.dark === true && darkLoginHash === '#/inv/overview', `hash=${darkLoginHash} dark=${theme2.dark}`)
  check(
    '暗色令牌与内容区底色都变深',
    theme2.appBg === '#0f1420' && theme2.bodyBg === theme2.contentBg && theme2.bodyBg !== theme0.bodyBg,
    `--app-bg=${theme2.appBg} body=${theme2.bodyBg} content=${theme2.contentBg}`,
  )

  const lightSurfaces = await evaluate(PROBE_LIGHT_SURFACES)
  check('暗色下无浅色底残留（面积 > 1200px²）', lightSurfaces.length === 0, lightSurfaces.length ? JSON.stringify(lightSurfaces.slice(0, 4)) : '未发现')

  const darkContrast = await evaluate(PROBE_CONTRAST)
  const darkLow = darkContrast.filter((c) => c.missing || c.ratio < 4.5)
  check('暗色下关键文字对比度 ≥ 4.5', darkLow.length === 0, darkContrast.map((c) => `${c.sel}${c.missing ? '=缺失' : `=${c.ratio}`}`).join('  '))
  report.phases.darkTheme = { theme0, theme1, theme2, lightSurfaces, contrast: darkContrast }

  // 暗色下逐页确认可读（同时验证深链 + 持久化）
  console.log('[7.1] 暗色下逐页渲染')
  const darkPages = []
  for (const [label, exp] of Object.entries(expected)) {
    await coldStart(`${APP}/#${exp.hash.slice(1)}`, 2600)
    const st = await inspectPage(exp.hash)
    const theme = JSON.parse(await evaluate(PROBE_THEME))
    const ok = st.visible && st.settled && !st.stuckEnter && st.hash === exp.hash && st.cls.includes(exp.cls) && theme.dark === true
    check(`暗色 ${label} 正常渲染且主题保持`, ok, `hash=${st.hash} cls="${st.cls}" settled=${st.settled} dark=${theme.dark}`)
    darkPages.push({ label, hash: st.hash, visible: st.visible, dark: theme.dark })
  }
  report.phases.darkPages = darkPages

  // 暗色下弹窗 / 下拉弹层的底色（最容易漏改的两类组件）
  console.log('[7.2] 暗色下弹窗与下拉弹层')
  await coldStart(`${APP}/#/system/user`, 2600)
  await evaluate(`(() => { const b = Array.from(document.querySelectorAll('button')).find(e => e.textContent.trim() === '新增用户'); if (b) b.click(); return !!b })()`)
  await sleep(1000)
  const dlgDark = await evaluate(`(() => {
    const lum = (c) => { const m = String(c).match(/rgba?\\(([^)]+)\\)/); if (!m) return null; const p = m[1].split(',').map(Number); const a = p.length > 3 ? p[3] : 1; if (a < 0.5) return null; return (0.2126*p[0] + 0.7152*p[1] + 0.0722*p[2]) / 255 };
    const dlg = document.querySelector('.el-dialog');
    const input = document.querySelector('.el-dialog .el-input__wrapper');
    return { dialog: dlg ? lum(getComputedStyle(dlg).backgroundColor) : null, input: input ? lum(getComputedStyle(input).backgroundColor) : null };
  })()`)
  check('弹窗与输入框在暗色下均为深底', dlgDark.dialog !== null && dlgDark.dialog < 0.3 && dlgDark.input !== null && dlgDark.input < 0.3, `dialog=${dlgDark.dialog} input=${dlgDark.input}`)

  const probe = await probeTreeSelect('新增用户', 0)
  const popDark = await evaluate(`(() => {
    const lum = (c) => { const m = String(c).match(/rgba?\\(([^)]+)\\)/); if (!m) return null; const p = m[1].split(',').map(Number); const a = p.length > 3 ? p[3] : 1; if (a < 0.5) return null; return (0.2126*p[0] + 0.7152*p[1] + 0.0722*p[2]) / 255 };
    const pops = Array.from(document.querySelectorAll('.el-popper')).filter(p => getComputedStyle(p).display !== 'none');
    const p = pops[pops.length - 1];
    return p ? lum(getComputedStyle(p).backgroundColor) : null;
  })()`)
  check('暗色下树选择器弹层为深底且节点渲染', probe.ok && probe.treeNodes > 0 && popDark !== null && popDark < 0.3, `treeNodes=${probe.treeNodes} popperLum=${popDark}`)
  await closePopper()
  await evaluate(`(() => { const b = Array.from(document.querySelectorAll('.el-dialog button')).find(e => e.textContent.trim() === '取消'); if (b) b.click(); return true })()`)
  await sleep(500)

  // 持久化：F5 后仍是暗色（内联脚本 + store 两条路都要生效）
  console.log('[7.3] 暗色持久化与切回亮色')
  await coldStart(`${APP}/#/inv/overview`, 3000)
  const persisted = JSON.parse(await evaluate(PROBE_THEME))
  check('刷新后仍是暗色（持久化生效）', persisted.dark === true && persisted.stored === 'dark', `dark=${persisted.dark} stored=${persisted.stored}`)

  await evaluate(`document.querySelector('.navbar__icon-btn--theme')?.click()`)
  await sleep(800)
  const backLight = JSON.parse(await evaluate(PROBE_THEME))
  check('顶栏按钮可切回亮色', backLight.dark === false && backLight.attr === 'light' && backLight.stored === 'light', `dark=${backLight.dark} stored=${backLight.stored}`)
  check('切回亮色后底色恢复', backLight.bodyBg === theme0.bodyBg && backLight.cardBg !== theme2.cardBg, `body=${backLight.bodyBg} 卡片变化=${backLight.cardBg !== theme2.cardBg}`)
  check('亮色下指标卡与暗色不同', String(backLight.cardBg) !== String(persisted.cardBg), `light="${String(backLight.cardBg).slice(0, 46)}"`)

  const lightContrast = await evaluate(PROBE_CONTRAST)
  const lightLow = lightContrast.filter((c) => c.missing || c.ratio < 4.5)
  check('亮色下关键文字对比度 ≥ 4.5', lightLow.length === 0, lightContrast.map((c) => `${c.sel}${c.missing ? '=缺失' : `=${c.ratio}`}`).join('  '))
  report.phases.themePersist = { persisted, backLight, lightContrast }

  // ── 阶段 7.4：未选择主题时跟随系统 ──────────────────
  console.log('[7.4] 未选择过主题时跟随系统偏好')
  const systemCases = []
  for (const scheme of ['dark', 'light']) {
    await emulateColorScheme(scheme)
    await evaluate(`localStorage.removeItem('nova_theme')`)
    await coldStart(`${APP}/#/inv/overview`, 2800)
    const st = JSON.parse(await evaluate(PROBE_THEME))
    const ok = st.dark === (scheme === 'dark') && st.stored === null
    check(`系统偏好 ${scheme} → 首屏即为 ${scheme}`, ok, `dark=${st.dark} stored=${st.stored} bg=${st.bodyBg}`)
    systemCases.push({ scheme, ...st })
  }
  await emulateColorScheme('light')
  report.phases.systemPreference = systemCases

  // ── 阶段 7.5：主按钮配色 ────────────────────────────
  // Element Plus 的 `el-button--default` 是尺寸类，`type="primary"` 的按钮同时带 --primary 与 --default。
  // 一旦有人用 .el-button--default 覆盖默认按钮样式，主按钮会变成「白底白字」——
  // 这类缺陷不影响任何接口与 DOM 断言，只能靠比色发现，所以固定在这里守一道。
  console.log('[7.5] 主按钮在两种主题下都清晰可见')
  const primaryButtons = []
  for (const [scheme, label] of [['light', '亮色'], ['dark', '暗色']]) {
    if (scheme === 'dark') {
      await evaluate(`document.querySelector('.navbar__icon-btn--theme')?.click()`)
      await sleep(800)
    }
    await coldStart(`${APP}/#/system/user`, 2600)
    await evaluate(`(() => { const b = Array.from(document.querySelectorAll('button')).find(e => e.textContent.trim() === '新增用户'); if (b) b.click(); return !!b })()`)
    await sleep(1000)
    const st = await evaluate(PROBE_PRIMARY_BUTTON)
    check(
      `${label}下弹窗主按钮文字与底色不同且对比度 ≥ 3`,
      st.missing !== true && st.ratio >= 3,
      st.missing ? st.reason : `text="${st.text}" fg=${st.fg} bg=${st.bg} ratio=${st.ratio}`,
    )
    primaryButtons.push({ scheme, ...st })
    await evaluate(`(() => { const b = Array.from(document.querySelectorAll('.el-dialog button')).find(e => e.textContent.trim() === '取消'); if (b) b.click(); return true })()`)
    await sleep(500)
  }
  report.phases.primaryButton = primaryButtons

  // ── 阶段 8：控制台 ──────────────────────────────────
  console.log('[8] 控制台检查')
  const errs = consoleLogs.filter((l) => ['error', 'warning', 'log:error', 'log:warning'].includes(l.type))
  const uniq = [...new Set(errs.map((e) => `${e.type}: ${String(e.text).slice(0, 160)}`))]
  uniq.slice(0, 20).forEach((t) => console.log(`     • ${t}`))
  check('无控制台错误/警告', uniq.length === 0, `共 ${uniq.length} 条`)
  const uniqErrors = [...new Set(pageErrors)]
  uniqErrors.slice(0, 5).forEach((t) => console.log(`     • ${String(t).slice(0, 200)}`))
  check('无未捕获异常', uniqErrors.length === 0, `共 ${uniqErrors.length} 条`)
  report.phases.console = { warnings: uniq, exceptions: uniqErrors }

  // ── 汇总 ────────────────────────────────────────────
  report.failures = failures
  fs.writeFileSync(OUT_DIR, JSON.stringify(report, null, 2))
  console.log(`\n${'─'.repeat(60)}`)
  if (failures.length === 0) {
    console.log('全部通过 ✅')
  } else {
    console.log(`失败 ${failures.length} 项 ❌`)
    failures.forEach((f) => console.log(`  • ${f.name} — ${f.detail || ''}`))
  }
  console.log(`明细：${OUT_DIR}`)

  ws.close()
  killChrome()
  process.exit(failures.length === 0 ? 0 : 1)
}

main().catch((e) => { console.error('脚本异常：', e); killChrome(); process.exit(1) })
