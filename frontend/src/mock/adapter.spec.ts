import axios from 'axios'
import { expect, it } from 'vitest'
import { createMockAdapter } from './adapter'
import { createMockService } from './service'

it('通过 Axios 接收序列化请求，并在裸 axios 刷新路径上剥离 API 前缀', async () => {
  const adapter = createMockAdapter(createMockService(), '/api/v1')
  const client = axios.create({ adapter, baseURL: '/api/v1' })
  const result = await client.post('/auth/login', { username: 'admin', password: 'admin123456' })
  expect(result.data).toMatchObject({ code: 0, data: { accessToken: 'mock-access-admin' } })
  const refreshed = await axios.post(
    '/api/v1/auth/refresh',
    { refreshToken: result.data.data.refreshToken },
    { adapter },
  )
  expect(refreshed.data.data.accessToken).toBe('mock-access-admin')
  await expect(client.get('/profile')).rejects.toMatchObject({
    response: { status: 401, data: { code: 40100 } },
  })
  await expect(
    client.get('/unknown', { headers: { Authorization: 'Bearer mock-access-admin' } }),
  ).rejects.toMatchObject({ response: { status: 501 } })
})
