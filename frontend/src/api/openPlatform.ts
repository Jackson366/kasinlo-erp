import { http } from '@/utils/request'

export interface DeviceAuthorization {
  clientId: string
  scope: string
  expiresAt: string
  accessLifetimeSeconds: number
}

export interface OpenPlatformGrant {
  id: string
  clientId: string
  expiresAt: string
}

export const openPlatformApi = {
  inspect(userCode: string) {
    return http.post<DeviceAuthorization>('/open-platform/device/inspect', { userCode }, { silent: true })
  },
  decide(userCode: string, approve: boolean) {
    return http.post<void>('/open-platform/device/decision', { userCode, approve }, { silent: true })
  },
  grants() {
    return http.get<OpenPlatformGrant[]>('/open-platform/grants', undefined, { silent: true })
  },
  revoke(id: string) {
    return http.post<void>(`/open-platform/grants/${encodeURIComponent(id)}/revoke`, undefined, { silent: true })
  },
}
