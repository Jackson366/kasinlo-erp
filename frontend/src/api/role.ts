import { http } from '@/utils/request'
import { downloadFile } from '@/utils/download'
import type { PageQuery, PageResult, RoleItem, RoleOption, RolePayload } from '@/types/api'

/** 角色管理接口 */
export const roleApi = {
  /** 分页查询角色 */
  list(params: PageQuery) {
    return http.get<PageResult<RoleItem>>('/system/roles', params as Record<string, unknown>)
  },
  /** 角色下拉选项 */
  options() {
    return http.get<RoleOption[]>('/system/roles/options')
  },
  /** 新增角色 */
  create(data: RolePayload) {
    return http.post<{ id: number }>('/system/roles', data)
  },
  /** 修改角色 */
  update(id: number, data: RolePayload) {
    return http.put<string>(`/system/roles/${id}`, data)
  },
  /** 删除角色 */
  remove(id: number) {
    return http.delete<string>(`/system/roles/${id}`)
  },
  /** 分配菜单权限 */
  assignMenus(id: number, menuIds: number[]) {
    return http.put<string>(`/system/roles/${id}/menus`, { menuIds })
  },
  /** 导出角色 */
  export(params: PageQuery, fallbackName: string) {
    return downloadFile('/system/roles/export', params as Record<string, unknown>, fallbackName)
  },
}
