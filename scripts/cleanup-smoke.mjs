// 清理冒烟测试残留数据：上一轮在「登出吊销令牌」加固上线后，清理阶段拿到 40105，
// 于是 smoke_user / smoke_role 留在库里，导致后续每一轮都在「新增」步骤撞 40900。
const API = 'http://127.0.0.1:6082/api/v1'

async function req(method, path, token, body) {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json().catch(() => null)
}

const login = await req('POST', '/auth/login', null, { username: 'admin', password: 'admin123456' })
const token = login?.data?.accessToken
if (!token) {
  console.log('登录失败', JSON.stringify(login))
  process.exit(1)
}

const users = await req('GET', '/system/users?username=smoke_user&pageSize=20', token)
for (const u of users?.data?.list || []) {
  if (u.username !== 'smoke_user') continue
  const r = await req('DELETE', `/system/users/${u.id}`, token)
  console.log(`删除用户 ${u.username} ->`, JSON.stringify(r))
}

const roles = await req('GET', '/system/roles?pageSize=100', token)
for (const r of roles?.data?.list || []) {
  if (r.code !== 'smoke_role') continue
  const out = await req('DELETE', `/system/roles/${r.id}`, token)
  console.log(`删除角色 ${r.code} ->`, JSON.stringify(out))
}
console.log('清理完成')
