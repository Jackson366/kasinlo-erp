import { http } from '@/utils/request'
import type { DashboardOverview, LoginLogItem, ModuleStat, TrendPoint } from '@/types/api'

/** 仪表盘接口 */
export const dashboardApi = {
  /** 概览统计 */
  overview() {
    return http.get<DashboardOverview>('/dashboard/overview')
  },
  /** 趋势数据 */
  trend(days = 7) {
    return http.get<TrendPoint[]>('/dashboard/trend', { days })
  },
  /** 模块操作排行 */
  modules(days = 7, limit = 8) {
    return http.get<ModuleStat[]>('/dashboard/modules', { days, limit })
  },
  /** 最近登录记录 */
  recentLogins(limit = 8) {
    return http.get<LoginLogItem[]>('/dashboard/recent-logins', { limit })
  },
}
