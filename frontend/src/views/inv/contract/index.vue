<script setup lang="ts">
/**
 * 合同管理台账（含付款计划、验收、质保与期次收付款）。
 *
 * 这一页承载「合同怎么变成钱」的全过程：
 * 签订 → 付款计划（N 期）→ 期次收付款 → 应收/应付 → 报表。
 *
 * 设计取舍：
 *   - 列表只展示汇总（金额 / 已收付 / 未收付 / 阶段），明细一律进详情抽屉 ——
 *     把计划表塞进列表会让一行合同占掉半屏，反而看不清有几份合同；
 *   - 阶段是派生的（待验收 / 已验收 / 质保中 / 质保到期 / 已完成 / 已终止），
 *     不是可选状态：改状态要改日期，否则会出现「状态说质保中、日期说早过期」；
 *   - 已发生收付款的期次，金额与比例不可改、不可删（后端强制，前端同步禁用）。
 */
defineOptions({ name: 'InvContract' })

import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvAttachBizType,
  InvContractBizType,
  InvContractPlanItem,
  InvContractPlanPayload,
  InvContractPayload,
  InvContractRow,
  InvContractStage,
  InvContractStatus,
  InvPartyRow,
  InvPlanKind,
  InvPlanSettleRecord,
  InvPlanTrigger,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import { useTableQuery } from '@/composables/useTableQuery'
import { useDebouncedSearch } from '@/composables/useDebouncedSearch'
import { formatDay, formatMoney } from '@/utils/format'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import AttachmentDrawer from '../components/AttachmentDrawer.vue'

/** 阶段筛选值：前四个走 stage，后两个映射成 status（已完成 / 已终止） */
type StageFilter = InvContractStage | ''

interface ContractQuery {
  keyword?: string
  bizType?: InvContractBizType | ''
  stage?: StageFilter
  partyId?: number
  /** 签订日期区间，提交时拆成 startDate / endDate */
  dateRange?: [string, string] | null
  page: number
  pageSize: number
}

const {
  query,
  loading,
  rows,
  total,
  load,
  search,
  reset,
} = useTableQuery<ContractQuery, InvContractRow>(
  { keyword: '', bizType: '', stage: '', partyId: undefined, dateRange: null, page: 1, pageSize: 10 },
  (p) => {
    const [startDate, endDate] = p.dateRange ?? []
    // 「已完成 / 已终止」不是日期条件，映射成 status；
    // 其余阶段直接传给后端做日期判定（见 contractQuery）
    const status = p.stage === 'finished' ? 2 : p.stage === 'terminated' ? 3 : undefined
    const stage = p.stage === 'finished' || p.stage === 'terminated' ? undefined : p.stage || undefined
    return invApi.contracts({
      page: p.page,
      pageSize: p.pageSize,
      keyword: p.keyword || undefined,
      bizType: p.bizType || undefined,
      stage,
      status,
      partyId: p.partyId || undefined,
      startDate,
      endDate,
    })
  },
)

// 关键字录入即查询（防抖），不必再按回车
const { searchSoon } = useDebouncedSearch(search)

type TagTone = 'primary' | 'success' | 'warning' | 'info' | 'danger'

const stageOptions: Array<{ label: string; value: StageFilter; tone: TagTone }> = [
  { label: '待验收', value: 'pending', tone: 'warning' },
  { label: '已验收', value: 'accepted', tone: 'primary' },
  { label: '质保中', value: 'warranty', tone: 'success' },
  { label: '质保到期', value: 'warranty_due', tone: 'danger' },
  { label: '已完成', value: 'finished', tone: 'info' },
  { label: '已终止', value: 'terminated', tone: 'info' },
]

function stageTone(v: InvContractStage): TagTone {
  return stageOptions.find((s) => s.value === v)?.tone ?? 'info'
}

function bizLabel(v: InvContractBizType): string {
  return v === 'purchase' ? '采购合同' : '销售合同'
}

/** 收付方向：销售合同是收款，采购合同是付款 */
function settleVerb(v: InvContractBizType): string {
  return v === 'purchase' ? '付款' : '收款'
}

/** 履约期展示：没填起止就显示「—」，不留两个空箭头 */
function periodText(row: InvContractRow): string {
  const start = row.startDate ? formatDay(row.startDate) : ''
  const end = row.endDate ? formatDay(row.endDate) : ''
  if (!start && !end) return '—'
  if (start && end) return `${start} ~ ${end}`
  return start ? `${start} 起` : `至 ${end}`
}

function progressText(row: InvContractRow): string {
  if (!row.planCount) return '—'
  if (!row.planAmount) return '—'
  const pct = Math.round((row.settledAmount / row.planAmount) * 100)
  return `${pct}%`
}

// ---------------- 往来单位（按合同类型切换客户 / 供应商） ----------------
const customers = ref<InvPartyRow[]>([])
const suppliers = ref<InvPartyRow[]>([])

async function loadParties(): Promise<void> {
  try {
    const [cus, sup] = await Promise.all([
      invApi.customers({ page: 1, pageSize: 500 }),
      invApi.suppliers({ page: 1, pageSize: 500 }),
    ])
    customers.value = cus.list
    suppliers.value = sup.list
  } catch {
    // 失败提示由响应拦截器统一处理
  }
}

// ---------------- 新增 / 编辑 ----------------
/** 编辑态的一期计划（既有期次带 id 与已收付金额） */
interface PlanDraft {
  id?: number
  seq: number
  name: string
  kind: InvPlanKind
  rate: number
  amount: number
  dueDate: string
  triggerNode: InvPlanTrigger | ''
  remark: string
  settledAmount: number
}

const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const editingId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<InvContractPayload>({
  contractNo: '',
  name: '',
  bizType: 'sales',
  partyId: 0,
  signDate: '',
  startDate: '',
  endDate: '',
  amount: 0,
  salesman: '',
  remark: '',
  warrantyMonths: 0,
  retentionRate: 0,
  retentionAmount: 0,
})
const plans = ref<PlanDraft[]>([])

const rules: FormRules<InvContractPayload> = {
  name: [{ required: true, message: '请输入合同名称', trigger: 'blur' }],
  partyId: [{ required: true, message: '请选择往来单位', trigger: 'change' }],
  signDate: [{ required: true, message: '请选择签订日期', trigger: 'change' }],
}

const partyOptions = computed(() => (form.bizType === 'purchase' ? suppliers.value : customers.value))
const partyLabel = computed(() => (form.bizType === 'purchase' ? '供应商' : '客户'))

const planTotal = computed(() => plans.value.reduce((s, p) => s + (Number(p.amount) || 0), 0))
const planOver = computed(() => planTotal.value - (Number(form.amount) || 0) > 0.005)
const retentionPlanTotal = computed(() =>
  plans.value.filter((p) => p.kind === 4).reduce((s, p) => s + (Number(p.amount) || 0), 0),
)

function today(): string {
  const d = new Date()
  const m = `${d.getMonth() + 1}`.padStart(2, '0')
  const day = `${d.getDate()}`.padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

function emptyPlan(seq: number): PlanDraft {
  return {
    seq,
    name: '',
    kind: 2,
    rate: 0,
    amount: 0,
    dueDate: '',
    triggerNode: '',
    remark: '',
    settledAmount: 0,
  }
}

function openCreate(): void {
  Object.assign(form, {
    contractNo: '',
    name: '',
    bizType: 'sales',
    partyId: 0,
    signDate: today(),
    startDate: '',
    endDate: '',
    amount: 0,
    salesman: '',
    remark: '',
    warrantyMonths: 0,
    retentionRate: 0,
    retentionAmount: 0,
  })
  plans.value = []
  dialogMode.value = 'create'
  editingId.value = null
  dialogVisible.value = true
  // 档案可能刚被别人新增：开弹窗时重拉一次，避免下拉里没有新客户
  loadParties()
}

async function openEdit(row: InvContractRow): Promise<void> {
  try {
    // 列表行不带 plans，编辑必须取详情
    const d = await invApi.contractDetail(row.id)
    Object.assign(form, {
      contractNo: d.contractNo,
      name: d.name,
      bizType: d.bizType,
      partyId: d.partyId,
      signDate: d.signDate ? formatDay(d.signDate) : today(),
      startDate: d.startDate ? formatDay(d.startDate) : '',
      endDate: d.endDate ? formatDay(d.endDate) : '',
      amount: d.amount,
      salesman: d.salesman,
      remark: d.remark,
      warrantyMonths: d.warrantyMonths,
      retentionRate: d.retentionRate,
      retentionAmount: d.retentionAmount,
    })
    plans.value = (d.plans ?? []).map((p) => ({
      id: p.id,
      seq: p.seq,
      name: p.name,
      kind: p.kind,
      rate: p.rate,
      amount: p.amount,
      dueDate: p.dueDate ? formatDay(p.dueDate) : '',
      triggerNode: p.triggerNode,
      remark: p.remark,
      settledAmount: p.settledAmount,
    }))
    dialogMode.value = 'edit'
    editingId.value = d.id
    dialogVisible.value = true
    loadParties()
  } catch {
    // 失败提示由响应拦截器统一处理
  }
}

/** 切换合同类型时清掉已选往来单位：采购合同不能挂客户 */
function onBizTypeChange(): void {
  form.partyId = 0
}

// ---------------- 付款计划编辑 ----------------

/** 一键生成常见分期：预付 30 / 进度 40 / 验收 20 / 质保金 10 */
function applyPlanTemplate(): void {
  const amount = Number(form.amount) || 0
  const rows: Array<{ name: string; kind: InvPlanKind; rate: number; trigger: InvPlanTrigger | '' }> = [
    { name: '预付款', kind: 1, rate: 30, trigger: 'sign' },
    { name: '进度款', kind: 2, rate: 40, trigger: '' },
    { name: '验收款', kind: 3, rate: 20, trigger: 'accept' },
    { name: '质保金', kind: 4, rate: 10, trigger: 'warranty_end' },
  ]
  plans.value = rows.map((r, i) => ({
    ...emptyPlan(i + 1),
    name: r.name,
    kind: r.kind,
    rate: r.rate,
    amount: Math.round(amount * r.rate) / 100,
    triggerNode: r.trigger,
  }))
  // 模板里的质保金按合同金额的 10% 兜底，用户可再改
  if (!form.retentionAmount) {
    form.retentionRate = 10
    form.retentionAmount = Math.round(amount * 10) / 100
  }
}

function addPlan(): void {
  plans.value.push(emptyPlan(plans.value.length + 1))
}

function removePlan(idx: number): void {
  const p = plans.value[idx]
  if (p.settledAmount > 0.005) {
    ElMessage.warning(`第 ${p.seq} 期已发生收付款，不能删除`)
    return
  }
  plans.value.splice(idx, 1)
  renumber()
}

/** 删除后重排期次号，避免出现 1、3、4 这种断号 */
function renumber(): void {
  plans.value.forEach((p, i) => {
    p.seq = i + 1
  })
}

/** 改比例算金额；金额按合同金额折算 */
function onPlanRate(idx: number): void {
  const p = plans.value[idx]
  const rate = Number(p.rate) || 0
  const amount = Number(form.amount) || 0
  p.amount = Math.round(amount * rate) / 100
}

/** 直接改金额表示「按金额约定」，比例清掉（后端比例优先，不清会被算回去） */
function onPlanAmount(idx: number): void {
  plans.value[idx].rate = 0
}

/** 质保金比例 → 金额 */
function onRetentionRate(): void {
  const rate = Number(form.retentionRate) || 0
  const amount = Number(form.amount) || 0
  form.retentionAmount = Math.round(amount * rate) / 100
}

async function submitForm(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  if (planOver.value) {
    ElMessage.warning(`付款计划合计 ${formatMoney(planTotal.value)} 超过合同金额，请调整`)
    return
  }
  if (plans.value.some((p) => !p.name.trim())) {
    ElMessage.warning('付款计划的每一期都要填名称')
    return
  }
  submitting.value = true
  try {
    const planPayload: InvContractPlanPayload[] = plans.value.map((p) => ({
      id: p.id,
      seq: p.seq,
      name: p.name.trim(),
      kind: p.kind,
      rate: Number(p.rate) || 0,
      amount: Number(p.amount) || 0,
      dueDate: p.dueDate || undefined,
      triggerNode: p.triggerNode || undefined,
      remark: p.remark || undefined,
    }))
    const payload: InvContractPayload = {
      contractNo: form.contractNo?.trim() || undefined,
      name: form.name.trim(),
      bizType: form.bizType,
      partyId: form.partyId,
      signDate: form.signDate,
      startDate: form.startDate || undefined,
      endDate: form.endDate || undefined,
      amount: Number(form.amount) || 0,
      salesman: form.salesman?.trim(),
      remark: form.remark,
      warrantyMonths: Number(form.warrantyMonths) || 0,
      retentionRate: Number(form.retentionRate) || 0,
      retentionAmount: Number(form.retentionAmount) || 0,
      plans: planPayload,
    }
    if (dialogMode.value === 'create') {
      await invApi.createContract(payload)
      ElMessage.success('新增成功')
    } else if (editingId.value) {
      await invApi.updateContract(editingId.value, payload)
      ElMessage.success('修改成功')
    }
    dialogVisible.value = false
    load()
    if (detailVisible.value) await refreshDetail()
  } catch {
    // 失败提示由响应拦截器统一处理（编号重复、计划超额、期次已有收付款等）
  } finally {
    submitting.value = false
  }
}

async function removeRow(row: InvContractRow): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确认删除合同「${row.contractNo} ${row.name}」？删除后付款计划与附件都不再可见。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除' },
    )
  } catch {
    return
  }
  try {
    await invApi.deleteContract(row.id)
    ElMessage.success('已删除')
    if (detailVisible.value) detailVisible.value = false
    load()
  } catch {
    // 已发生收付款的合同会被后端拒绝，提示由拦截器弹出
  }
}

// ---------------- 详情抽屉 ----------------
const detailVisible = ref(false)
const detail = ref<InvContractRow | null>(null)
const detailLoading = ref(false)

async function openDetail(row: InvContractRow): Promise<void> {
  detailVisible.value = true
  detailLoading.value = true
  try {
    detail.value = await invApi.contractDetail(row.id)
    statusBaseline.value = detail.value.status
  } catch {
    detail.value = null
  } finally {
    detailLoading.value = false
  }
}

async function refreshDetail(): Promise<void> {
  if (!detail.value) return
  try {
    detail.value = await invApi.contractDetail(detail.value.id)
    statusBaseline.value = detail.value.status
    load()
  } catch {
    // 忽略
  }
}

const detailPlans = computed<InvContractPlanItem[]>(() => detail.value?.plans ?? [])

// ---------------- 改状态 ----------------
//
// 阶段是派生的（验收 / 质保由日期决定），但「已完成 / 已终止」是人工判断，
// 必须给一个入口 —— 否则合同永远停在执行中，报表里也没法把它划走。
// 复用 update 接口（它要完整字段），所以按详情回填一份，plans 原样带回：
// 已收付款的期次带的还是原金额与原比例，不会触发「已收付款不可改」的拦截。
const statusOptions: Array<{ label: string; value: InvContractStatus }> = [
  { label: '执行中', value: 1 },
  { label: '已完成', value: 2 },
  { label: '已终止', value: 3 },
]
const statusSaving = ref(false)
/**
 * 已落库的状态基线。
 *
 * 不能拿 `detail.status` 判断「值有没有变」：el-select 的 v-model 在 change
 * 之前就把 detail.status 改成新值了，那样判等永远成立，请求根本不会发出。
 * 基线只在详情加载成功后更新，代表库里真实的值。
 */
const statusBaseline = ref<InvContractStatus>(1)

async function changeStatus(next: InvContractStatus): Promise<void> {
  const d = detail.value
  if (!d || next === statusBaseline.value) return
  const prev = statusBaseline.value
  statusSaving.value = true
  try {
    await invApi.updateContract(d.id, {
      contractNo: d.contractNo,
      name: d.name,
      bizType: d.bizType,
      partyId: d.partyId,
      signDate: formatDay(d.signDate),
      startDate: d.startDate ? formatDay(d.startDate) : undefined,
      endDate: d.endDate ? formatDay(d.endDate) : undefined,
      amount: d.amount,
      status: next,
      salesman: d.salesman,
      remark: d.remark,
      warrantyMonths: d.warrantyMonths,
      retentionRate: d.retentionRate,
      retentionAmount: d.retentionAmount,
      plans: (d.plans ?? []).map((p) => ({
        id: p.id,
        seq: p.seq,
        name: p.name,
        kind: p.kind,
        rate: p.rate,
        amount: p.amount,
        dueDate: p.dueDate ? formatDay(p.dueDate) : undefined,
        triggerNode: p.triggerNode || undefined,
        remark: p.remark || undefined,
      })),
    })
    ElMessage.success('状态已更新')
    await refreshDetail()
  } catch {
    // 失败时把下拉恢复成库里的值，避免界面显示一个并没有生效的状态
    d.status = prev
  } finally {
    statusSaving.value = false
  }
}

// ---------------- 验收 ----------------
const acceptVisible = ref(false)
const acceptSubmitting = ref(false)
const acceptForm = reactive({
  acceptedAt: '',
  acceptResult: '合格' as '合格' | '不合格' | '整改',
  warrantyMonths: 0,
  remark: '',
})

function openAccept(): void {
  if (!detail.value) return
  acceptForm.acceptedAt = detail.value.acceptedAt ? formatDay(detail.value.acceptedAt) : today()
  acceptForm.acceptResult = (detail.value.acceptResult as '合格') || '合格'
  acceptForm.warrantyMonths = detail.value.warrantyMonths
  acceptForm.remark = ''
  acceptVisible.value = true
}

async function submitAccept(): Promise<void> {
  if (!detail.value) return
  acceptSubmitting.value = true
  try {
    await invApi.acceptContract(detail.value.id, {
      acceptedAt: acceptForm.acceptedAt,
      acceptResult: acceptForm.acceptResult,
      warrantyMonths: Number(acceptForm.warrantyMonths) || 0,
      remark: acceptForm.remark || undefined,
    })
    ElMessage.success('验收已登记')
    acceptVisible.value = false
    await refreshDetail()
  } catch {
    // 提示由拦截器统一处理
  } finally {
    acceptSubmitting.value = false
  }
}

// ---------------- 期次收付款 ----------------
const settleVisible = ref(false)
const settleSubmitting = ref(false)
const settleCtx = ref<{ plan: InvContractPlanItem; bizType: InvContractBizType; contractNo: string } | null>(null)
const settleForm = reactive({ amount: 0, method: '银行转账', settledAt: '', remark: '' })

function openSettle(plan: InvContractPlanItem, c: InvContractRow): void {
  if (plan.outstanding <= 0) {
    ElMessage.info('该期已结清')
    return
  }
  settleCtx.value = { plan, bizType: c.bizType, contractNo: c.contractNo }
  settleForm.amount = plan.outstanding
  settleForm.method = '银行转账'
  settleForm.settledAt = today()
  settleForm.remark = ''
  settleVisible.value = true
}

async function submitSettle(): Promise<void> {
  if (!settleCtx.value || !detail.value) return
  const amount = Number(settleForm.amount) || 0
  if (amount <= 0) {
    ElMessage.warning('金额必须大于 0')
    return
  }
  if (amount > settleCtx.value.plan.outstanding + 0.005) {
    ElMessage.warning(`金额不能超过该期未收付余额 ${formatMoney(settleCtx.value.plan.outstanding)}`)
    return
  }
  settleSubmitting.value = true
  try {
    await invApi.settleContractPlan({
      planId: settleCtx.value.plan.id,
      amount,
      method: settleForm.method || undefined,
      settledAt: settleForm.settledAt || undefined,
      remark: settleForm.remark || undefined,
    })
    ElMessage.success('收付款已登记')
    settleVisible.value = false
    await refreshDetail()
  } catch {
    // 提示由拦截器统一处理
  } finally {
    settleSubmitting.value = false
  }
}

// ---------------- 期次收付款流水 ----------------
//
// 期次上只显示「已收付 / 未收付」两个数，但真要对账时需要知道
// 「哪天、收了多少、什么方式、谁记的」。流水是只读的，改不了历史，
// 所以单独开一个小弹窗，不打断详情里的操作。
const historyVisible = ref(false)
const historyLoading = ref(false)
const historyPlan = ref<InvContractPlanItem | null>(null)
const historyRows = ref<InvPlanSettleRecord[]>([])

async function openHistory(plan: InvContractPlanItem): Promise<void> {
  historyPlan.value = plan
  historyRows.value = []
  historyVisible.value = true
  historyLoading.value = true
  try {
    historyRows.value = await invApi.contractPlanSettles(plan.id)
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    historyLoading.value = false
  }
}

// ---------------- 附件 ----------------
const attachVisible = ref(false)
const attachBizId = ref<number | null>(null)
const attachTitle = ref('')

function openAttach(row: InvContractRow): void {
  attachBizId.value = row.id
  attachTitle.value = `合同 ${row.contractNo} 的附件`
  attachVisible.value = true
}

// ---------------- 导出 ----------------
//
// 导出与列表共用同一批筛选（含签订日期区间）：页面上筛什么、导出就是什么。
const exporting = ref(false)

async function exportRows(): Promise<void> {
  exporting.value = true
  try {
    const [startDate, endDate] = query.dateRange ?? []
    const status = query.stage === 'finished' ? 2 : query.stage === 'terminated' ? 3 : undefined
    const stage = query.stage === 'finished' || query.stage === 'terminated' ? undefined : query.stage || undefined
    await invApi.exportList(
      {
        entity: 'contract',
        keyword: query.keyword || undefined,
        bizType: query.bizType || undefined,
        stage,
        status,
        partyId: query.partyId || undefined,
        startDate,
        endDate,
      },
      '合同台账.xlsx',
    )
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exporting.value = false
  }
}

// ---------------- 打印 ----------------
const printVisible = ref(false)
const printColumns: ListPrintColumn<InvContractRow>[] = [
  { label: '合同编号', prop: 'contractNo' },
  { label: '合同名称', prop: 'name' },
  { label: '类型', value: (r: InvContractRow) => bizLabel(r.bizType) },
  { label: '往来单位', prop: 'partyName' },
  { label: '签订日期', value: (r: InvContractRow) => formatDay(r.signDate) },
  { label: '金额', value: (r: InvContractRow) => formatMoney(r.amount), align: 'right' as const },
  { label: '已收付', value: (r: InvContractRow) => formatMoney(r.settledAmount), align: 'right' as const },
  { label: '未收付', value: (r: InvContractRow) => formatMoney(r.outstanding), align: 'right' as const },
  { label: '阶段', value: (r: InvContractRow) => r.stageLabel },
  { label: '业务员', value: (r: InvContractRow) => r.salesman || '-' },
]
const printRows = computed(() => rows.value.map((r) => ({ ...r })))

onMounted(() => {
  load()
  loadParties()
})

const attachType: InvAttachBizType = 'contract'
</script>

<template>
  <div class="contract-page">
    <div class="page-card">
      <div class="list-head">
        <span class="list-head__title">
          合同管理<em>共 {{ total }} 份</em>
        </span>
        <el-form class="filter-form" label-width="76px" @submit.prevent>
          <el-form-item label="关键字">
            <el-input
              v-model="query.keyword"
              placeholder="编号 / 名称 / 往来单位 / 业务员"
              clearable
              @input="searchSoon"
              @keyup.enter="search"
            />
          </el-form-item>
          <el-form-item label="类型">
            <el-select v-model="query.bizType" placeholder="全部" clearable>
              <el-option label="采购合同" value="purchase" />
              <el-option label="销售合同" value="sales" />
            </el-select>
          </el-form-item>
          <el-form-item label="阶段">
            <el-select v-model="query.stage" placeholder="全部" clearable>
              <el-option v-for="s in stageOptions" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="签订日期">
            <el-date-picker
              v-model="query.dateRange"
              type="daterange"
              value-format="YYYY-MM-DD"
              range-separator="至"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
              unlink-panels
            />
          </el-form-item>
        </el-form>
        <div class="list-head__actions">
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
          <span class="list-head__divider" />
          <el-button :loading="exporting" @click="exportRows">导出</el-button>
          <el-button @click="printVisible = true">打印</el-button>
          <el-button v-perm="'inv:contract:create'" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>新增合同</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="contractNo" label="合同编号" width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="合同名称" min-width="170" show-overflow-tooltip />
        <el-table-column label="类型" width="96">
          <template #default="{ row }">
            <el-tag size="small" :type="row.bizType === 'purchase' ? 'warning' : 'success'" effect="plain">
              {{ bizLabel((row as InvContractRow).bizType) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="partyName" label="往来单位" min-width="140" show-overflow-tooltip />
        <el-table-column label="签订日期" width="108">
          <template #default="{ row }">{{ formatDay(row.signDate) }}</template>
        </el-table-column>
        <el-table-column label="合同金额" width="120" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="已收付" width="118" align="right">
          <template #default="{ row }">
            <span :class="{ 'text-muted': !row.settledAmount }">
              {{ row.planCount ? formatMoney(row.settledAmount) : '—' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="未收付" width="118" align="right">
          <template #default="{ row }">
            <span :class="{ 'text-warning': row.outstanding > 0 }">
              {{ row.planCount ? formatMoney(row.outstanding) : '—' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="76" align="right">
          <template #default="{ row }">{{ progressText(row as InvContractRow) }}</template>
        </el-table-column>
        <el-table-column label="阶段" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="stageTone(row.stage)" effect="light">
              {{ row.stageLabel }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="质保到期" width="106">
          <template #default="{ row }">
            {{ row.warrantyEndAt ? formatDay(row.warrantyEndAt) : '—' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="216" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row as InvContractRow)">详情</el-button>
            <el-button link type="primary" @click="openAttach(row as InvContractRow)">
              附件{{ row.attachmentCount ? `(${row.attachmentCount})` : '' }}
            </el-button>
            <el-button v-perm="'inv:contract:update'" link type="primary" @click="openEdit(row as InvContractRow)">
              编辑
            </el-button>
            <el-button v-perm="'inv:contract:delete'" link type="danger" @click="removeRow(row as InvContractRow)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="table-pager">
        <Pagination
          v-model:page="query.page"
          v-model:page-size="query.pageSize"
          :total="total"
          @change="load"
        />
      </div>
    </div>

    <!-- 新增 / 编辑 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增合同' : '编辑合同'"
      width="980px"
      top="6vh"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="合同编号">
              <el-input v-model="form.contractNo" placeholder="留空自动生成" />
            </el-form-item>
          </el-col>
          <el-col :span="16">
            <el-form-item label="合同名称" prop="name">
              <el-input v-model="form.name" placeholder="如：2026 年度钢材采购框架合同" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="合同类型">
              <el-select v-model="form.bizType" style="width: 100%" @change="onBizTypeChange">
                <el-option label="采购合同" value="purchase" />
                <el-option label="销售合同" value="sales" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="16">
            <el-form-item :label="partyLabel" prop="partyId">
              <el-select v-model="form.partyId" filterable placeholder="请选择" style="width: 100%">
                <el-option v-for="p in partyOptions" :key="p.id" :label="p.name" :value="p.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="签订日期" prop="signDate">
              <el-date-picker
                v-model="form.signDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="必填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="开始日期">
              <el-date-picker
                v-model="form.startDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="结束日期">
              <el-date-picker
                v-model="form.endDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="合同金额">
              <el-input-number
                v-model="form.amount"
                :min="0"
                :precision="2"
                :controls="false"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="业务员">
              <el-input v-model="form.salesman" placeholder="选填" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="备注">
              <el-input v-model="form.remark" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">质保条款</el-divider>
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="质保期">
              <el-input-number
                v-model="form.warrantyMonths"
                :min="0"
                :max="120"
                :controls="false"
                style="width: 100%"
              />
              <span class="unit-suffix">月（验收后自动算到期日）</span>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="质保金比例">
              <el-input-number
                v-model="form.retentionRate"
                :min="0"
                :max="100"
                :precision="2"
                :controls="false"
                style="width: 100%"
                @change="onRetentionRate"
              />
              <span class="unit-suffix">%</span>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="质保金金额">
              <el-input-number
                v-model="form.retentionAmount"
                :min="0"
                :precision="2"
                :controls="false"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">
          付款计划
          <el-button link type="primary" style="margin-left: 8px" @click="applyPlanTemplate">
            按 30/40/20/10 生成
          </el-button>
          <el-button link type="primary" @click="addPlan">添加一期</el-button>
        </el-divider>

        <el-table :data="plans" size="small" border empty-text="未设置付款计划（合同金额不计入应收应付）">
          <el-table-column label="期次" width="58" align="center">
            <template #default="{ row }">{{ row.seq }}</template>
          </el-table-column>
          <el-table-column label="名称" width="130">
            <template #default="{ row }">
              <el-input v-model="row.name" placeholder="预付款 / 尾款" />
            </template>
          </el-table-column>
          <el-table-column label="类型" width="112">
            <template #default="{ row }">
              <el-select v-model="row.kind">
                <el-option label="预付款" :value="1" />
                <el-option label="进度款" :value="2" />
                <el-option label="验收款" :value="3" />
                <el-option label="质保金" :value="4" />
                <el-option label="其他" :value="5" />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="比例%" width="96">
            <template #default="{ $index, row }">
              <el-input-number
                v-model="row.rate"
                :min="0"
                :max="100"
                :precision="2"
                :controls="false"
                :disabled="row.settledAmount > 0"
                style="width: 100%"
                @change="onPlanRate($index)"
              />
            </template>
          </el-table-column>
          <el-table-column label="金额" width="126">
            <template #default="{ $index, row }">
              <el-input-number
                v-model="row.amount"
                :min="0"
                :precision="2"
                :controls="false"
                :disabled="row.settledAmount > 0"
                style="width: 100%"
                @change="onPlanAmount($index)"
              />
            </template>
          </el-table-column>
          <el-table-column label="约定日期" width="148">
            <template #default="{ row }">
              <el-date-picker v-model="row.dueDate" type="date" value-format="YYYY-MM-DD" placeholder="选填" />
            </template>
          </el-table-column>
          <el-table-column label="触发节点" width="124">
            <template #default="{ row }">
              <el-select v-model="row.triggerNode" clearable placeholder="按日期">
                <el-option label="签订即收付" value="sign" />
                <el-option label="验收后" value="accept" />
                <el-option label="质保到期" value="warranty_end" />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="已收付" width="96" align="right">
            <template #default="{ row }">
              <span class="text-muted">{{ row.settledAmount ? formatMoney(row.settledAmount) : '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="" width="52" align="center">
            <template #default="{ $index, row }">
              <el-button
                link
                type="danger"
                :disabled="row.settledAmount > 0"
                @click="removePlan($index)"
              >
                删
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="plan-sum">
          计划合计
          <strong :class="{ over: planOver }">{{ formatMoney(planTotal) }}</strong>
          / 合同金额 {{ formatMoney(Number(form.amount) || 0) }}
          <span v-if="planOver" class="plan-sum__warn">超出合同金额</span>
          <span v-else-if="plans.length" class="plan-sum__hint">
            质保金期次合计 {{ formatMoney(retentionPlanTotal) }}
          </span>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <!-- 详情抽屉 -->
    <el-drawer v-model="detailVisible" size="920px" :title="detail ? `合同 ${detail.contractNo}` : '合同详情'">
      <div v-loading="detailLoading" class="detail-body">
        <template v-if="detail">
          <div class="detail-head">
            <div class="detail-head__main">
              <h3>{{ detail.name }}</h3>
              <div class="detail-head__tags">
                <el-tag size="small" :type="detail.bizType === 'purchase' ? 'warning' : 'success'" effect="plain">
                  {{ bizLabel(detail.bizType) }}
                </el-tag>
                <el-tag size="small" :type="stageTone(detail.stage)" effect="light">{{ detail.stageLabel }}</el-tag>
                <span class="detail-head__party">{{ detail.partyName }}</span>
              </div>
            </div>
            <div class="detail-head__actions">
              <el-select
                v-perm="'inv:contract:update'"
                v-model="detail.status"
                size="small"
                style="width: 108px"
                :loading="statusSaving"
                @change="changeStatus"
              >
                <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
              </el-select>
              <el-button v-perm="'inv:contract:accept'" size="small" @click="openAccept">
                {{ detail.acceptedAt ? '重新验收' : '登记验收' }}
              </el-button>
              <el-button
                v-perm="'inv:contract:update'"
                size="small"
                @click="openEdit(detail)"
              >
                编辑
              </el-button>
              <el-button size="small" @click="openAttach(detail)">
                附件{{ detail.attachmentCount ? `(${detail.attachmentCount})` : '' }}
              </el-button>
            </div>
          </div>

          <el-descriptions :column="3" border size="small" class="detail-desc">
            <el-descriptions-item label="签订日期">{{ formatDay(detail.signDate) }}</el-descriptions-item>
            <el-descriptions-item label="履约期">{{ periodText(detail) }}</el-descriptions-item>
            <el-descriptions-item label="业务员">{{ detail.salesman || '—' }}</el-descriptions-item>
            <el-descriptions-item label="合同金额">{{ formatMoney(detail.amount) }}</el-descriptions-item>
            <el-descriptions-item label="计划合计">{{ formatMoney(detail.planAmount) }}</el-descriptions-item>
            <el-descriptions-item label="已收付">{{ formatMoney(detail.settledAmount) }}</el-descriptions-item>
            <el-descriptions-item label="未收付">{{ formatMoney(detail.outstanding) }}</el-descriptions-item>
            <el-descriptions-item label="质保金">
              {{ detail.retentionAmount ? formatMoney(detail.retentionAmount) : '—' }}
              <span v-if="detail.retentionRate" class="text-muted">（{{ detail.retentionRate }}%）</span>
            </el-descriptions-item>
            <el-descriptions-item label="质保金未收">{{ formatMoney(detail.retentionOutstanding) }}</el-descriptions-item>
            <el-descriptions-item label="验收日期">
              {{ detail.acceptedAt ? formatDay(detail.acceptedAt) : '未验收' }}
            </el-descriptions-item>
            <el-descriptions-item label="验收结论">{{ detail.acceptResult || '—' }}</el-descriptions-item>
            <el-descriptions-item label="质保到期">
              {{ detail.warrantyEndAt ? formatDay(detail.warrantyEndAt) : '—' }}
              <span v-if="detail.warrantyMonths" class="text-muted">（{{ detail.warrantyMonths }} 个月）</span>
            </el-descriptions-item>
            <el-descriptions-item label="备注" :span="3">{{ detail.remark || '—' }}</el-descriptions-item>
          </el-descriptions>

          <el-divider content-position="left">付款计划与收付</el-divider>
          <el-table :data="detailPlans" size="small" border stripe empty-text="未设置付款计划">
            <el-table-column label="期次" width="60" align="center">
              <template #default="{ row }">第 {{ row.seq }} 期</template>
            </el-table-column>
            <el-table-column prop="name" label="名称" width="110" show-overflow-tooltip />
            <el-table-column prop="kindLabel" label="类型" width="90" />
            <el-table-column label="比例" width="72" align="right">
              <template #default="{ row }">{{ row.rate ? `${row.rate}%` : '—' }}</template>
            </el-table-column>
            <el-table-column label="金额" width="112" align="right">
              <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
            </el-table-column>
            <el-table-column label="约定日期" width="104">
              <template #default="{ row }">{{ row.dueDate ? formatDay(row.dueDate) : '—' }}</template>
            </el-table-column>
            <el-table-column label="已收付" width="112" align="right">
              <template #default="{ row }">{{ formatMoney(row.settledAmount) }}</template>
            </el-table-column>
            <el-table-column label="未收付" width="112" align="right">
              <template #default="{ row }">
                <span :class="{ 'text-warning': row.outstanding > 0 }">{{ formatMoney(row.outstanding) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="92">
              <template #default="{ row }">
                <el-tag
                  size="small"
                  effect="light"
                  :type="row.status === 'settled' ? 'success' : row.status === 'due' ? 'warning' : 'info'"
                >
                  {{ row.statusLabel }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="118" fixed="right">
              <template #default="{ row }">
                <el-button
                  v-perm="'inv:contract:settle'"
                  link
                  type="primary"
                  :disabled="row.outstanding <= 0"
                  @click="openSettle(row as InvContractPlanItem, detail as InvContractRow)"
                >
                  {{ settleVerb((detail as InvContractRow).bizType) }}
                </el-button>
                <el-button link :disabled="!row.settledAmount" @click="openHistory(row as InvContractPlanItem)">
                  流水
                </el-button>
              </template>
            </el-table-column>
          </el-table>

          <div class="plan-sum detail-plan-sum">
            计划 <strong>{{ formatMoney(detail.planAmount) }}</strong>
            ｜ 已收付 <strong>{{ formatMoney(detail.settledAmount) }}</strong>
            ｜ 未收付 <strong class="text-warning">{{ formatMoney(detail.outstanding) }}</strong>
            ｜ 质保金未收 <strong>{{ formatMoney(detail.retentionOutstanding) }}</strong>
          </div>
        </template>
      </div>
    </el-drawer>

    <!-- 验收 -->
    <el-dialog v-model="acceptVisible" title="合同验收" width="480px" destroy-on-close>
      <el-form label-width="88px">
        <el-form-item label="验收日期">
          <el-date-picker
            v-model="acceptForm.acceptedAt"
            type="date"
            value-format="YYYY-MM-DD"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="验收结论">
          <el-radio-group v-model="acceptForm.acceptResult">
            <el-radio value="合格">合格</el-radio>
            <el-radio value="不合格">不合格</el-radio>
            <el-radio value="整改">整改</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="质保期">
          <el-input-number
            v-model="acceptForm.warrantyMonths"
            :min="0"
            :max="120"
            :controls="false"
            style="width: 120px"
          />
          <span class="unit-suffix">月（到期日 = 验收日 + 质保期）</span>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="acceptForm.remark" type="textarea" :rows="2" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="acceptVisible = false">取消</el-button>
        <el-button type="primary" :loading="acceptSubmitting" @click="submitAccept">确定</el-button>
      </template>
    </el-dialog>

    <!-- 期次收付款 -->
    <el-dialog
      v-model="settleVisible"
      :title="settleCtx ? `${settleVerb(settleCtx.bizType)}登记` : '收付款'"
      width="460px"
      destroy-on-close
    >
      <el-form v-if="settleCtx" label-width="88px">
        <el-form-item label="合同">
          <span>{{ settleCtx.contractNo }} · 第 {{ settleCtx.plan.seq }} 期 {{ settleCtx.plan.name }}</span>
        </el-form-item>
        <el-form-item label="该期金额">
          <span>{{ formatMoney(settleCtx.plan.amount) }}（未收付 {{ formatMoney(settleCtx.plan.outstanding) }}）</span>
        </el-form-item>
        <el-form-item label="本次金额">
          <el-input-number
            v-model="settleForm.amount"
            :min="0.01"
            :precision="2"
            :controls="false"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="方式">
          <el-select v-model="settleForm.method" style="width: 100%">
            <el-option label="银行转账" value="银行转账" />
            <el-option label="现金" value="现金" />
            <el-option label="承兑" value="承兑" />
          </el-select>
        </el-form-item>
        <el-form-item label="日期">
          <el-date-picker v-model="settleForm.settledAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="settleForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settleVisible = false">取消</el-button>
        <el-button type="primary" :loading="settleSubmitting" @click="submitSettle">确定</el-button>
      </template>
    </el-dialog>

    <!-- 期次收付款流水 -->
    <el-dialog
      v-model="historyVisible"
      :title="historyPlan ? `第${historyPlan.seq}期 ${historyPlan.name} · 收付款流水` : '收付款流水'"
      width="580px"
      destroy-on-close
    >
      <el-table v-loading="historyLoading" :data="historyRows" size="small" border empty-text="暂无流水">
        <el-table-column label="日期" width="112">
          <template #default="{ row }">{{ formatDay(row.settledAt) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="132" align="right">
          <template #default="{ row }">
            <strong>{{ formatMoney(row.amount) }}</strong>
          </template>
        </el-table-column>
        <el-table-column prop="method" label="方式" width="112" />
        <el-table-column prop="remark" label="备注" min-width="140" show-overflow-tooltip />
      </el-table>
      <template #footer>
        <el-button @click="historyVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <AttachmentDrawer
      v-model="attachVisible"
      :biz-type="attachType"
      :biz-id="attachBizId"
      :title="attachTitle"
      @changed="detailVisible ? refreshDetail() : load()"
    />

    <ListPrintDialog
      v-model="printVisible"
      title="合同台账"
      :subtitle="`共 ${total} 份`"
      :columns="printColumns"
      :rows="printRows"
    />
  </div>
</template>

<style scoped lang="scss">
.table-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.unit-suffix {
  margin-left: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  white-space: nowrap;
}

.text-muted {
  color: var(--el-text-color-secondary);
}

.text-warning {
  color: var(--el-color-warning);
}

.plan-sum {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
  font-size: 13px;
  color: var(--el-text-color-regular);

  strong {
    font-weight: 600;
  }

  .over {
    color: var(--el-color-danger);
  }

  &__warn {
    color: var(--el-color-danger);
    font-size: 12px;
  }

  &__hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }
}

.detail-body {
  padding: 0 4px 24px;
  min-height: 200px;
}

.detail-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 12px;

  &__main h3 {
    margin: 0 0 8px;
    font-size: 16px;
    font-weight: 600;
  }

  &__tags {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__party {
    color: var(--el-text-color-regular);
    font-size: 13px;
  }

  &__actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
}

.detail-desc {
  margin-bottom: 8px;
}

.detail-plan-sum {
  margin-top: 12px;
  justify-content: flex-end;
}
</style>
