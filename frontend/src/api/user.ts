import { http } from '@/utils/request'
import { downloadFile, uploadFile } from '@/utils/download'
import type {
  ImportResult,
  PageResult,
  Status,
  UserExportQuery,
  UserItem,
  UserPayload,
  UserQuery,
} from '@/types/api'

/** 用户管理接口 */
export const userApi = {
  /** 分页查询用户 */
  list(params: UserQuery) {
    return http.get<PageResult<UserItem>>('/system/users', params as Record<string, unknown>)
  },
  /** 新增用户 */
  create(data: UserPayload) {
    return http.post<{ id: number }>('/system/users', data)
  },
  /** 修改用户 */
  update(id: number, data: UserPayload) {
    return http.put<string>(`/system/users/${id}`, data)
  },
  /** 删除用户 */
  remove(id: number) {
    return http.delete<string>(`/system/users/${id}`)
  },
  /** 启用/禁用 */
  updateStatus(id: number, status: Status) {
    return http.put<string>(`/system/users/${id}/status`, { status })
  },
  /** 重置密码 */
  resetPassword(id: number, password: string) {
    return http.put<string>(`/system/users/${id}/password`, { password })
  },
  /** 导出用户（params.ids 非空时导出选中项，否则按筛选条件导出全部） */
  export(params: UserExportQuery, fallbackName: string) {
    return downloadFile('/system/users/export', params as Record<string, unknown>, fallbackName)
  },
  /** 下载导入模板 */
  downloadTemplate() {
    return downloadFile('/system/users/import-template', undefined, '用户导入模板.xlsx')
  },
  /** 导入用户；dryRun 为 true 时仅校验不写库 */
  import(file: File, dryRun = false) {
    return uploadFile<ImportResult>('/system/users/import', file, {
      params: dryRun ? { dryRun: 1 } : undefined,
    })
  },
}
