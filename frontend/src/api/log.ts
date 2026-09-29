import { http } from '@/utils/request'
import { downloadFile } from '@/utils/download'
import type { LoginLogItem, OperationLogItem, PageQuery, PageResult } from '@/types/api'

/** 操作日志查询条件 */
export interface OperationLogQuery extends PageQuery {
  username?: string
  module?: string
  method?: string
  status?: number
  startAt?: string
  endAt?: string
}

/** 登录日志查询条件 */
export interface LoginLogQuery extends PageQuery {
  username?: string
  success?: boolean
  startAt?: string
  endAt?: string
}

/** 日志管理接口 */
export const logApi = {
  /** 操作日志列表 */
  operations(params: OperationLogQuery) {
    return http.get<PageResult<OperationLogItem>>(
      '/v1/system/logs/operations',
      params as Record<string, unknown>,
    )
  },
  /** 登录日志列表 */
  logins(params: LoginLogQuery) {
    return http.get<PageResult<LoginLogItem>>('/v1/system/logs/logins', params as Record<string, unknown>)
  },
  /** 操作日志模块下拉 */
  modules() {
    return http.get<string[]>('/v1/system/logs/modules')
  },
  /** 清空操作日志，before 为空表示全部清空 */
  clearOperations(before?: string) {
    return http.delete<{ deleted: number }>('/v1/system/logs/operations', { before: before || '' })
  },
  /** 清空登录日志 */
  clearLogins(before?: string) {
    return http.delete<{ deleted: number }>('/v1/system/logs/logins', { before: before || '' })
  },
  /** 导出操作日志 */
  exportOperations(params: OperationLogQuery, fallbackName: string) {
    return downloadFile(
      '/v1/system/logs/operations/export',
      params as Record<string, unknown>,
      fallbackName,
    )
  },
  /** 导出登录日志 */
  exportLogins(params: LoginLogQuery, fallbackName: string) {
    return downloadFile(
      '/v1/system/logs/logins/export',
      params as Record<string, unknown>,
      fallbackName,
    )
  },
}
