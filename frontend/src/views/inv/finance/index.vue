<script setup lang="ts">
/**
 * 应收应付页：应收（销售）与应付（采购）两个视图，结构一致。
 * 汇总数据随列表接口一并返回，避免多打一次请求。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type { InvFinanceRow, InvFinanceSummary, InvSettlePayload } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import { formatDay, formatMoney } from '@/utils/format'

defineOptions({ name: 'InvFinance' })

const activeTab = ref<'receivable' | 'payable'>('receivable')
const loading = ref(false)
const rows = ref<InvFinanceRow[]>([])
const total = ref(0)
const summary = ref<InvFinanceSummary>({ total: 0, settled: 0, outstanding: 0, overdue: 0 })

const query = reactive<{ page: number; pageSize: number; keyword: string; settle: string }>({
  page: 1,
  pageSize: 10,
  keyword: '',
  settle: '',
})

const settleMeta: Record<string, { text: string; type: 'danger' | 'warning' | 'success' }> = {
  unpaid: { text: '未结清', type: 'danger' },
  partial: { text: '部分结算', type: 'warning' },
  paid: { text: '已结清', type: 'success' },
}

const summaryCards = computed(() => [
  { label: '单据总额', value: summary.value.total, tone: 'blue' },
  { label: '已结算', value: summary.value.settled, tone: 'green' },
  { label: '未结余额', value: summary.value.outstanding, tone: 'amber' },
  { label: '逾期金额', value: summary.value.overdue, tone: 'red' },
])

async function load(): Promise<void> {
  loading.value = true
  try {
    const params = {
      page: query.page,
      pageSize: query.pageSize,
      keyword: query.keyword || undefined,
      settle: query.settle || undefined,
    }
    const res =
      activeTab.value === 'receivable'
        ? await invApi.receivables(params)
        : await invApi.payables(params)
    rows.value = res.list
    total.value = res.total
    summary.value = res.summary
  } finally {
    loading.value = false
  }
}

function search(): void {
  query.page = 1
  load()
}

function reset(): void {
  query.keyword = ''
  query.settle = ''
  query.page = 1
  load()
}

function onTabChange(): void {
  query.page = 1
  load()
}

// ---------------- 登记收付款 ----------------
const formRef = ref<FormInstance>()
const dialogVisible = ref(false)
const submitting = ref(false)
const currentRow = ref<InvFinanceRow | null>(null)

const form = reactive<InvSettlePayload>({
  orderId: 0,
  amount: 0,
  method: '银行转账',
  settledAt: '',
  remark: '',
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
  const valid = formRef.value ? await formRef.value.validate().catch(() => false) : false
  if (!valid) return
  submitting.value = true
  try {
    await invApi.settle({ ...form, amount: Number(form.amount) })
    ElMessage.success('登记成功')
    dialogVisible.value = false
    load()
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="finance-page">
    <div class="summary-row">
      <div v-for="card in summaryCards" :key="card.label" class="summary-card" :class="`tone-${card.tone}`">
        <span class="summary-card__label">{{ card.label }}</span>
        <strong class="summary-card__value">{{ formatMoney(card.value) }}</strong>
      </div>
    </div>

    <div class="page-card">
      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <el-tab-pane label="应收明细" name="receivable" />
        <el-tab-pane label="应付明细" name="payable" />
      </el-tabs>

      <div class="toolbar">
        <div class="toolbar__title">
          <el-input
            v-model="query.keyword"
            placeholder="单号 / 往来单位"
            clearable
            style="width: 220px"
            @keyup.enter="search"
          />
          <el-select v-model="query.settle" placeholder="全部状态" clearable style="width: 150px">
            <el-option label="未结清" value="unpaid" />
            <el-option label="部分结算" value="partial" />
            <el-option label="已结清" value="paid" />
            <el-option label="已逾期" value="overdue" />
          </el-select>
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
        </div>
        <div class="toolbar__actions">
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="orderNo" label="单号" width="160" />
        <el-table-column prop="partyName" label="往来单位" min-width="150" show-overflow-tooltip />
        <el-table-column prop="orderDate" label="单据日期" width="120">
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
              {{ activeTab === 'receivable' ? '登记收款' : '登记付款' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="table-pager">
        <Pagination v-model:page="query.page" v-model:page-size="query.pageSize" :total="total" @change="load" />
      </div>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="activeTab === 'receivable' ? '登记收款' : '登记付款'"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-descriptions v-if="currentRow" :column="1" border size="small" class="mb-16">
        <el-descriptions-item label="单号">{{ currentRow.orderNo }}</el-descriptions-item>
        <el-descriptions-item label="往来单位">{{ currentRow.partyName }}</el-descriptions-item>
        <el-descriptions-item label="未结余额">{{ formatMoney(currentRow.outstanding) }}</el-descriptions-item>
      </el-descriptions>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-form-item label="金额" prop="amount">
          <el-input-number v-model="form.amount" :min="0.01" :max="currentRow?.outstanding ?? 0" :controls="false" style="width: 100%" />
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
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
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

.table-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
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
