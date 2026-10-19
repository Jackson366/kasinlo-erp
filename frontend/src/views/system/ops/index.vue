<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { opsApi } from '@/api/ops'
import type {
  BackupItem,
  BizCleanupReport,
  BizScopeView,
  CheckReport,
  CleanupStep,
  CleanupView,
  PreflightItem,
  UpgradePlan,
  UpgradeRecord,
  UpgradeStatus,
} from '@/api/ops'
import AppIcon from '@/components/AppIcon.vue'
import { formatDateTime, fromNow } from '@/utils/format'

defineOptions({ name: 'SystemOps' })

/** 字节数格式化：备份体积从 165 KB 到几十 MB，用 KB/MB 读起来最省心 */
function humanSize(bytes: number): string {
  if (!bytes || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  const kb = bytes / 1024
  if (kb < 1024) return `${kb.toFixed(1)} KB`
  return `${(kb / 1024).toFixed(1)} MB`
}

/** 摘要只显示前后各 6 位：完整 64 位在表格里只会把列撑爆，而核对时前后几位已经够 */
function shortSha(sha: string): string {
  if (!sha) return '-'
  return sha.length > 14 ? `${sha.slice(0, 6)}…${sha.slice(-6)}` : sha
}

/** 升级包大小上限，与后端一致（512 MB）。改这里时记得同步后端 maxPackageBytes */
const MAX_PACKAGE_MB = 512
const MAX_PACKAGE_BYTES = MAX_PACKAGE_MB * 1024 * 1024

const activeTab = ref('backup')

// ── 备份管理 ────────────────────────────────────────────
const backups = ref<BackupItem[]>([])
const backupsLoading = ref(false)
const backupRunning = ref(false)

async function loadBackups(): Promise<void> {
  backupsLoading.value = true
  try {
    backups.value = await opsApi.backups()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    backupsLoading.value = false
  }
}

async function runBackup(skipPrune = false): Promise<void> {
  backupRunning.value = true
  try {
    const item = await opsApi.runBackup(skipPrune)
    ElMessage.success(`备份完成：${item.name}（${humanSize(item.size)}）`)
    await loadBackups()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    backupRunning.value = false
  }
}

async function deleteBackup(row: BackupItem): Promise<void> {
  await ElMessageBox.confirm(
    `删除后无法恢复，确认删除备份「${row.name}」？`,
    '删除备份',
    { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
  )
  await opsApi.deleteBackup(row.name)
  ElMessage.success('已删除')
  await loadBackups()
}

interface BackupAction {
  key: string
  label: string
  perm: string
  type: 'primary' | 'warning' | 'danger'
  loading: boolean
  onClick: () => void
}

/** 操作列按钮。抽成函数而不是在模板里写死：v-perm 要按权限收，
 *  而下载 / 恢复 / 删除三者的权限不同，写死就会出现「看得见但点不动」的半残状态。 */
function backupActions(row: BackupItem): BackupAction[] {
  return [
    {
      key: 'download', label: '下载', perm: 'ops:backup', type: 'primary', loading: false,
      onClick: () => { void opsApi.downloadBackup(row.name) },
    },
    {
      key: 'restore', label: '恢复演练', perm: 'ops:restore', type: 'warning', loading: restoreRunning.value,
      onClick: () => openRestore(row),
    },
    {
      key: 'delete', label: '删除', perm: 'ops:backup', type: 'danger', loading: false,
      onClick: () => { void deleteBackup(row) },
    },
  ]
}

/** 恢复演练：把备份恢复到一个新库，核对表数与行数 */
const restoreVisible = ref(false)
const restoreTarget = ref('')
const restoreFile = ref('')
const restoreRunning = ref(false)
interface RestoreResult {
  Target: string
  Tables: number
  Rows: Record<string, number>
  Warn: string[]
}
const restoreResult = ref<RestoreResult | null>(null)

function openRestore(row: BackupItem): void {
  restoreFile.value = row.name
  restoreTarget.value = ''
  restoreResult.value = null
  restoreVisible.value = true
}

async function doRestore(): Promise<void> {
  restoreRunning.value = true
  try {
    restoreResult.value = await opsApi.restore(restoreFile.value, restoreTarget.value || undefined)
    ElMessage.success('恢复完成，请核对下表数与行数')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    restoreRunning.value = false
  }
}

// ── 数据清理 ────────────────────────────────────────────
// 留空表示「用配置默认值」。刻意不预填 180：填了用户会以为 180 是这个页面定的，
// 而真值在 config.yaml 的 maintenance 段；更糟的是填 0 会被误读成「保留 0 天」。
const cleanupDays = ref<{ opLogDays?: number; loginLogDays?: number; idempotencyDays?: number }>({})
const cleanupLoading = ref(false)
const cleanupView = ref<CleanupView | null>(null)
const cleanupApplying = ref(false)

// ── 业务数据清理（不可逆）─────────────────────────────
const bizScopes = ref<BizScopeView[]>([])
const bizScopeKey = ref('inv_documents')
const bizResetStock = ref(false)
const bizLoading = ref(false)
const bizRunning = ref(false)
const bizBackupRunning = ref(false)
const bizResult = ref<BizCleanupReport | null>(null)
const bizBackupAge = ref('')

const currentBizScope = computed(() =>
  bizScopes.value.find((s) => s.key === bizScopeKey.value) || null,
)

// 切换范围后，上一档的清理结果与「清零库存」勾选都会残留：
// 结果表是上一档的表、勾选是上一档才有的语义，都会和当前范围对不上，
// 让界面看起来像「已经对这档清理过」。跟着范围一起复位才不留错觉。
watch(bizScopeKey, () => {
  bizResult.value = null
  bizResetStock.value = false
})

async function loadBizScopes(): Promise<void> {
  bizLoading.value = true
  try {
    bizScopes.value = await opsApi.cleanupScopes()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    bizLoading.value = false
  }
}

/** 看一下最近一份备份有多旧：业务清理要求 24 小时内有回滚点。 */
async function refreshBizBackupAge(): Promise<void> {
  try {
    const items = await opsApi.backups()
    if (items.length === 0) {
      bizBackupAge.value = '尚未备份'
      return
    }
    const latest = items.reduce((a, b) => (new Date(a.time) > new Date(b.time) ? a : b))
    bizBackupAge.value = `${formatDateTime(latest.time)}（${fromNow(latest.time)}）`
  } catch {
    bizBackupAge.value = ''
  }
}

async function runBizBackup(): Promise<void> {
  bizBackupRunning.value = true
  try {
    await opsApi.runBackup(false)
    ElMessage.success('备份完成，回滚点已刷新')
    await refreshBizBackupAge()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    bizBackupRunning.value = false
  }
}

/**
 * 执行业务清理。确认口令由服务端按范围下发，前端不存第二份 ——
 * 口令与范围表必须同源，否则会出现「页面让输 A、服务端要 B」的死局。
 */
async function applyBizCleanup(): Promise<void> {
  const scope = currentBizScope.value
  if (!scope) return
  const { value } = await ElMessageBox.prompt(
    `将${scope.desc}，删除后无法恢复。请输入「${scope.confirm}」以继续。`,
    `业务数据清理：${scope.title}`,
    {
      type: 'warning',
      confirmButtonText: '执行删除',
      cancelButtonText: '取消',
      inputPlaceholder: scope.confirm,
      inputValidator: (v: string) => (v.trim() === scope.confirm ? true : `请准确输入「${scope.confirm}」`),
    },
  )
  if (value?.trim() !== scope.confirm) return

  bizRunning.value = true
  try {
    bizResult.value = await opsApi.businessCleanup({
      scope: scope.key,
      confirm: scope.confirm,
      resetStock: bizResetStock.value,
    })
    ElMessage.success(`清理完成，共删除 ${bizResult.value.totalDeleted} 行`)
    await Promise.all([loadBizScopes(), loadCheck(), refreshBizBackupAge()])
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    bizRunning.value = false
  }
}

async function previewCleanup(): Promise<void> {
  cleanupLoading.value = true
  try {
    cleanupView.value = await opsApi.cleanupPreview()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    cleanupLoading.value = false
  }
}

/**
 * 真删。两道闸门：ElMessageBox 确认 + 输入「确认清理」。
 *
 * 为什么不用 el-input 的默认校验就完事：清理不可逆，而「手滑点到确认」
 * 与「想清楚后输入四个字」之间的距离，就是一次事故与一次正常运维的距离。
 */
async function applyCleanup(): Promise<void> {
  const { value } = await ElMessageBox.prompt(
    '删除后无法恢复。请输入「确认清理」以继续。',
    '执行数据清理',
    {
      type: 'warning',
      confirmButtonText: '执行删除',
      cancelButtonText: '取消',
      inputPlaceholder: '确认清理',
      inputValidator: (v: string) => (v.trim() === '确认清理' ? true : '请准确输入「确认清理」'),
    },
  )
  if (value?.trim() !== '确认清理') return

  cleanupApplying.value = true
  try {
    const days = cleanupDays.value
    const view = await opsApi.cleanupApply('确认清理', {
      opLogDays: days.opLogDays || undefined,
      loginLogDays: days.loginLogDays || undefined,
      idempotencyDays: days.idempotencyDays || undefined,
    })
    cleanupView.value = view
    ElMessage.success(view.dryRun ? '已按预演处理，未删除数据' : '清理完成')
    await loadCheck()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    cleanupApplying.value = false
  }
}

function totalCleanupRows(steps: CleanupStep[]): number {
  return steps.reduce((sum, s) => sum + (s.rows || 0), 0)
}

// ── 系统巡检 ────────────────────────────────────────────
const checkLoading = ref(false)
const checkReport = ref<CheckReport | null>(null)
const checkWarnings = ref<string[]>([])
const keyTables = ref<{ name: string; rows: number; softDeleted: number; sizeBytes: number }[]>([])

async function loadCheck(): Promise<void> {
  checkLoading.value = true
  try {
    const res = await opsApi.check()
    checkReport.value = res.report
    checkWarnings.value = res.warnings || []
    keyTables.value = res.report.keyTables || []
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    checkLoading.value = false
  }
}

// ── 系统升级 ────────────────────────────────────────────
const upgradeLoading = ref(false)
const upgrade = ref<UpgradeStatus | null>(null)
const uploadRunning = ref(false)
const plan = ref<UpgradePlan | null>(null)
const planRecord = ref<UpgradeRecord | null>(null)
const upgradeBackupRunning = ref(false)
const applyRunning = ref(false)

async function loadUpgrade(): Promise<void> {
  upgradeLoading.value = true
  try {
    upgrade.value = await opsApi.upgradeStatus()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    upgradeLoading.value = false
  }
}

async function onPackageChange(file: { raw: File } | undefined): Promise<void> {
  const raw = file?.raw
  if (!raw) return
  if (!raw.name.toLowerCase().endsWith('.zip')) {
    ElMessage.warning('升级包必须是 .zip 文件')
    return
  }
  // 先在本地量一下：超过后端上限的包传上去也是被拒，白白占几百 MB 上行。
  // 阈值与后端一致（512 MB），后端仍会再卡一次——前端校验只是省一次白传。
  if (raw.size > MAX_PACKAGE_BYTES) {
    ElMessage.warning(`升级包不能超过 ${MAX_PACKAGE_MB} MB，当前 ${(raw.size / 1024 / 1024).toFixed(1)} MB`)
    return
  }
  uploadRunning.value = true
  try {
    const res = await opsApi.uploadPackage(raw)
    plan.value = res.plan
    planRecord.value = res.record
    ElMessage.success(`升级包校验通过：${res.plan.fromVersion} → ${res.plan.toVersion}`)
    await loadUpgrade()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    uploadRunning.value = false
  }
}

async function runUpgradeBackup(): Promise<void> {
  if (!planRecord.value) return
  upgradeBackupRunning.value = true
  try {
    const item = await opsApi.upgradeBackup(planRecord.value.planId)
    ElMessage.success(`升级前备份完成：${item.name}`)
    if (planRecord.value) planRecord.value.backupPath = item.path
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    upgradeBackupRunning.value = false
  }
}

async function applyUpgrade(): Promise<void> {
  if (!planRecord.value) return
  const { value } = await ElMessageBox.prompt(
    '将替换程序与前端产物，且必须重启后才生效。请输入「确认升级」以继续。',
    '执行系统升级',
    {
      type: 'warning',
      confirmButtonText: '执行升级',
      cancelButtonText: '取消',
      inputPlaceholder: '确认升级',
      inputValidator: (v: string) => (v.trim() === '确认升级' ? true : '请准确输入「确认升级」'),
    },
  )
  if (value?.trim() !== '确认升级') return

  applyRunning.value = true
  try {
    const rec = await opsApi.applyUpgrade(planRecord.value.planId, '确认升级')
    planRecord.value = rec
    ElMessage.success('升级已执行，请按提示重启服务')
    await loadUpgrade()
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    applyRunning.value = false
  }
}

const preflightAllPass = computed(() =>
  (upgrade.value?.preflight || []).every((p: PreflightItem) => p.pass),
)

function statusTagType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  if (status === 'success') return 'success'
  if (status === 'applied' || status === 'executing') return 'warning'
  if (status === 'failed') return 'danger'
  return 'info'
}

function statusText(status: string): string {
  const map: Record<string, string> = {
    pending: '待执行',
    executing: '执行中',
    applied: '已执行待重启',
    success: '已完成',
    failed: '未生效',
  }
  return map[status] || status
}

onMounted(async () => {
  await Promise.all([loadBackups(), loadCheck(), loadUpgrade(), loadBizScopes(), refreshBizBackupAge()])
})
</script>

<template>
  <div class="ops-page">
    <el-tabs v-model="activeTab">
      <!-- ═══════════ 备份管理 ═══════════ -->
      <el-tab-pane label="备份管理" name="backup">
        <div class="page-card">
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>数据备份</h3>
              <span class="text-muted">共 {{ backups.length }} 份 · 备份与数据应在不同磁盘</span>
            </div>
            <div class="toolbar__actions">
              <el-button :loading="backupsLoading" @click="loadBackups">
                <AppIcon name="Refresh" /><span>刷新</span>
              </el-button>
              <el-button v-perm="'ops:backup'" type="primary" :loading="backupRunning" @click="runBackup(false)">
                <AppIcon name="Download" /><span>立即备份</span>
              </el-button>
            </div>
          </div>

          <el-table :data="backups" :loading="backupsLoading" border stripe>
            <el-table-column label="备份文件" min-width="260" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="mono">{{ row.name }}</span>
              </template>
            </el-table-column>
            <el-table-column label="备份时间" width="170">
              <template #default="{ row }">
                <div>{{ formatDateTime(row.time) }}</div>
                <div class="text-muted">{{ fromNow(row.time) }}</div>
              </template>
            </el-table-column>
            <el-table-column label="大小" width="110" align="right">
              <template #default="{ row }">{{ humanSize(row.size) }}</template>
            </el-table-column>
            <el-table-column label="表数" width="80" align="right">
              <template #default="{ row }">{{ row.hasMeta ? row.tables : '-' }}</template>
            </el-table-column>
            <el-table-column label="校验" width="90" align="center">
              <template #default="{ row }">
                <el-tooltip
                  v-if="row.hasMeta && !row.verified"
                  content="写完后未回读校验（缺少 pg_restore），文件未验过"
                  placement="top"
                >
                  <el-tag type="warning" size="small">未校验</el-tag>
                </el-tooltip>
                <span v-else-if="row.hasMeta" class="text-muted">已校验</span>
                <span v-else class="text-muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="摘要" width="150">
              <template #default="{ row }">
                <span class="mono" :title="row.sha256">{{ row.hasMeta ? shortSha(row.sha256) : '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="{ row }">
                <el-button
                  v-for="act in backupActions(row as unknown as BackupItem)"
                  :key="act.key"
                  v-perm="act.perm"
                  link
                  :type="act.type"
                  :loading="act.loading"
                  @click="act.onClick"
                >
                  {{ act.label }}
                </el-button>
              </template>
            </el-table-column>
            <template #empty>
              <span class="text-muted">还没有备份。建议先点一次「立即备份」确认环境可用。</span>
            </template>
          </el-table>
        </div>
      </el-tab-pane>

      <!-- ═══════════ 数据清理 ═══════════ -->
      <el-tab-pane label="数据清理" name="cleanup">
        <div class="page-card">
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>数据清理</h3>
              <span class="text-muted">只清理日志与过期幂等键，业务数据永不自动删除</span>
            </div>
            <div class="toolbar__actions">
              <el-button :loading="cleanupLoading" @click="previewCleanup">
                <AppIcon name="Search" /><span>预演（不删数据）</span>
              </el-button>
              <el-button v-perm="'ops:cleanup'" type="danger" :loading="cleanupApplying" @click="applyCleanup">
                <AppIcon name="Delete" /><span>执行清理</span>
              </el-button>
            </div>
          </div>

          <el-alert type="info" :closable="false" show-icon class="ops-alert">
            <template #title>
              预演只统计不删除；真删需输入「确认清理」。登录日志会额外避开账号锁定窗口内的记录，
              避免清理动作把账号锁定一起解除。
            </template>
          </el-alert>

          <el-form class="filter-form" label-width="140px" @submit.prevent>
            <el-form-item label="操作日志保留天数">
              <el-input-number v-model="cleanupDays.opLogDays" :min="1" :max="3650" :controls="false" placeholder="留空用默认" />
            </el-form-item>
            <el-form-item label="登录日志保留天数">
              <el-input-number v-model="cleanupDays.loginLogDays" :min="1" :max="3650" :controls="false" placeholder="留空用默认" />
            </el-form-item>
            <el-form-item label="幂等键保留天数">
              <el-input-number v-model="cleanupDays.idempotencyDays" :min="1" :max="365" :controls="false" placeholder="留空用默认" />
            </el-form-item>
          </el-form>

          <el-alert
            v-if="cleanupView?.blocked"
            type="warning"
            :closable="false"
            show-icon
            class="ops-alert"
            :title="cleanupView.blocked"
          />

          <el-table v-if="cleanupView" :data="cleanupView.steps" border>
            <el-table-column label="对象" min-width="140">
              <template #default="{ row }">{{ row.name }}</template>
            </el-table-column>
            <el-table-column prop="table" label="表" min-width="170" />
            <el-table-column label="删除边界" width="180">
              <template #default="{ row }">{{ formatDateTime(row.cutoff) }}</template>
            </el-table-column>
            <el-table-column label="行数" width="100" align="right">
              <template #default="{ row }">
                <b>{{ row.rows }}</b>
              </template>
            </el-table-column>
            <el-table-column label="批次" width="80" align="right">
              <template #default="{ row }">{{ row.dryRun || row.batches === 0 ? '-' : row.batches }}</template>
            </el-table-column>
            <el-table-column label="说明" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">{{ row.note || '-' }}</template>
            </el-table-column>
            <template #empty>
              <span class="text-muted">暂无可清理的数据</span>
            </template>
          </el-table>

          <div v-if="cleanupView" class="ops-summary">
            <span>
              模式：<el-tag :type="cleanupView.dryRun ? 'info' : 'success'" size="small">
                {{ cleanupView.dryRun ? '预演' : '已执行' }}
              </el-tag>
            </span>
            <span>合计将删除 <b>{{ totalCleanupRows(cleanupView.steps) }}</b> 行</span>
            <span v-if="cleanupView.vacuumed?.length">已回收空间：{{ cleanupView.vacuumed.join('、') }}</span>
          </div>
        </div>
      </el-tab-pane>

      <!-- ═══════════ 业务数据清理 ═══════════ -->
      <div class="ops-biz">
        <el-alert type="error" :closable="false" show-icon class="ops-alert">
          <template #title>
            业务数据清理是<b>不可逆的硬删除</b>，与上面的日志清理不是一回事：
            执行前 24 小时内必须有备份（回滚点），且需要输入该范围专属的确认口令。
            系统框架（账号 / 角色 / 菜单 / 权限 / 部门）在任何范围下都不会被删除。
          </template>
        </el-alert>

        <el-radio-group v-model="bizScopeKey" class="ops-biz__scopes">
          <!--
            用 el-radio 而不是 el-radio-button：后者的插槽内容渲染在
            .el-radio-button__inner 里，改造成卡片时必须把那个 span 藏掉，
            结果连卡片内容一起藏了（第一版就踩了这个坑，卡片整块消失）。
          -->
          <el-radio
            v-for="sc in bizScopes"
            :key="sc.key"
            :value="sc.key"
            class="ops-biz__scope"
          >
            <div class="ops-biz__scope-title">
              <span>{{ sc.title }}</span>
              <span class="text-muted">共 {{ sc.totalRows }} 行</span>
            </div>
            <div class="ops-biz__scope-desc">{{ sc.desc }}</div>
            <div class="ops-biz__scope-keep">保留：{{ sc.keep }}</div>
          </el-radio>
        </el-radio-group>

        <div class="ops-biz__tables">
          <div class="ops-biz__tables-head">
            <span>将删除以下表（当前行数）</span>
            <span class="text-muted">顺序即依赖顺序：子表在前</span>
          </div>
          <el-table :data="currentBizScope?.tables || []" border size="small" :loading="bizLoading">
            <el-table-column prop="name" label="表" min-width="200" />
            <el-table-column label="行数" width="120" align="right">
              <template #default="{ row }"><b>{{ row.rows }}</b></template>
            </el-table-column>
            <template #empty><span class="text-muted">该范围没有数据</span></template>
          </el-table>
        </div>

        <div class="ops-biz__gate">
          <el-checkbox
            v-if="currentBizScope?.resetStockable"
            v-model="bizResetStock"
          >
            同时清零商品规格库存（inv_product_specs.stock）
          </el-checkbox>
          <span v-else class="text-muted">该范围会连商品一起清空，无需单独处理库存</span>
        </div>

        <div class="ops-biz__actions">
          <span class="text-muted">
            回滚点：{{ bizBackupAge || '读取中…' }}（要求 24 小时内）
          </span>
          <span class="ops-biz__btns">
            <el-button :loading="bizBackupRunning" @click="runBizBackup">
              <AppIcon name="Download" /><span>先备份</span>
            </el-button>
            <el-button
              v-perm="'ops:reset'"
              type="danger"
              :loading="bizRunning"
              :disabled="!currentBizScope || currentBizScope.totalRows === 0"
              @click="applyBizCleanup"
            >
              <AppIcon name="Delete" /><span>执行业务清理</span>
            </el-button>
          </span>
        </div>

        <el-table v-if="bizResult" :data="bizResult.tables" border size="small" class="ops-biz__result">
          <el-table-column prop="name" label="表" min-width="200" />
          <el-table-column label="清理前" width="110" align="right">
            <template #default="{ row }">{{ row.rows }}</template>
          </el-table-column>
          <el-table-column label="已删除" width="110" align="right">
            <template #default="{ row }"><b>{{ row.deleted }}</b></template>
          </el-table-column>
          <el-table-column label="序号重置" width="110" align="center">
            <template #default="{ row }">
              <el-tag :type="row.seqReset ? 'success' : 'info'" size="small">{{ row.seqReset ? '是' : '否' }}</el-tag>
            </template>
          </el-table-column>
          <template #empty><span class="text-muted">无可删除数据</span></template>
        </el-table>
        <div v-if="bizResult" class="ops-summary">
          <span>
            模式：<el-tag :type="bizResult.dryRun ? 'info' : 'success'" size="small">
              {{ bizResult.dryRun ? '预演' : '已执行' }}
            </el-tag>
          </span>
          <span>合计删除 <b>{{ bizResult.totalDeleted }}</b> 行</span>
          <span v-if="bizResult.stockReset">已清零规格库存 <b>{{ bizResult.stockResetRows }}</b> 行</span>
        </div>
      </div>

      <!-- ═══════════ 系统巡检 ═══════════ -->
      <el-tab-pane label="系统巡检" name="check">
        <div class="page-card">
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>系统体检</h3>
              <span class="text-muted">结构版本 / 体积 / 日志膨胀 / 备份新鲜度</span>
            </div>
            <div class="toolbar__actions">
              <el-button :loading="checkLoading" @click="loadCheck">
                <AppIcon name="Refresh" /><span>重新体检</span>
              </el-button>
            </div>
          </div>

          <div v-if="checkReport" class="stat-grid">
            <div class="stat-grid__item">
              <span class="stat-grid__label">数据库</span>
              <span class="stat-grid__value">{{ checkReport.database }}</span>
              <span class="text-muted">PostgreSQL {{ checkReport.serverVersion }}</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">结构版本</span>
              <span class="stat-grid__value">
                库 v{{ checkReport.schemaVersionDb }} / 程序 v{{ checkReport.schemaVersionBinary }}
              </span>
              <span class="text-muted">{{ checkReport.schemaVersionKnown ? '' : '版本表未登记' }}</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">库体积</span>
              <span class="stat-grid__value">{{ humanSize(checkReport.sizeBytes) }}</span>
              <span class="text-muted">{{ checkReport.tables }} 张表</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">最近备份</span>
              <span class="stat-grid__value">{{ checkReport.latestBackupAt ? fromNow(checkReport.latestBackupAt) : '无' }}</span>
              <span class="text-muted">{{ checkReport.latestBackup || '尚未备份' }}</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">操作日志</span>
              <span class="stat-grid__value">{{ checkReport.opLogRows }}</span>
              <span class="text-muted">最早 {{ checkReport.opLogOldest || '-' }}</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">登录日志 / 幂等键</span>
              <span class="stat-grid__value">{{ checkReport.loginLogRows }} / {{ checkReport.idempotencyRows }}</span>
              <span class="text-muted">已过期幂等键 {{ checkReport.idempotencyExpired }}</span>
            </div>
          </div>

          <el-alert
            v-if="checkWarnings.length"
            type="warning"
            :closable="false"
            show-icon
            class="ops-alert"
          >
            <template #title>
              <div v-for="(w, i) in checkWarnings" :key="i">{{ w }}</div>
            </template>
          </el-alert>
          <el-alert v-else type="success" :closable="false" show-icon class="ops-alert" title="没有告警项" />

          <el-table v-if="keyTables.length" :data="keyTables" border>
            <el-table-column prop="name" label="表" min-width="180" />
            <el-table-column label="存活行" width="110" align="right">
              <template #default="{ row }">{{ row.rows }}</template>
            </el-table-column>
            <el-table-column label="软删除" width="110" align="right">
              <template #default="{ row }">{{ row.softDeleted }}</template>
            </el-table-column>
            <el-table-column label="体积" width="120" align="right">
              <template #default="{ row }">{{ humanSize(row.sizeBytes) }}</template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <!-- ═══════════ 系统升级 ═══════════ -->
      <el-tab-pane label="系统升级" name="upgrade">
        <div class="page-card">
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>系统升级</h3>
              <span class="text-muted">上传升级包 → 升级前备份 → 执行 → 重启生效</span>
            </div>
            <div class="toolbar__actions">
              <el-button :loading="upgradeLoading" @click="loadUpgrade">
                <AppIcon name="Refresh" /><span>刷新</span>
              </el-button>
            </div>
          </div>

          <div v-if="upgrade" class="stat-grid">
            <div class="stat-grid__item">
              <span class="stat-grid__label">程序版本</span>
              <span class="stat-grid__value">{{ upgrade.release }}</span>
              <span class="text-muted">由构建脚本注入，dev 表示未注入</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">结构版本</span>
              <span class="stat-grid__value">库 v{{ upgrade.schemaDb }} / 程序 v{{ upgrade.schemaBinary }}</span>
              <span class="text-muted">{{ upgrade.schemaNote }}</span>
            </div>
            <div class="stat-grid__item">
              <span class="stat-grid__label">升级前备份</span>
              <span class="stat-grid__value">{{ upgrade.latestBackupAt ? fromNow(upgrade.latestBackupAt) : '无' }}</span>
              <span class="text-muted">{{ upgrade.latestBackup || '尚未备份' }}</span>
            </div>
          </div>

          <el-alert
            v-if="upgrade && !preflightAllPass"
            type="warning"
            :closable="false"
            show-icon
            class="ops-alert"
            title="存在未通过的预检项，请先处理再升级"
          />

          <div class="preflight">
            <div v-for="p in upgrade?.preflight || []" :key="p.key" class="preflight__item" :class="{ 'preflight__item--fail': !p.pass }">
              <AppIcon :name="p.pass ? 'CircleCheck' : 'Warning'" />
              <div class="preflight__body">
                <span class="preflight__title">{{ p.title }}</span>
                <span class="text-muted">{{ p.hint }}</span>
              </div>
            </div>
          </div>

          <!-- 步骤一：上传升级包 -->
          <div class="ops-step">
            <div class="ops-step__head">
              <span class="ops-step__no">1</span>
              <span class="ops-step__title">上传升级包</span>
              <span class="text-muted">zip，内含 manifest.json 与后端二进制 / 前端产物</span>
            </div>
            <el-upload
              v-perm="'ops:upgrade'"
              :auto-upload="true"
              :show-file-list="false"
              :limit="1"
              accept=".zip"
              :http-request="({ file }) => onPackageChange({ raw: file as File })"
            >
              <el-button :loading="uploadRunning" type="primary">
                <AppIcon name="Upload" /><span>选择升级包</span>
              </el-button>
            </el-upload>
          </div>

          <!-- 升级计划 -->
          <div v-if="plan" class="ops-plan">
            <div class="ops-plan__head">
              <span>升级计划：<b class="mono">{{ plan.fromVersion }} → {{ plan.toVersion }}</b></span>
              <span class="text-muted mono" :title="plan.packageSha256">{{ shortSha(plan.packageSha256) }}</span>
            </div>
            <p v-if="plan.notes" class="ops-plan__notes">{{ plan.notes }}</p>
            <ul class="ops-plan__actions">
              <li v-for="a in plan.actions" :key="a.key">
                <AppIcon name="CircleCheck" />
                <b>{{ a.title }}</b>
                <span class="text-muted">{{ a.detail }}</span>
              </li>
            </ul>
            <el-alert
              v-for="(w, i) in plan.warnings"
              :key="i"
              type="warning"
              :closable="false"
              show-icon
              class="ops-alert"
              :title="w"
            />
          </div>

          <!-- 步骤二：升级前备份 -->
          <div class="ops-step">
            <div class="ops-step__head">
              <span class="ops-step__no">2</span>
              <span class="ops-step__title">升级前备份</span>
              <span class="text-muted">没有回滚点不允许升级</span>
            </div>
            <el-button
              v-if="planRecord"
              v-perm="'ops:upgrade'"
              :loading="upgradeBackupRunning"
              :disabled="planRecord.status !== 'pending'"
              @click="runUpgradeBackup"
            >
              <AppIcon name="Download" /><span>立即备份并留档</span>
            </el-button>
            <span v-if="planRecord?.backupPath" class="text-muted mono ops-step__hint">{{ planRecord.backupPath }}</span>
            <span v-else-if="planRecord" class="text-muted ops-step__hint">尚未备份</span>
          </div>

          <!-- 步骤三：执行升级 -->
          <div class="ops-step">
            <div class="ops-step__head">
              <span class="ops-step__no">3</span>
              <span class="ops-step__title">执行升级</span>
              <span class="text-muted">替换程序与前端产物，需重启后生效</span>
            </div>
            <el-button
              v-if="planRecord"
              v-perm="'ops:upgrade'"
              type="danger"
              :loading="applyRunning"
              :disabled="planRecord.status !== 'pending' || !planRecord.backupPath"
              @click="applyUpgrade"
            >
              <AppIcon name="SwitchButton" /><span>执行升级</span>
            </el-button>
            <span v-if="planRecord && planRecord.status === 'applied'" class="text-muted ops-step__hint">
              已执行，等待重启生效
            </span>
          </div>

          <!-- 步骤四：重启 -->
          <div v-if="planRecord && planRecord.status === 'applied'" class="ops-step">
            <div class="ops-step__head">
              <span class="ops-step__no">4</span>
              <span class="ops-step__title">重启生效</span>
              <span class="text-muted">重启后由启动流程自动核对版本，记录转为「已完成」</span>
            </div>
            <el-alert type="info" :closable="false" show-icon class="ops-alert">
              <template #title>
                新程序已落位为 <span class="mono">nova-erp.exe.new</span>，前端产物已切换。
                请停止当前服务，把 .new 替换为正式文件名后重新启动；
                若不升级，删掉 .new 并还原前端目录即可回滚。
              </template>
            </el-alert>
          </div>

          <!-- 升级记录 -->
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>升级记录</h3>
              <span class="text-muted">共 {{ upgrade?.records?.length || 0 }} 条</span>
            </div>
          </div>
          <el-table :data="upgrade?.records || []" border>
            <el-table-column label="计划" min-width="220">
              <template #default="{ row }">
                <span class="mono">{{ row.planId }}</span>
              </template>
            </el-table-column>
            <el-table-column label="版本" width="140">
              <template #default="{ row }">
                <span class="mono">{{ row.fromVersion }} → {{ row.toVersion }}</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="120" align="center">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="升级前备份" min-width="220" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="mono">{{ row.backupPath || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="说明" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">{{ row.message || '-' }}</template>
            </el-table-column>
            <el-table-column label="时间" width="170">
              <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 恢复演练弹窗 -->
    <el-dialog v-model="restoreVisible" title="恢复演练" width="560px">
      <el-alert type="warning" :closable="false" show-icon class="ops-alert"
        title="备份将恢复到一个新库，不会覆盖生产库。目标库名已存在时会被拒绝。" />
      <el-form label-width="100px">
        <el-form-item label="备份文件">
          <span class="mono">{{ restoreFile }}</span>
        </el-form-item>
        <el-form-item label="目标库名">
          <el-input v-model="restoreTarget" placeholder="留空自动生成 <生产库>_restore_<时间戳>" />
        </el-form-item>
      </el-form>

      <div v-if="restoreResult" class="ops-restore-result">
        <p>已恢复到 <b class="mono">{{ restoreResult.Target }}</b>，共 {{ restoreResult.Tables }} 张表。</p>
        <p v-for="(v, k) in restoreResult.Rows" :key="k">{{ k }}：{{ v }} 行</p>
        <el-alert
          v-for="(w, i) in restoreResult.Warn"
          :key="i"
          type="warning"
          :closable="false"
          show-icon
          class="ops-alert"
          :title="w"
        />
        <p class="text-muted">
          核对无误后请删除该库：DROP DATABASE "{{ restoreResult.Target }}";
        </p>
      </div>

      <template #footer>
        <el-button @click="restoreVisible = false">关闭</el-button>
        <el-button v-perm="'ops:restore'" type="primary" :loading="restoreRunning" @click="doRestore">
          开始恢复
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.ops-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ops-alert {
  margin-bottom: 16px;
}

.ops-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 24px;
  align-items: center;
  margin-top: 16px;
  color: var(--el-text-color-regular);
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.92em;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 16px;

  &__item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 12px 14px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: var(--el-border-radius-base);
    background: var(--el-fill-color-lighter);
  }

  &__label {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }

  &__value {
    font-size: 16px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }
}

.preflight {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 12px;
  margin-bottom: 16px;

  &__item {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    padding: 10px 12px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: var(--el-border-radius-base);
    color: var(--el-color-success);

    &--fail {
      color: var(--el-color-warning);
      border-color: var(--el-color-warning-light-7);
      background: var(--el-color-warning-light-9);
    }
  }

  &__body {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  &__title {
    font-size: 13px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }
}

.ops-step {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  padding: 14px 16px;
  margin-bottom: 12px;
  border: 1px dashed var(--el-border-color);
  border-radius: var(--el-border-radius-base);

  &__head {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    flex: 1 1 100%;
  }

  &__no {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: var(--el-color-primary);
    color: #fff;
    font-size: 12px;
    font-weight: 600;
  }

  &__title {
    font-size: 14px;
    font-weight: 600;
  }

  &__hint {
    flex: 1 1 100%;
    font-size: 12px;
  }
}

.ops-plan {
  padding: 14px 16px;
  margin-bottom: 12px;
  border: 1px solid var(--el-color-primary-light-7);
  border-radius: var(--el-border-radius-base);
  background: var(--el-color-primary-light-9);

  &__head {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    font-size: 14px;
  }

  &__notes {
    margin: 8px 0;
    font-size: 13px;
    color: var(--el-text-color-regular);
    white-space: pre-wrap;
  }

  &__actions {
    margin: 0;
    padding: 0;
    list-style: none;

    li {
      display: flex;
      gap: 8px;
      align-items: center;
      padding: 4px 0;
      font-size: 13px;
      color: var(--el-color-success);
    }

    b {
      color: var(--el-text-color-primary);
    }
  }
}

.ops-biz {
  margin-top: 20px;
  padding-top: 20px;
  border-top: 1px solid var(--el-border-color-lighter);

  &__scopes {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 12px;
    width: 100%;
    margin-bottom: 16px;
  }

  // 三档范围的影响面差异很大，必须一眼看全「删什么、留什么」再选，
  // 所以把默认的一排小圆点改成卡片。
  &__scope {
    margin: 0 !important;
    width: 100%;
    height: auto !important;
    padding: 12px 14px;
    border: 1px solid var(--el-border-color);
    border-radius: var(--el-border-radius-base);
    background: var(--el-fill-color-lighter);
    align-items: flex-start;

    :deep(.el-radio__input) {
      margin-top: 3px;
    }

    :deep(.el-radio__label) {
      flex: 1;
      padding-left: 8px;
      line-height: 1.5;
      white-space: normal;
    }
  }

  // 选中态：描边与底色都要变，只改边框在暗色下几乎看不出来
  &__scope.is-checked {
    border-color: var(--el-color-danger);
    background: var(--el-color-danger-light-9);
  }

  &__scope-title {
    display: flex;
    flex-wrap: wrap; // 窄屏时让行数换行，而不是被裁掉半个数字
    justify-content: space-between;
    gap: 4px 8px;
    font-size: 14px;
    font-weight: 600;
    color: var(--el-text-color-primary);
  }

  &__scope-desc {
    margin-top: 6px;
    font-size: 12px;
    color: var(--el-text-color-regular);
  }

  &__scope-keep {
    margin-top: 4px;
    font-size: 12px;
    color: var(--el-color-success);
  }

  &__tables {
    margin-bottom: 16px;

    &-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 8px;
      font-size: 13px;
      color: var(--el-text-color-regular);
    }
  }

  &__gate {
    margin-bottom: 12px;
  }

  &__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
    font-size: 13px;
  }

  &__btns {
    display: flex;
    gap: 8px;
  }

  &__result {
    margin-bottom: 12px;
  }
}

.ops-restore-result {
  margin-top: 12px;
  padding: 12px;
  border-radius: var(--el-border-radius-base);
  background: var(--el-fill-color-lighter);

  p {
    margin: 4px 0;
  }
}
</style>
