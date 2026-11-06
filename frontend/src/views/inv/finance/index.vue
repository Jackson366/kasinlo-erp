<script setup lang="ts">
import { KasTable, useKasTableSelection, useKasTableExport, type KasTableHandle } from '@/components/KasTable'
/**
 * 应收应付页：应收（销售）与应付（采购）两个视图，结构一致。
 * 汇总数据随列表接口一并返回，避免多打一次请求。
 */
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import { useRoute } from 'vue-router'
import { usePurchaseHandoff } from '@/composables/usePurchaseHandoff'
import PurchaseReturnLink from '../components/PurchaseReturnLink.vue'
import type { InvFinanceRow, InvFinanceSummary, InvSettlePayload } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import ListFilters from '@/components/ListFilters.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import { formatDay, formatMoney } from '@/utils/format'
import { useLatestGuard } from '@/composables/useLatestGuard'
import { useDebouncedSearch } from '@/composables/useDebouncedSearch'

defineOptions({ name: 'InvFinance' })

const route = useRoute()
const { returnTo, follow } = usePurchaseHandoff()
const loadError = ref(false)
let applyingHandoff = false
const activeTab = ref<'receivable' | 'payable'>('receivable')
const loading = ref(false)
const rows = ref<InvFinanceRow[]>([])
const total = ref(0)
const summary = ref<InvFinanceSummary>({
  total: 0,
  settled: 0,
  outstanding: 0,
  overdue: 0,
  orderTotal: 0,
  planTotal: 0,
})

/** 表格勾选（批量收付款用）。只允许勾「还有未结余额」的单据 */
const tableRef = ref<KasTableHandle>()
const { selection, onSelectionChange, clearSelection } = useKasTableSelection<InvFinanceRow>()

const query = reactive<{
  page: number
  pageSize: number
  keyword: string
  settle: string
  source: '' | 'order' | 'plan'
}>({
  page: 1,
  pageSize: 10,
  keyword: '',
  settle: '',
  source: '',
})

const settleMeta: Record<string, { text: string; type: 'danger' | 'warning' | 'success' }> = {
  unpaid: { text: '未结清', type: 'danger' },
  partial: { text: '部分结算', type: 'warning' },
  paid: { text: '已结清', type: 'success' },
}

const summaryCards = computed(() => [
  { label: activeTab.value === 'payable' ? '应付总额' : '应收总额', value: summary.value.total, tone: 'blue' },
  { label: '已结算', value: summary.value.settled, tone: 'green' },
  { label: '未结余额', value: summary.value.outstanding, tone: 'amber' },
  { label: '逾期金额', value: summary.value.overdue, tone: 'red' },
])

/** 总额的构成：单据 / 合同期次，两个「其中」口径 */
const summarySplit = computed(
  () => `其中 单据 ${formatMoney(summary.value.orderTotal)} ｜ 合同 ${formatMoney(summary.value.planTotal)}`,
)

const guard = useLatestGuard()

async function load(): Promise<void> {
  const ticket = guard.start()
  loading.value = true
  loadError.value = false
  clearSelection(tableRef.value)
  try {
    const params = {
      page: query.page,
      pageSize: query.pageSize,
      keyword: query.keyword || undefined,
      settle: query.settle || undefined,
      source: query.source || undefined,
    }
    const res =
      activeTab.value === 'receivable'
        ? await invApi.receivables(params)
        : await invApi.payables(params)
    // 快速切 tab / 连续回车时，慢响应不能覆盖新响应
    if (!guard.isCurrent(ticket)) return
    rows.value = res.list
    total.value = res.total
    summary.value = res.summary
    // 列表换了，勾选必须清空：留着旧行会让批量操作作用到看不见的单据上
    clearSelection(tableRef.value)
  } catch {
    if (guard.isCurrent(ticket)) {
      loadError.value = true
      rows.value = []
      total.value = 0
      summary.value = { total: 0, settled: 0, outstanding: 0, overdue: 0, orderTotal: 0, planTotal: 0 }
    }
  } finally {
    if (guard.isCurrent(ticket)) loading.value = false
  }
}

function search(): void {
  query.page = 1
  load()
}

function reset(): void {
  query.keyword = ''
  query.settle = ''
  query.source = ''
  query.page = 1
  load()
}

function onTabChange(): void {
  if (applyingHandoff) return
  query.page = 1
  load()
}

// ---------------- 登记收付款 ----------------
const formRef = ref<FormInstance>()
const dialogVisible = ref(false)
const submitting = ref(false)
const currentRow = ref<InvFinanceRow | null>(null)
const settlementVerb = ref<'收款' | '付款'>('收款')
const activeSettlementVerb = computed(() => activeTab.value === 'receivable' ? '收款' : '付款')

const form = reactive<InvSettlePayload>({
  orderId: 0,
  amount: 0,
  method: '银行转账',
  settledAt: '',
  remark: '',
})

const settlementPreview = computed(() => {
  const amount = Number(form.amount)
  if (!currentRow.value || !Number.isFinite(amount) || amount < 0.01 || amount > currentRow.value.outstanding) return null
  return { amount, remaining: currentRow.value.outstanding - amount }
})

const rules: FormRules = {
  amount: [{ required: true, message: '请输入金额', trigger: 'blur' }],
}

function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function openSettle(row: InvFinanceRow): void {
  currentRow.value = row
  settlementVerb.value = activeSettlementVerb.value
  Object.assign(form, {
    orderId: row.orderId,
    amount: row.outstanding,
    method: '银行转账',
    settledAt: today(),
    remark: '',
  })
  dialogVisible.value = true
}

async function submitForm(): Promise<void> {
  if (submitting.value) return
  const valid = formRef.value ? await formRef.value.validate().catch(() => false) : false
  if (!valid || !dialogVisible.value || submitting.value) return
  if (!settlementPreview.value) {
    ElMessage.warning('请输入有效金额，且金额不能超过未结余额')
    return
  }
  submitting.value = true
  try {
    const row = currentRow.value
    if (row?.source === 'plan') {
      // 合同期次走期次接口：订单结算接口只认 orderId，拿 planId 去调会 40000
      await invApi.settleContractPlan({
        planId: row.planId,
        amount: Number(form.amount),
        method: form.method || undefined,
        settledAt: form.settledAt || undefined,
        remark: form.remark || undefined,
      })
    } else {
      await invApi.settle({ ...form, amount: Number(form.amount) })
    }
    ElMessage.success('登记成功')
    dialogVisible.value = false
    load()
  } finally {
    submitting.value = false
  }
}

// ---------------- 批量收付款 ----------------
/**
 * 批量口径：**按每张单据的未结余额全额结清**。
 *
 * 一批单子一起付款，最常做的就是照着余额付干净；要只付一部分的，
 * 走行内的单张登记。这样批量弹窗里不需要维护一张金额表——
 * 金额清单在选中时就已经在页面上摆着了（合计未结余额）。
 */
const batchVisible = ref(false)
const batchSubmitting = ref(false)
const batchRows = ref<InvFinanceRow[]>([])
const batchVerb = ref<'收款' | '付款'>('收款')
const batchOutstanding = computed(() =>
  batchRows.value.reduce((sum, row) => sum + Number(row.outstanding), 0),
)
const batchForm = reactive<{ method: string; settledAt: string; remark: string }>({
  method: '银行转账',
  settledAt: '',
  remark: '',
})

function openBatch(): void {
  if (!selection.value.length) return
  batchRows.value = [...selection.value]
  batchVerb.value = activeSettlementVerb.value
  Object.assign(batchForm, { method: '银行转账', settledAt: today(), remark: '' })
  batchVisible.value = true
}

async function submitBatch(): Promise<void> {
  if (!batchRows.value.length || batchSubmitting.value) return
  batchSubmitting.value = true
  try {
    const res = await invApi.settleBatch({
      orderIds: batchRows.value.map((r) => r.orderId),
      method: batchForm.method,
      settledAt: batchForm.settledAt || undefined,
      remark: batchForm.remark || undefined,
    })
    ElMessage.success(`已登记 ${res.count} 张，合计 ${formatMoney(res.amount)}`)
    batchVisible.value = false
    load()
  } catch {
    // 失败提示由响应拦截器统一处理（整批回滚，不会留半截账）
  } finally {
    batchSubmitting.value = false
  }
}

// ---------------- 导出 / 打印 ----------------
const printVisible = ref(false)

const bizType = computed(() => (activeTab.value === 'payable' ? 'purchase' : 'sales'))
const tabLabel = computed(() => (activeTab.value === 'payable' ? '应付明细' : '应收明细'))

const printRows = computed(() => rows.value)
const printColumns: ListPrintColumn<InvFinanceRow>[] = [
  { label: '来源', value: (r) => (r.source === 'plan' ? '合同' : '单据') },
  {
    label: '单号 / 合同',
    value: (r) => (r.source === 'plan' ? `${r.contractNo} 第${r.planSeq}期 ${r.planName}` : r.orderNo),
  },
  { label: '往来单位', prop: 'partyName' },
  { label: '日期', value: (r) => formatDay(r.orderDate) },
  { label: '到期日', value: (r) => formatDay(r.dueDate) },
  { label: '单据金额', value: (r) => formatMoney(r.totalAmount), align: 'right' },
  { label: '已结算', value: (r) => formatMoney(r.settledAmount), align: 'right' },
  { label: '未结余额', value: (r) => formatMoney(r.outstanding), align: 'right' },
  { label: '状态', value: (r) => (r.overdue ? '已逾期' : settleMeta[r.settleStatus]?.text || '-') },
]
const printSubtitle = computed(() => {
  const parts = [tabLabel.value]
  if (query.keyword) parts.push(`关键字「${query.keyword}」`)
  if (query.settle) parts.push(settleMeta[query.settle]?.text || '已逾期')
  if (query.source) parts.push(query.source === 'plan' ? '合同期次' : '单据')
  return parts.join(' · ')
})

const { exporting, exportRows: handleExport } = useKasTableExport({
  exportUrl: '/inv/export',
  exportParams: () => ({
    entity: 'finance',
    bizType: bizType.value,
    keyword: query.keyword || undefined,
    settleStatus: query.settle || undefined,
    source: query.source || undefined,
  }),
  get fallbackName() { return `${tabLabel.value}.xlsx` },
})

onMounted(() => { if (!returnTo.value) void load() })
// 关键字录入即查询（防抖），不必再按回车
const { searchSoon, searchNow, cancel: cancelSearch } = useDebouncedSearch(search)
follow('/inv/finance', () => {
  applyingHandoff = true
  void nextTick(() => { applyingHandoff = false })
  cancelSearch()
  activeTab.value = route.query.view === 'payable' ? 'payable' : 'receivable'
  Object.assign(query, {
    page: 1, pageSize: 10,
    keyword: typeof route.query.q === 'string' ? route.query.q : '',
    settle: '', source: route.query.source === 'order' ? 'order' : '',
  })
  rows.value = []
  void load()
})
</script>

<template>
  <div class="finance-page">
    <PurchaseReturnLink />
    <div class="summary-row">
      <div v-for="card in summaryCards" :key="card.label" class="summary-card" :class="`tone-${card.tone}`">
        <span class="summary-card__label">{{ card.label }}</span>
        <strong class="summary-card__value">{{ formatMoney(card.value) }}</strong>
      </div>
      <div class="summary-split">{{ summarySplit }}</div>
    </div>

    <KasTable
      ref="tableRef"
      v-model:page="query.page"
      v-model:page-size="query.pageSize"
      :title="tabLabel"
      :rows="rows"
      :loading="loading"
      :error="loadError"
      error-text="应收应付加载失败，请重新加载后办理。"
      :empty-text="returnTo ? '当前可见范围内未找到记录，请核查单号或联系财务负责人' : '暂无数据'"
      :total="total"
      :selected-count="selection.length"
      @change="load"
      @retry="load"
      @selection-change="onSelectionChange"
      @clear-selection="clearSelection(tableRef)"
    >
      <template #before>
        <el-tabs v-model="activeTab" @tab-change="onTabChange">
          <el-tab-pane label="应收明细" name="receivable" />
          <el-tab-pane label="应付明细" name="payable" />
        </el-tabs>
      </template>
      <template #filters>
        <ListFilters :active-values="[query.source]">
          <el-form-item label="关键字">
            <el-input
              v-model="query.keyword"
              @input="searchSoon"
              placeholder="单号 / 往来单位"
              clearable
              @keyup.enter="searchNow"
            />
          </el-form-item>
          <el-form-item label="结算状态">
            <el-select v-model="query.settle" placeholder="全部状态" clearable>
              <el-option label="未结清" value="unpaid" />
              <el-option label="部分结算" value="partial" />
              <el-option label="已结清" value="paid" />
              <el-option label="已逾期" value="overdue" />
            </el-select>
          </el-form-item>
          <template #advanced>
            <el-form-item label="来源">
              <el-select v-model="query.source" placeholder="全部" clearable>
                <el-option label="单据" value="order" />
                <el-option label="合同期次" value="plan" />
              </el-select>
            </el-form-item>
          </template>
          <template #actions>
            <el-button type="primary" @click="searchNow">
              <AppIcon name="Search" />
              <span>查询</span>
            </el-button>
            <el-button
              @click="cancelSearch(); reset()"
              >重置</el-button
            >
          </template>
          <template #tools>
            <el-button
              v-perm="'inv:finance:settle'"
              type="primary"
              plain
              :disabled="!selection.length"
              @click="openBatch"
            >
              {{ activeTab === 'receivable' ? '批量收款' : '批量付款' }}{{ selection.length ? `（${selection.length}）` : '' }}
            </el-button>
            <el-button :loading="exporting" :disabled="loading || loadError" @click="handleExport()">导出</el-button>
            <el-button :disabled="loading || loadError" @click="printVisible = true">打印</el-button>
            <el-button @click="load">
              <AppIcon name="Refresh" />
            </el-button>
          </template>
        </ListFilters>
      </template>
      <el-table-column
        type="selection"
        width="46"
        :selectable="(row: InvFinanceRow) => row.outstanding > 0 && row.source === 'order'"
      />
      <el-table-column label="来源" width="76" align="center">
        <template #default="{ row }">
          <el-tag size="small" :type="row.source === 'plan' ? 'warning' : 'info'" effect="plain">
            {{ row.source === 'plan' ? '合同' : '单据' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="单号 / 合同" width="196">
        <template #default="{ row }">
          <template v-if="row.source === 'plan'">
            <div class="contract-cell">
              <span>{{ row.contractNo }}</span>
              <em>第{{ row.planSeq }}期 {{ row.planName }}</em>
            </div>
          </template>
          <span v-else>{{ row.orderNo }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="partyName" label="往来单位" min-width="150" show-overflow-tooltip />
      <el-table-column prop="orderDate" label="日期" width="120">
        <template #default="{ row }">{{ formatDay(row.orderDate) }}</template>
      </el-table-column>
      <el-table-column prop="dueDate" label="到期日" width="120">
        <template #default="{ row }">
          <span :class="row.overdue ? 'overdue' : ''">
            {{ formatDay(row.dueDate) }}
            <em v-if="row.overdue">逾期</em>
          </span>
        </template>
      </el-table-column>
      <el-table-column label="金额" width="130" align="right">
        <template #default="{ row }">{{ formatMoney(row.totalAmount) }}</template>
      </el-table-column>
      <el-table-column label="已结算" width="130" align="right">
        <template #default="{ row }">
          <span class="text-secondary">{{ formatMoney(row.settledAmount) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="未结余额" width="130" align="right">
        <template #default="{ row }">
          <strong :class="row.outstanding > 0 ? 'outstanding' : ''">
            {{ formatMoney(row.outstanding) }}
          </strong>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="settleMeta[row.settleStatus]?.type" size="small" effect="light">
            {{ settleMeta[row.settleStatus]?.text }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="110" fixed="right" align="center">
        <template #default="{ row }">
          <el-button
            v-perm="'inv:finance:settle'"
            link
            type="primary"
            :disabled="row.outstanding <= 0"
            @click="openSettle(row as InvFinanceRow)"
          >
            {{ row.source === 'plan' ? (activeTab === 'receivable' ? '收款' : '付款') : (activeTab === 'receivable' ? '登记收款' : '登记付款') }}
          </el-button>
        </template>
      </el-table-column>
    </KasTable>

    <ListPrintDialog
      v-model="printVisible"
      :title="tabLabel"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />

    <el-dialog
      v-model="dialogVisible"
      :title="`登记${settlementVerb}`"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      :close-on-press-escape="!submitting"
      :show-close="!submitting"
      destroy-on-close
    >
      <el-descriptions v-if="currentRow" :column="1" border size="small" class="mb-16">
        <el-descriptions-item label="单号 / 合同">
          {{
            currentRow.source === 'plan'
              ? `${currentRow.contractNo} · 第${currentRow.planSeq}期 ${currentRow.planName}`
              : currentRow.orderNo
          }}
        </el-descriptions-item>
        <el-descriptions-item label="往来单位">{{ currentRow.partyName }}</el-descriptions-item>
        <el-descriptions-item label="未结余额">{{ formatMoney(currentRow.outstanding) }}</el-descriptions-item>
      </el-descriptions>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-form-item :label="`本次${settlementVerb}金额`" prop="amount">
          <el-input-number v-model="form.amount" :min="0.01" :max="currentRow?.outstanding ?? 0" :precision="2" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="结算方式">
          <el-select v-model="form.method" style="width: 100%">
            <el-option label="银行转账" value="银行转账" />
            <el-option label="现金" value="现金" />
            <el-option label="承兑汇票" value="承兑汇票" />
          </el-select>
        </el-form-item>
        <el-form-item label="结算日期">
          <el-date-picker v-model="form.settledAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <dl class="settlement-summary" aria-label="登记金额摘要" aria-live="polite">
        <div class="settlement-summary__item">
          <dt>本次登记</dt>
          <dd>{{ settlementPreview ? formatMoney(settlementPreview.amount) : '—' }}</dd>
        </div>
        <div class="settlement-summary__item">
          <dt>登记后剩余</dt>
          <dd>{{ settlementPreview ? formatMoney(settlementPreview.remaining) : '—' }}</dd>
        </div>
      </dl>
      <template #footer>
        <el-button :disabled="submitting" @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">登记{{ settlementVerb }}</el-button>
      </template>
    </el-dialog>

    <!-- 批量收付款 -->
    <el-dialog
      v-model="batchVisible"
      :title="`批量登记${batchVerb}`"
      width="620px"
      top="8vh"
      :close-on-click-modal="false"
      :close-on-press-escape="!batchSubmitting"
      :show-close="!batchSubmitting"
      destroy-on-close
    >
      <dl class="settlement-summary" aria-label="批量登记摘要">
        <div class="settlement-summary__item">
          <dt>本次登记单据</dt>
          <dd>{{ batchRows.length }} 张</dd>
        </div>
        <div class="settlement-summary__item">
          <dt>预计登记合计</dt>
          <dd>{{ formatMoney(batchOutstanding) }}</dd>
        </div>
      </dl>
      <p class="settlement-note">按每张单据提交时的未结余额全额登记。</p>
      <el-form label-width="88px">
        <el-form-item label="结算方式">
          <el-select v-model="batchForm.method" style="width: 100%">
            <el-option label="银行转账" value="银行转账" />
            <el-option label="现金" value="现金" />
            <el-option label="承兑汇票" value="承兑汇票" />
          </el-select>
        </el-form-item>
        <el-form-item label="结算日期">
          <el-date-picker v-model="batchForm.settledAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="batchForm.remark" maxlength="255" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="batchSubmitting" @click="batchVisible = false">取消</el-button>
        <el-button type="primary" :loading="batchSubmitting" @click="submitBatch">
          批量登记{{ batchVerb }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.summary-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 16px;
}

/* 总额的构成说明：跨满整行，跟在四张卡片下面 */
.summary-split {
  grid-column: 1 / -1;
  margin-top: -6px;
  font-size: 12.5px;
  color: var(--app-text-secondary);
}

.contract-cell {
  display: flex;
  flex-direction: column;
  line-height: 1.35;

  em {
    font-style: normal;
    font-size: 12px;
    color: var(--app-text-secondary);
  }
}

.summary-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 18px 20px;
  background: var(--app-surface);
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-card);

  &__label {
    font-size: 12.5px;
    color: var(--app-text-secondary);
  }

  &__value {
    font-size: 22px;
    font-weight: 700;
    color: var(--app-text);
  }

  &.tone-blue &__value {
    color: var(--tone-blue);
  }

  &.tone-green &__value {
    color: var(--tone-green);
  }

  &.tone-amber &__value {
    color: var(--tone-amber);
  }

  &.tone-red &__value {
    color: var(--tone-red);
  }
}

.settlement-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 32px;
  margin: 0;
  padding: 12px 16px;
  background: var(--app-surface-subtle);
  border-radius: var(--radius-sm);

  &__item {
    display: grid;
    gap: 4px;
    font-size: 13px;
    color: var(--app-text-secondary);

    dd {
      margin: 0;
      color: var(--app-text);
      font-size: 16px;
      font-weight: 600;
      font-variant-numeric: tabular-nums;
    }
  }
}

.settlement-note {
  margin: 8px 0 16px;
  color: var(--app-text-secondary);
  font-size: 13px;
}

.overdue {
  color: var(--tone-red);
  font-weight: 600;

  em {
    margin-left: 4px;
    font-style: normal;
    font-size: 12px;
  }
}

.outstanding {
  color: var(--tone-amber);
}
</style>
