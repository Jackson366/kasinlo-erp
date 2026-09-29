/**
 * Nova Admin 后端接口自测脚本（零依赖，Node 18+ 内置 fetch）
 * 用法：node scripts/api-smoke.mjs [baseUrl]
 */
const BASE = process.argv[2] || 'http://127.0.0.1:8080'
const API = `${BASE}/api/v1`

// 同步落盘，便于在进程被外部终止时仍能查到执行进度
import fs from 'node:fs'
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

async function request(method, path, { token, body, raw } = {}) {
  const headers = {}
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
  ok('路由包含仪表盘与系统管理', titles.includes('仪表盘') && titles.includes('系统管理'), titles)
  const flat = JSON.stringify(routeList)
  ok('动态路由中不含按钮权限点', !flat.includes('system:user:create'), '按钮项应被过滤')

  section('仪表盘')
  const overview = await request('GET', '/dashboard/overview', { token })
  ok('概览统计', overview.json?.code === 0 && overview.json.data.userCount >= 2, JSON.stringify(overview.json?.data))
  const trend = await request('GET', '/dashboard/trend?days=7', { token })
  ok('趋势数据 7 天', Array.isArray(trend.json?.data) && trend.json.data.length === 7, `len=${trend.json?.data?.length}`)
  const modules = await request('GET', '/dashboard/modules?days=7', { token })
  ok('模块排行', Array.isArray(modules.json?.data))

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
    body: { name: '自测角色', code: 'smoke_role', sort: 9, dataScope: 4, menuIds: [1, 10, 11] },
  })
  ok('新增角色', newRole.json?.code === 0, JSON.stringify(newRole.json))
  const newRoleId = newRole.json?.data?.id

  const roleDetail = await request('GET', `/system/roles/${newRoleId}`, { token })
  ok('角色详情含已授权菜单', roleDetail.json?.data?.menuIds?.length === 3, JSON.stringify(roleDetail.json?.data?.menuIds))

  const assign = await request('PUT', `/system/roles/${newRoleId}/menus`, { token, body: { menuIds: [1, 10, 11, 12] } })
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

  section('菜单与部门')
  const menus = await request('GET', '/system/menus', { token })
  ok('菜单树非空', Array.isArray(menus.json?.data) && menus.json.data.length >= 2, `len=${menus.json?.data?.length}`)
  const menuDetail = await request('GET', '/system/menus/11', { token })
  ok('菜单详情', menuDetail.json?.data?.permission === 'system:user:list', JSON.stringify(menuDetail.json?.data))
  const delHasChild = await request('DELETE', '/system/menus/10', { token })
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

  section('令牌刷新与登出')
  const refreshed = await request('POST', '/auth/refresh', { body: { refreshToken } })
  ok('刷新令牌', refreshed.json?.code === 0 && !!refreshed.json?.data?.accessToken, JSON.stringify(refreshed.json))
  const logout = await request('POST', '/auth/logout', { token: refreshed.json?.data?.accessToken })
  ok('退出登录', logout.json?.code === 0, JSON.stringify(logout.json))

  section('清理测试数据')
  const cleanRole = await request('DELETE', `/system/roles/${newRoleId}`, { token })
  ok('删除自测角色', cleanRole.json?.code === 0, JSON.stringify(cleanRole.json))
  const cleanUser = await request('DELETE', `/system/users/${newUserId}`, { token })
  ok('删除自测用户', cleanUser.json?.code === 0, JSON.stringify(cleanUser.json))

  emit(`\n结果：通过 ${pass} 项，失败 ${fail} 项`)
  if (fail > 0) process.exit(1)
}

main().catch((e) => {
  emit(`自测脚本异常终止: ${e}`)
  process.exit(1)
})
