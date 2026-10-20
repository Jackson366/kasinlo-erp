<script setup lang="ts">
/**
 * 采购 / 销售单据页面主体。两张页面的字段、列、交互完全一致，
 * 差异只有「走哪条业务线」「往来单位是什么」——收敛成一个组件，两个薄壳页面传 bizType。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvOrderItem,
  InvOrderPayload,
  InvOrderRow,
  InvOrderStatus,
  InvPartyRow,
  InvSettleStatus,
  InvPickSpec,
  InvPickUnit,
  InvProductPick,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import OrderPrintDialog from './OrderPrintDialog.vue'
import { formatDay, formatMoney, formatQty } from '@/utils/format'
import { useTableQuery } from '@/composables/useTableQuery'
import { useListTools } from '@/composables/useListTools'

const props = defineProps<{
  bizType: 'purchase' | 'sales'
  title: string
  createText: string
  /** 往来单位列标题：供应商 / 客户 */
  partyLabel: string
}>()

const isPurchase = computed(() => props.bizType === 'purchase')

const statusMeta: Record<number, { text: string; type: 'warning' | 'success' | 'info' }> = {
  1: { text: '待处理', type: 'warning' },
  2: { text: '已审核', type: 'success' },
  3: { text: '已取消', type: 'info' },
}
const settleMeta: Record<string, { text: string; type: 'danger' | 'warning' | 'success' }> = {
  unpaid: { text: '未结清', type: 'danger' },
  partial: { text: '部分结算', type: 'warning' },
  paid: { text: '已结清', type: 'success' },
}

// ---------------- 列表 ----------------
interface OrderQuery {
  page?: number
  pageSize?: number
  keyword?: string
  status?: number
  settleStatus?: InvSettleStatus
  /** 单据日期区间 [开始, 结束]，空数组表示不限 */
  dateRange?: string[]
}

const { query, loading, rows, total, load, search, reset } = useTableQuery<OrderQuery, InvOrderRow>(
  { keyword: '', status: undefined, settleStatus: undefined, dateRange: [] },
  (params) =>
    invApi.orders({
      bizType: props.bizType,
      page: params.page,
      pageSize: params.pageSize,
      keyword: params.keyword || undefined,
      status: params.status ?? undefined,
      settleStatus: params.settleStatus || undefined,
      startDate: params.dateRange?.[0] || undefined,
      endDate: params.dateRange?.[1] || undefined,
    }),
)


// ---------------- 备选数据 ----------------
const parties = ref<InvPartyRow[]>([])
/** 往来单位数超过单次上限：下拉里只是一部分，必须说清楚，否则用户以为没这个客户/供应商 */
const partyTruncated = ref(false)
/** 往来单位单次拉取上限（与商品下拉的 pickLimit 对齐，避免一次性拉全量） */
const partyLimit = 200
/**
 * 商品 → 规格 → 计量单位 的级联选项。
 *
 * 为什么不用商品列表：明细的可选项是「规格 + 单位」，商品列表里没有这一层。
 * 一次拉全量（现场商品量级在几百条内）比在每一行里按需请求简单得多，
 * 也避免了三级下拉各自的异步竞态。
 */
const picks = ref<InvProductPick[]>([])
/** 商品数超过单次上限：下拉里只是一部分，必须说清楚，否则用户以为没录这个商品 */
const picksTruncated = ref(false)
/** 单次上限的实际数值，用于提示文案（后端 pickLimit） */
const pickLimit = 1000

async function loadOptions(): Promise<void> {
  const [partyRes, pickRes] = await Promise.all([
    isPurchase.value ? invApi.suppliers({ pageSize: partyLimit }) : invApi.customers({ pageSize: partyLimit }),
    invApi.productPick(),
  ])
  parties.value = partyRes.list
  partyTruncated.value = partyRes.total > partyRes.list.length
  picks.value = pickRes.items
  picksTruncated.value = pickRes.truncated
  applyPickFilter(pickQuery.value)
}

// ---------------- 表单 ----------------
interface FormItem {
  productId: number | undefined
  specId: number | undefined
  unitId: number | undefined
  qty: number
  price: number
}

/** 明细行选中商品下的规格清单 */
function specOptions(item: FormItem): InvPickSpec[] {
  if (!item.productId) return []
  return picks.value.find((p) => p.id === item.productId)?.specs ?? []
}

/** 明细行选中规格下的计量单位清单 */
function unitOptions(item: FormItem): InvPickUnit[] {
  return specOptions(item).find((s) => s.id === item.specId)?.units ?? []
}

/** 明细行当前选中的计量单位 */
function unitOf(item: FormItem): InvPickUnit | undefined {
  return unitOptions(item).find((u) => u.id === item.unitId)
}

// ---------------- 商品下拉的模糊查找 ----------------
/**
 * 下拉里当前显示的商品。
 *
 * 为什么不用 el-select 的 `remote`：**remote 模式下 el-select 不做本地过滤**，
 * 它原样显示你给的全部 options。之前把服务端搜索结果「合并」进选项后，
 * 下拉里仍然是全部商品（含不匹配的），用户看到的就像「搜索没生效」。
 * 正确做法是 `filterable` + `filter-method`，过滤结果自己维护。
 */
const filteredPicks = ref<InvProductPick[]>([])
/** 当前搜索词；选项列表变化时要按它重新过滤 */
const pickQuery = ref('')

/** 三字段模糊匹配，与后端 ProductList 的口径一致（现场报「φ20」比编码更常见） */
function matchProduct(p: InvProductPick, q: string): boolean {
  // 条码也参与匹配：扫码枪就是键盘，扫完等于把整串条码敲进搜索框，
  // 命中的通常只剩一条，回车即选中（el-select filterable 的原生行为）
  if (p.name.toLowerCase().includes(q) || p.code.toLowerCase().includes(q)) return true
  if (p.barcode && p.barcode.toLowerCase().includes(q)) return true
  return p.specs.some((s) => s.spec.toLowerCase().includes(q))
}

function applyPickFilter(query: string): void {
  pickQuery.value = query
  const q = query.trim().toLowerCase()
  const all = picks.value
  const matched = q ? all.filter((p) => matchProduct(p, q)) : all
  // 已选中的商品必须始终留在选项里：el-select 靠 options 反查 label，
  // 找不到就会把「热轧钢板」显示成数字 3 —— 用户会以为选错了。
  const picked = new Set(form.items.map((it) => it.productId).filter(Boolean) as number[])
  const missing = all.filter((p) => picked.has(p.id) && !matched.some((m) => m.id === p.id))
  filteredPicks.value = missing.length ? [...missing, ...matched] : matched
}

/** 传给 el-select 的 filter-method */
function filterProducts(query: string): void {
  applyPickFilter(query)
}
/**
 * 换算后的基本数量 = 数量 × 该单位的换算率。
 *
 * 它必须在选择的那一刻就显示：用户选「6 米」时，真正入账的是 6 × rate 个
 * 基本单位；不把折算结果摆出来，库存为什么会变成 4.0002 就只能靠猜。
 */
function qtyBase(item: FormItem): number {
  return (Number(item.qty) || 0) * (unitOf(item)?.rate ?? 1)
}

/**
 * 规格的基本单位名，用于把「折算成多少」写全（如「×0.1667 = 1 米」）。
 * 明细快照里只有成交单位，基本单位名要从商品档案补；档案已变更则留空，
 * 不因为拿不到名字就让整行显示不出来。
 */
function baseUnitNameOf(productId: number, specId: number): string {
  const spec = picks.value.find((p) => p.id === productId)?.specs.find((s) => s.id === specId)
  return spec?.units.find((u) => u.isBase)?.unit ?? ''
}

/** 规格一变，单位回到该规格的基本单位，单价跟着刷新 */
function syncUnit(item: FormItem): void {
  const units = unitOptions(item)
  item.unitId = (units.find((u) => u.isBase) ?? units[0])?.id
  applyPrice(item)
}

/** 单价默认取所选单位的采购价 / 销售价（价格挂在单位上，不在商品上） */
function applyPrice(item: FormItem): void {
  const u = unitOf(item)
  if (!u) return
  item.price = isPurchase.value ? u.purchasePrice : u.salePrice
}

const formRef = ref<FormInstance>()
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const submitting = ref(false)
const editingId = ref<number | null>(null)
const detailVisible = ref(false)
const detail = ref<{ row: InvOrderRow; items: InvOrderItem[] } | null>(null)

/**
 * 明细表的合计行。只合计「金额」一列 ——
 * 数量跨单位（箱 / 个 / 根）相加没有意义，价格相加更没有。
 * 合计取单据头部的 totalAmount 而不是把明细金额再加一遍：
 * 明细是快照，单据金额是审核后的口径，两者不一致时以单据为准。
 */
function detailSummary(param: { columns: unknown[] }): string[] {
  const last = param.columns.length - 1
  const total = detail.value?.row.totalAmount ?? 0
  return param.columns.map((_, i) => {
    if (i === 0) return '合计'
    if (i === last) return formatMoney(total)
    return ''
  })
}
const printRef = ref<InstanceType<typeof OrderPrintDialog> | null>(null)

const form = reactive<{
  orderNo: string
  partyId: number | undefined
  orderDate: string
  dueDate: string
  salesman: string
  remark: string
  items: FormItem[]
}>({
  orderNo: '',
  partyId: undefined,
  orderDate: '',
  dueDate: '',
  salesman: '',
  remark: '',
  items: [],
})

const rules: FormRules = {
  partyId: [{ required: true, message: '请选择往来单位', trigger: 'change' }],
  orderDate: [{ required: true, message: '请选择单据日期', trigger: 'change' }],
}

const formTotal = computed(() =>
  form.items.reduce((sum, it) => sum + (Number(it.qty) || 0) * (Number(it.price) || 0), 0),
)

function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 商品下拉的显示文案：名称 + 编码 + 规格数。
 *
 * 明细下行还会选到具体规格与单位，所以这里刻意**不**把规格写死进下拉文案 ——
 * 一个商品下挂了「φ20 / φ25」两个规格，把它拼进商品名会让每一行都长得一样，
 * 反而看不出「这个商品有几个规格可选」。
 */
function productLabel(p: InvProductPick): string {
  // 有码就带上：扫码枪敲完一串数字，用户要能一眼确认「选中的是这件货」
  const key = p.barcode ? `${p.code} / ${p.barcode}` : p.code
  return `${p.name}（${key}）· ${p.specs.length} 个规格`
}

function addItem(): void {
  form.items.push({ productId: undefined, specId: undefined, unitId: undefined, qty: 1, price: 0 })
}

function removeItem(index: number): void {
  form.items.splice(index, 1)
}

/** 换商品：规格与单位清空后重新落到第一个规格的基本单位 */
function onProductChange(item: FormItem): void {
  item.specId = undefined
  item.unitId = undefined
  const specs = specOptions(item)
  if (!specs.length) {
    item.price = 0
    return
  }
  item.specId = specs[0].id
  syncUnit(item)
}

/**
 * 选中商品后把搜索状态清掉。
 *
 * el-select 单选选中后会关闭下拉并清空输入框，但**我们维护的选项列表还是
 * 上一次过滤的结果** —— 不清就会留下「输入框空了、列表只剩几条」的错位，
 * 下次打开下拉看不到别的商品，用户会以为商品不见了。
 */
function onProductPickChange(item: FormItem): void {
  onProductChange(item)
  applyPickFilter('')
}

/** 打开下拉时恢复全量列表（输入框此时是空的） */
function onPickVisible(open: boolean): void {
  if (open) applyPickFilter('')
}

function onSpecChange(item: FormItem): void {
  syncUnit(item)
}

function onUnitChange(item: FormItem): void {
  applyPrice(item)
}

function openCreate(): void {
  Object.assign(form, {
    orderNo: '',
    partyId: undefined,
    orderDate: today(),
    dueDate: '',
    salesman: '',
    remark: '',
    items: [],
  })
  addItem()
  dialogMode.value = 'create'
  editingId.value = null
  dialogVisible.value = true
}

/** 编辑 / 详情的请求序号：快速点开不同行时，慢响应不能覆盖后点的那一行 */
const editSeq = ref(0)
const detailViewSeq = ref(0)

async function openEdit(row: InvOrderRow): Promise<void> {
  const seq = ++editSeq.value
  let data: Awaited<ReturnType<typeof invApi.orderDetail>>
  try {
    data = await invApi.orderDetail(row.id)
  } catch {
    // 失败提示由响应拦截器统一处理；不接住会变成未捕获 rejection
    return
  }
  if (seq !== editSeq.value) return
  // 到期日走 formatDay 归一：历史数据里的 0001-01-01 要当成「未填」，
  // 否则日期选择器会显示一个荒唐的年份，用户一保存就把脏值又写回去
  const due = formatDay(data.dueDate)
  Object.assign(form, {
    orderNo: data.orderNo,
    partyId: data.partyId,
    orderDate: formatDay(data.orderDate),
    dueDate: due === '-' ? '' : due,
    salesman: data.salesman,
    remark: data.remark,
    items: data.items.map((it) => ({
      productId: it.productId,
      specId: it.specId,
      unitId: it.unitId,
      qty: it.qty,
      price: it.price,
    })),
  })
  dialogMode.value = 'edit'
  editingId.value = row.id
  dialogVisible.value = true
}

async function submitForm(): Promise<void> {
  const valid = formRef.value ? await formRef.value.validate().catch(() => false) : false
  if (!valid) return
  const items = form.items.filter((it) => it.productId && it.specId && it.unitId)
  if (items.length === 0) {
    ElMessage.warning('请至少添加一条明细')
    return
  }
  // 有商品还没选到规格/单位时不能静默丢掉这一行 —— 用户会以为它被算进去了
  if (items.length !== form.items.length) {
    ElMessage.warning('有明细行还没选完规格与单位，请补全后再提交')
    return
  }
  const payload: InvOrderPayload = {
    bizType: props.bizType,
    // 新建时编号留空 = 后端自动生成；编辑时回填原单号原样提交（后端编辑不改单号）
    orderNo: dialogMode.value === 'create' ? form.orderNo.trim() || undefined : undefined,
    partyId: form.partyId as number,
    orderDate: form.orderDate,
    dueDate: form.dueDate || undefined,
    salesman: form.salesman.trim() || undefined,
    remark: form.remark,
    items: items.map((it) => ({
      productId: it.productId as number,
      specId: it.specId as number,
      unitId: it.unitId as number,
      qty: Number(it.qty),
      price: Number(it.price),
    })),
  }
  submitting.value = true
  try {
    if (dialogMode.value === 'create') {
      await invApi.createOrder(payload)
      ElMessage.success('新增成功')
    } else if (editingId.value) {
      await invApi.updateOrder(editingId.value, payload)
      ElMessage.success('修改成功')
    }
    dialogVisible.value = false
    load()
  } finally {
    submitting.value = false
  }
}

/** 审核确认弹窗：审核通过会过账并调整库存；可勾选「同时结清全额款项」
 *  （一手交钱一手交货的场景，省得再去应收应付页补一笔收付款） */
const approveVisible = ref(false)
const approveRow = ref<InvOrderRow | null>(null)
const approveSettleNow = ref(false)
const approveRunning = ref(false)

function openApprove(row: InvOrderRow): void {
  approveRow.value = row
  approveSettleNow.value = false
  approveVisible.value = true
}

async function confirmApprove(): Promise<void> {
  if (!approveRow.value || approveRunning.value) return
  approveRunning.value = true
  try {
    await invApi.changeOrderStatus(approveRow.value.id, 2, approveSettleNow.value)
    ElMessage.success(approveSettleNow.value ? '已审核并结清全额款项' : '审核通过')
    approveVisible.value = false
    load()
  } finally {
    approveRunning.value = false
  }
}

async function changeStatus(row: InvOrderRow, status: InvOrderStatus): Promise<void> {
  if (status === 2) {
    openApprove(row)
    return
  }
  await invApi.changeOrderStatus(row.id, status)
  ElMessage.success('操作成功')
  load()
}

/**
 * 反审核：撤销审核 —— 作废审核时生成的出入库单并回滚库存，单据退回待处理。
 * 副作用不在这一层，前端只负责说清后果再确认。
 */
async function reverseOrder(row: InvOrderRow): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确认反审核单据「${row.orderNo}」？将作废生成的出入库单并回滚库存，单据退回「待处理」。`,
      '反审核确认',
      { type: 'warning', confirmButtonText: '反审核' },
    )
  } catch {
    return
  }
  try {
    await invApi.reverseOrder(row.id)
    ElMessage.success('已反审核')
    load()
  } catch {
    // 失败提示（如「已登记收付款，不能反审核」）由响应拦截器统一处理
  }
}

async function openDetail(row: InvOrderRow): Promise<void> {
  const seq = ++detailViewSeq.value
  try {
    const data = await invApi.orderDetail(row.id)
    if (seq !== detailViewSeq.value) return
    detail.value = { row: data, items: data.items }
    detailVisible.value = true
  } catch {
    // 失败提示由响应拦截器统一处理
  }
}

// ---------------- 导出 / 打印 / 批量 ----------------
const tableRef = ref()
const {
  selection,
  exporting,
  batchRunning,
  onSelectionChange,
  clearSelection,
  exportRows,
  runBatch,
} = useListTools<InvOrderRow>({
  exportUrl: '/inv/export',
  exportParams: () => ({
    entity: 'order',
    bizType: props.bizType,
    keyword: query.keyword || undefined,
    status: query.status ?? undefined,
    settleStatus: query.settleStatus || undefined,
    startDate: query.dateRange?.[0] || undefined,
    endDate: query.dateRange?.[1] || undefined,
  }),
  fallbackName: `${props.title}.xlsx`,
})

const printVisible = ref(false)
/** 有勾选就打印勾选的，没勾选打印当前页 —— 与导出的默认口径一致 */
const printRows = computed(() => (selection.value.length ? selection.value : rows.value))

const printColumns = computed<ListPrintColumn<InvOrderRow>[]>(() => [
  { label: '单号', prop: 'orderNo' },
  { label: props.partyLabel, prop: 'partyName' },
  { label: '单据日期', value: (r) => formatDay(r.orderDate) },
  { label: '到期日', value: (r) => formatDay(r.dueDate) },
  { label: '状态', value: (r) => statusMeta[r.status]?.text || '-' },
  { label: '金额', value: (r) => formatMoney(r.totalAmount), align: 'right' },
  { label: '已结算', value: (r) => formatMoney(r.settledAmount), align: 'right' },
  { label: '结算', value: (r) => settleMeta[r.settleStatus]?.text || '-' },
  { label: '业务员', value: (r) => r.salesman || '-' },
  { label: '操作人员', value: (r) => r.operator || '-' },
])

const printSubtitle = computed(() => {
  const parts: string[] = []
  if (query.keyword) parts.push(`关键字「${query.keyword}」`)
  if (query.dateRange?.length === 2) parts.push(`日期 ${query.dateRange[0]} ~ ${query.dateRange[1]}`)
  if (query.status) parts.push(`状态 ${statusMeta[query.status]?.text}`)
  if (query.settleStatus) parts.push(`结算 ${settleMeta[query.settleStatus]?.text}`)
  if (selection.value.length) parts.push('仅打印勾选')
  return parts.join(' · ') || '全部单据'
})

/** 批量审核 / 反审核：跳过状态不符的行，避免把只读的已审核单重复提交 */
async function batchAudit(): Promise<void> {
  const target = selection.value.filter((r) => r.status === 1)
  if (!target.length) {
    ElMessage.warning('请先勾选待处理的单据')
    return
  }
  const done = await runBatch(
    '批量审核',
    target,
    (r) => invApi.changeOrderStatus(r.id, 2),
    (r) => r.orderNo,
  )
  if (done) {
    clearSelection(tableRef.value)
    load()
  }
}

async function batchReverse(): Promise<void> {
  const target = selection.value.filter((r) => r.status === 2)
  if (!target.length) {
    ElMessage.warning('请先勾选已审核的单据')
    return
  }
  const done = await runBatch('批量反审核', target, (r) => invApi.reverseOrder(r.id), (r) => r.orderNo)
  if (done) {
    clearSelection(tableRef.value)
    load()
  }
}

onMounted(() => {
  load()
  loadOptions()
})
</script>

<template>
  <div class="order-page">
    <!-- 筛选区：网格自适应列数。
         以前是 el-form inline + 固定像素宽，加了时间 / 结算之后控件宽度不一，
         第二行会参差不齐地挤在一起。改成 grid 之后每列等宽、行高一致，
         窗口变窄自动从四列折成两列，不会出现「一个控件孤零零占一行」。 -->
    <div class="page-card search-bar">
      <el-form class="filter-form" label-width="76px" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="单号 / 往来单位 / 业务员"
            clearable
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="单据日期">
          <el-date-picker
            v-model="query.dateRange"
            type="daterange"
            value-format="YYYY-MM-DD"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            unlink-panels
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="单据状态">
          <el-select v-model="query.status" placeholder="全部" clearable>
            <el-option label="待处理" :value="1" />
            <el-option label="已审核" :value="2" />
            <el-option label="已取消" :value="3" />
          </el-select>
        </el-form-item>
        <el-form-item label="结算状态">
          <el-select v-model="query.settleStatus" placeholder="全部" clearable>
            <el-option label="未结清" value="unpaid" />
            <el-option label="部分结算" value="partial" />
            <el-option label="已结清" value="paid" />
          </el-select>
        </el-form-item>
        <div class="filter-form__actions">
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
        </div>
      </el-form>
    </div>

    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <h3>{{ title }}</h3>
          <span class="text-muted">共 {{ total }} 张单据</span>
          <span v-if="selection.length" class="selection-tip">
            已选 {{ selection.length }} 项
            <a @click="clearSelection(tableRef)">清空</a>
          </span>
        </div>
        <div class="toolbar__actions">
          <!-- 批量动作只在有勾选时出现：没勾选时摆一排灰按钮，反而要多想一步 -->
          <el-dropdown v-if="selection.length" trigger="click">
            <el-button v-perm="`inv:${bizType}:finish`" :loading="batchRunning">
              批量处理
              <AppIcon name="ArrowDown" />
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="batchAudit">审核（仅待处理）</el-dropdown-item>
                <el-dropdown-item @click="batchReverse">反审核（仅已审核）</el-dropdown-item>
                <el-dropdown-item divided @click="exportRows(selection)">导出选中</el-dropdown-item>
                <el-dropdown-item @click="printVisible = true">打印选中</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button :loading="exporting" @click="exportRows()">导出</el-button>
          <el-button @click="printVisible = true">打印</el-button>
          <el-button v-perm="`inv:${bizType}:create`" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>{{ createText }}</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table
        ref="tableRef"
        v-loading="loading"
        :data="rows"
        stripe
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="44" />
        <el-table-column prop="orderNo" label="单号" width="138" />
        <el-table-column :label="partyLabel" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ row.partyName }}</template>
        </el-table-column>
        <el-table-column prop="orderDate" label="单据日期" width="100">
          <template #default="{ row }">{{ formatDay(row.orderDate) }}</template>
        </el-table-column>
        <el-table-column prop="dueDate" label="到期日" width="100">
          <template #default="{ row }">{{ formatDay(row.dueDate) }}</template>
        </el-table-column>
        <el-table-column label="明细" width="52" align="center">
          <template #default="{ row }">{{ row.itemCount }} 行</template>
        </el-table-column>
        <el-table-column label="金额" width="112" align="right">
          <template #default="{ row }">
            <strong>{{ formatMoney(row.totalAmount) }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="已结算" width="100" align="right">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatMoney(row.settledAmount) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="结算" width="84" align="center">
          <template #default="{ row }">
            <el-tag :type="settleMeta[row.settleStatus]?.type" size="small" effect="light">
              {{ settleMeta[row.settleStatus]?.text }}
            </el-tag>
          </template>
        </el-table-column>
        <!-- 业务员 / 操作人员紧挨着放：一个说「这单是谁跑来的」，一个说「谁录的」，
             现场追责时看的是这两列，分开摆反而要在屏幕上左右找 -->
        <el-table-column label="业务员" width="72" show-overflow-tooltip>
          <template #default="{ row }">{{ row.salesman || '-' }}</template>
        </el-table-column>
        <el-table-column label="操作人员" width="96" show-overflow-tooltip>
          <template #default="{ row }">{{ row.operator || '-' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="84" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta[row.status]?.type" size="small" effect="light">
              {{ statusMeta[row.status]?.text }}
            </el-tag>
          </template>
        </el-table-column>
        <!-- 宽度按最多按钮的状态算（待处理单：详情/打印/编辑/审核 4 个，
             已审核单：详情/打印/反审核 3 个），写小了按钮会折成两行 -->
        <el-table-column label="操作" width="200" fixed="right" align="center">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row as InvOrderRow)">详情</el-button>
            <el-button link type="primary" @click="printRef?.open(row as InvOrderRow)">打印</el-button>
            <el-button
              v-if="row.status === 1"
              v-perm="`inv:${bizType}:update`"
              link
              type="primary"
              @click="openEdit(row as InvOrderRow)"
            >
              编辑
            </el-button>
            <el-button
              v-if="row.status === 1"
              v-perm="`inv:${bizType}:finish`"
              link
              type="success"
              @click="changeStatus(row as InvOrderRow, 2)"
            >
              审核
            </el-button>
            <el-button
              v-if="row.status === 2"
              v-perm="`inv:${bizType}:finish`"
              link
              type="warning"
              @click="reverseOrder(row as InvOrderRow)"
            >
              反审核
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
      :title="dialogMode === 'create' ? createText : '编辑单据'"
      width="1280px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="6">
            <el-form-item label="单据编号">
              <el-input
                v-model="form.orderNo"
                :disabled="dialogMode === 'edit'"
                placeholder="留空自动生成"
                clearable
                maxlength="32"
              />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item :label="partyLabel" prop="partyId">
              <el-select v-model="form.partyId" filterable placeholder="请选择" style="width: 100%">
                <el-option v-for="p in parties" :key="p.id" :label="p.name" :value="p.id" />
              </el-select>
              <div v-if="partyTruncated" class="pick-hint">
                仅显示前 {{ partyLimit }} 个{{ partyLabel }}，请用关键字搜索其余
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="单据日期" prop="orderDate">
              <el-date-picker
                v-model="form.orderDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选择日期"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="到期日">
              <el-date-picker
                v-model="form.dueDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <!-- 业务员是自由文本而不是人员下拉：跑业务的常常没有后台账号
                 （门店、外包、已离职但历史单据还要能查），挂账号后这些人就录不进单 -->
            <el-form-item label="业务员">
              <el-input v-model="form.salesman" maxlength="64" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="明细">
          <div v-if="picksTruncated" class="pick-hint">
            商品超过 {{ pickLimit }} 个，下拉只加载了一部分 —— 可以先在「商品档案」里按关键字找到它
          </div>
          <div class="item-editor">
            <div class="item-editor__head">
              <span class="col-product">商品</span>
              <span class="col-spec">规格</span>
              <span class="col-unit">单位</span>
              <span class="col-num">数量</span>
              <span class="col-base">折算</span>
              <span class="col-num">单价</span>
              <span class="col-num">金额</span>
              <span class="col-act" />
            </div>
            <div v-for="(item, index) in form.items" :key="index" class="item-editor__row">
              <el-select
                v-model="item.productId"
                class="col-product"
                filterable
                :filter-method="filterProducts"
                placeholder="输入商品名称 / 编码 / 规格搜索"
                @change="onProductPickChange(item)"
                @visible-change="onPickVisible"
              >
                <el-option
                  v-for="p in filteredPicks"
                  :key="p.id"
                  :label="productLabel(p)"
                  :value="p.id"
                />
              </el-select>
              <el-select
                v-model="item.specId"
                class="col-spec"
                placeholder="规格"
                :disabled="!item.productId"
                @change="onSpecChange(item)"
              >
                <el-option
                  v-for="s in specOptions(item)"
                  :key="s.id"
                  :label="`${s.spec}（库存 ${formatQty(s.stock)}）`"
                  :value="s.id"
                />
              </el-select>
              <el-select
                v-model="item.unitId"
                class="col-unit"
                placeholder="单位"
                :disabled="!item.specId"
                @change="onUnitChange(item)"
              >
                <el-option
                  v-for="u in unitOptions(item)"
                  :key="u.id"
                  :label="u.unit"
                  :value="u.id"
                />
              </el-select>
              <el-input-number v-model="item.qty" class="col-num" :min="0.01" :precision="3" :controls="false" />
              <span class="col-base">
                {{ formatQty(qtyBase(item)) }}
                {{ unitOptions(item).find((u) => u.isBase)?.unit || '' }}
              </span>
              <el-input-number v-model="item.price" class="col-num" :min="0" :precision="2" :controls="false" />
              <span class="col-num amount">{{ formatMoney((item.qty || 0) * (item.price || 0)) }}</span>
              <el-button class="col-act" link type="danger" @click="removeItem(index)">删除</el-button>
            </div>
            <div class="item-editor__foot">
              <el-button link type="primary" @click="addItem">
                <AppIcon name="Plus" />
                <span>添加明细</span>
              </el-button>
              <span class="item-editor__total">
                合计：<strong>{{ formatMoney(formTotal) }}</strong>
              </span>
            </div>
          </div>
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

    <!-- 详情：与新增 / 编辑同宽同档（同一份数据的三种看法，宽度不该跳）。
         版式与「新增商品」等弹窗共用 form-section / form-grid，
         全站的弹窗因此是同一套视觉语言。 -->
    <el-dialog v-model="detailVisible" :title="`${title}详情`" width="1280px" top="6vh">
      <template v-if="detail">
        <!-- 概览条：单号与两个状态在左，金额与结算情况在右 ——
             这三项是看单据时最先要确认的东西，不该埋在描述表里 -->
        <div class="detail-head">
          <div class="detail-head__main">
            <span class="detail-head__no">{{ detail.row.orderNo }}</span>
            <el-tag :type="statusMeta[detail.row.status]?.type" size="small" effect="light">
              {{ statusMeta[detail.row.status]?.text }}
            </el-tag>
            <el-tag :type="settleMeta[detail.row.settleStatus]?.type" size="small" effect="light">
              {{ settleMeta[detail.row.settleStatus]?.text }}
            </el-tag>
          </div>
          <div class="detail-head__amount">
            <span class="detail-head__label">单据金额</span>
            <strong>{{ formatMoney(detail.row.totalAmount) }}</strong>
            <span class="detail-head__sub">
              已结算 {{ formatMoney(detail.row.settledAmount) }} · 未结
              {{ formatMoney(detail.row.totalAmount - detail.row.settledAmount) }}
            </span>
          </div>
        </div>

        <section class="form-section">
          <div class="form-section__head">
            <div class="form-section__title">基本信息</div>
          </div>
          <div class="detail-grid">
            <div class="detail-field">
              <span class="detail-field__label">{{ partyLabel }}</span>
              <div class="detail-field__value">{{ detail.row.partyName }}</div>
            </div>
            <div class="detail-field">
              <span class="detail-field__label">单据日期</span>
              <div class="detail-field__value">{{ formatDay(detail.row.orderDate) }}</div>
            </div>
            <div class="detail-field">
              <span class="detail-field__label">到期日</span>
              <div class="detail-field__value">{{ formatDay(detail.row.dueDate) }}</div>
            </div>
            <div class="detail-field">
              <span class="detail-field__label">业务员</span>
              <div class="detail-field__value">{{ detail.row.salesman || '-' }}</div>
            </div>
            <div class="detail-field">
              <span class="detail-field__label">操作人员</span>
              <div class="detail-field__value">{{ detail.row.operator || '-' }}</div>
            </div>
            <div class="detail-field">
              <span class="detail-field__label">备注</span>
              <div class="detail-field__value">{{ detail.row.remark || '-' }}</div>
            </div>
          </div>
        </section>

        <section class="form-section">
          <div class="form-section__head">
            <div class="form-section__title">
              单据明细
              <small>共 {{ detail.items.length }} 行</small>
            </div>
          </div>
          <!-- 合计做成表格的合计行而不是表格外面的一行文字：
               金额与「金额」列自然对齐，扫一眼就知道总数对应哪一列 -->
          <el-table
            :data="detail.items"
            size="small"
            show-summary
            :summary-method="detailSummary"
            class="detail-items"
          >
            <el-table-column type="index" label="#" width="52" align="center" />
            <el-table-column prop="productName" label="商品" min-width="150" show-overflow-tooltip />
            <el-table-column prop="spec" label="规格" width="120" show-overflow-tooltip />
            <el-table-column label="数量" width="120" align="right">
              <template #default="{ row }">{{ formatQty(row.qty) }} {{ row.unit }}</template>
            </el-table-column>
            <el-table-column label="折算基本数量" width="150" align="right">
              <template #default="{ row }">
                <span v-if="row.rate === 1" class="text-muted">—</span>
                <span v-else>
                  ×{{ row.rate }} = {{ formatQty(row.qtyBase) }}
                  {{ baseUnitNameOf(row.productId, row.specId) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="单价" width="110" align="right">
              <template #default="{ row }">{{ formatMoney(row.price) }}</template>
            </el-table-column>
            <el-table-column label="金额" width="120" align="right">
              <template #default="{ row }">
                <strong>{{ formatMoney(row.amount) }}</strong>
              </template>
            </el-table-column>
          </el-table>
        </section>
      </template>

      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
        <el-button type="primary" @click="detail && printRef?.open(detail.row)">打印单据</el-button>
      </template>
    </el-dialog>

    <!-- 打印 -->
    <OrderPrintDialog ref="printRef" :biz-type="bizType" :party-label="partyLabel" />

    <!-- 列表打印：有勾选打勾选的，没勾选打当前页 -->
    <ListPrintDialog
      v-model="printVisible"
      :title="title"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />

    <!-- 审核确认：可勾选同时结清全额款项 -->
    <el-dialog v-model="approveVisible" title="审核确认" width="460px">
      <el-alert type="warning" :closable="false" show-icon class="approve-alert"
        :title="`审核通过后将为「${approveRow?.orderNo}」生成出入库单并调整库存。`" />
      <el-checkbox v-model="approveSettleNow">
        同时结清全额款项
        <span class="text-muted">（{{ partyLabel === '客户' ? '登记一笔收款' : '登记一笔付款' }}，适合一手交钱一手交货）</span>
      </el-checkbox>
      <template #footer>
        <el-button @click="approveVisible = false">取消</el-button>
        <el-button type="primary" :loading="approveRunning" @click="confirmApprove">确认审核</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.approve-alert {
  margin-bottom: 14px;
}

.filter-form {
  display: grid;
  // 四列等宽；容器窄了自动折行（minmax 保证每个控件都有可用宽度，
  // 不会像 flex 那样把日期区间压成一条缝）
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 12px 20px;
  align-items: end;

  :deep(.el-form-item) {
    margin: 0;
  }

  // 按钮单独占一整行并右对齐：混在 grid 里会按列宽左对齐，
  // 看起来像「第五个筛选条件」而不是动作区
  &__actions {
    grid-column: 1 / -1;
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
}

.table-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.pick-hint {
  width: 100%;
  margin-bottom: 8px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--app-text-muted);
  background: var(--app-fill-lighter);
  border-radius: var(--radius-sm);
}

.item-editor {
  width: 100%;
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-sm);
  padding: 10px;

  &__head,
  &__row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__head {
    font-size: 12px;
    color: var(--app-text-muted);
    padding-bottom: 6px;
  }

  &__row {
    margin-bottom: 8px;
  }

  &__foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 4px;
  }

  &__total {
    font-size: 13px;
    color: var(--app-text-secondary);
  }

  // 商品列要给足宽度：下拉里是「名称（编码）· N 个规格」，被压到一百多像素时
  // 只能看到「热轧钢…」，用户根本分不清选的是哪一个。
  // 其余列都压到「刚好放下内容」的量级，把省下的空间全给商品列。
  .col-product {
    flex: 1;
    min-width: 260px;
  }

  .col-spec {
    width: 130px;
    flex: none;
  }

  .col-unit {
    width: 80px;
    flex: none;
  }

  .col-num {
    width: 94px;
    flex: none;
    text-align: right;
  }

  // 折算列是计算结果而非输入，用居中 + 次要色和可编辑列区分开
  .col-base {
    width: 100px;
    flex: none;
    text-align: center;
    font-size: 12px;
    color: var(--app-text-secondary);
  }

  .col-act {
    width: 52px;
    flex: none;
  }

  .amount {
    color: var(--app-text);
    font-weight: 600;
  }
}

// ---------------- 详情弹窗 ----------------
// 概览条：左侧单号 + 状态，右侧金额与结算情况。
// 用浅色底而不是描边卡片 —— 它只是「抬头」，不该和下面的表格抢层级。
.detail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
  margin-bottom: 18px;
  background: var(--app-bg-subtle, #f8fafc);
  border-radius: var(--radius-md);

  &__main {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  &__no {
    font-family: var(--app-font-mono, monospace);
    font-size: 15px;
    font-weight: 600;
    color: var(--app-text);
  }

  &__amount {
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex: none;
  }

  &__label {
    font-size: 12px;
    color: var(--app-text-muted);
  }

  &__amount strong {
    font-size: 20px;
    font-weight: 600;
    color: var(--app-text);
    font-variant-numeric: tabular-nums;
  }

  &__sub {
    font-size: 12px;
    color: var(--app-text-muted);
  }
}

// 详情字段：三列等宽，标签在上、值在下 ——
// 值的长短差别很大（单号 / 备注），左对齐的标签在右会让右侧全空。
.detail-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px 20px;
}

.detail-field {
  min-width: 0;

  &__label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--app-text-muted);
  }

  &__value {
    font-size: 13px;
    color: var(--app-text);
    word-break: break-word;
  }
}
</style>
