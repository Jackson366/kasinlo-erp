import { http } from '@/utils/request'
import type { MenuNode, MenuPayload } from '@/types/api'

/** 菜单管理接口 */
export const menuApi = {
  /** 菜单树 */
  list(params?: { keyword?: string; status?: number }) {
    return http.get<MenuNode[]>('/system/menus', params as Record<string, unknown>)
  },
  /** 菜单详情 */
  detail(id: number) {
    return http.get<MenuNode>(`/system/menus/${id}`)
  },
  /** 新增菜单 */
  create(data: MenuPayload) {
    return http.post<{ id: number }>('/system/menus', data)
  },
  /** 修改菜单 */
  update(id: number, data: MenuPayload) {
    return http.put<string>(`/system/menus/${id}`, data)
  },
  /** 删除菜单 */
  remove(id: number) {
    return http.delete<string>(`/system/menus/${id}`)
  },
}
