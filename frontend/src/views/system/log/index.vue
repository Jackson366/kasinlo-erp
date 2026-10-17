<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { useTableQuery } from '@/composables/useTableQuery'
import { logApi, type LoginLogQuery, type OperationLogQuery } from '@/api/log'
import type { LoginLogItem, OperationLogItem } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import { exportStamp, formatDateTime, formatLatency, fromNow } from '@/utils/format'

defineOptions({ name: 'SystemLog' })

const activeTab = ref<'operation' | 'login'>('operation')

// ---------------- 操作日志 ----------------
const opModules = ref<string[]>([])

const methodOptions = ['POST', 'PUT', 'DELETE', 'PATCH']

const {
  query: opQuery,
  loading: opLoading,
  rows: opRows,
  total: opTotal,
  load: loadOperations,
  search: searchOperations,
  reset: resetOperations,
} = useTableQuery<OperationLogQuery, OperationLogItem>(
  { keyword: '', module: undefined, method: undefined, status: undefined },
  (params) => logApi.operations({ ...params, keyword: params.keyword || undefined }),
)

// ---------------- 登录日志 ----------------
const {
  query: loginQuery,
  loading: loginLoading,
  rows: loginRows,
  total: loginTotal,
  load: loadLogins,
  search: searchLogins,
  reset: resetLogins,
} = useTableQuery<LoginLogQuery, LoginLogItem>(
  { keyword: '', success: undefined },
  (params) => logApi.logins({ ...params, keyword: params.keyword || undefined }),
)

// ---------------- 导出 ----------------
const exportingOp = ref(false)
const exportingLogin = ref(false)

/**
 * 导出日志。
 * 导出的是「当前筛选条件下的全部数据」而非当前页，因此这里把查询条件原样传给后端；
 * 分页参数对导出接口无影响，后端只按筛选条件取数。
 */
async function exportOperations(): Promise<void> {
  exportingOp.value = true
  try {
    await logApi.exportOperations(
      { ...opQuery, keyword: opQuery.keyword || undefined },
      exportStamp('操作日志'),
    )
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exportingOp.value = false
  }
}

async function exportLogins(): Promise<void> {
  exportingLogin.value = true
  try {
    await logApi.exportLogins(
      { ...loginQuery, keyword: loginQuery.keyword || undefined },
      exportStamp('登录日志'),
    )
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exportingLogin.value = false
  }
}

// ---------------- 清空 ----------------
async function clearLogs(): Promise<void> {
  const isOperation = activeTab.value === 'operation'
  const label = isOperation ? '操作日志' : '登录日志'
  try {
    await ElMessageBox.confirm(`确定要清空全部${label}吗？此操作不可恢复。`, '清空确认', {
      type: 'warning',
      confirmButtonText: '清空',
    })
  } catch {
    return
  }
  const result = isOperation ? await logApi.clearOperations() : await logApi.clearLogins()
  ElMessage.success(`已清空 ${result.deleted} 条${label}`)
  if (isOperation) {
    loadOperations()
  } else {
    loadLogins()
  }
}

/** 加载模块下拉选项 */
async function loadModules(): Promise<void> {
  try {
    opModules.value = await logApi.modules()
  } catch {
    opModules.value = []
  }
}

// ---------------- 按需加载 ----------------
/**
 * 哪个 tab 已经拉过数据。
 *
 * 原实现在 onMounted 里 `Promise.all([loadOperations(), loadLogins(), loadModules()])`
 * —— 三个请求同时发出，但默认只显示「操作日志」，登录日志那一屏用户根本看不到。
 * 日志页首屏白等一次登录日志的往返，而它还会拖慢首屏表格的 loading 结束时机。
 */
const operationLoaded = ref(false)
const loginLoaded = ref(false)

async function ensureOperations(): Promise<void> {
  if (operationLoaded.value) return
  try {
    await loadOperations()
    operationLoaded.value = true // 只有成功才标记：失败后切 tab 还能重试
  } catch {
    // 失败提示由响应拦截器统一处理；不标记，留给下一次进入 tab 重试
  }
}

async function ensureLogins(): Promise<void> {
  if (loginLoaded.value) return
  try {
    await loadLogins()
    loginLoaded.value = true // 只有成功才标记：失败后切 tab 还能重试
  } catch {
    // 失败提示由响应拦截器统一处理；不标记，留给下一次进入 tab 重试
  }
}

/**
 * 切换 tab：首次进入才加载，之后直接用已有数据。
 *
 * 数据「陈旧」由刷新动作负责，而不是靠每次切 tab 重拉 —— 日志页切 tab 是高频操作，
 * 每次都发请求既浪费又会打断正在进行的阅读。
 */
async function handleTabChange(name: string | number): Promise<void> {
  if (name === 'operation') {
    await ensureOperations()
  } else if (name === 'login') {
    await ensureLogins()
  }
}

/** 状态码对应的标签类型 */
function statusTagType(status: number): 'success' | 'warning' | 'danger' {
  if (status < 300) return 'success'
  if (status < 500) return 'warning'
  return 'danger'
}

// ---------------- 打印 ----------------
const printVisible = ref(false)
const printKind = ref<'operation' | 'login'>('operation')

function openPrint(kind: 'operation' | 'login'): void {
  printKind.value = kind
  printVisible.value = true
}

/** 日志打印统一取当前 tab 的当前页：日志量太大，打全量既慢也没人看 */
const printRows = computed(() =>
  printKind.value === 'operation' ? opRows.value : loginRows.value,
)
const printColumns = computed<ListPrintColumn<Record<string, any>>[]>(() =>
  printKind.value === 'operation'
    ? [
        { label: '账号', value: (r) => r.username || '匿名' },
        { label: '模块', value: (r) => r.module || '-' },
        { label: '操作', value: (r) => r.action || '-' },
        { label: '方法', value: (r) => r.method || '-' },
        { label: '路径', value: (r) => r.path || '-' },
        { label: 'IP', value: (r) => r.ip || '-' },
        { label: '状态', value: (r) => String(r.status ?? '-'), align: 'center' },
        { label: '耗时(ms)', value: (r) => String(r.latencyMs ?? '-'), align: 'right' },
        { label: '时间', value: (r) => formatDateTime(r.createdAt) },
      ]
    : [
        { label: '账号', value: (r) => r.username || '-' },
        { label: '登录 IP', value: (r) => r.ip || '-' },
        { label: '结果', value: (r) => (r.success ? '成功' : '失败') },
        { label: '说明', value: (r) => r.message || '-' },
        { label: '客户端', value: (r) => r.userAgent || '-' },
        { label: '时间', value: (r) => formatDateTime(r.createdAt) },
      ],
)
const printSubtitle = computed(() =>
  printKind.value === 'operation' ? '操作日志（当前页）' : '登录日志（当前页）',
)

onMounted(async () => {
  // 首屏只有「操作日志」可见：只加载它 + 模块下拉，登录日志等切过去再说
  await Promise.all([ensureOperations(), loadModules()])
})
</script>

<template>
  <div class="log-page">
    <div class="page-card">
      <el-tabs v-model="activeTab" @tab-change="handleTabChange">
        <!-- 操作日志 -->
        <el-tab-pane label="操作日志" name="operation">
          <el-form class="search-bar filter-form" label-width="76px" @submit.prevent>
            <el-form-item label="关键字">
              <el-input
                v-model="opQuery.keyword"
                placeholder="账号 / 操作 / 路径"
                clearable
                
                @keyup.enter="searchOperations"
              />
            </el-form-item>
            <el-form-item label="模块">
              <el-select v-model="opQuery.module" placeholder="全部" clearable >
                <el-option v-for="item in opModules" :key="item" :label="item" :value="item" />
              </el-select>
            </el-form-item>
            <el-form-item label="方法">
              <el-select v-model="opQuery.method" placeholder="全部" clearable >
                <el-option v-for="item in methodOptions" :key="item" :label="item" :value="item" />
              </el-select>
            </el-form-item>
            <div class="filter-form__actions">
              <el-button type="primary" @click="searchOperations">
                <AppIcon name="Search" />
                <span>查询</span>
              </el-button>
              <el-button @click="resetOperations">重置</el-button>
              <el-button v-perm="'system:log:export'" :loading="exportingOp" @click="exportOperations">
                <AppIcon name="Download" />
                <span>导出</span>
              </el-button>
              <el-button @click="openPrint('operation')">打印</el-button>
              <el-button v-perm="'system:log:delete'" type="danger" plain @click="clearLogs">
                <AppIcon name="Delete" />
                <span>清空</span>
              </el-button>
            </div>
          </el-form>

          <el-table v-loading="opLoading" :data="opRows" stripe>
            <el-table-column type="expand">
              <template #default="{ row }">
                <div class="log-detail">
                  <div class="log-detail__item">
                    <span class="log-detail__label">请求体</span>
                    <pre class="log-detail__value">{{ row.request || '（无）' }}</pre>
                  </div>
                  <div class="log-detail__item">
                    <span class="log-detail__label">User-Agent</span>
                    <span class="log-detail__value">{{ row.userAgent || '-' }}</span>
                  </div>
                  <div v-if="row.errorMsg" class="log-detail__item">
                    <span class="log-detail__label">错误信息</span>
                    <span class="log-detail__value log-detail__value--error">{{ row.errorMsg }}</span>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="username" label="操作账号" min-width="120">
              <template #default="{ row }">{{ row.username || '匿名' }}</template>
            </el-table-column>
            <el-table-column prop="module" label="模块" width="110" />
            <el-table-column prop="action" label="操作" width="120" />
            <el-table-column label="请求" min-width="240">
              <template #default="{ row }">
                <span class="method" :class="`method--${row.method.toLowerCase()}`">{{ row.method }}</span>
                <span class="path">{{ row.path }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="ip" label="IP" min-width="130" />
            <el-table-column label="状态" width="90" align="center">
              <template #default="{ row }">
                <el-tag :type="statusTagType(row.status)" size="small" effect="light">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="耗时" width="100" align="right">
              <template #default="{ row }">
                <span class="text-secondary">{{ formatLatency(row.latencyMs) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="时间" width="180" align="right">
              <template #default="{ row }">
                <span class="text-secondary">{{ formatDateTime(row.createdAt) }}</span>
              </template>
            </el-table-column>
            <template #empty>
              <el-empty description="暂无操作日志" :image-size="90" />
            </template>
          </el-table>

          <Pagination
            v-model:page="opQuery.page!"
            v-model:page-size="opQuery.pageSize!"
            :total="opTotal"
            @change="loadOperations"
          />
        </el-tab-pane>

        <!-- 登录日志 -->
        <el-tab-pane label="登录日志" name="login">
          <el-form class="search-bar filter-form" label-width="76px" @submit.prevent>
            <el-form-item label="关键字">
              <el-input
                v-model="loginQuery.keyword"
                placeholder="账号 / IP / 说明"
                clearable
                style="width: 220px"
                @keyup.enter="searchLogins"
              />
            </el-form-item>
            <el-form-item label="结果">
              <el-select v-model="loginQuery.success" placeholder="全部" clearable >
                <el-option label="成功" :value="true" />
                <el-option label="失败" :value="false" />
              </el-select>
            </el-form-item>
            <div class="filter-form__actions">
              <el-button type="primary" @click="searchLogins">
                <AppIcon name="Search" />
                <span>查询</span>
              </el-button>
              <el-button @click="resetLogins">重置</el-button>
              <el-button v-perm="'system:log:export'" :loading="exportingLogin" @click="exportLogins">
                <AppIcon name="Download" />
                <span>导出</span>
              </el-button>
              <el-button @click="openPrint('login')">打印</el-button>
              <el-button v-perm="'system:log:delete'" type="danger" plain @click="clearLogs">
                <AppIcon name="Delete" />
                <span>清空</span>
              </el-button>
            </div>
          </el-form>

          <el-table v-loading="loginLoading" :data="loginRows" stripe>
            <el-table-column prop="username" label="账号" min-width="130" />
            <el-table-column prop="ip" label="登录 IP" min-width="140" />
            <el-table-column label="结果" width="100" align="center">
              <template #default="{ row }">
                <el-tag :type="row.success ? 'success' : 'danger'" size="small" effect="light">
                  {{ row.success ? '成功' : '失败' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="message" label="说明" min-width="140" />
            <el-table-column prop="userAgent" label="客户端" min-width="240" show-overflow-tooltip />
            <el-table-column label="时间" width="200" align="right">
              <template #default="{ row }">
                <span class="text-secondary">{{ formatDateTime(row.createdAt) }}（{{ fromNow(row.createdAt) }}）</span>
              </template>
            </el-table-column>
            <template #empty>
              <el-empty description="暂无登录日志" :image-size="90" />
            </template>
          </el-table>

          <Pagination
            v-model:page="loginQuery.page!"
            v-model:page-size="loginQuery.pageSize!"
            :total="loginTotal"
            @change="loadLogins"
          />
        </el-tab-pane>
      </el-tabs>
    </div>

    <ListPrintDialog
      v-model="printVisible"
      :title="printKind === 'operation' ? '操作日志' : '登录日志'"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />
  </div>
</template>

<style scoped lang="scss">
.log-page {
  :deep(.el-tabs__header) {
    margin-bottom: 16px;
  }
}

.log-detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 16px 12px 48px;

  &__item {
    display: flex;
    gap: 12px;
    font-size: 12.5px;
  }

  &__label {
    flex-shrink: 0;
    width: 78px;
    color: var(--app-text-muted);
  }

  &__value {
    min-width: 0;
    margin: 0;
    color: var(--app-text);
    word-break: break-all;
    white-space: pre-wrap;

    &--error {
      color: var(--app-danger);
    }
  }
}

.method {
  display: inline-block;
  min-width: 54px;
  margin-right: 8px;
  padding: 1px 6px;
  font-size: 11px;
  font-weight: 600;
  text-align: center;
  border-radius: 4px;

  &--post {
    color: var(--tone-blue);
    background: var(--tone-blue-soft);
  }

  &--put,
  &--patch {
    color: var(--tone-amber);
    background: var(--tone-amber-soft);
  }

  &--delete {
    color: var(--tone-red);
    background: var(--tone-red-soft);
  }

  &--get {
    color: var(--tone-green);
    background: var(--tone-green-soft);
  }
}

.path {
  font-size: 12.5px;
  color: var(--app-text-secondary);
  word-break: break-all;
}
</style>
