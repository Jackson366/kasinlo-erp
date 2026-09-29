import { http } from '@/utils/request'
import type { DeptNode, DeptPayload } from '@/types/api'

/** 部门管理接口 */
export const deptApi = {
  /** 部门树 */
  list(params?: { keyword?: string; status?: number }) {
    return http.get<DeptNode[]>('/v1/system/depts', params as Record<string, unknown>)
  },
  /** 部门详情 */
  detail(id: number) {
    return http.get<DeptNode>(`/v1/system/depts/${id}`)
  },
  /** 新增部门 */
  create(data: DeptPayload) {
    return http.post<{ id: number }>('/v1/system/depts', data)
  },
  /** 修改部门 */
  update(id: number, data: DeptPayload) {
    return http.put<string>(`/v1/system/depts/${id}`, data)
  },
  /** 删除部门 */
  remove(id: number) {
    return http.delete<string>(`/v1/system/depts/${id}`)
  },
}
