import { http } from '@/utils/request'
import { downloadFile, uploadFile } from '@/utils/download'

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

/** 系统运维接口 */
export const opsApi = {
  /** 备份列表 */
  backups() {
    return http.get<BackupItem[]>('/ops/backups')
  },

  /** 执行备份；skipPrune 为 true 时不轮转（升级前留档用） */
  runBackup(skipPrune = false) {
    return http.post<BackupItem>('/ops/backups', { skipPrune })
  },

  /** 删除一个备份 */
  deleteBackup(file: string) {
    return http.delete('/ops/backups', { file })
  },

  /** 下载备份文件 */
  downloadBackup(file: string) {
    return downloadFile(
      `/ops/backups/download?file=${encodeURIComponent(file)}`,
      undefined,
      file,
    )
  },

  /** 恢复演练：恢复到新库 */
  restore(file: string, target?: string) {
    return http.post<{ Target: string; Tables: number; Rows: Record<string, number>; Warn: string[] }>(
      '/ops/restore',
      { file, target },
    )
  },

  /** 清理预演（只统计不删） */
  cleanupPreview() {
    return http.get<CleanupView>('/ops/cleanup/preview')
  },

  /** 真删；confirm 必须正好是「确认清理」 */
  cleanupApply(confirm: string, days?: { opLogDays?: number; loginLogDays?: number; idempotencyDays?: number }) {
    return http.post<CleanupView>('/ops/cleanup/apply', { confirm, ...days })
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
    return uploadFile<UpgradePlanResult>('/ops/upgrade/package', file)
  },

  /** 升级前强制留档备份 */
  upgradeBackup(planId: string) {
    return http.post<BackupItem>('/ops/upgrade/backup', { planId })
  },

  /** 执行升级；confirm 必须正好是「确认升级」 */
  applyUpgrade(planId: string, confirm: string) {
    return http.post<UpgradeRecord>('/ops/upgrade/apply', { planId, confirm })
  },
}
