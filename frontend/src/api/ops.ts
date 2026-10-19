import type { AxiosRequestConfig } from 'axios'
import { http } from '@/utils/request'
import { downloadFile, uploadFile } from '@/utils/download'

/**
 * 生成一次性幂等键：crypto.randomUUID 只在安全上下文（https / localhost）可用，
 * 退化到 Math.random（幂等键只要求「一次操作内唯一」，不要求密码学强度）。
 */
function newKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `ops-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

/** 运维长任务超时（10 分钟）。普通列表/详情走 request.ts 的 20s，互不干扰。 */
const OPS_TIMEOUT = 10 * 60 * 1000

/** 带 Idempotency-Key 的 POST：双击 / 超时重试由服务端兜住，不会重复执行。 */
function postIdempotent<T>(url: string, data?: unknown): Promise<T> {
  const config: AxiosRequestConfig = {
    headers: { 'Idempotency-Key': newKey() },
    // 运维写接口是长任务（备份大库、恢复、清理、执行升级都可能远超 20s），
    // 单独放宽到 10 分钟，不能沿用 request.ts 里给普通 CRUD 定的 20s。
    timeout: OPS_TIMEOUT,
  }
  return http.post<T>(url, data, config)
}

/** 备份文件 */
export interface BackupItem {
  name: string
  path: string
  size: number
  time: string
  sha256: string
  database: string
  serverVersion: string
  tables: number
  hasMeta: boolean
  /** false 表示写完未用 pg_restore 回读校验（缺 pg_restore 的精简安装） */
  verified: boolean
}

/** 清理的一类数据 */
export interface CleanupStep {
  name: string
  table: string
  cutoff: string
  rows: number
  batches: number
  note: string
}

/** 清理结果 */
export interface CleanupView {
  dryRun: boolean
  steps: CleanupStep[]
  vacuumed: string[]
  blocked: string
}

/** 体检报告（只取页面要用的字段） */
export interface CheckReport {
  database: string
  serverVersion: string
  sizeBytes: number
  tables: number
  schemaVersionDb: number
  schemaVersionBinary: number
  schemaVersionKnown: boolean
  opLogRows: number
  opLogOldest: string
  loginLogRows: number
  idempotencyRows: number
  idempotencyExpired: number
  latestBackup: string
  latestBackupAt: string
  checkedAt: string
  /** 关键表逐张统计（巡检页） */
  keyTables?: TableStat[]
}

/** 表统计 */
export interface TableStat {
  name: string
  rows: number
  softDeleted: number
  sizeBytes: number
}

/** 升级动作 */
export interface UpgradeAction {
  key: string
  title: string
  detail: string
  done: boolean
  error: string
}

/** 升级记录 */
export interface UpgradeRecord {
  id: number
  planId: string
  fromVersion: string
  toVersion: string
  packageSha256: string
  packageSize: number
  backupPath: string
  status: string
  actions: UpgradeAction[]
  message: string
  createdAt: string
  appliedAt: string | null
  finishedAt: string | null
}

/** 升级前预检项 */
export interface PreflightItem {
  key: string
  title: string
  pass: boolean
  hint: string
}

/** 升级页初始数据 */
export interface UpgradeStatus {
  release: string
  schemaDb: number
  schemaBinary: number
  schemaKnown: boolean
  schemaNote: string
  latestBackup: string
  latestBackupAt: string
  database: string
  serverVersion: string
  sizeBytes: number
  preflight: PreflightItem[]
  records: UpgradeRecord[]
  warnings: string[]
  restartHint: string
}

/** 上传升级包的返回 */
export interface UpgradePlanResult {
  plan: UpgradePlan
  record: UpgradeRecord
}

/** 升级计划（解析后的动作清单） */
export interface UpgradePlan {
  planId: string
  fromVersion: string
  toVersion: string
  minSchemaVersion: number
  packageSha256: string
  packageSize: number
  notes: string
  actions: UpgradeAction[]
  warnings: string[]
}

/** 一档业务清理范围 + 各表行数 */
export interface BizScopeView {
  key: string
  title: string
  desc: string
  keep: string
  confirm: string
  resetStockable: boolean
  totalRows: number
  tables: { name: string; rows: number }[]
}

/** 业务清理结果 */
export interface BizCleanupReport {
  scope: string
  title: string
  dryRun: boolean
  tables: { name: string; rows: number; deleted: number; seqReset: boolean }[]
  stockReset: boolean
  stockResetRows: number
  totalDeleted: number
}

/** 系统运维接口 */
export const opsApi = {
  /** 备份列表 */
  backups() {
    return http.get<BackupItem[]>('/ops/backups')
  },

  /** 执行备份；skipPrune 为 true 时不轮转（升级前留档用） */
  runBackup(skipPrune = false) {
    return postIdempotent<BackupItem>('/ops/backups', { skipPrune })
  },

  /** 删除一个备份 */
  deleteBackup(file: string) {
    return http.delete('/ops/backups', { file }, { headers: { 'Idempotency-Key': newKey() } })
  },

  /** 下载备份文件 */
  downloadBackup(file: string) {
    return downloadFile(
      `/ops/backups/download?file=${encodeURIComponent(file)}`,
      undefined,
      file,
      { timeout: OPS_TIMEOUT },
    )
  },

  /** 恢复演练：恢复到新库 */
  restore(file: string, target?: string) {
    return postIdempotent<{ Target: string; Tables: number; Rows: Record<string, number>; Warn: string[] }>(
      '/ops/restore',
      { file, target },
    )
  },

  /** 原地恢复：把选定备份覆盖当前库（服务端先自动备份）；confirm 必须正好是「确认恢复」 */
  restoreInPlace(file: string, confirm: string) {
    return postIdempotent<{ Target: string; Tables: number; Rows: Record<string, number>; Warn: string[] }>(
      '/ops/restore/in-place',
      { file, confirm },
    )
  },

  /** 清理预演（只统计不删） */
  cleanupPreview() {
    return http.get<CleanupView>('/ops/cleanup/preview')
  },

  /** 真删；confirm 必须正好是「确认清理」 */
  cleanupApply(confirm: string, days?: { opLogDays?: number; loginLogDays?: number; idempotencyDays?: number }) {
    return postIdempotent<CleanupView>('/ops/cleanup/apply', { confirm, ...days })
  },

  /** 业务清理范围与各表行数预览 */
  cleanupScopes() {
    return http.get<BizScopeView[]>('/ops/cleanup/scopes')
  },

  /** 执行业务数据清理（不可逆）；confirm 必须正好等于该范围的口令 */
  businessCleanup(body: { scope: string; confirm: string; resetStock?: boolean }) {
    return postIdempotent<BizCleanupReport>('/ops/cleanup/business', body)
  },

  /** 系统体检 */
  check() {
    return http.get<{ report: CheckReport; warnings: string[] }>('/ops/check')
  },

  /** 升级页初始数据（版本 + 预检 + 记录） */
  upgradeStatus() {
    return http.get<UpgradeStatus>('/ops/upgrade')
  },

  /** 升级记录 */
  upgradeRecords() {
    return http.get<UpgradeRecord[]>('/ops/upgrade/records')
  },

  /** 上传升级包 */
  uploadPackage(file: File) {
    return uploadFile<UpgradePlanResult>('/ops/upgrade/package', file, {
      idempotencyKey: newKey(),
      config: { timeout: OPS_TIMEOUT },
    })
  },

  /** 升级前强制留档备份 */
  upgradeBackup(planId: string) {
    return postIdempotent<BackupItem>('/ops/upgrade/backup', { planId })
  },

  /** 执行升级；confirm 必须正好是「确认升级」 */
  applyUpgrade(planId: string, confirm: string) {
    return postIdempotent<UpgradeRecord>('/ops/upgrade/apply', { planId, confirm })
  },
}
