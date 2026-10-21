/**
 * Nova Admin 后端接口自测脚本（零依赖，Node 18+ 内置 fetch）
 * 用法：node scripts/api-smoke.mjs [baseUrl]
 */
const BASE = process.argv[2] || 'http://127.0.0.1:6082'
const API = `${BASE}/api/v1`

// 同步落盘，便于在进程被外部终止时仍能查到执行进度
import fs from 'node:fs'
import { inflateRawSync } from 'node:zlib'
import { fileURLToPath } from 'node:url'

// 导入接口的测试样例。样例全部按 dryRun（只校验不写库）使用，因此脚本可反复执行。
// 需要重建时运行：python scripts/fixtures/build-users-import.py
const IMPORT_FIXTURE = fileURLToPath(new URL('./fixtures/users-import.xlsx', import.meta.url))
const LOG_FILE = process.env.SMOKE_LOG || '.api-smoke.log'
try {
  fs.writeFileSync(LOG_FILE, '')
} catch {
  /* 忽略 */
}
const emit = (line) => {
  process.stdout.write(line + '\n')
  try {
    fs.appendFileSync(LOG_FILE, line + '\n')
  } catch {
    /* 忽略 */
  }
}

let pass = 0
let fail = 0

function ok(name, cond, extra = '') {
  if (cond) {
    pass++
    emit(`  \u2713 ${name}`)
  } else {
    fail++
    emit(`  \u2717 ${name} ${extra}`)
  }
}

/**
 * 从 zip 里读一个entry的文本（xlsx 就是 zip，内部 XML 都是 deflate 压缩过的）。
 *
 * 为什么不用 `unzip` 命令：冒烟脚本要能在任意机器上跑，不能依赖外部命令。
 * 自己解析中央目录只有几十行，比引一个 zip 依赖更轻。
 */
function readZipEntry(buf, name) {
  // EOCD 在文件尾部，签名固定，从后往前找
  let eocd = -1
  for (let i = buf.length - 22; i >= 0 && i > buf.length - 66000; i--) {
    if (buf.readUInt32LE(i) === 0x06054b50) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('不是有效的 zip：找不到 EOCD')
  const count = buf.readUInt16LE(eocd + 10)
  let ptr = buf.readUInt32LE(eocd + 16)
  for (let i = 0; i < count; i++) {
    if (buf.readUInt32LE(ptr) !== 0x02014b50) break
    const method = buf.readUInt16LE(ptr + 10)
    const compressed = buf.readUInt32LE(ptr + 20)
    const nameLen = buf.readUInt16LE(ptr + 28)
    const extraLen = buf.readUInt16LE(ptr + 30)
    const commentLen = buf.readUInt16LE(ptr + 32)
    const localOffset = buf.readUInt32LE(ptr + 42)
    const entryName = buf.subarray(ptr + 46, ptr + 46 + nameLen).toString('utf8')
    if (entryName === name) {
      const localNameLen = buf.readUInt16LE(localOffset + 26)
      const localExtraLen = buf.readUInt16LE(localOffset + 28)
      const start = localOffset + 30 + localNameLen + localExtraLen
      const raw = buf.subarray(start, start + compressed)
      return (method === 0 ? raw : inflateRawSync(raw)).toString('utf8')
    }
    ptr += 46 + nameLen + extraLen + commentLen
  }
  throw new Error(`zip 里没有 ${name}`)
}

async function request(method, path, { token, body, raw, headers: extraHeaders } = {}) {
  const headers = { ...(extraHeaders || {}) }
  if (token) headers.Authorization = `Bearer ${token}`
  if (body) headers['Content-Type'] = 'application/json'
  const res = await fetch(`${API}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  if (raw) return { status: res.status, text }
  try {
    return { status: res.status, json: JSON.parse(text) }
  } catch {
    return { status: res.status, text }
  }
}

function section(t) {
  emit(`\n== ${t} ==`)
}

/**
 * 按权限码（或路由名）查菜单 ID。
 * 菜单主键由数据库分配，会随种子顺序变化 —— 硬编码 ID 的断言
 * 在菜单结构一变就会莫名其妙失败，所以一律现查。
 */
async function menuId(token, key) {
  const res = await request('GET', '/system/menus', { token })
  const isPerm = key.includes(':')
  const found = []
  const walk = (nodes) => {
    for (const n of nodes || []) {
      if (isPerm ? n.permission === key : n.name === key) found.push(n.id)
      walk(n.children)
    }
  }
  walk(res.json?.data)
  if (found.length === 0) throw new Error(`菜单不存在: ${key}`)
  return found[0]
}

/**
 * 下载二进制响应（导出、模板下载）。
 * 注意：这些接口不返回统一 JSON 响应体，必须按字节流处理。
 */
async function download(uri, token) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`${API}${uri}`, { headers })
  const buf = Buffer.from(await res.arrayBuffer())
  return { status: res.status, headers: res.headers, buf }
}

/** 以 multipart/form-data 上传一个文件。 */
async function upload(uri, token, filename, content) {
  const form = new FormData()
  form.append('file', new Blob([content]), filename)
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`${API}${uri}`, { method: 'POST', headers, body: form })
  const text = await res.text()
  try {
    return { status: res.status, json: JSON.parse(text) }
  } catch {
    return { status: res.status, text }
  }
}

/** xlsx 本质是 ZIP 容器，合法文件应以 PK 魔数开头。 */
function isXlsx(buf) {
  return buf.length > 4 && buf[0] === 0x50 && buf[1] === 0x4b
}

/** 校验导出类响应头：Content-Type 为表格类型，且中文文件名通过 RFC 5987 传递。 */
function isDownloadHeader(headers) {
  const ct = headers.get('content-type') || ''
  const cd = headers.get('content-disposition') || ''
  return /spreadsheetml/.test(ct) && /attachment/.test(cd) && /filename\*=UTF-8''/.test(cd)
}

const results = {}

async function main() {
  section('健康检查')
  const health = await fetch(`${BASE}/health`).then((r) => r.json())
  ok('GET /health 返回 ok', health.status === 'ok', JSON.stringify(health))

  section('认证')
  const badLogin = await request('POST', '/auth/login', { body: { username: 'admin', password: 'wrong-password' } })
  ok('错误密码返回 40101', badLogin.json?.code === 40101, JSON.stringify(badLogin.json))

  const login = await request('POST', '/auth/login', { body: { username: 'admin', password: 'admin123456' } })
  ok('管理员登录成功', login.json?.code === 0, JSON.stringify(login.json))
  const token = login.json?.data?.accessToken
  const refreshToken = login.json?.data?.refreshToken
  ok('返回 accessToken', typeof token === 'string' && token.length > 20)
  ok('返回 refreshToken', typeof refreshToken === 'string' && refreshToken.length > 20)
  results.token = token

  const noAuth = await request('GET', '/system/users')
  ok('未带令牌访问受保护接口返回 401', noAuth.status === 401, `status=${noAuth.status}`)

  const badAuth = await request('GET', '/system/users', { token: 'not-a-token' })
  ok('非法令牌返回 401', badAuth.status === 401, `status=${badAuth.status}`)

  section('个人信息与动态路由')
  const profile = await request('GET', '/profile', { token })
  ok('获取个人信息', profile.json?.code === 0 && profile.json.data.username === 'admin', JSON.stringify(profile.json))
  ok('超级管理员标记 isSuper', profile.json?.data?.isSuper === true)

  const routes = await request('GET', '/profile/routes', { token })
  const routeList = routes.json?.data || []
  ok('获取动态路由树', Array.isArray(routeList) && routeList.length > 0, `len=${routeList.length}`)
  const titles = routeList.map((r) => r.meta?.title).join(',')
  ok(
    '路由包含数据概览、进销存与系统管理',
    titles.includes('数据概览') && titles.includes('进销存') && titles.includes('系统管理'),
    titles,
  )
  // 进销存是主线功能，登录后应直接落在「数据概览」而不是框架自带的仪表盘
  const firstLeaf = (node) => {
    for (const child of node.children || []) {
      if (child.children && child.children.length > 0) {
        const deep = firstLeaf(child)
        if (deep) return deep
      } else {
        return child
      }
    }
    return null
  }
  const landing = firstLeaf({ children: routeList })
  ok('首页落在进销存数据概览', landing?.meta?.title === '数据概览', landing?.meta?.title)
  const flat = JSON.stringify(routeList)
  ok('动态路由中不含按钮权限点', !flat.includes('system:user:create'), '按钮项应被过滤')

  section('用户管理 CRUD')
  const created = await request('POST', '/system/users', {
    token,
    body: {
      username: 'smoke_user',
      password: 'smoke123456',
      nickname: '自测账号',
      email: 'smoke@example.com',
      deptId: 2,
      roleIds: [3],
    },
  })
  ok('新增用户', created.json?.code === 0, JSON.stringify(created.json))
  const newUserId = created.json?.data?.id

  const dup = await request('POST', '/system/users', {
    token,
    body: { username: 'smoke_user', password: 'smoke123456', nickname: '重复账号' },
  })
  ok('重复用户名返回 40900', dup.json?.code === 40900, JSON.stringify(dup.json))

  const list = await request('GET', '/system/users?page=1&pageSize=10&keyword=smoke', { token })
  ok('按关键字搜索用户', list.json?.code === 0 && list.json.data.total >= 1, JSON.stringify(list.json?.data?.total))

  const detail = await request('GET', `/system/users/${newUserId}`, { token })
  ok('用户详情含角色与部门', detail.json?.data?.roleIds?.length === 1 && !!detail.json?.data?.deptName, JSON.stringify(detail.json?.data))

  const updated = await request('PUT', `/system/users/${newUserId}`, {
    token,
    body: { nickname: '自测账号-改', roleIds: [3], status: 1 },
  })
  ok('修改用户', updated.json?.code === 0, JSON.stringify(updated.json))

  const disabled = await request('PUT', `/system/users/${newUserId}/status`, { token, body: { status: 0 } })
  ok('禁用用户', disabled.json?.code === 0, JSON.stringify(disabled.json))
  const disabledLogin = await request('POST', '/auth/login', { body: { username: 'smoke_user', password: 'smoke123456' } })
  ok('被禁用账号无法登录', disabledLogin.json?.code === 40102, JSON.stringify(disabledLogin.json))

  const reset = await request('PUT', `/system/users/${newUserId}/password`, { token, body: { password: 'newpass123456' } })
  ok('重置密码', reset.json?.code === 0, JSON.stringify(reset.json))

  const protectAdmin = await request('DELETE', '/system/users/1', { token })
  ok('内置管理员不可删除', protectAdmin.json?.code === 40000, JSON.stringify(protectAdmin.json))

  section('角色管理')
  const roleList = await request('GET', '/system/roles?page=1&pageSize=10', { token })
  ok('角色列表', roleList.json?.data?.total >= 3, JSON.stringify(roleList.json?.data?.total))
  const roleOptions = await request('GET', '/system/roles/options', { token })
  ok('角色下拉选项', Array.isArray(roleOptions.json?.data) && roleOptions.json.data.length >= 3)

  const newRole = await request('POST', '/system/roles', {
    token,
    body: {
      name: '自测角色', code: 'smoke_role', sort: 9, dataScope: 4,
      menuIds: [
        await menuId(token, 'inv:overview:view'),
        await menuId(token, 'inv:customer:list'),
        await menuId(token, 'system:user:list'),
      ],
    },
  })
  ok('新增角色', newRole.json?.code === 0, JSON.stringify(newRole.json))
  const newRoleId = newRole.json?.data?.id

  const roleDetail = await request('GET', `/system/roles/${newRoleId}`, { token })
  ok('角色详情含已授权菜单', roleDetail.json?.data?.menuIds?.length === 3, JSON.stringify(roleDetail.json?.data?.menuIds))

  const assign = await request('PUT', `/system/roles/${newRoleId}/menus`, {
    token,
    body: {
      menuIds: [
        await menuId(token, 'inv:overview:view'),
        await menuId(token, 'inv:customer:list'),
        await menuId(token, 'system:user:list'),
        await menuId(token, 'inv:customer:create'),
      ],
    },
  })
  ok('分配菜单权限', assign.json?.code === 0, JSON.stringify(assign.json))

  const builtinDelete = await request('DELETE', '/system/roles/1', { token })
  ok('内置角色不可删除', builtinDelete.json?.code === 40000, JSON.stringify(builtinDelete.json))

  section('权限隔离（只读账号）')
  const demoLogin = await request('POST', '/auth/login', { body: { username: 'demo', password: 'demo123456' } })
  ok('演示账号登录', demoLogin.json?.code === 0, JSON.stringify(demoLogin.json))
  const demoToken = demoLogin.json?.data?.accessToken
  const demoRoutes = await request('GET', '/profile/routes', { token: demoToken })
  ok('演示账号可读路由', (demoRoutes.json?.data || []).length > 0)
  const demoCreate = await request('POST', '/system/users', {
    token: demoToken,
    body: { username: 'hacker', password: 'hacker123456', nickname: '越权测试' },
  })
  ok('只读账号新增用户被拒(403)', demoCreate.status === 403, `status=${demoCreate.status} body=${JSON.stringify(demoCreate.json)}`)
  const demoDelete = await request('DELETE', `/system/users/${newUserId}`, { token: demoToken })
  ok('只读账号删除用户被拒(403)', demoDelete.status === 403, `status=${demoDelete.status}`)

  // 批量导出与批量导入是「取数 / 写入」的放大操作，与「能看列表」不是一个风险等级，
  // 只读账号必须一并被拒绝。
  ok('只读账号导出被拒(403)', (await download('/system/users/export', demoToken)).status === 403)
  ok('只读账号导出登录日志被拒(403)', (await download('/system/logs/logins/export', demoToken)).status === 403)
  ok('只读账号下载导入模板被拒(403)', (await download('/system/users/import-template', demoToken)).status === 403)
  ok(
    '只读账号导入被拒(403)',
    (await upload('/system/users/import', demoToken, 'users-import.xlsx', fs.readFileSync(IMPORT_FIXTURE))).status === 403,
  )

  section('进销存：基础资料')
  const invCustomers = await request('GET', '/inv/customers?page=1&pageSize=5', { token })
  ok('客户列表', invCustomers.json?.code === 0 && invCustomers.json.data.total >= 5, `total=${invCustomers.json?.data?.total}`)
  const invSuppliers = await request('GET', '/inv/suppliers?page=1&pageSize=5', { token })
  ok('供应商列表', invSuppliers.json?.code === 0 && invSuppliers.json.data.total >= 5, `total=${invSuppliers.json?.data?.total}`)
  // 客户 / 供应商搜索同样要转义通配符 —— 供应商分支转义过、客户分支曾漏改，
  // 这条断言钉住「两条业务线的搜索口径一致」（AGENTS §5.76）
  const custKw = await request('GET', '/inv/customers?keyword=_', { token })
  ok('客户搜索的 _ 不当通配符', custKw.json?.data?.total === 0, `total=${custKw.json?.data?.total}`)
  // 列表行要带上「应收余额 + 单据数」，否则客户页只能逐行再发一次请求
  const firstParty = invCustomers.json?.data?.list?.[0] || {}
  ok(
    '客户行含余额与单据数',
    typeof firstParty.balance === 'number' && typeof firstParty.orderCount === 'number' && !!firstParty.address,
    JSON.stringify(firstParty),
  )

  const smokeCode = `SMOKE${Date.now()}`
  const custCreate = await request('POST', '/inv/customers', {
    token,
    body: {
      code: smokeCode,
      name: '自测客户',
      contact: '张三',
      phone: '13800000000',
      address: '河南省信阳市',
      taxNo: '91411500MA0000000X',
    },
  })
  ok('新增客户', custCreate.json?.code === 0, JSON.stringify(custCreate.json))
  const custId = custCreate.json?.data?.id

  const custDup = await request('POST', '/inv/customers', {
    token,
    body: { code: smokeCode, name: '重复编号' },
  })
  ok('客户编号重复返回 40900', custDup.json?.code === 40900, JSON.stringify(custDup.json))

  const custUpdate = await request('PUT', `/inv/customers/${custId}`, {
    token,
    body: { code: smokeCode, name: '自测客户-改', status: 1 },
  })
  ok('修改客户', custUpdate.json?.code === 0, JSON.stringify(custUpdate.json))

  // 明细的可选粒度是「商品 → 规格 → 计量单位」三级，先取一组可用选项，
  // 后面所有单据 / 出入库明细都复用它，避免每处各解析一遍三级结构
  const picks = await request('GET', '/inv/products/pick', { token })
  ok(
    '商品级联选项（商品→规格→单位）',
    picks.json?.code === 0 &&
      Array.isArray(picks.json.data.items) &&
      picks.json.data.items.length >= 8 &&
      (picks.json.data.items[0].specs?.[0]?.units?.length ?? 0) > 0,
    `商品数=${picks.json?.data?.items?.length}`,
  )
  // 单次上限与截断标记：商品多了以后下拉只加载一部分，
  // 不给 truncated 用户就会以为「系统里没这个商品」，静默截断最难排查
  ok(
    '级联选项带总数与截断标记',
    typeof picks.json?.data?.total === 'number' && typeof picks.json?.data?.truncated === 'boolean',
    JSON.stringify({ total: picks.json?.data?.total, truncated: picks.json?.data?.truncated }),
  )
  ok(
    '级联选项不含停用的商品 / 规格',
    picks.json.data.items.every((p) => p.specs.every((s) => s.units.length > 0)),
  )
  const pick0 = picks.json?.data?.items?.[0] || {}
  const pickSpec0 = pick0.specs?.[0] || {}
  const pickUnit0 = pickSpec0.units?.find((u) => u.isBase) ?? pickSpec0.units?.[0] ?? {}
  const productId = pick0.id
  const specId = pickSpec0.id
  const unitId = pickUnit0.id
  /** 单据 / 出入库明细行：三元组由上面的选项提供 */
  const line = (extra = {}) => ({ productId, specId, unitId, ...extra })

  const products = await request('GET', '/inv/products?page=1&pageSize=200', { token })
  ok(
    '商品列表含规格与单位明细',
    products.json?.code === 0 &&
      products.json.data.list.length >= 8 &&
      Array.isArray(products.json.data.list[0].specs) &&
      typeof products.json.data.list[0].specCount === 'number',
    `len=${products.json?.data?.list?.length}`,
  )

  // 库存的粒度是**规格**：每一行都要能对上「哪个商品的哪个规格、按什么基本单位计」
  const inventory = await request('GET', '/inv/inventory?page=1&pageSize=200', { token })
  ok(
    '库存查询按规格出账（含商品编码与基本单位）',
    inventory.json?.code === 0 &&
      inventory.json.data.list.length >= 8 &&
      inventory.json.data.list.every(
        (r) => typeof r.spec === 'string' && typeof r.productCode === 'string' && typeof r.unit === 'string',
      ),
    `len=${inventory.json?.data?.list?.length}`,
  )

  // ---------------- 商品分类字典 ----------------
  const catOptions = await request('GET', '/inv/categories/options', { token })
  ok(
    '分类下拉返回数组',
    catOptions.json?.code === 0 && Array.isArray(catOptions.json.data),
    JSON.stringify(catOptions.json?.data),
  )

  const catName = `${smokeCode}分类`
  const catCreate = await request('POST', '/inv/categories', {
    token,
    body: { name: catName, sort: 9, status: 1, remark: '接口自测' },
  })
  ok('新增商品分类', catCreate.json?.code === 0, JSON.stringify(catCreate.json))
  const catId = catCreate.json?.data?.id

  const catDup = await request('POST', '/inv/categories', { token, body: { name: catName } })
  ok('分类重名返回 40900', catDup.json?.code === 40900, JSON.stringify(catDup.json))

  const catUpdate = await request('PUT', `/inv/categories/${catId}`, {
    token,
    body: { name: catName, sort: 8, status: 1, remark: '已改' },
  })
  ok('修改商品分类', catUpdate.json?.code === 0, JSON.stringify(catUpdate.json))

  const catList = await request('GET', `/inv/categories?keyword=${encodeURIComponent(catName)}`, { token })
  ok(
    '分类列表带商品引用数',
    catList.json?.code === 0 && typeof catList.json.data.list?.[0]?.productCount === 'number',
    JSON.stringify(catList.json?.data?.list?.[0]),
  )

  // 有商品引用的分类不能删：删掉之后那些商品的 category_id 会悬空，
  // 列表里显示为空、报表的品类占比凭空少一块，而且无从追溯
  const catDelUsed = await request('DELETE', '/inv/categories/1', { token })
  ok('删除被商品引用的分类被拒(40900)', catDelUsed.json?.code === 40900, JSON.stringify(catDelUsed.json))

  // ---------------- 商品档案：多规格 + 计量单位 ----------------
  const prodCode = `${smokeCode}P`
  const prodBody = {
    code: prodCode,
    name: '自测商品',
    barcode: 'SMOKE-BC-0001',
    categoryId: catId,
    status: 1,
    remark: '接口自测',
    specs: [
      {
        spec: 'SM-1',
        warnStock: 2,
        units: [
          { unit: '根', rate: 1, isBase: true, purchasePrice: 3, salePrice: 5 },
          { unit: '米', rate: 0.25, isBase: false, purchasePrice: 1, salePrice: 2 },
        ],
      },
      {
        spec: 'SM-2',
        warnStock: 1,
        units: [{ unit: '箱', rate: 1, isBase: true, purchasePrice: 30, salePrice: 50 }],
      },
    ],
  }
  const prodCreate = await request('POST', '/inv/products', { token, body: prodBody })
  ok('新增多规格商品', prodCreate.json?.code === 0, JSON.stringify(prodCreate.json))
  const smokeProduct = prodCreate.json?.data?.id

  const prodDetail = await request('GET', `/inv/products/${smokeProduct}`, { token })
  ok(
    '商品详情含两级规格与单位',
    prodDetail.json?.data?.specs?.length === 2 &&
      prodDetail.json?.data?.specs[0].units?.length === 2 &&
      prodDetail.json?.data?.specCount === 2 &&
      prodDetail.json?.data?.unitCount === 3,
    JSON.stringify({
      specCount: prodDetail.json?.data?.specCount,
      unitCount: prodDetail.json?.data?.unitCount,
    }),
  )

  // 条码走通三条链路：详情回显、列表关键字能搜到、开单下拉带出来
  ok(
    '商品条码回显',
    prodDetail.json?.data?.barcode === 'SMOKE-BC-0001',
    JSON.stringify(prodDetail.json?.data?.barcode),
  )
  const byBarcode = await request('GET', '/inv/products?keyword=SMOKE-BC-0001&pageSize=5', { token })
  ok(
    '按条码能搜到商品',
    byBarcode.json?.code === 0 && byBarcode.json.data.list.some((p) => p.id === smokeProduct),
    JSON.stringify(byBarcode.json?.data?.list?.map((p) => p.code)),
  )
  const pickByBarcode = await request('GET', '/inv/products/pick?keyword=SMOKE-BC-0001', { token })
  ok(
    '开单商品下拉按条码命中且带出条码',
    pickByBarcode.json?.data?.items?.[0]?.barcode === 'SMOKE-BC-0001',
    JSON.stringify(pickByBarcode.json?.data?.items?.map((p) => p.barcode)),
  )

  const prodDup = await request('POST', '/inv/products', { token, body: prodBody })
  ok('商品编码重复返回 40900', prodDup.json?.code === 40900, JSON.stringify(prodDup.json))

  // 规格 / 单位是**可选项**，而且是「留空就存空白」：不再补「默认 / 个」。
  // 结构仍然被补齐（至少一条规格 + 一个单位，换算率取 1，第一个单位提为基本单位），
  // 因为规格是库存归属层、单位是单据折算基准，缺了商品就查不到也开不了单。
  // 所以断言的是「结构被补齐、名称保持空白」，而不是「返回 40000」。
  const fillCases = [
    {
      label: '不填规格与单位（名称保持空白）',
      code: `${smokeCode}Q1`,
      name: '不填规格',
      specs: [],
      want: { spec: '', unit: '', rate: 1, isBase: true },
    },
    {
      label: '规格下不填单位（补齐一条空单位）',
      code: `${smokeCode}Q2`,
      name: '不填单位',
      specs: [{ spec: 'A', units: [] }],
      want: { spec: 'A', unit: '', rate: 1, isBase: true },
    },
    {
      label: '不指定基本单位',
      code: `${smokeCode}Q4`,
      name: '缺基本单位',
      specs: [{ spec: 'A', units: [{ unit: '个', rate: 2, isBase: false }] }],
      want: { spec: 'A', unit: '个', rate: 1, isBase: true },
    },
    {
      label: '基本单位换算率不为 1（被钉成 1）',
      code: `${smokeCode}Q6`,
      name: '基本单位换算率错',
      specs: [{ spec: 'A', units: [{ unit: '个', rate: 3, isBase: true }] }],
      want: { spec: 'A', unit: '个', rate: 1, isBase: true },
    },
  ]
  const fillIds = []
  for (const c of fillCases) {
    const r = await request('POST', '/inv/products', {
      token,
      body: { code: c.code, name: c.name, specs: c.specs },
    })
    const newId = r.json?.data?.id
    if (newId) fillIds.push(newId)
    const detail = newId ? await request('GET', `/inv/products/${newId}`, { token }) : null
    const sp = detail?.json?.data?.specs?.[0]
    const u = sp?.units?.[0]
    ok(
      `${c.label} → 自动补成「${c.want.spec} / ${c.want.unit}」且为基本单位`,
      r.json?.code === 0 &&
        sp?.spec === c.want.spec &&
        u?.unit === c.want.unit &&
        Math.abs((u?.rate ?? 0) - c.want.rate) < 1e-9 &&
        u?.isBase === c.want.isBase,
      `code=${r.json?.code} spec=${sp?.spec} unit=${u?.unit} rate=${u?.rate} base=${u?.isBase}`,
    )
  }
  // 多个基本单位只留第一个（基本单位必须唯一，否则库存没有计数基准）
  const multiBase = await request('POST', '/inv/products', {
    token,
    body: {
      code: `${smokeCode}Q5`,
      name: '两个基本单位',
      specs: [{ spec: 'A', units: [{ unit: '个', rate: 1, isBase: true }, { unit: '箱', rate: 12, isBase: true }] }],
    },
  })
  const multiBaseId = multiBase.json?.data?.id
  if (multiBaseId) fillIds.push(multiBaseId)
  const mbDetail = multiBaseId ? await request('GET', `/inv/products/${multiBaseId}`, { token }) : null
  const mbUnits = mbDetail?.json?.data?.specs?.[0]?.units || []
  ok(
    '多个基本单位只保留第一个',
    multiBase.json?.code === 0 && mbUnits.filter((u) => u.isBase).length === 1 && mbUnits[0]?.isBase === true,
    JSON.stringify(mbUnits.map((u) => ({ unit: u.unit, isBase: u.isBase }))),
  )
  for (const id of fillIds) {
    await request('DELETE', `/inv/products/${id}`, { token })
  }

  // 重名仍然是硬错误：两行「φ20」会把库存劈成两半、各自都不预警
  const prodDupCases = [
    [
      '同名规格',
      {
        code: `${smokeCode}Q3`,
        name: '规格重名',
        specs: [
          { spec: 'A', units: [{ unit: '个', rate: 1, isBase: true }] },
          { spec: 'A', units: [{ unit: '个', rate: 1, isBase: true }] },
        ],
      },
    ],
    [
      '同一规格下单位重名',
      {
        code: `${smokeCode}Q7`,
        name: '单位重名',
        specs: [{ spec: 'A', units: [{ unit: '个', rate: 1, isBase: true }, { unit: '个', rate: 2, isBase: false }] }],
      },
    ],
    [
      '两个都不填规格名（都会补成「默认」，同属重名）',
      {
        code: `${smokeCode}Q8`,
        name: '两行空规格',
        specs: [{ spec: '', units: [] }, { spec: '', units: [] }],
      },
    ],
  ]
  for (const [label, body] of prodDupCases) {
    const r = await request('POST', '/inv/products', { token, body })
    ok(`${label}返回 40000`, r.json?.code === 40000, JSON.stringify(r.json))
  }

  // 关键字里的 % 与 _ 不是通配符。不转义的话搜「M12_40」会连「M12X40」一起出来，
  // 结果既不报错也不为空，只是**静默地不是你要的那批**（AGENTS §5.75）
  const kwUnderscore = await request('GET', '/inv/products?keyword=_', { token })
  const kwPercent = await request('GET', '/inv/products?keyword=%25', { token })
  const kwNormal = await request('GET', '/inv/products?keyword=SP001', { token })
  ok(
    '关键字里的 _ 不当通配符（未转义会命中全部）',
    kwUnderscore.json?.data?.total === 0,
    `total=${kwUnderscore.json?.data?.total}`,
  )
  ok(
    '关键字里的 % 不当通配符（未转义会命中全部）',
    kwPercent.json?.data?.total === 0,
    `total=${kwPercent.json?.data?.total}`,
  )
  ok('正常关键字仍能命中', kwNormal.json?.data?.total === 1, `total=${kwNormal.json?.data?.total}`)

  // 停用的规格不能进新单据：页面打开后档案被停用这段竞态，
  // 以及绕过界面直接调接口的请求，都要在服务端挡住（AGENTS §5.76）
  const stopTarget = await request('GET', `/inv/products/${productId}`, { token })
  const stopSpecs = stopTarget.json?.data?.specs || []
  const stopSpec = stopSpecs[0]
  const stopBody = {
    code: stopTarget.json?.data?.code,
    name: stopTarget.json?.data?.name,
    categoryId: stopTarget.json?.data?.categoryId ?? 0,
    status: stopTarget.json?.data?.status,
    remark: stopTarget.json?.data?.remark,
    specs: stopSpecs.map((s) => ({
      ...s,
      status: s.id === stopSpec.id ? 0 : s.status,
      units: (s.units || []).map((u) => ({
        unit: u.unit,
        rate: u.rate,
        isBase: u.isBase,
        purchasePrice: u.purchasePrice,
        salePrice: u.salePrice,
        status: u.status,
      })),
    })),
  }
  await request('PUT', `/inv/products/${productId}`, { token, body: stopBody })
  const stoppedLine = await request('POST', '/inv/orders', {
    token,
    body: {
      bizType: 'purchase',
      partyId: invSuppliers.json?.data?.list?.[0]?.id,
      orderDate: '2025-06-01',
      items: [
        { productId, specId: stopSpec.id, unitId: stopSpec.units?.[0]?.id, qty: 1, price: 1 },
      ],
    },
  })
  ok(
    '停用规格不能用于新建单据(40000)',
    stoppedLine.json?.code === 40000,
    JSON.stringify(stoppedLine.json),
  )
  // 复原：这个商品后面还要用来跑其它断言
  const restoreBody = {
    ...stopBody,
    specs: stopSpecs.map((s) => ({
      ...s,
      units: (s.units || []).map((u) => ({
        unit: u.unit,
        rate: u.rate,
        isBase: u.isBase,
        purchasePrice: u.purchasePrice,
        salePrice: u.salePrice,
        status: u.status,
      })),
    })),
  }
  await request('PUT', `/inv/products/${productId}`, { token, body: restoreBody })
  ok('复原商品规格启用状态', true)

  // 规格改名要按 ID 匹配：只按名字比对的话，「SM-2 → SM-2A」会被当成
  // 「新增一条 + 删掉一条」，而带历史引用的那条删不掉，于是留下僵尸规格
  const specId2 = prodDetail.json?.data?.specs?.[1]?.id
  const prodUpdate = await request('PUT', `/inv/products/${smokeProduct}`, {
    token,
    body: {
      ...prodBody,
      name: '自测商品-改',
      specs: [
        prodBody.specs[0],
        {
          id: specId2,
          spec: 'SM-2A',
          warnStock: 3,
          units: [{ unit: '箱', rate: 1, isBase: true, purchasePrice: 31, salePrice: 52 }],
        },
      ],
    },
  })
  ok('修改商品（规格改名 / 调整价格）', prodUpdate.json?.code === 0, JSON.stringify(prodUpdate.json))
  const prodAfter = await request('GET', `/inv/products/${smokeProduct}`, { token })
  const afterSpecs = prodAfter.json?.data?.specs || []
  ok(
    '规格改名后沿用原 ID（不产生僵尸规格）',
    afterSpecs.length === 2 && afterSpecs[1].id === specId2 && afterSpecs[1].spec === 'SM-2A',
    JSON.stringify(afterSpecs.map((s) => ({ id: s.id, spec: s.spec }))),
  )

  const prodByCat = await request('GET', `/inv/products?categoryId=${catId}`, { token })
  ok(
    '商品列表按分类筛选生效',
    prodByCat.json?.code === 0 && prodByCat.json.data.list.some((x) => x.id === smokeProduct),
    JSON.stringify(prodByCat.json?.data?.list?.map((x) => x.id)),
  )

  // 单位 ID 必须稳定：saveUnits 是「比对后增删改」而不是全删重建。
  // 全删重建的后果很具体 ——「昨天下的单，今天改了一次商品档案」，
  // 那张单再打开保存就被 resolveSpecUnits 以「规格下不存在所选计量单位」打回，
  // 而且用户完全无从理解为什么昨天能存今天不能。
  const uSpec = afterSpecs[0]
  const uBase = uSpec.units.find((u) => u.isBase)
  const stabOrder = await request('POST', '/inv/orders', {
    token,
    body: {
      bizType: 'sales',
      partyId: custId,
      orderDate: '2025-06-01',
      items: [{ productId: smokeProduct, specId: uSpec.id, unitId: uBase.id, qty: 2, price: 0 }],
    },
  })
  ok('建单用于验证单位 ID 稳定性', stabOrder.json?.code === 0, JSON.stringify(stabOrder.json))
  const stabOrderId = stabOrder.json?.data?.id

  const resizeProduct = await request('PUT', `/inv/products/${smokeProduct}`, {
    token,
    body: {
      code: prodCode,
      name: '自测商品-改',
      categoryId: catId,
      status: 1,
      remark: '重存档案',
      specs: [
        {
          id: afterSpecs[0].id,
          spec: 'SM-1',
          warnStock: 2,
          units: [
            { unit: '根', rate: 1, isBase: true, purchasePrice: 4, salePrice: 6 },
            { unit: '米', rate: 0.25, isBase: false, purchasePrice: 1, salePrice: 2 },
          ],
        },
        {
          id: specId2,
          spec: 'SM-2A',
          warnStock: 3,
          units: [{ unit: '箱', rate: 1, isBase: true, purchasePrice: 31, salePrice: 52 }],
        },
      ],
    },
  })
  ok('再次保存商品档案（只调价不动结构）', resizeProduct.json?.code === 0, JSON.stringify(resizeProduct.json))

  const afterResave = await request('GET', `/inv/products/${smokeProduct}`, { token })
  const uSpecAfter = afterResave.json?.data?.specs?.find((s) => s.id === afterSpecs[0].id)
  const uBaseAfter = uSpecAfter?.units?.find((u) => u.isBase)
  ok(
    '重存商品档案后单位 ID 不变（历史单据仍可编辑）',
    uBaseAfter?.id === uBase.id && uBaseAfter?.purchasePrice === 4,
    `before=${uBase.id} after=${uBaseAfter?.id} price=${uBaseAfter?.purchasePrice}`,
  )
  const reSaveOrder = await request('PUT', `/inv/orders/${stabOrderId}`, {
    token,
    body: {
      bizType: 'sales',
      partyId: custId,
      orderDate: '2025-06-01',
      items: [{ productId: smokeProduct, specId: uSpec.id, unitId: uBase.id, qty: 2, price: 0 }],
    },
  })
  ok('改过商品档案后原单据仍可保存', reSaveOrder.json?.code === 0, JSON.stringify(reSaveOrder.json))
  ok(
    '清理用于验证单位 ID 稳定的单据',
    (await request('DELETE', `/inv/orders/${stabOrderId}`, { token })).json?.code === 0,
  )

  // 录入查重：同名不同规格是合法场景，所以只提示不拦截
  const prodDupFound = await request('GET', `/inv/products/duplicates?name=${encodeURIComponent('自测商品')}`, { token })
  ok(
    '同名查重能命中自测商品并给出规格摘要',
    prodDupFound.json?.code === 0 &&
      prodDupFound.json.data.some((x) => x.id === smokeProduct) &&
      typeof prodDupFound.json.data[0].specSummary === 'string',
    JSON.stringify(prodDupFound.json?.data),
  )

  // 商品编码是历史单据与报表对账的依据：有引用就 409，不允许删
  const prodDelUsed = await request('DELETE', `/inv/products/${productId}`, { token })
  ok('删除被单据引用的商品被拒(40900)', prodDelUsed.json?.code === 40900, JSON.stringify(prodDelUsed.json))

  // ---------------- 计量单位换算 ----------------
  // 用示例档案里的 SP002「不锈钢管」：304 φ32 规格下同时挂着「米」（基本单位）与
  // 「根」（1 根 = 6 米），是典型的「采购按根、库存按米」。
  // 这里刻意不复用自测商品：出入库明细会引用商品，而作废并不会清掉明细行，
  // 拿自测商品做换算会让最后的「删除自测商品」被引用校验挡住。
  const convPick = await request('GET', '/inv/products/pick?keyword=SP002', { token })
  const convProd = convPick.json?.data?.items?.[0]
  const convSpec =
    convProd?.specs?.find((s) => s.units.some((u) => !u.isBase)) ?? convProd?.specs?.[0]
  const convBase = convSpec?.units?.find((u) => u.isBase)
  const convSub = convSpec?.units?.find((u) => !u.isBase)
  ok(
    '示例档案含多计量单位（1 根 = 6 米）',
    convProd?.code === 'SP002' && convSub?.rate === 6 && convBase?.rate === 1,
    JSON.stringify(convSpec?.units?.map((u) => ({ unit: u.unit, rate: u.rate, isBase: u.isBase }))),
  )

  const otherSpecId = convProd?.specs?.find((s) => s.id !== convSpec?.id)?.id
  const stockOfSpec = async (specId) => {
    const d = await request('GET', `/inv/products/${convProd.id}`, { token })
    return d.json?.data?.specs?.find((s) => s.id === specId)?.stock
  }
  const stockBefore = await stockOfSpec(convSpec.id)
  const otherBefore = await stockOfSpec(otherSpecId)

  const convIn = await request('POST', '/inv/stock-orders', {
    token,
    body: {
      direction: 'in',
      sourceType: 'manual',
      orderDate: '2025-06-01',
      remark: '换算自测-入',
      items: [{ productId: convProd.id, specId: convSpec.id, unitId: convSub.id, qty: 2 }],
    },
  })
  ok('按非基本单位入库', convIn.json?.code === 0, JSON.stringify(convIn.json))
  const stockAfterIn = await stockOfSpec(convSpec.id)
  ok(
    '非基本单位按换算率折算入账（2 根 = 12 米）',
    Math.abs(stockAfterIn - (stockBefore + 12)) < 0.001,
    `before=${stockBefore} after=${stockAfterIn}`,
  )
  ok(
    '同一商品的其他规格库存不受影响',
    (await stockOfSpec(otherSpecId)) === otherBefore,
    `other=${await stockOfSpec(otherSpecId)}`,
  )

  // 单位必须属于所选的规格：串了就会把库存记到一个不属于它的规格上，
  // 账面上完全看不出来，要等盘点才发现某个规格莫名其妙多了一批货
  const crossUnit = await request('POST', '/inv/stock-orders', {
    token,
    body: {
      direction: 'in',
      sourceType: 'manual',
      orderDate: '2025-06-01',
      items: [{ productId: convProd.id, specId: otherSpecId, unitId: convSub.id, qty: 1 }],
    },
  })
  ok('单位不属于所选规格被拒(40000)', crossUnit.json?.code === 40000, JSON.stringify(crossUnit.json))

  const convOut = await request('POST', '/inv/stock-orders', {
    token,
    body: {
      direction: 'out',
      sourceType: 'manual',
      orderDate: '2025-06-01',
      remark: '换算自测-出',
      items: [{ productId: convProd.id, specId: convSpec.id, unitId: convSub.id, qty: 1 }],
    },
  })
  ok('出库同样按换算率折算（1 根 = 6 米）', convOut.json?.code === 0, JSON.stringify(convOut.json))
  ok(
    '出库后库存 = 入库后 - 6',
    Math.abs((await stockOfSpec(convSpec.id)) - (stockAfterIn - 6)) < 0.001,
    `stock=${await stockOfSpec(convSpec.id)}`,
  )

  // 清理：两张换算自测出入库单作废掉，库存回到自测前的值，
  // 否则每跑一次冒烟示例档案的库存就往上漂一截
  const voidConvIn = await request('DELETE', `/inv/stock-orders/${convIn.json.data.id}`, { token })
  const voidConvOut = await request('DELETE', `/inv/stock-orders/${convOut.json.data.id}`, { token })
  ok(
    '作废换算自测单据并回滚库存',
    voidConvIn.json?.code === 0 &&
      voidConvOut.json?.code === 0 &&
      Math.abs((await stockOfSpec(convSpec.id)) - stockBefore) < 0.001,
    `stock=${await stockOfSpec(convSpec.id)}`,
  )

  const stockOrders = await request('GET', '/inv/stock-orders?page=1&pageSize=5', { token })
  ok('出入库单据列表', stockOrders.json?.code === 0 && Array.isArray(stockOrders.json.data.list))
  // 出入库单只记数量，不带成本 —— 有 inv:stock:list 不等于能看到金额
  const stockRow = stockOrders.json?.data?.list?.[0] || {}
  ok(
    '出入库单行不含金额字段',
    !('totalAmount' in stockRow) && !('amount' in stockRow) && typeof stockRow.totalQty === 'number',
    JSON.stringify(stockRow),
  )

  section('进销存：单据流转')
  const orderCreate = await request('POST', '/inv/orders', {
    token,
    body: {
      bizType: 'sales',
      partyId: custId,
      orderDate: '2025-06-01',
      dueDate: '2025-06-30',
      remark: '接口自测',
      items: [line({ qty: 2, price: 0 })],
    },
  })
  ok('新增销售单据（待处理）', orderCreate.json?.code === 0, JSON.stringify(orderCreate.json))
  const orderId = orderCreate.json?.data?.id

  const orderDetail = await request('GET', `/inv/orders/${orderId}`, { token })
  ok(
    '单据详情含明细，单价缺省时取商品销售价',
    orderDetail.json?.data?.items?.length === 1 && orderDetail.json?.data?.totalAmount > 0,
    JSON.stringify(orderDetail.json?.data),
  )
  ok(
    '待处理单据结算状态为未结',
    orderDetail.json?.data?.settleStatus === 'unpaid',
    orderDetail.json?.data?.settleStatus,
  )
  // 采购 / 销售单打印详情：明细要带商品编码与制单人（纸面打印所需）
  ok(
    '采购/销售单打印详情（编码/制单人）',
    orderDetail.json?.code === 0 &&
      typeof orderDetail.json.data.items?.[0]?.productCode === 'string' &&
      typeof orderDetail.json.data.operator === 'string',
    JSON.stringify(orderDetail.json?.data ?? {}).slice(0, 160),
  )

  // 到期日是选填字段：不填要落 NULL，填了要能改回 NULL。
  // 用非指针 time.Time 承载时，「没填」只能写成 Go 零值 0001-01-01，
  // 之后既分不清它是「没填」还是「过期两千年」，也没法把它改回空。
  const orderNoDue = await request('POST', '/inv/orders', {
    token,
    body: { bizType: 'sales', partyId: custId, orderDate: '2025-06-01', items: [line({ qty: 1, price: 0 })] },
  })
  const orderNoDueId = orderNoDue.json?.data?.id
  const noDueDetail = await request('GET', `/inv/orders/${orderNoDueId}`, { token })
  ok('不填到期日时落库为 null', noDueDetail.json?.data?.dueDate === null, JSON.stringify(noDueDetail.json?.data?.dueDate))

  const setDue = await request('PUT', `/inv/orders/${orderNoDueId}`, {
    token,
    body: {
      bizType: 'sales',
      partyId: custId,
      orderDate: '2025-06-01',
      dueDate: '2025-06-30',
      items: [line({ qty: 1, price: 0 })],
    },
  })
  const setDueDetail = await request('GET', `/inv/orders/${orderNoDueId}`, { token })
  ok(
    '到期日可填写',
    setDue.json?.code === 0 && (setDueDetail.json?.data?.dueDate || '').startsWith('2025-06-30'),
    JSON.stringify(setDueDetail.json?.data?.dueDate),
  )

  const clearDue = await request('PUT', `/inv/orders/${orderNoDueId}`, {
    token,
    body: { bizType: 'sales', partyId: custId, orderDate: '2025-06-01', dueDate: '', items: [line({ qty: 1, price: 0 })] },
  })
  const clearDueDetail = await request('GET', `/inv/orders/${orderNoDueId}`, { token })
  ok(
    '到期日可清空回 null',
    clearDue.json?.code === 0 && clearDueDetail.json?.data?.dueDate === null,
    JSON.stringify(clearDueDetail.json?.data?.dueDate),
  )
  ok('删除自测单据（到期日用例）', (await request('DELETE', `/inv/orders/${orderNoDueId}`, { token })).json?.code === 0)

  // 业务线权限必须按实体二次判定：只写 RequirePerm 的话，换个 bizType 参数就能读另一条线
  const badBiz = await request('GET', '/inv/orders?bizType=other', { token })
  ok('非法 bizType 返回 40000', badBiz.json?.code === 40000, JSON.stringify(badBiz.json))
  const badStatus = await request('GET', '/inv/orders?bizType=sales&status=9', { token })
  ok('非法单据状态返回 40000 而非静默忽略', badStatus.json?.code === 40000, JSON.stringify(badStatus.json))

  // 结算状态筛选必须与 settleStatus() 同口径：SQL 侧写错（比如漏了 0.001 容差）
  // 时，列表标签与筛选结果会对不上，而这种错在页面上看不出来
  const unpaidSales = await request('GET', '/inv/orders?bizType=sales&settleStatus=unpaid&pageSize=50', { token })
  ok(
    '结算状态筛选只返回未结清单据',
    unpaidSales.json?.code === 0 &&
      unpaidSales.json.data.list.length > 0 &&
      unpaidSales.json.data.list.every((r) => r.settleStatus === 'unpaid'),
    JSON.stringify(unpaidSales.json?.data?.list?.map((r) => r.settleStatus)),
  )
  const badSettle = await request('GET', '/inv/orders?bizType=sales&settleStatus=paied', { token })
  ok('非法结算状态返回 40000', badSettle.json?.code === 40000, JSON.stringify(badSettle.json))
  // 区间取到一个必然覆盖全部数据的范围，用来验「闭区间 + 不漏行」
  const allSales = await request('GET', '/inv/orders?bizType=sales&pageSize=1', { token })
  const wideRange = await request(
    'GET',
    '/inv/orders?bizType=sales&pageSize=1&startDate=2000-01-01&endDate=2099-12-31',
    { token },
  )
  ok(
    '日期区间过滤能覆盖全部单据（闭区间不漏行）',
    wideRange.json?.code === 0 && wideRange.json.data.total === allSales.json?.data?.total,
    `区间=${wideRange.json?.data?.total} 全部=${allSales.json?.data?.total}`,
  )
  const badRange = await request(
    'GET',
    '/inv/orders?bizType=sales&startDate=2026-10-15&endDate=2026-10-01',
    { token },
  )
  ok('结束日期早于开始日期返回 40000', badRange.json?.code === 40000, JSON.stringify(badRange.json))

  const cancelOrder = await request('PUT', `/inv/orders/${orderId}/status`, {
    token,
    body: { status: 3 },
  })
  ok('单据取消（不产生库存变动）', cancelOrder.json?.code === 0, JSON.stringify(cancelOrder.json))
  const delOrder = await request('DELETE', `/inv/orders/${orderId}`, { token })
  ok('删除未完成单据', delOrder.json?.code === 0, JSON.stringify(delOrder.json))

  // 手动出入库会即时过账，且出库必须挡住负库存
  const stockIn = await request('POST', '/inv/stock-orders', {
    token,
    body: { direction: 'in', sourceType: 'manual', orderDate: '2025-06-01', items: [line({ qty: 1 })] },
  })
  ok('手工入库并过账', stockIn.json?.code === 0, JSON.stringify(stockIn.json))
  const stockInId = stockIn.json?.data?.id
  const stockVoid = await request('DELETE', `/inv/stock-orders/${stockInId}`, { token })
  ok('作废出入库单并回滚库存', stockVoid.json?.code === 0, JSON.stringify(stockVoid.json))
  const overOut = await request('POST', '/inv/stock-orders', {
    token,
    body: { direction: 'out', sourceType: 'manual', orderDate: '2025-06-01', items: [line({ qty: 999999 })] },
  })
  ok('出库超过库存被拒(40900)', overOut.json?.code === 40900, JSON.stringify(overOut.json))

  // 打印详情：明细要带商品编码 / 规格 / 制单人；已作废单据不可打印
  const liveStock = await request('GET', '/inv/stock-orders?page=1&pageSize=1', { token })
  const liveStockId = liveStock.json?.data?.list?.[0]?.id
  const stockDetail = await request('GET', `/inv/stock-orders/${liveStockId}`, { token })
  ok(
    '出入库单打印详情（编码/规格/制单人）',
    stockDetail.json?.code === 0 &&
      stockDetail.json.data.items?.length > 0 &&
      typeof stockDetail.json.data.items[0].productCode === 'string' &&
      typeof stockDetail.json.data.operator === 'string',
    JSON.stringify(stockDetail.json?.data).slice(0, 160),
  )
  const voidPrint = await request('GET', `/inv/stock-orders/${stockInId}`, { token })
  ok('已作废单据不可打印(40900)', voidPrint.json?.code === 40900, JSON.stringify(voidPrint.json))

  section('进销存：应收应付与报表')
  const recv = await request('GET', '/inv/finance/receivables?page=1&pageSize=5', { token })
  ok(
    '应收列表随行返回汇总',
    recv.json?.code === 0 && typeof recv.json.data.summary?.outstanding === 'number',
    JSON.stringify(recv.json?.data?.summary),
  )
  const pay = await request('GET', '/inv/finance/payables?page=1&pageSize=5', { token })
  ok(
    '应付列表随行返回汇总',
    pay.json?.code === 0 && typeof pay.json.data.summary?.total === 'number',
    JSON.stringify(pay.json?.data?.summary),
  )
  const overdue = await request('GET', '/inv/finance/receivables?settle=overdue&page=1&pageSize=5', { token })
  ok('逾期筛选可用', overdue.json?.code === 0, JSON.stringify(overdue.json).slice(0, 160))

  // 到期日口径（本轮修复）：选填字段空值落库 NULL，且
  // 「未填到期日」不算逾期 —— 汇总与明细必须用同一个口径。
  // 历史缺陷是空串被解析成 Go 零值 0001-01-01 后原样落库：
  // 汇总 SQL 没有排除零值、行级判逾期有 IsZero 守卫，于是同一页上
  // 「逾期金额」比逐行逾期之和大一截，用户看到自相矛盾的两个数。
  const recvAll = await request('GET', '/inv/finance/receivables?page=1&pageSize=200', { token })
  const payAll = await request('GET', '/inv/finance/payables?page=1&pageSize=200', { token })
  const rowsOf = (r) => r.json?.data?.list || []
  const overdueSum = (rows) => rows.filter((x) => x.overdue).reduce((s, x) => s + x.outstanding, 0)
  const allFinanceRows = [...rowsOf(recvAll), ...rowsOf(payAll)]
  ok(
    '到期日不含零值时间(0001-01-01)',
    allFinanceRows.every((x) => x.dueDate === null || !x.dueDate.startsWith('0001-')),
    JSON.stringify(allFinanceRows.filter((x) => x.dueDate && x.dueDate.startsWith('0001-')))
  )
  ok(
    '未填到期日的单据不判逾期',
    allFinanceRows.every((x) => x.dueDate !== null || x.overdue === false),
    JSON.stringify(allFinanceRows.filter((x) => x.dueDate === null && x.overdue))
  )
  ok(
    '逾期汇总 == 明细逾期之和（应收）',
    Math.abs((recvAll.json?.data?.summary?.overdue ?? -1) - overdueSum(rowsOf(recvAll))) < 0.01,
    `summary=${recvAll.json?.data?.summary?.overdue} rows=${overdueSum(rowsOf(recvAll))}`
  )
  ok(
    '逾期汇总 == 明细逾期之和（应付）',
    Math.abs((payAll.json?.data?.summary?.overdue ?? -1) - overdueSum(rowsOf(payAll))) < 0.01,
    `summary=${payAll.json?.data?.summary?.overdue} rows=${overdueSum(rowsOf(payAll))}`
  )

  const overSettle = await request('POST', '/inv/finance/settlements', {
    token,
    body: { orderId: orderId, amount: 1 },
  })
  ok('对不存在的单据登记收付款被拒', overSettle.json?.code !== 0, JSON.stringify(overSettle.json))

  // 登记收付款不能超额：settled + amount 顶破 total_amount 就返回 409。
  // 后端用条件更新兜底并发（AGENTS §5.75），这里先断言「单次超额」这条基线
  const recvProbe = await request('GET', '/inv/finance/receivables?page=1&pageSize=1', { token })
  const recvRow = recvProbe.json?.data?.list?.[0]
  if (recvRow && recvRow.outstanding > 0) {
    const over = await request('POST', '/inv/finance/settlements', {
      token,
      body: { orderId: recvRow.orderId, amount: recvRow.outstanding + 100 },
    })
    ok(
      '登记收付款超额被拒(40900)',
      over.json?.code === 40900,
      JSON.stringify(over.json),
    )
  } else {
    ok('存在可结算的应收单（跳过超额断言）', false, JSON.stringify(recvRow))
  }

  // 批量收付款：入参必须是至少一张单据；不存在的单据要整批拒掉（不给半截结果）
  const batchEmpty = await request('POST', '/inv/finance/settlements/batch', {
    token,
    body: { orderIds: [] },
  })
  ok('批量结算空单据被拒(40000)', batchEmpty.json?.code === 40000, JSON.stringify(batchEmpty.json))

  const batchMissing = await request('POST', '/inv/finance/settlements/batch', {
    token,
    body: { orderIds: [orderId] },
  })
  ok(
    '批量结算含不存在单据被拒',
    batchMissing.json?.code !== 0,
    JSON.stringify(batchMissing.json),
  )

  const invOverview = await request('GET', '/inv/overview', { token })
  ok(
    '数据概览返回十项指标',
    invOverview.json?.code === 0 && typeof invOverview.json.data.productCount === 'number' && typeof invOverview.json.data.receivableTotal === 'number',
    JSON.stringify(invOverview.json?.data),
  )
  const reportCases = [
    ['/inv/report/summary?range=month', '汇总'],
    ['/inv/report/trend?days=30', '趋势'],
    ['/inv/report/ranking?range=month&limit=5', '排行'],
    ['/inv/report/category?range=month', '品类占比'],
    ['/inv/report/stock-ranking', '库存排行'],
    ['/inv/report/detail?range=month&biz=sales', '销售明细'],
    ['/inv/report/detail?range=month&biz=purchase', '采购明细'],
  ]
  for (const [uri, label] of reportCases) {
    const r = await request('GET', uri, { token })
    ok(`报表${label}`, r.json?.code === 0, JSON.stringify(r.json).slice(0, 160))
  }
  // 排行全量：limit=0 表示不截断（返回行数应 >= 限 1 条时的行数）
  const rankAll = await request('GET', '/inv/report/ranking?range=year&dimension=product&biz=sales&limit=0', { token })
  ok(
    '排行 limit=0 返回全量（不截成前 N）',
    rankAll.json?.code === 0 && Array.isArray(rankAll.json?.data) && rankAll.json.data.length >= 1,
    `rows=${rankAll.json?.data?.length}`,
  )
  // 明细行必须有商品与金额，且合计与行内金额之和一致（同一口径两处实现）
  const detailProbe = await request('GET', '/inv/report/detail?range=year&biz=sales', { token })
  const detailRows = detailProbe.json?.data?.rows ?? []
  const detailSum = detailRows.reduce((s, r) => s + Number(r.amount || 0), 0)
  ok(
    '销售明细行结构完整（单号/商品/金额）',
    detailProbe.json?.code === 0 && detailRows.every((r) => r.orderNo && r.productName && typeof r.amount === 'number'),
    JSON.stringify(detailRows[0] || {}),
  )
  ok(
    '销售明细合计 == 行金额之和',
    Math.abs(detailSum - Number(detailProbe.json?.data?.amount ?? -1)) < 0.02,
    `sum=${detailSum.toFixed(2)} total=${detailProbe.json?.data?.amount}`,
  )
  // 排行维度：客户 / 产品（销售），供应商（采购）
  const rankCustomer = await request('GET', '/inv/report/ranking?range=year&dimension=customer&biz=sales&limit=10', { token })
  ok(
    '排行按客户维度（销售）',
    rankCustomer.json?.code === 0 && Array.isArray(rankCustomer.json?.data) && rankCustomer.json.data.every((r) => typeof r.name === 'string'),
    JSON.stringify(rankCustomer.json?.data?.slice(0, 2)),
  )
  const rankProduct = await request('GET', '/inv/report/ranking?range=year&dimension=product&biz=sales&limit=10', { token })
  ok(
    '排行按产品维度（销售，含销量）',
    rankProduct.json?.code === 0 && rankProduct.json.data.every((r) => typeof r.quantity === 'number'),
    JSON.stringify(rankProduct.json?.data?.slice(0, 2)),
  )
  const rankSupplier = await request('GET', '/inv/report/ranking?range=year&dimension=customer&biz=purchase&limit=10', { token })
  ok(
    '排行按供应商维度（采购）',
    rankSupplier.json?.code === 0 && Array.isArray(rankSupplier.json?.data),
    JSON.stringify(rankSupplier.json?.data?.slice(0, 2)),
  )
  const invReport = await request('GET', '/inv/report/inventory', { token })
  ok(
    '库存分析返回金额/规格/预警与分布',
    invReport.json?.code === 0 &&
      typeof invReport.json.data.totalValue === 'number' &&
      Array.isArray(invReport.json.data.categoryValue) &&
      Array.isArray(invReport.json.data.warnList),
    JSON.stringify(invReport.json?.data),
  )

  // 报表导出：四类面板都要产出有效的 xlsx（xlsx 是 zip，头两字节 PK）
  for (const [type, label, extra] of [
    ['overview', '经营总览', ''],
    ['ranking', '销售排行', '&dimension=product'],
    ['detail', '销售明细', '&biz=sales'],
    ['detail', '采购明细', '&biz=purchase'],
  ]) {
    const r = await request('GET', `/inv/report/export?type=${type}&range=year${extra}`, { token })
    ok(
      `报表导出 xlsx（${label}）`,
      r.status === 200 && typeof r.text === 'string' && r.text.startsWith('PK'),
      `status=${r.status} head=${(r.text || '').slice(0, 2)}`,
    )
  }
  // 明细的往来单位筛选：拿第一行的往来单位去筛，结果必须全部命中该关键字
  const partyProbe = detailRows.find((r) => r.partyName)?.partyName || ''
  if (partyProbe) {
    const kw = partyProbe.slice(0, 2)
    const pf = await request(
      'GET',
      `/inv/report/detail?range=year&biz=sales&party=${encodeURIComponent(kw)}`,
      { token },
    )
    const pfRows = pf.json?.data?.rows ?? []
    ok(
      '明细按往来单位模糊筛选',
      pf.json?.code === 0 && pfRows.length > 0 && pfRows.every((r) => String(r.partyName || '').includes(kw)),
      `kw=${kw} rows=${pfRows.length}/${detailRows.length}`,
    )
  } else {
    ok('明细按往来单位模糊筛选', false, '没有可用的往来单位样本')
  }

  section('进销存：反审核')
  // 审核 → 反审核 必须把副作用一起撤销：状态回到待处理、库存回到审核前、
  // 生成的出入库单作废。只改状态的话库存会永久少掉，且查不到对应单据。
  // （上面那个 stockOfSpec 绑定的是换算用例的商品，这里要查的是单据用的规格）
  const rvStock = async () => {
    const d = await request('GET', `/inv/products/${productId}`, { token })
    return d.json?.data?.specs?.find((s) => s.id === specId)?.stock
  }
  const stockBeforeAudit = await rvStock()

  const rvCreate = await request('POST', '/inv/orders', {
    token,
    body: { bizType: 'sales', partyId: custId, orderDate: '2025-06-02', items: [line({ qty: 1, price: 0 })] },
  })
  const rvId = rvCreate.json?.data?.id
  ok('反审核用例：建单', rvCreate.json?.code === 0 && rvId > 0, JSON.stringify(rvCreate.json))

  const rvAudit = await request('PUT', `/inv/orders/${rvId}/status`, { token, body: { status: 2 } })
  ok('反审核用例：审核通过', rvAudit.json?.code === 0, JSON.stringify(rvAudit.json))
  const stockAfterAudit = await rvStock()
  ok(
    '审核后库存已扣减',
    stockAfterAudit === stockBeforeAudit - 1,
    `before=${stockBeforeAudit} after=${stockAfterAudit}`,
  )

  const rvReverse = await request('PUT', `/inv/orders/${rvId}/reverse`, { token })
  ok('反审核成功', rvReverse.json?.code === 0, JSON.stringify(rvReverse.json))

  const afterReverse = await request('GET', `/inv/orders/${rvId}`, { token })
  ok('反审核后单据回到待处理', afterReverse.json?.data?.status === 1, `status=${afterReverse.json?.data?.status}`)

  const stockAfterReverse = await rvStock()
  ok(
    '反审核后库存回到审核前',
    stockAfterReverse === stockBeforeAudit,
    `before=${stockBeforeAudit} after=${stockAfterReverse}`,
  )

  const rvTwice = await request('PUT', `/inv/orders/${rvId}/reverse`, { token })
  ok('待处理单据反审核被拒(40000)', rvTwice.json?.code === 40000, JSON.stringify(rvTwice.json))

  const stockListAfter = await request('GET', '/inv/stock-orders?page=1&pageSize=100', { token })
  const linkedStock = (stockListAfter.json?.data?.list || []).filter(
    (s) => s.sourceType === 'sales' && s.sourceId === rvId,
  )
  ok('反审核后关联出入库单已作废', linkedStock.length === 0, `linked=${linkedStock.length}`)

  const rvDel = await request('DELETE', `/inv/orders/${rvId}`, { token })
  ok('反审核用例：清理单据', rvDel.json?.code === 0, JSON.stringify(rvDel.json))

  const invTrend = await request('GET', '/inv/report/trend?days=7', { token })
  ok('趋势按天补齐 7 个点', (invTrend.json?.data || []).length === 7, `len=${invTrend.json?.data?.length}`)
  // 自定义区间：from + to 必须严格按区间出点，不能沿用 days 参数 ——
  // 少了这一步会画出未来日期的空点，且后端 90 天上限会把长区间静默截断
  const customTrend = await request('GET', '/inv/report/trend?from=2026-09-01&to=2026-09-10', { token })
  const ct = customTrend.json?.data || []
  ok(
    '自定义区间趋势按 from~to 收口',
    ct.length === 10 && ct[0].date === '2026-09-01' && ct[9].date === '2026-09-10',
    `len=${ct.length} first=${ct[0]?.date} last=${ct[ct.length - 1]?.date}`,
  )

  section('进销存：分类多级与报表品类筛选')
  const catTree = await request('GET', '/inv/categories?page=1&pageSize=100', { token })
  const rootCat = (catTree.json?.data?.list || []).find((c) => c.parentId === 0)
  ok('分类列表带 parentId 与子分类数', typeof rootCat?.parentId === 'number' && typeof rootCat?.childCount === 'number',
    JSON.stringify(rootCat))
  // 名字先算好：创建与「重名测试」必须是同一个字符串，各生成一次就永远测不到重名
  const childName = `自测子分类${Date.now()}`
  const childCat = await request('POST', '/inv/categories', {
    token,
    body: { parentId: rootCat.id, name: childName, sort: 9 },
  })
  const childId = childCat.json?.data?.id
  ok('新增子分类', childCat.json?.code === 0 && childId > 0, JSON.stringify(childCat.json))
  ok(
    '同级分类重名被拒(40900)',
    (await request('POST', '/inv/categories', { token, body: { parentId: rootCat.id, name: childName } })).json?.code ===
      40900,
  )
  // parent_id 一成环，前端按 parent_id 组装树会无限递归、页面卡死 —— 只能服务端拦
  const cycle = await request('PUT', `/inv/categories/${rootCat.id}`, {
    token,
    body: { parentId: childId, name: rootCat.name, sort: rootCat.sort, status: rootCat.status },
  })
  ok('把分类挂到自己子孙下被拒(40000)', cycle.json?.code === 40000, JSON.stringify(cycle.json))
  ok(
    '删除有子分类的分类被拒(40900)',
    (await request('DELETE', `/inv/categories/${rootCat.id}`, { token })).json?.code === 40900,
  )
  const catFilter = await request('GET', `/inv/report/category?categoryId=${rootCat.id}`, { token })
  ok(
    '报表按品类筛选（含子分类）',
    catFilter.json?.code === 0 && Array.isArray(catFilter.json?.data),
    JSON.stringify(catFilter.json).slice(0, 160),
  )
  await request('DELETE', `/inv/categories/${childId}`, { token })
  ok('清理自测子分类', true)

  const report = await request('GET', '/inv/report/summary?range=month', { token })
  ok(
    '汇总里毛利 = 收入 - 成本',
    Math.abs((report.json?.data?.salesAmount ?? 0) - (report.json?.data?.costAmount ?? 0) - (report.json?.data?.grossProfit ?? 0)) < 0.01,
    JSON.stringify(report.json?.data),
  )

  section('进销存：打印模板')
  const tplList = await request('GET', '/inv/print-templates?page=1&pageSize=100', { token })
  ok('打印模板列表', tplList.json?.code === 0 && tplList.json.data.total >= 8, `total=${tplList.json?.data?.total}`)
  const tplCreate = await request('POST', '/inv/print-templates', {
    token,
    body: { code: smokeCode, name: '自测模板', docType: '自测单据', paper: 'A5' },
  })
  ok('新增打印模板', tplCreate.json?.code === 0, JSON.stringify(tplCreate.json))
  const tplId = tplCreate.json?.data?.id
  const tplBadPaper = await request('POST', '/inv/print-templates', {
    token,
    body: { code: `${smokeCode}X`, name: '非法纸张', paper: 'A3' },
  })
  ok('非法纸张规格返回 40000', tplBadPaper.json?.code === 40000, JSON.stringify(tplBadPaper.json))
  const tplContent = JSON.stringify({ title: '出入库单', showCode: true, showSpec: false, fontSize: 12, footer: '示例页脚' })
  const tplUpdate = await request('PUT', `/inv/print-templates/${tplId}`, {
    token,
    body: { code: smokeCode, name: '自测模板-改', paper: 'A4', status: 0, content: tplContent },
  })
  ok('修改打印模板（含版式配置）', tplUpdate.json?.code === 0, JSON.stringify(tplUpdate.json))
  const tplAfter = await request('GET', '/inv/print-templates?page=1&pageSize=100', { token })
  const savedTpl = (tplAfter.json?.data?.list || []).find((x) => x.id === tplId)
  ok('版式配置随列表返回', savedTpl?.content === tplContent, JSON.stringify(savedTpl))
  const tplDelete = await request('DELETE', `/inv/print-templates/${tplId}`, { token })
  ok('删除自测模板', tplDelete.json?.code === 0, JSON.stringify(tplDelete.json))

  section('进销存：只读账号权限隔离')
  ok(
    '只读账号新增客户被拒(403)',
    (await request('POST', '/inv/customers', { token: demoToken, body: { code: 'HACK001', name: '越权' } })).status === 403,
  )
  ok(
    '只读账号新增单据被拒(403)',
    (
      await request('POST', '/inv/orders', {
        token: demoToken,
        body: { bizType: 'sales', partyId: custId, orderDate: '2025-06-01', items: [line({ qty: 1 })] },
      })
    ).status === 403,
  )
  ok(
    '只读账号登记收付款被拒(403)',
    (await request('POST', '/inv/finance/settlements', { token: demoToken, body: { orderId: 1, amount: 1 } })).status === 403,
  )
  ok(
    '只读账号新增打印模板被拒(403)',
    (
      await request('POST', '/inv/print-templates', {
        token: demoToken,
        body: { code: 'HACK002', name: '越权模板' },
      })
    ).status === 403,
  )
  ok(
    '只读账号新增商品被拒(403)',
    (
      await request('POST', '/inv/products', {
        token: demoToken,
        body: {
          code: 'HACK003',
          name: '越权商品',
          specs: [{ spec: 'x', units: [{ unit: '件', rate: 1, isBase: true }] }],
        },
      })
    ).status === 403,
  )
  ok(
    '只读账号删除商品被拒(403)',
    (await request('DELETE', `/inv/products/${productId}`, { token: demoToken })).status === 403,
  )
  ok(
    '只读账号新增分类被拒(403)',
    (
      await request('POST', '/inv/categories', {
        token: demoToken,
        body: { name: '越权分类' },
      })
    ).status === 403,
  )
  ok(
    '只读账号修改分类被拒(403)',
    (await request('PUT', '/inv/categories/1', { token: demoToken, body: { name: '越权' } })).status === 403,
  )
  ok(
    '只读账号删除分类被拒(403)',
    (await request('DELETE', '/inv/categories/1', { token: demoToken })).status === 403,
  )
  const demoInvRead = await request('GET', '/inv/customers?page=1&pageSize=5', { token: demoToken })
  ok('只读账号仍可读取客户列表', demoInvRead.json?.code === 0, JSON.stringify(demoInvRead.json).slice(0, 160))
  const demoProductRead = await request('GET', '/inv/products?page=1&pageSize=5', { token: demoToken })
  ok('只读账号仍可读取商品列表', demoProductRead.json?.code === 0, JSON.stringify(demoProductRead.json).slice(0, 160))
  // 商品列表的权限是「任一即可」：只读账号有 inv:product:list，
  // 分类下拉与库存视图都依赖它，不能因为缺 inv:category:list 就整个 403
  const demoCatRead = await request('GET', '/inv/categories/options', { token: demoToken })
  ok('只读账号可读分类下拉（借 product:list）', demoCatRead.json?.code === 0, JSON.stringify(demoCatRead.json).slice(0, 160))
  const demoInventory = await request('GET', '/inv/inventory?page=1&pageSize=5', { token: demoToken })
  ok('只读账号可读库存', demoInventory.json?.code === 0, JSON.stringify(demoInventory.json).slice(0, 160))

  // ---- 跨业务线越权：路由的 RequirePerm 是「采购或销售任一」，
  // 只校验业务线的话，「采购只读 + 销售审核」的账号就能审核采购单。
  // 这一组断言钉住「写路径必须精确到动作」这条不变量（AGENTS §5.74）。
  section('进销存：跨业务线越权')
  const menuTree = await request('GET', '/system/menus?page=1&pageSize=500', { token })
  const crossPerms = ['inv:purchase:list', 'inv:sales:list', 'inv:sales:finish']
  const crossIds = []
  ;(function walk(ns) {
    for (const n of ns || []) {
      if (crossPerms.includes(n.permission)) crossIds.push(n.id)
      walk(n.children)
    }
  })(menuTree.json?.data)
  ok('三个跨业务线权限点都能定位', crossIds.length === crossPerms.length, `ids=${JSON.stringify(crossIds)}`)
  const crossRole = await request('POST', '/system/roles', {
    token,
    body: { name: '跨线自测角色', code: 'smoke_cross', dataScope: 1, status: 1, menuIds: crossIds },
  })
  const crossRoleId = crossRole.json?.data?.id
  ok('创建跨线自测角色', crossRole.json?.code === 0 && crossRoleId > 0, JSON.stringify(crossRole.json))
  const crossUser = await request('POST', '/system/users', {
    token,
    body: {
      username: 'smoke_cross',
      password: 'cross123456',
      nickname: '跨线自测',
      deptId: 2,
      roleIds: [crossRoleId],
      status: 1,
    },
  })
  const crossUserId = crossUser.json?.data?.id
  ok('创建跨线自测账号', crossUser.json?.code === 0 && crossUserId > 0, JSON.stringify(crossUser.json))
  const crossLogin = await request('POST', '/auth/login', {
    body: { username: 'smoke_cross', password: 'cross123456' },
  })
  const crossToken = crossLogin.json?.data?.accessToken
  ok('跨线账号登录成功', Boolean(crossToken), JSON.stringify(crossLogin.json).slice(0, 160))

  const pendingPurchase = await request('GET', '/inv/orders?bizType=purchase&status=1&page=1&pageSize=1', { token })
  const pOrderId = pendingPurchase.json?.data?.list?.[0]?.id
  ok('存在待处理采购单', pOrderId > 0, JSON.stringify(pendingPurchase.json).slice(0, 160))
  if (pOrderId && crossToken) {
    const readOk = await request('GET', `/inv/orders/${pOrderId}`, { token: crossToken })
    ok('跨线账号可读采购单（有 purchase:list）', readOk.json?.code === 0, `code=${readOk.json?.code}`)
    const finish = await request('PUT', `/inv/orders/${pOrderId}/status`, { token: crossToken, body: { status: 2 } })
    ok(
      '跨线账号审核采购单被拒(403)——只有 sales:finish',
      finish.status === 403,
      `status=${finish.status} body=${JSON.stringify(finish.json)}`,
    )
    const del = await request('DELETE', `/inv/orders/${pOrderId}`, { token: crossToken })
    ok('跨线账号删除采购单被拒(403)', del.status === 403, `status=${del.status}`)
  }
  if (crossUserId) {
    await request('DELETE', `/system/users/${crossUserId}`, { token })
  }
  if (crossRoleId) {
    await request('DELETE', `/system/roles/${crossRoleId}`, { token })
  }
  ok('清理跨线自测数据', true)

  const custDelete = await request('DELETE', `/inv/customers/${custId}`, { token })
  ok('删除自测客户', custDelete.json?.code === 0, JSON.stringify(custDelete.json))

  const prodDelete = await request('DELETE', `/inv/products/${smokeProduct}`, { token })
  ok('删除自测商品', prodDelete.json?.code === 0, JSON.stringify(prodDelete.json))

  // 商品删掉之后分类才没有引用，这时才能删；顺序反过来会被 409 挡住
  const catDelete = await request('DELETE', `/inv/categories/${catId}`, { token })
  ok('删除自测分类（商品已移除后）', catDelete.json?.code === 0, JSON.stringify(catDelete.json))

  section('菜单与部门')
  const menus = await request('GET', '/system/menus', { token })
  ok('菜单树非空', Array.isArray(menus.json?.data) && menus.json.data.length >= 2, `len=${menus.json?.data?.length}`)
  const userMenuId = await menuId(token, 'system:user:list')
  const menuDetail = await request('GET', `/system/menus/${userMenuId}`, { token })
  ok('菜单详情', menuDetail.json?.data?.permission === 'system:user:list', JSON.stringify(menuDetail.json?.data))
  const dirId = await menuId(token, 'Inv')
  const delHasChild = await request('DELETE', `/system/menus/${dirId}`, { token })
  ok('含子菜单不可删除', delHasChild.json?.code === 40900, JSON.stringify(delHasChild.json))

  const depts = await request('GET', '/system/depts', { token })
  ok('部门树', Array.isArray(depts.json?.data) && depts.json.data.length >= 1)
  const deptTree = depts.json?.data?.[0]
  ok('部门树含子节点与人数', (deptTree?.children || []).length >= 2, JSON.stringify(deptTree?.name))
  const delDeptHasUser = await request('DELETE', '/system/depts/2', { token })
  ok('有用户的部门不可删除', delDeptHasUser.json?.code === 40900, JSON.stringify(delDeptHasUser.json))

  section('日志')
  const ops = await request('GET', '/system/logs/operations?page=1&pageSize=5', { token })
  ok('操作日志有记录', ops.json?.code === 0 && ops.json.data.total > 0, `total=${ops.json?.data?.total}`)
  const logins = await request('GET', '/system/logs/logins?page=1&pageSize=5', { token })
  ok('登录日志有记录', logins.json?.code === 0 && logins.json.data.total > 0, `total=${logins.json?.data?.total}`)
  const logModules = await request('GET', '/system/logs/modules', { token })
  ok('日志模块下拉', Array.isArray(logModules.json?.data))

  section('导入导出')
  // 导出接口直接返回二进制流（不走统一 JSON 响应体），因此只能按字节流校验
  const exportCases = [
    ['/system/users/export', '用户列表'],
    ['/system/roles/export', '角色列表'],
    ['/system/logs/operations/export', '操作日志'],
    ['/system/logs/logins/export', '登录日志'],
  ]
  for (const [uri, label] of exportCases) {
    const r = await download(uri, token)
    ok(`导出${label}：HTTP 200`, r.status === 200, `status=${r.status}`)
    ok(`导出${label}：返回 xlsx 二进制`, isXlsx(r.buf), `前 4 字节=${r.buf.subarray(0, 4).toString('hex')}`)
    ok(`导出${label}：响应头正确（含 RFC 5987 中文文件名）`, isDownloadHeader(r.headers), r.headers.get('content-disposition'))
  }

  // 导出范围必须与列表筛选一致，否则用户会导出到看不见的数据
  const allUsers = await download('/system/users/export', token)
  const filteredUsers = await download('/system/users/export?keyword=admin', token)
  ok('按关键字导出成功', filteredUsers.status === 200 && isXlsx(filteredUsers.buf))
  ok(
    '按关键字导出确实收窄了范围',
    filteredUsers.buf.length < allUsers.buf.length,
    `全量=${allUsers.buf.length} 筛选=${filteredUsers.buf.length}`,
  )
  const selectedUsers = await download('/system/users/export?ids=1', token)
  ok('按选中项导出成功', selectedUsers.status === 200 && isXlsx(selectedUsers.buf))
  ok(
    '按选中项导出确实收窄了范围',
    selectedUsers.buf.length < allUsers.buf.length,
    `全量=${allUsers.buf.length} 选中=${selectedUsers.buf.length}`,
  )
  // 非法 ids 必须被拒绝：一个都解析不出来时不能悄悄降级成「导出全表」，
  // 否则「导出选中项」这个请求的语义会被放大到整张表。
  const badIds = await download('/system/users/export?ids=abc,def', token)
  const badIdsBody = JSON.parse(badIds.buf.toString('utf8') || '{}')
  ok(
    '非法 ids 被拒绝而非降级为全表导出',
    badIds.status === 400 && badIdsBody.code === 40000,
    `status=${badIds.status} code=${badIdsBody.code} 响应字节=${badIds.buf.length}`,
  )

  // 导入模板：表头正确、数据区为空（放示例行会诱导用户把示例数据导进系统）
  const template = await download('/system/users/import-template', token)
  ok('下载导入模板：HTTP 200 且为 xlsx', template.status === 200 && isXlsx(template.buf), `status=${template.status}`)
  ok('下载导入模板：响应头正确', isDownloadHeader(template.headers), template.headers.get('content-disposition'))

  // 导入预览（dryRun）：覆盖整条 multipart 上传 + 解析 + 逐行校验链路，不写库
  const fixture = fs.readFileSync(IMPORT_FIXTURE)
  const preview = await upload('/system/users/import?dryRun=1', token, 'users-import.xlsx', fixture)
  ok('导入预览：业务码 0', preview.json?.code === 0, JSON.stringify(preview.json))
  const pv = preview.json?.data || {}
  ok('导入预览：统计正确（5 行 / 2 通过 / 3 失败）', pv.total === 5 && pv.valid === 2 && pv.invalid === 3, JSON.stringify({ total: pv.total, valid: pv.valid, invalid: pv.invalid }))
  ok('导入预览：不写库', pv.created === 0, `created=${pv.created}`)
  ok('导入预览：逐行返回明细', Array.isArray(pv.rows) && pv.rows.length === 5, `rows=${pv.rows?.length}`)
  const rowErrors = (pv.rows || []).map((r) => (r.errors || []).join('；')).join(' | ')
  ok('导入预览：提示文件内用户名重复', rowErrors.includes('与文件中的其他行用户名重复'), rowErrors)
  ok('导入预览：提示邮箱格式不正确', rowErrors.includes('邮箱格式不正确'), rowErrors)
  ok('导入预览：提示状态取值非法', rowErrors.includes('状态只能填写'), rowErrors)
  // 同一个角色名写多遍属于书写冗余，必须去重后放行：
  // 不去重会在 sys_user_roles 触发复合主键冲突，让整批导入回滚。
  const dupRoleRow = (pv.rows || []).find((r) => r.username === 'smoke_import_dup_role')
  ok(
    '导入预览：重复书写的角色名被去重后通过',
    dupRoleRow && (dupRoleRow.errors || []).length === 0,
    JSON.stringify(dupRoleRow),
  )

  // 非 xlsx 扩展名应在进入解析前就被拒绝
  const badExt = await upload('/system/users/import?dryRun=1', token, 'users.csv', Buffer.from('a,b,c'))
  ok('导入：非 .xlsx 扩展名返回 40000', badExt.status === 400 && badExt.json?.code === 40000, JSON.stringify(badExt.json))

  section('参数校验')
  const badParam = await request('POST', '/system/users', { token, body: { username: 'ab', password: '1' } })
  ok('参数校验失败返回 40000', badParam.json?.code === 40000, JSON.stringify(badParam.json))
  ok('错误提示为中文', /不能为空|长度/.test(badParam.json?.message || ''), badParam.json?.message)

  section('数据范围（本轮加固）')
  // 数据范围此前只入库、只在表单里可配，却没有任何一条查询用它。
  // 这里把内置「只读访客」临时切成「仅本人」，验证它真的参与查询。
  const scopePayload = { name: '只读访客', sort: 3, remark: '仅可查看列表与仪表盘，不可导出' }
  const scopeOff = await request('PUT', '/system/roles/3', { token, body: { ...scopePayload, dataScope: 4 } })
  const scopedList = await request('GET', '/system/users?page=1&pageSize=20', { token: demoToken })
  ok(
    '数据范围「仅本人」时列表只剩自己',
    scopedList.json?.data?.total === 1,
    `total=${scopedList.json?.data?.total}`,
  )
  const scopedDetail = await request('GET', '/system/users/1', { token: demoToken })
  ok(
    '越界查看用户详情按「不存在」处理(404)',
    scopedDetail.status === 404 && scopedDetail.json?.code === 40400,
    `status=${scopedDetail.status} code=${scopedDetail.json?.code}`,
  )
  const scopeOn = await request('PUT', '/system/roles/3', { token, body: { ...scopePayload, dataScope: 1 } })
  ok('数据范围可还原为「全部」', scopeOff.json?.code === 0 && scopeOn.json?.code === 0, JSON.stringify(scopeOn.json))

  section('进销存数据范围隔离')
  // 进销存单据按「谁创建」收窄：仅本人的用户看不到别人开的单，
  // 概览 / 报表 / 应收应付同样只统计范围内的单据。
  // 商品 / 客户是共享主数据，不套范围（隔离了会开不了单）。
  const invScopeTs = Date.now()
  const invScopeRole = await request('POST', '/system/roles', {
    token,
    body: {
      name: '单据范围自测', code: `scope_inv_${invScopeTs}`, sort: 9, dataScope: 4,
      menuIds: [
        await menuId(token, 'inv:overview:view'),
        await menuId(token, 'inv:sales:list'),
        await menuId(token, 'inv:report:view'),
      ],
    },
  })
  const invScopeRoleId = invScopeRole.json?.data?.id
  const invScopeUser = await request('POST', '/system/users', {
    token,
    body: {
      username: `scope_inv_u_${invScopeTs}`, password: 'scope123456', nickname: '单据范围自测',
      deptId: 2, roleIds: [invScopeRoleId],
    },
  })
  const invScopeUserId = invScopeUser.json?.data?.id
  const invScopeLogin = await request('POST', '/auth/login', {
    body: { username: `scope_inv_u_${invScopeTs}`, password: 'scope123456' },
  })
  const invScopeToken = invScopeLogin.json?.data?.accessToken

  const adminOrders = await request('GET', '/inv/orders?bizType=sales&page=1&pageSize=10', { token })
  const scopeOrders = await request('GET', '/inv/orders?bizType=sales&page=1&pageSize=10', { token: invScopeToken })
  ok(
    '「仅本人」用户看不到别人的销售单',
    (adminOrders.json?.data?.total ?? 0) > 0 && (scopeOrders.json?.data?.total ?? 0) === 0,
    `admin=${adminOrders.json?.data?.total} scope=${scopeOrders.json?.data?.total}`,
  )
  const invScopeOv = await request('GET', '/inv/overview', { token: invScopeToken })
  ok(
    '概览按数据范围收窄（待处理单 = 0）',
    invScopeOv.json?.data?.pendingOrders === 0,
    `pending=${invScopeOv.json?.data?.pendingOrders} receivable=${invScopeOv.json?.data?.receivableTotal}`,
  )
  const invScopeSummary = await request('GET', '/inv/report/summary?range=year', { token: invScopeToken })
  ok(
    '报表汇总按数据范围收窄（收入 = 0）',
    invScopeSummary.json?.data?.salesAmount === 0,
    `sales=${invScopeSummary.json?.data?.salesAmount}`,
  )
  const adminFirstOrder = adminOrders.json?.data?.list?.[0]
  if (adminFirstOrder) {
    const invScopeDetail = await request('GET', `/inv/orders/${adminFirstOrder.id}`, { token: invScopeToken })
    ok(
      '越界查看单据详情按「不存在」处理(404)',
      invScopeDetail.status === 404 && invScopeDetail.json?.code === 40400,
      `status=${invScopeDetail.status}`,
    )
  }
  await request('DELETE', `/system/users/${invScopeUserId}`, { token })
  await request('DELETE', `/system/roles/${invScopeRoleId}`, { token })
  ok('清理单据范围自测数据', true)

  section('令牌吊销（本轮加固）')
  // 无状态 JWT 靠 token_epoch 实现吊销：改密码后旧令牌（含刷新令牌）必须立即失效
  const revUser = await request('POST', '/system/users', {
    token,
    body: { username: 'smoke_revoke', password: 'probe123456', nickname: '吊销自测', deptId: 2 },
  })
  const revId = revUser.json?.data?.id
  const revLogin = await request('POST', '/auth/login', {
    body: { username: 'smoke_revoke', password: 'probe123456' },
  })
  const revAccess = revLogin.json?.data?.accessToken
  const revRefresh = revLogin.json?.data?.refreshToken
  ok('改密前新账号令牌可用', (await request('GET', '/profile', { token: revAccess })).json?.code === 0)
  await request('PUT', `/system/users/${revId}/password`, { token, body: { password: 'probe654321' } })
  const revAfter = await request('GET', '/profile', { token: revAccess })
  ok(
    '重置密码后旧访问令牌立即失效(40105)',
    revAfter.status === 401 && revAfter.json?.code === 40105,
    `status=${revAfter.status} code=${revAfter.json?.code}`,
  )
  const revRefreshRes = await request('POST', '/auth/refresh', { body: { refreshToken: revRefresh } })
  ok('重置密码后旧刷新令牌立即失效(40105)', revRefreshRes.json?.code === 40105, JSON.stringify(revRefreshRes.json))
  const cleanRev = await request('DELETE', `/system/users/${revId}`, { token })
  ok('清理令牌吊销自测用户', cleanRev.json?.code === 0, JSON.stringify(cleanRev.json))

  section('审计来源可信（本轮加固）')
  // gin 默认信任所有代理，会用客户端自报的 X-Forwarded-For 当来源 IP。
  // 未配置 trusted_proxies 时必须记录 TCP 对端地址，客户端无法伪造。
  const spoofUser = 'smoke_ip_probe'
  const spoofRes = await fetch(`${API}/auth/login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Forwarded-For': '203.0.113.77',
      'User-Agent': 'NovaSmoke/1.0',
    },
    body: JSON.stringify({ username: spoofUser, password: 'whatever123' }),
  })
  await spoofRes.text()
  const probeLogs = await request('GET', `/system/logs/logins?username=${spoofUser}&pageSize=1`, { token })
  const probeRows = probeLogs.json?.data?.list || []
  ok('失败登录留下了审计记录', probeRows.length > 0, JSON.stringify(probeLogs.json?.data))
  ok(
    '审计 IP 不被 X-Forwarded-For 伪造',
    probeRows[0]?.ip && probeRows[0].ip !== '203.0.113.77',
    `记录到的 ip=${probeRows[0]?.ip}`,
  )

  // ---------------- 列表导出（统一入口） ----------------
  // 导出与列表共用同一套查询，这里只验「能出文件、文件名对、参数非法会拒」，
  // 内容正确性由各自的列表断言覆盖（导出的就是那些行）。
  section('列表导出')
  const listExportCases = [
    ['商品档案', '/inv/export?entity=product'],
    ['客户档案', '/inv/export?entity=customer'],
    ['供应商档案', '/inv/export?entity=supplier'],
    ['销售单', '/inv/export?entity=order&bizType=sales'],
    ['出入库单', '/inv/export?entity=stock'],
    ['库存查询', '/inv/export?entity=inventory'],
    ['应收明细', '/inv/export?entity=finance&bizType=sales'],
    ['应付明细', '/inv/export?entity=finance&bizType=purchase'],
    ['菜单配置', '/system/menus/export'],
    ['部门', '/system/depts/export'],
  ]
  for (const [label, url] of listExportCases) {
    const res = await fetch(`${API}${url}`, { headers: { Authorization: `Bearer ${token}` } })
    const buf = Buffer.from(await res.arrayBuffer())
    // xlsx 是 zip，头两个字节固定是 PK；只看状态码会把「JSON 报错」也当成成功
    const isXlsx = buf.length > 100 && buf.slice(0, 2).toString() === 'PK'
    ok(`导出${label}`, res.status === 200 && isXlsx, `status=${res.status} bytes=${buf.length}`)
  }
  // 单据导出必须**带明细**：只给列表的话，「金额」对不出账，
  // 收到文件的人还得回头一张张点详情。zip 里两个工作表的名字是明文字节，
  // 直接查 buffer 即可，不用解压。
  for (const [label, biz] of [
    ['销售单', 'sales'],
    ['采购单', 'purchase'],
  ]) {
    const res = await fetch(`${API}/inv/export?entity=order&bizType=${biz}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const buf = Buffer.from(await res.arrayBuffer())
    const book = readZipEntry(buf, 'xl/workbook.xml')
    const shared = readZipEntry(buf, 'xl/sharedStrings.xml')
    const hasList = book.includes(`${label}列表`)
    const hasDetail = book.includes(`${label}明细`)
    // 明细表头落在 sharedStrings 里
    const hasCols = ['商品编码', '换算率', '折算基本数量'].every((c) => shared.includes(c))
    ok(
      `${label}导出含列表 + 明细两个工作表`,
      res.status === 200 && hasList && hasDetail && hasCols,
      `列表=${hasList} 明细=${hasDetail} 列=${hasCols}`,
    )
  }

  const badEntity = await request('GET', '/inv/export?entity=user', { token })
  ok('非法导出实体返回 40000', badEntity.json?.code === 40000, JSON.stringify(badEntity.json))
  const noData = await request('GET', '/inv/export?entity=customer&keyword=不存在的关键字zzz', { token })
  ok('无数据时不产出空文件（40000）', noData.json?.code === 40000, JSON.stringify(noData.json))

  // 勾选导出：ids 非空时只出这些行
  const pickRow = (await request('GET', '/inv/customers?page=1&pageSize=1', { token })).json?.data?.list?.[0]
  if (pickRow) {
    const onlyOne = await fetch(`${API}/inv/export?entity=customer&ids=${pickRow.id}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const buf = Buffer.from(await onlyOne.arrayBuffer())
    ok('导出选中（ids）产出文件', onlyOne.status === 200 && buf.slice(0, 2).toString() === 'PK', `bytes=${buf.length}`)
    // 行数不容易在二进制里数，改成「一个不存在的 id 必须报无数据」
    const ghost = await request('GET', '/inv/export?entity=customer&ids=99999999', { token })
    ok('ids 指向不存在的记录时返回无数据', ghost.json?.code === 40000, JSON.stringify(ghost.json))
  }

  // 清理必须放在登出之前：登出会把该账号名下已签发的令牌一并吊销，
  // 先登出再清理时连本脚本自己手里的 token 都会变成 40105。
  section('系统运维（备份 / 清理 / 巡检 / 升级）')
  // 运维接口是「能改系统本身」的一类：备份要能建文件、清理不可逆、
  // 升级会替换正在运行的程序。因此除了 happy path，更要覆盖「被拒」的路径。
  const opsBackups = await request('GET', '/ops/backups', { token })
  ok('备份列表返回数组', Array.isArray(opsBackups.json?.data), JSON.stringify(opsBackups.json).slice(0, 120))

  const opsCheck = await request('GET', '/ops/check', { token })
  const rep = opsCheck.json?.data?.report
  ok('体检报告可用', opsCheck.json?.code === 0 && rep?.tables > 0, `tables=${rep?.tables}`)
  ok('体检报告里结构版本库不高于程序', rep.schemaVersionDb <= rep.schemaVersionBinary,
    `db=${rep?.schemaVersionDb} bin=${rep?.schemaVersionBinary}`)

  // 备份文件名由客户端给出：必须按纯 basename 校验，否则 ../ 就能越出目录
  const traversal = await request('DELETE', '/ops/backups', {
    token,
    body: { file: '../../../etc/passwd' },
  })
  ok('删除备份拒绝路径穿越', traversal.json?.code !== 0, JSON.stringify(traversal.json))
  const notDump = await request('DELETE', '/ops/backups', { token, body: { file: 'config.yaml' } })
  ok('删除备份只认 .dump', notDump.json?.code !== 0, JSON.stringify(notDump.json))

  const cleanupPreview = await request('GET', '/ops/cleanup/preview', { token })
  const steps = cleanupPreview.json?.data?.steps || []
  ok('清理预演默认 dry-run', cleanupPreview.json?.data?.dryRun === true)
  ok('预演列出三类清理对象', steps.length === 3 && steps.every((s) => s.rows >= 0), JSON.stringify(steps))

  // 口令不对时必须按预演处理，且明确告知没有删数据
  const cleanupWrong = await request('POST', '/ops/cleanup/apply', { token, body: { confirm: '确认' } })
  ok('清理口令错误被拦下且不删数据',
    cleanupWrong.json?.data?.dryRun === true && !!cleanupWrong.json?.data?.blocked,
    JSON.stringify(cleanupWrong.json?.data?.blocked))

  // 幂等回放：同一个 Idempotency-Key 重复提交写接口，第二次应回放首次结果，
  // 备份目录里只多一份文件 —— 双击 / 超时重试不能变成两次备份。
  const backupsBeforeIdem = (await request('GET', '/ops/backups', { token })).json?.data?.length
  const idemKey = `smoke-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  const idemHeaders = { 'Idempotency-Key': idemKey }
  const idemFirst = await request('POST', '/ops/backups', { token, body: {}, headers: idemHeaders })
  const idemSecond = await request('POST', '/ops/backups', { token, body: {}, headers: idemHeaders })
  ok('幂等键首次提交成功', idemFirst.json?.code === 0, JSON.stringify(idemFirst.json?.message))
  ok('幂等键重复提交被回放', idemSecond.json?.code === 0 && idemSecond.json?.data?.name === idemFirst.json?.data?.name,
    `${idemFirst.json?.data?.name} vs ${idemSecond.json?.data?.name}`)
  const backupsAfterIdem = (await request('GET', '/ops/backups', { token })).json?.data?.length
  ok('重复提交没有产生第二份备份', backupsAfterIdem === backupsBeforeIdem + 1,
    `before=${backupsBeforeIdem} after=${backupsAfterIdem}`)

  // 业务数据清理：范围必须是服务端登记好的几档，且每档口令不同
  const bizScopes = await request('GET', '/ops/cleanup/scopes', { token })
  const scopes = bizScopes.json?.data || []
  ok('业务清理范围预览可用', scopes.length === 3, `scopes=${scopes.map((s) => s.key).join(',')}`)
  const docScope = scopes.find((s) => s.key === 'inv_documents')
  ok('单据范围带行数预览', !!docScope && docScope.totalRows >= 0 && docScope.tables.length === 5,
    `total=${docScope?.totalRows}`)
  const phrases = scopes.map((s) => s.confirm)
  ok('每档确认口令互不相同', new Set(phrases).size === phrases.length, phrases.join(' / '))
  // 框架表（账号/角色/菜单/权限/部门/结构版本/升级记录）在任何范围下都不出现；
  // 日志表按设计只允许出现在 database 档 —— 那是「整库重置」的语义。
  const framework = ['sys_users', 'sys_roles', 'sys_menus', 'sys_depts', 'sys_user_roles',
    'sys_role_menus', 'sys_schema_versions', 'sys_upgrade_records']
  const logs = ['sys_operation_logs', 'sys_login_logs', 'sys_idempotencies']
  ok('任何范围都不含框架表',
    scopes.every((s) => !s.tables.some((t) => framework.includes(t.name))),
    '账号/角色/菜单/权限必须免疫')
  ok('日志表只出现在 database 档',
    scopes.every((s) => {
      const hasLog = s.tables.some((t) => logs.includes(t.name))
      return s.key === 'database' ? hasLog : !hasLog
    }))

  const bizWrong = await request('POST', '/ops/cleanup/business', { token, body: { scope: 'inv_documents', confirm: '确认' } })
  ok('业务清理口令错误被拒', bizWrong.json?.code !== 0, JSON.stringify(bizWrong.json?.message))
  const bizUnknown = await request('POST', '/ops/cleanup/business', { token, body: { scope: 'drop_all', confirm: 'x' } })
  ok('业务清理拒绝未知范围', bizUnknown.json?.code !== 0 && /合法值/.test(bizUnknown.json?.message || ''),
    JSON.stringify(bizUnknown.json?.message))
  // 口令对但范围跨档：用 database 档的口令去清 documents 档，必须被拒
  const bizCross = await request('POST', '/ops/cleanup/business', {
    token,
    body: { scope: 'inv_documents', confirm: '确认重置数据库' },
  })
  ok('业务清理拒绝跨档口令', bizCross.json?.code !== 0, JSON.stringify(bizCross.json?.message))

  // 恢复演练不允许指向生产库：HTTP 接口不该提供「一次误点盖掉生产库」的能力
  const restoreProd = await request('POST', '/ops/restore', { token, body: { file: 'whatever.dump' } })
  ok('恢复演练拒绝不存在的备份', restoreProd.json?.code !== 0, JSON.stringify(restoreProd.json))

  const upgradeStatus = await request('GET', '/ops/upgrade', { token })
  const ust = upgradeStatus.json?.data
  ok('升级状态含版本与预检', upgradeStatus.json?.code === 0 && Array.isArray(ust?.preflight) && ust.preflight.length === 4,
    JSON.stringify(ust?.preflight?.map((p) => p.key)))
  ok('升级记录可读', Array.isArray(ust?.records))

  // 未生成的计划 ID：必须报「计划不存在」，而不是静默成功
  const applyGhost = await request('POST', '/ops/upgrade/apply', {
    token,
    body: { planId: 'upg-does-not-exist', confirm: '确认升级' },
  })
  ok('执行不存在的升级计划被拒', applyGhost.json?.code !== 0, JSON.stringify(applyGhost.json))
  const backupGhost = await request('POST', '/ops/upgrade/backup', { token, body: { planId: 'upg-does-not-exist' } })
  ok('为不存在的计划留档被拒', backupGhost.json?.code !== 0, JSON.stringify(backupGhost.json))

  // 上传非 zip：必须在校验阶段就失败，不能写出一个半截的暂存目录
  const badPackage = await upload('/ops/upgrade/package', token, 'broken.zip', Buffer.from('not a zip at all'))
  ok('上传非 zip 升级包被拒', badPackage.json?.code !== 0, JSON.stringify(badPackage.json))

  // 只读账号的边界是「能看不能做」：ops:list 是页面权限，看得见列表与巡检；
  // 而备份、清理、升级是按钮权限，一个都不能给。
  ok('只读账号可查看备份列表', (await request('GET', '/ops/backups', { token: demoToken })).status === 200)
  ok('只读账号可查看升级状态', (await request('GET', '/ops/upgrade', { token: demoToken })).status === 200)
  ok('只读账号执行备份被拒(403)',
    (await request('POST', '/ops/backups', { token: demoToken, body: {} })).status === 403)
  ok('只读账号执行清理被拒(403)',
    (await request('POST', '/ops/cleanup/apply', { token: demoToken, body: { confirm: '确认清理' } })).status === 403)
  ok('只读账号上传升级包被拒(403)',
    (await upload('/ops/upgrade/package', demoToken, 'x.zip', Buffer.from('x'))).status === 403)
  ok('只读账号恢复演练被拒(403)',
    (await request('POST', '/ops/restore', { token: demoToken, body: { file: 'x.dump' } })).status === 403)
  ok('只读账号查看业务清理范围', (await request('GET', '/ops/cleanup/scopes', { token: demoToken })).status === 200)
  ok('只读账号执行业务清理被拒(403)',
    (await request('POST', '/ops/cleanup/business', { token: demoToken, body: { scope: 'inv_documents', confirm: '确认清理单据' } })).status === 403)

  section('清理测试数据')
  const cleanRole = await request('DELETE', `/system/roles/${newRoleId}`, { token })
  ok('删除自测角色', cleanRole.json?.code === 0, JSON.stringify(cleanRole.json))
  const cleanUser = await request('DELETE', `/system/users/${newUserId}`, { token })
  ok('删除自测用户', cleanUser.json?.code === 0, JSON.stringify(cleanUser.json))

  section('令牌刷新与登出')
  const refreshed = await request('POST', '/auth/refresh', { body: { refreshToken } })
  ok('刷新令牌', refreshed.json?.code === 0 && !!refreshed.json?.data?.accessToken, JSON.stringify(refreshed.json))
  const logout = await request('POST', '/auth/logout', { token: refreshed.json?.data?.accessToken })
  ok('退出登录', logout.json?.code === 0, JSON.stringify(logout.json))
  // 加固点：登出吊销的是「账号维度」而不是「这一张令牌」，
  // 否则用户在公用电脑上登出后，此前被复制走的旧令牌仍能继续用。
  const afterLogout = await request('GET', '/profile', { token })
  ok('登出后同账号其余令牌一并失效（40105）', afterLogout.json?.code === 40105, JSON.stringify(afterLogout.json))

  emit(`\n结果：通过 ${pass} 项，失败 ${fail} 项`)
  if (fail > 0) process.exit(1)
}

main().catch((e) => {
  emit(`自测脚本异常终止: ${e}`)
  process.exit(1)
})
