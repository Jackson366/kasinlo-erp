// ============================================================
// 与后端约定的通用类型定义
// 后端统一响应结构：{ code, message, data, traceId }
// ============================================================

/** 统一响应体 */
export interface ApiResult<T = unknown> {
  code: number
  message: string
  data: T
  traceId?: string
}

/** 分页数据结构 */
export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  pageSize: number
}

/** 分页查询参数 */
export interface PageQuery {
  page?: number
  pageSize?: number
  keyword?: string
  orderBy?: string
  order?: 'asc' | 'desc'
}

/** 通用启用状态：1 启用 0 禁用 */
export type Status = 0 | 1

/** 数据范围：1 全部 2 本部门 3 本部门及以下 4 仅本人 */
export type DataScope = 1 | 2 | 3 | 4

// ---------------- 认证相关 ----------------

export interface LoginParams {
  username: string
  password: string
}

export interface LoginResult {
  accessToken: string
  refreshToken: string
  expiresIn: number
  tokenType: string
  user: UserIdentity
}

export interface UserIdentity {
  userId: number
  username: string
  nickname: string
  avatar: string
  deptId: number | null
  roleIds: number[]
  roleCodes: string[]
  dataScope: DataScope
  isSuper: boolean
}

export interface ProfileInfo {
  id: number
  username: string
  nickname: string
  email: string
  phone: string
  avatar: string
  deptId: number | null
  deptName: string
  status: Status
  roles: string[]
  roleCodes: string[]
  isSuper: boolean
  permissions: string[]
}

// ---------------- 动态路由 ----------------

export interface RouteMeta {
  title: string
  icon: string
  hidden: boolean
  keepAlive: boolean
  isExternal: boolean
  permission?: string
}

export interface RouteRecord {
  id: number
  name: string
  path: string
  component: string
  redirect?: string
  meta: RouteMeta
  children?: RouteRecord[]
}

// ---------------- 用户 ----------------

export interface UserItem {
  id: number
  username: string
  nickname: string
  email: string
  phone: string
  avatar: string
  deptId: number | null
  deptName: string
  status: Status
  remark: string
  roleIds: number[]
  roleNames: string[]
  lastLoginAt: string | null
  lastLoginIp: string
  createdAt: string
}

export interface UserQuery extends PageQuery {
  deptId?: number
  status?: Status
  roleId?: number
}

/** 用户导出条件：在列表筛选条件之上附加「按选中行导出」 */
export interface UserExportQuery extends UserQuery {
  /** 选中项 ID，逗号分隔；为空表示按筛选条件导出全部 */
  ids?: string
}

/** 单行导入校验结果 */
export interface ImportRow {
  /** Excel 中的实际行号（含表头，从 1 开始），便于用户直接定位 */
  row: number
  username: string
  nickname: string
  /** 为空数组表示该行校验通过 */
  errors: string[]
}

/** 导入（或导入预览）结果 */
export interface ImportResult {
  /** 数据行总数（不含表头） */
  total: number
  valid: number
  invalid: number
  /** 实际写入的行数，预览阶段恒为 0 */
  created: number
  /** 逐行明细 */
  rows: ImportRow[]
}

export interface UserPayload {
  username?: string
  password?: string
  nickname: string
  email?: string
  phone?: string
  deptId?: number | null
  status?: Status
  roleIds?: number[]
  remark?: string
}

// ---------------- 角色 ----------------

export interface RoleItem {
  id: number
  name: string
  code: string
  sort: number
  status: Status
  dataScope: DataScope
  isBuiltin: boolean
  remark: string
  userCount: number
  menuIds: number[]
  createdAt: string
}

export interface RolePayload {
  name: string
  code?: string
  sort?: number
  status?: Status
  dataScope?: DataScope
  remark?: string
  menuIds?: number[]
}

export interface RoleOption {
  id: number
  name: string
  code: string
}

// ---------------- 菜单 ----------------

export type MenuType = 'dir' | 'menu' | 'button'

export interface MenuNode {
  id: number
  parentId: number
  name: string
  path: string
  component: string
  redirect: string
  title: string
  icon: string
  type: MenuType
  permission: string
  sort: number
  hidden: boolean
  keepAlive: boolean
  isExternal: boolean
  status: Status
  children?: MenuNode[]
}

export interface MenuPayload {
  parentId: number
  name: string
  path?: string
  component?: string
  redirect?: string
  title: string
  icon?: string
  type: MenuType
  permission?: string
  sort?: number
  hidden?: boolean
  keepAlive?: boolean
  isExternal?: boolean
  status?: Status
}

// ---------------- 部门 ----------------

export interface DeptNode {
  id: number
  parentId: number
  name: string
  leader: string
  phone: string
  email: string
  sort: number
  status: Status
  userCount: number
  createdAt: string
  children?: DeptNode[]
}

export interface DeptPayload {
  parentId: number
  name: string
  leader?: string
  phone?: string
  email?: string
  sort?: number
  status?: Status
}

// ---------------- 日志 ----------------

export interface OperationLogItem {
  id: number
  userId: number
  username: string
  module: string
  action: string
  method: string
  path: string
  ip: string
  userAgent: string
  request: string
  status: number
  latencyMs: number
  errorMsg: string
  createdAt: string
}

export interface LoginLogItem {
  id: number
  userId: number
  username: string
  ip: string
  userAgent: string
  success: boolean
  message: string
  createdAt: string
}

// ---------------- 仪表盘 ----------------

export interface DashboardOverview {
  userCount: number
  roleCount: number
  deptCount: number
  menuCount: number
  todayLogins: number
  todayOps: number
  todayFailures: number
  activeUserWeek: number
}

export interface TrendPoint {
  date: string
  logins: number
  opers: number
  failCount: number
}

export interface ModuleStat {
  module: string
  total: number
}
