<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { invApi } from '@/api'
import type {
  InvOrderSettlementRow, InvOrderSettlements, InvRedReversalOrderLink,
  InvSettlementOffsetTarget, SettlementAdjustmentKind,
} from '@/types/api'
import { formatDay, formatMoney } from '@/utils/format'

const props = defineProps<{
  order: { id: number; orderNo: string; partyName: string }
  canOpenLinked: boolean
}>()
const emit = defineEmits<{ updated: []; openLinked: [order: InvRedReversalOrderLink] }>()
const visible = ref(false)
const loading = ref(false)
const saving = ref(false)
const history = ref<InvOrderSettlements>()
const error = ref('')
const selected = ref<InvOrderSettlementRow>()
const kind = ref<SettlementAdjustmentKind>('correction')
const amount = ref<number>()
const reason = ref('')
const reference = ref('')
const refundDate = ref('')
const targetID = ref<number>()
const targets = ref<InvSettlementOffsetTarget[]>([])
const searching = ref(false)
const targetError = ref('')
let targetRequest = 0
let key = ''
let changed = false

const target = computed(() => targets.value.find((row) => row.id === targetID.value))
const methods: Record<InvOrderSettlementRow['kind'], string> = {
  '': '登记', correction: '误录更正', refund: '实际退款', offset: '抵扣转出', offset_in: '抵扣转入',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    history.value = await invApi.orderSettlements(props.order.id)
  } catch (err) {
    history.value = undefined
    error.value = err instanceof Error ? err.message : '无法加载收付款明细'
  } finally {
    loading.value = false
  }
}

function open() {
  changed = false
  selected.value = undefined
  history.value = undefined
  visible.value = true
  void load()
}
defineExpose({ open })

function start(row: InvOrderSettlementRow) {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6]! & 15) | 64
  bytes[8] = (bytes[8]! & 63) | 128
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  key = hex.replace(/^(........)(....)(....)(....)(............)$/, '$1-$2-$3-$4-$5')
  selected.value = row
  kind.value = row.kind === 'offset_in' ? 'offset' : 'correction'
  amount.value = row.available
  reason.value = ''
  reference.value = ''
  refundDate.value = ''
  targetID.value = undefined
  targets.value = []
  error.value = ''
  if (kind.value === 'offset') void searchTargets('')
}

async function searchTargets(keyword: string) {
  const request = ++targetRequest
  searching.value = true
  targetError.value = ''
  try {
    const found = await invApi.settlementOffsetTargets(props.order.id, keyword)
    if (request === targetRequest) {
      const current = target.value
      targets.value = current && !found.some((row) => row.id === current.id) ? [current, ...found] : found
    }
  } catch (err) {
    if (request === targetRequest) {
      targetError.value = err instanceof Error ? err.message : '加载失败，请重新搜索单号'
    }
  } finally {
    if (request === targetRequest) searching.value = false
  }
}

function changeKind() {
  error.value = ''
  if (kind.value === 'offset') void searchTargets('')
}

async function confirm() {
  const row = selected.value
  const value = amount.value
  if (!row || saving.value) return
  if (value === undefined || value <= 0 || value > row.available || !reason.value.trim()) {
    error.value = '请填写处理原因，并核对处理金额。'
    return
  }
  if (kind.value === 'refund' && (!refundDate.value || !reference.value.trim())) {
    error.value = '请填写已实际退款的日期和凭证号。'
    return
  }
  if (kind.value === 'offset' && (!target.value || value > target.value.outstanding)) {
    error.value = '请选择抵扣单据，处理金额须在目标单据未结余额内。'
    return
  }
  saving.value = true
  error.value = ''
  try {
    await invApi.adjustSettlement(props.order.id, row.id, {
      kind: kind.value, key, amount: value, reason: reason.value.trim(),
      settledAt: kind.value === 'refund' ? refundDate.value : undefined,
      reference: kind.value === 'refund' ? reference.value.trim() : undefined,
      targetOrderId: kind.value === 'offset' ? targetID.value : undefined,
    })
    changed = true
    selected.value = undefined
    ElMessage.success('收付款处理已登记')
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '处理结果未确认，请核对明细后重试'
  } finally {
    saving.value = false
  }
}

function close() {
  if (changed) emit('updated')
}

function openCounter(order: InvRedReversalOrderLink) {
  changed = false
  visible.value = false
  emit('openLinked', order)
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="selected ? '处理收付款' : '收付款明细'"
    :width="selected ? '640px' : '1100px'"
    top="6vh"
    append-to-body
    :show-close="!saving"
    :close-on-click-modal="false"
    :close-on-press-escape="!saving"
    @closed="close"
  >
    <div class="settlement-content">
      <p><strong>{{ order.orderNo }}</strong> · {{ order.partyName }}</p>
      <p v-if="history">
        已结算 <strong>{{ formatMoney(history.settled) }}</strong>
        · 未结 <strong>{{ formatMoney(history.outstanding) }}</strong>
      </p>
      <p v-if="loading" role="status">正在读取收付款明细…</p>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
      <template v-if="selected">
        <p>处理原流水 #{{ selected.id }} · 可处理 <strong>{{ formatMoney(selected.available) }}</strong></p>
        <el-form label-position="top" :disabled="saving" @submit.prevent="confirm">
          <el-form-item label="处理方式">
            <el-radio-group v-model="kind" aria-label="处理方式" @change="changeKind">
              <el-radio v-if="selected.kind !== 'offset_in'" value="correction">误录更正</el-radio>
              <el-radio value="refund">登记实际退款</el-radio>
              <el-radio value="offset">往来抵扣</el-radio>
            </el-radio-group>
          </el-form-item>
          <p class="settlement-hint">
            {{ {
              correction: '用于实际未收付或多录的金额。系统会追加一笔更正记录。',
              refund: '请在款项实际退回后登记凭证。此操作会更新单据余额。',
              offset: '将这笔已收付金额转到同一往来单位的另一张未结单据。',
            }[kind] }}
          </p>
          <el-form-item label="处理金额（元）">
            <el-input-number v-model="amount" :min="0.01" :precision="2" :controls="false" aria-label="处理金额（元）" />
          </el-form-item>
          <template v-if="kind === 'refund'">
            <el-form-item label="实际退款日期">
              <el-date-picker v-model="refundDate" type="date" value-format="YYYY-MM-DD" placeholder="选择退款日期" aria-label="实际退款日期" />
            </el-form-item>
            <el-form-item label="退款凭证号">
              <el-input v-model="reference" :maxlength="128" aria-label="退款凭证号" />
            </el-form-item>
          </template>
          <el-form-item v-if="kind === 'offset'" label="抵扣单据">
            <el-select
              v-model="targetID" filterable remote :remote-method="searchTargets"
              :loading="searching" placeholder="输入单号查找" aria-label="抵扣单据"
            >
              <el-option
                v-for="row in targets" :key="row.id" :value="row.id"
                :label="`${row.orderNo} · 未结 ${formatMoney(row.outstanding)}`"
              />
            </el-select>
            <p v-if="targetError" role="alert">{{ targetError }}</p>
          </el-form-item>
          <el-form-item label="处理原因（必填）">
            <el-input v-model="reason" type="textarea" :maxlength="255" aria-label="处理原因（必填）" />
          </el-form-item>
        </el-form>
        <p v-if="history && amount && amount <= selected.available">
          本单已结算 {{ formatMoney(history.settled) }} → <strong>{{ formatMoney(history.settled - amount) }}</strong>
          <template v-if="target && kind === 'offset' && amount <= target.outstanding">
            · 目标单未结 {{ formatMoney(target.outstanding) }} → <strong>{{ formatMoney(target.outstanding - amount) }}</strong>
          </template>
        </p>
      </template>
      <el-table v-else-if="history" :data="history.rows" empty-text="暂无收付款记录" size="small" row-key="id">
        <el-table-column label="流水" width="108">
          <template #default="{ row }">#{{ row.id }}</template>
        </el-table-column>
        <el-table-column label="处理类型" width="98">
          <template #default="{ row }">{{ methods[(row as InvOrderSettlementRow).kind] }}</template>
        </el-table-column>
        <el-table-column label="金额" width="115" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="日期 / 经办人" min-width="130">
          <template #default="{ row }">{{ formatDay(row.settledAt) }}<br />{{ row.operator || '—' }}</template>
        </el-table-column>
        <el-table-column label="凭证与说明" min-width="210">
          <template #default="{ row }">
            {{ row.method }}
            <div v-if="row.adjustsId">关联流水 #{{ row.adjustsId }}</div>
            <div v-if="row.reference">凭证 {{ row.reference }}</div>
            <div v-if="row.remark">{{ row.remark }}</div>
            <el-button v-if="row.counterOrder && canOpenLinked" link type="primary" @click="openCounter(row.counterOrder)">
              {{ row.counterOrder.orderNo }}
            </el-button>
            <div v-else-if="row.counterOrder">{{ row.counterOrder.orderNo }}</div>
          </template>
        </el-table-column>
        <el-table-column v-if="history.canAdjust" label="操作" width="110">
          <template #default="{ row }">
            <el-button v-if="row.available > 0" link type="primary" @click="start(row as InvOrderSettlementRow)">处理款项</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <template #footer>
      <template v-if="selected">
        <el-button :disabled="saving" @click="selected = undefined; error = ''">返回明细</el-button>
        <el-button type="primary" :loading="saving" @click="confirm">确认登记</el-button>
      </template>
      <template v-else>
        <el-button v-if="error" @click="load">重新加载</el-button>
        <el-button @click="visible = false">关闭</el-button>
      </template>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.settlement-content {
  display: grid;
  gap: 12px;
  p { margin: 0; }
  :deep(.el-form-item) { margin-bottom: 16px; }
  .settlement-hint { margin: -8px 0 16px; color: var(--app-text-secondary); }
}
</style>
