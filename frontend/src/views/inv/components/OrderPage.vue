<script setup lang="ts">
/**
 * 共享单据查询、录入与过账逻辑。采购另提供连续核对工作区，销售保留列表路径。
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
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
  InvAttachBizType,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import ListPrintDialog, {
  type ListPrintColumn,
} from '@/components/ListPrintDialog.vue'
import OrderPrintDialog from './OrderPrintDialog.vue'
import AttachmentDrawer from './AttachmentDrawer.vue'
import PurchaseWorkbench from './PurchaseWorkbench.vue'
import OrderDetailContent from './OrderDetailContent.vue'
import { statusMeta, settleMeta } from './orderPresentation'
import { formatDay, formatMoney, formatQty } from '@/utils/format'
import { useTableQuery } from '@/composables/useTableQuery'
import { useUserStore } from '@/store'
import { useListTools } from '@/composables/useListTools'
import { useDebouncedSearch } from '@/composables/useDebouncedSearch'

const props = defineProps<{
  bizType: 'purchase' | 'sales'
  title: string
  createText: string
  /** 往来单位列标题：供应商 / 客户 */
  partyLabel: string
}>()

const userStore = useUserStore()
const isPurchase = computed(() => props.bizType === 'purchase')
const route = useRoute()
const router = useRouter()
const pagePath = route.path
const workspaceView = ref<'pending' | 'all'>(
  isPurchase.value && route.query.view !== 'all' ? 'pending' : 'all',
)
const isWorkbench = computed(
  () => isPurchase.value && workspaceView.value === 'pending',
)
const dueDateLabel = computed(() =>
  isPurchase.value ? '付款到期日' : '到期日',
)
const composing = ref(false)
const selectedId = ref<number | null>(null)
const mobileDetail = ref(false)
const detailLoading = ref(false)
const detailError = ref(false)
const detailTarget = ref<Pick<InvOrderRow, 'id'> | null>(null)
// 审核后保留已处理单据及结果，用户确认后再选择下一张。
const keepReviewResult = ref(false)
const reviewResult = ref('')

function urlNumber(value: unknown, fallback: number, max: number): number {
  const n = Number(value)
  return Number.isInteger(n) && n > 0 && n <= max ? n : fallback
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

const {
  query,
  loading,
  error: listError,
  rows,
  total,
  load: loadList,
} = useTableQuery<OrderQuery, InvOrderRow>(
  {
    keyword:
      isPurchase.value && typeof route.query.q === 'string'
        ? route.query.q
        : '',
    status: isWorkbench.value
      ? 1
      : isPurchase.value
        ? [1, 2, 3].includes(Number(route.query.status))
          ? Number(route.query.status)
          : undefined
        : undefined,
    settleStatus:
      !isWorkbench.value &&
      isPurchase.value &&
      ['unpaid', 'partial', 'paid'].includes(String(route.query.settle))
        ? (route.query.settle as InvSettleStatus)
        : undefined,
    dateRange:
      isPurchase.value &&
      typeof route.query.start === 'string' &&
      typeof route.query.end === 'string'
        ? [route.query.start, route.query.end]
        : [],
    page: isPurchase.value ? urlNumber(route.query.page, 1, 100000) : 1,
    pageSize: isPurchase.value ? urlNumber(route.query.size, 10, 200) : 10,
  },
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
const hasFilters = computed(() =>
  Boolean(
    query.keyword ||
    query.dateRange?.length ||
    query.settleStatus ||
    (!isWorkbench.value && query.status),
  ),
)

function persistQuery(): void {
  if (!isPurchase.value || route.path !== pagePath) return
  void router.replace({
    path: pagePath,
    query: {
      view: workspaceView.value,
      q: query.keyword || undefined,
      status:
        !isWorkbench.value && query.status ? String(query.status) : undefined,
      settle: query.settleStatus || undefined,
      start: query.dateRange?.[0] || undefined,
      end: query.dateRange?.[1] || undefined,
      page: query.page > 1 ? String(query.page) : undefined,
      size: query.pageSize !== 10 ? String(query.pageSize) : undefined,
    },
  })
}
let listLoadSeq = 0
async function load(): Promise<void> {
  const seq = ++listLoadSeq
  clearSelection(tableRef.value)
  persistQuery()
  await loadList()
  if (seq !== listLoadSeq) return
  // 审核移走最后一行后，回到有效页，避免留在空的末页。
  const maxPage = Math.max(1, Math.ceil(total.value / query.pageSize))
  if (!listError.value && query.page > maxPage) {
    query.page = maxPage
    persistQuery()
    await loadList()
  }
}
function search(): void {
  if (composing.value) return
  cancelSearch()
  keepReviewResult.value = false
  reviewResult.value = ''
  query.page = 1
  void load()
}
function reset(): void {
  cancelSearch()
  Object.assign(query, {
    keyword: '',
    dateRange: [],
    status: isWorkbench.value ? 1 : undefined,
    settleStatus: undefined,
    page: 1,
    pageSize: 10,
  })
  keepReviewResult.value = false
  reviewResult.value = ''
  void load()
}
function changeView(value: string | number): void {
  workspaceView.value = value === 'all' ? 'all' : 'pending'
  selectedId.value = null
  detail.value = null
  detailViewSeq.value++
  mobileDetail.value = false
  keepReviewResult.value = false
  reviewResult.value = ''
  query.status = isWorkbench.value ? 1 : undefined
  query.settleStatus = undefined
  search()
}
function selectOrder(row: InvOrderRow): void {
  keepReviewResult.value = false
  reviewResult.value = ''
  mobileDetail.value = true
  void openDetail(row)
}
function nextOrder(): void {
  keepReviewResult.value = false
  reviewResult.value = ''
  if (rows.value.length) selectOrder(rows.value[0])
  else {
    selectedId.value = null
    detail.value = null
    mobileDetail.value = false
  }
}

// ---------------- 附件 ----------------
//
// 附件挂在单据上，列表只给一个带角标的入口。
// 数量用一次批量查询拿（attachmentCounts），不逐行发请求 ——
// 20 行列表逐行查就是 20 个请求，而它只是个角标。
const attachCounts = ref<Record<string, number>>({})
const attachVisible = ref(false)
const attachBizId = ref<number | null>(null)
const attachTitle = ref('')

watch(
  rows,
  (list) => {
    const ids = list.map((r) => r.id)
    if (!ids.length) {
      attachCounts.value = {}
      return
    }
    invApi
      .attachmentCounts(props.bizType as InvAttachBizType, ids)
      .then((m) => {
        attachCounts.value = m
      })
      .catch(() => {
        // 角标是装饰：这个查询失败不该让列表看起来像坏了
      })
  },
  { immediate: true },
)

function openAttach(row: InvOrderRow): void {
  attachBizId.value = row.id
  attachTitle.value = `${props.bizType === 'purchase' ? '采购单' : '销售单'} ${row.orderNo} 的附件`
  attachVisible.value = true
}

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

const optionsLoading = ref(false)
const optionsError = ref(false)
let optionsSeq = 0
async function loadOptions(preservePrices = false): Promise<void> {
  const seq = ++optionsSeq
  optionsLoading.value = true
  optionsError.value = false
  try {
    const [partyRes, pickRes] = await Promise.all([
      isPurchase.value
        ? invApi.suppliers({ pageSize: partyLimit })
        : invApi.customers({ pageSize: partyLimit }),
      invApi.productPick(),
    ])
    if (seq !== optionsSeq) return
    parties.value = partyRes.list
    partyTruncated.value = partyRes.total > partyRes.list.length
    picks.value = pickRes.items
    picksTruncated.value = pickRes.truncated
    applyPickFilter(pickQuery.value)
    // 档案是异步拉回来的：用户手快时（新商品刚建完就开单）可能先选了商品、
    // 而那一瞬间 picks 里还没有它，applyPrice 找不到商品会把单价留成 0。
    // 拿到档案后补一次，单价才不会再是 0。
    form.items.forEach((it) => {
      if (!preservePrices && it.productId && !Number(it.price)) applyPrice(it)
    })
  } catch (error) {
    if (seq === optionsSeq) optionsError.value = true
    throw error
  } finally {
    if (seq === optionsSeq) optionsLoading.value = false
  }
}
function refreshOptions(): void {
  void loadOptions(true).catch(() => {})
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
  if (p.name.toLowerCase().includes(q) || p.code.toLowerCase().includes(q))
    return true
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
  const picked = new Set(
    form.items.map((it) => it.productId).filter(Boolean) as number[],
  )
  const missing = all.filter(
    (p) => picked.has(p.id) && !matched.some((m) => m.id === p.id),
  )
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

/** 规格一变，单位回到该规格的基本单位，单价跟着刷新 */
function syncUnit(item: FormItem): void {
  const units = unitOptions(item)
  item.unitId = (units.find((u) => u.isBase) ?? units[0])?.id
  applyPrice(item)
}

/** 单价默认取所选单位的采购价 / 销售价（价格挂在单位上，不在商品上） */
/**
 * 选中商品 / 规格 / 单位后带出默认单价。
 * 优先级：商品级参考价（>0）→ 该单位的采购价 / 销售价。
 * 参考价是「开单价的心理锚点」，录了就用它；没录才回落到单位上的历史成交价。
 */
function applyPrice(item: FormItem): void {
  const u = unitOf(item)
  if (!u) return
  const p = picks.value.find((x) => x.id === item.productId)
  const ref = isPurchase.value
    ? Number(p?.refPurchasePrice ?? 0)
    : Number(p?.refSalePrice ?? 0)
  item.price = ref > 0 ? ref : isPurchase.value ? u.purchasePrice : u.salePrice
}

const formRef = ref<FormInstance>()
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const submitting = ref(false)
const validating = ref(false)
const saveError = ref(false)
const formBaseline = ref('')
const editingId = ref<number | null>(null)
const detailVisible = ref(false)
const detail = ref<{ row: InvOrderRow; items: InvOrderItem[] } | null>(null)

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
  form.items.reduce(
    (sum, it) => sum + (Number(it.qty) || 0) * (Number(it.price) || 0),
    0,
  ),
)

function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 商品下拉的显示文案：名称（编码）+ 参考价。
 *
 * 刻意**不**带规格数量与单位：明细行后面就有「规格」「单位」两列，
 * 再塞进商品名里只会让每一行都很长、还看不出这是同一件货。
 */
function productLabel(p: InvProductPick): string {
  // 有码就带上：扫码枪敲完一串数字，用户要能一眼确认「选中的是这件货」
  const key = p.barcode ? `${p.code} / ${p.barcode}` : p.code
  const ref = isPurchase.value ? p.refPurchasePrice : p.refSalePrice
  const refText =
    ref > 0
      ? ` · 参考${isPurchase.value ? '进价' : '售价'} ¥${ref.toFixed(2)}`
      : ''
  return `${p.name}（${key}）${refText}`
}

function addItem(): void {
  form.items.push({
    productId: undefined,
    specId: undefined,
    unitId: undefined,
    qty: 1,
    price: 0,
  })
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
  formBaseline.value = JSON.stringify(form)
  saveError.value = false
  dialogVisible.value = true
  // 重拉档案下拉：客户 / 供应商 / 商品可能在其它页面刚新增过，
  // 只在页面挂载时拉一次的话，弹窗里永远是旧数据（要刷新整页才看得见新档案）。
  // 拉取失败不拦弹窗 —— 用现有缓存，错误提示由拦截器统一处理。
  loadOptions().catch(() => {})
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
  formBaseline.value = JSON.stringify(form)
  saveError.value = false
  dialogVisible.value = true
  // 同 openCreate：编辑时换商品也要能看到刚新增的档案
  loadOptions().catch(() => {})
}

async function submitForm(): Promise<void> {
  if (submitting.value || validating.value) return
  validating.value = true
  const valid = formRef.value
    ? await formRef.value.validate().catch(() => false)
    : false
  validating.value = false
  if (!valid) return
  const items = form.items.filter(
    (it) => it.productId && it.specId && it.unitId,
  )
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
    orderNo:
      dialogMode.value === 'create'
        ? form.orderNo.trim() || undefined
        : undefined,
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
  saveError.value = false
  try {
    let createdId: number | null = null
    if (dialogMode.value === 'create') {
      const created = await invApi.createOrder(payload)
      createdId = created.id
      ElMessage.success(
        isPurchase.value ? '采购单已保存，尚未入库' : '销售单已保存',
      )
    } else if (editingId.value) {
      await invApi.updateOrder(editingId.value, payload)
      ElMessage.success('修改成功')
    }
    dialogVisible.value = false
    formBaseline.value = JSON.stringify(form)
    if (isWorkbench.value && createdId) {
      // 新单即使不属于当前筛选页，也在核对区保留明确的保存结果。
      keepReviewResult.value = true
      reviewResult.value =
        '采购单已保存，尚未增加库存。确认收到货物并核对后，再审核并入库。'
      mobileDetail.value = true
      await openDetail({ id: createdId })
    } else if (
      isPurchase.value &&
      detailTarget.value &&
      (detailVisible.value || keepReviewResult.value)
    ) {
      await openDetail(detailTarget.value)
    }
    await load()
  } catch {
    saveError.value = true
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
  const target = approveRow.value
  approveRunning.value = true
  try {
    await invApi.changeOrderStatus(target.id, 2, approveSettleNow.value)
    ElMessage.success(
      approveSettleNow.value ? '已审核并结清全额款项' : '审核通过',
    )
    approveVisible.value = false
    if (isWorkbench.value) {
      keepReviewResult.value = true
      reviewResult.value = approveSettleNow.value
        ? '已生成入库单、增加库存，并登记全额付款。'
        : '已生成入库单、增加库存，并形成应付；尚未登记付款。'
      const reviewed = target
      await openDetail(reviewed)
    } else if (
      isPurchase.value &&
      detailVisible.value &&
      detail.value?.row.id === target.id
    ) {
      await openDetail(target)
    }
    await load()
  } catch {
    // 请求层显示服务端错误；保留确认与当前单据，允许核实后重试。
  } finally {
    approveRunning.value = false
  }
}

async function changeStatus(
  row: InvOrderRow,
  status: InvOrderStatus,
): Promise<void> {
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
    await load()
    if (
      isPurchase.value &&
      detailVisible.value &&
      detailTarget.value?.id === row.id
    )
      await openDetail(row)
  } catch {
    // 失败提示（如「已登记收付款，不能反审核」）由响应拦截器统一处理
  }
}

async function openDetail(row: Pick<InvOrderRow, 'id'>): Promise<void> {
  const seq = ++detailViewSeq.value
  const inline = isWorkbench.value
  selectedId.value = row.id
  detailTarget.value = row
  detailLoading.value = true
  detailError.value = false
  if (inline) detail.value = null
  try {
    const data = await invApi.orderDetail(row.id)
    if (seq !== detailViewSeq.value) return
    detail.value = { row: data, items: data.items }
    if (!inline) detailVisible.value = true
  } catch {
    if (seq === detailViewSeq.value) detailError.value = true
  } finally {
    if (seq === detailViewSeq.value) detailLoading.value = false
  }
}
function retryDetail(): void {
  if (detailTarget.value) void openDetail(detailTarget.value)
}
watch(rows, (list) => {
  if (!isWorkbench.value || keepReviewResult.value) return
  const row = list.find((r) => r.id === selectedId.value) ?? list[0]
  if (row) void openDetail(row)
  else {
    ++detailViewSeq.value
    detailLoading.value = false
    detail.value = null
    selectedId.value = null
    mobileDetail.value = false
  }
})

const hasUnsavedChanges = computed(
  () =>
    isPurchase.value &&
    dialogVisible.value &&
    JSON.stringify(form) !== formBaseline.value,
)
async function canCloseForm(): Promise<boolean> {
  if (submitting.value || validating.value) return false
  if (!hasUnsavedChanges.value) return true
  try {
    await ElMessageBox.confirm(
      '采购单尚未保存，离开后本次修改将丢失。',
      '放弃本次修改？',
      {
        confirmButtonText: '放弃修改',
        cancelButtonText: '继续编辑',
        type: 'warning',
        autofocus: false,
      },
    )
    return true
  } catch {
    return false
  }
}
async function requestCloseForm(): Promise<void> {
  if (await canCloseForm()) dialogVisible.value = false
}
async function beforeFormClose(done: () => void): Promise<void> {
  if (await canCloseForm()) done()
}
onBeforeRouteLeave(async () => {
  const allowed = await canCloseForm()
  if (allowed) dialogVisible.value = false
  return allowed
})
function beforeUnload(event: BeforeUnloadEvent): void {
  if (!hasUnsavedChanges.value) return
  event.preventDefault()
  event.returnValue = ''
}
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))

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
const printRows = computed(() =>
  selection.value.length ? selection.value : rows.value,
)

const printColumns = computed<ListPrintColumn<InvOrderRow>[]>(() => [
  { label: '单号', prop: 'orderNo' },
  { label: props.partyLabel, prop: 'partyName' },
  { label: '单据日期', value: (r) => formatDay(r.orderDate) },
  { label: dueDateLabel.value, value: (r) => formatDay(r.dueDate) },
  { label: '状态', value: (r) => statusMeta[r.status]?.text || '-' },
  { label: '金额', value: (r) => formatMoney(r.totalAmount), align: 'right' },
  {
    label: '已结算',
    value: (r) => formatMoney(r.settledAmount),
    align: 'right',
  },
  {
    label: isPurchase.value ? '付款状态' : '结算',
    value: (r) =>
      isPurchase.value && r.status !== 2
        ? r.status === 1
          ? '审核后形成应付'
          : '—'
        : settleMeta[r.settleStatus]?.text || '-',
  },
  { label: '业务员', value: (r) => r.salesman || '-' },
  { label: '操作人员', value: (r) => r.operator || '-' },
])

const printSubtitle = computed(() => {
  const parts: string[] = []
  if (query.keyword) parts.push(`关键字「${query.keyword}」`)
  if (query.dateRange?.length === 2)
    parts.push(`日期 ${query.dateRange[0]} ~ ${query.dateRange[1]}`)
  if (query.status) parts.push(`状态 ${statusMeta[query.status]?.text}`)
  if (query.settleStatus)
    parts.push(`结算 ${settleMeta[query.settleStatus]?.text}`)
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
  const done = await runBatch(
    '批量反审核',
    target,
    (r) => invApi.reverseOrder(r.id),
    (r) => r.orderNo,
  )
  if (done) {
    clearSelection(tableRef.value)
    load()
  }
}

function consumeCreateAction(): void {
  if (route.path !== pagePath || route.query.action !== 'create') return
  if (!userStore.hasAnyPerm([`inv:${props.bizType}:create`])) return
  openCreate()
  const { action: _action, ...rest } = route.query
  void router.replace({ path: pagePath, query: rest })
}
watch(() => route.query.action, consumeCreateAction)
onMounted(() => {
  consumeCreateAction()
  window.addEventListener('beforeunload', beforeUnload)
  void load()
  void loadOptions().catch(() => {})
})
// 关键字录入即查询（防抖），不必再按回车
const {
  searchSoon,
  searchNow,
  cancel: cancelSearch,
} = useDebouncedSearch(search)
function onCompositionEnd(): void {
  composing.value = false
  searchSoon()
}
function onKeywordInput(): void {
  if (!composing.value) searchSoon()
}
</script>

<template>
  <div class="order-page" :class="{ 'purchase-page': isPurchase }">
    <header v-if="isPurchase" class="purchase-header">
      <h1>采购工作台</h1>
      <el-tabs
        :model-value="workspaceView"
        class="purchase-tabs"
        @tab-change="changeView"
      >
        <el-tab-pane label="待处理" name="pending" />
        <el-tab-pane label="全部单据" name="all" />
      </el-tabs>
      <el-button
        v-perm="'inv:purchase:create'"
        type="primary"
        @click="openCreate"
      >
        <AppIcon name="Plus" />新增采购单
      </el-button>
    </header>
    <div class="page-card">
      <!-- 搜索与操作合并一行：左侧列表名，中间筛选（自动折行），右侧查询/重置/新建/刷新 -->
      <div class="list-head">
        <span class="list-head__title">
          {{ isWorkbench ? '待处理采购单' : title
          }}<em>{{ hasFilters ? '筛选结果' : '共' }} {{ total }} 张</em>
          <em v-if="selection.length">
            已选 {{ selection.length }} 项
            <el-button link type="primary" @click="clearSelection(tableRef)"
              >清空</el-button
            >
          </em>
        </span>
        <el-form
          class="filter-form"
          label-width="76px"
          novalidate
          @submit.prevent
        >
          <el-form-item label="关键字">
            <el-input
              v-model="query.keyword"
              @input="onKeywordInput"
              @compositionstart="composing = true"
              @compositionend="onCompositionEnd"
              @clear="searchNow"
              placeholder="单号 / 往来单位 / 业务员"
              clearable
              @keyup.enter="searchNow"
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
            />
          </el-form-item>
          <el-form-item v-if="!isWorkbench" label="单据状态">
            <el-select v-model="query.status" placeholder="全部" clearable>
              <el-option label="待处理" :value="1" />
              <el-option label="已审核" :value="2" />
              <el-option label="已取消" :value="3" />
            </el-select>
          </el-form-item>
          <el-form-item
            v-if="!isWorkbench"
            :label="isPurchase ? '付款状态' : '结算状态'"
          >
            <el-select
              v-model="query.settleStatus"
              placeholder="全部"
              clearable
            >
              <el-option label="未结清" value="unpaid" />
              <el-option label="部分结算" value="partial" />
              <el-option label="已结清" value="paid" />
            </el-select>
          </el-form-item>
        </el-form>
        <div class="list-head__actions">
          <el-button
            :type="isPurchase ? 'default' : 'primary'"
            @click="searchNow"
          >
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
          <span v-if="!isWorkbench" class="list-head__divider" />
          <!-- 批量动作只在有勾选时出现：没勾选时摆一排灰按钮，反而要多想一步 -->
          <el-dropdown v-if="selection.length" trigger="click">
            <el-button v-perm="`inv:${bizType}:finish`" :loading="batchRunning">
              批量处理
              <AppIcon name="ArrowDown" />
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="batchAudit"
                  >审核（仅待处理）</el-dropdown-item
                >
                <el-dropdown-item @click="batchReverse"
                  >反审核（仅已审核）</el-dropdown-item
                >
                <el-dropdown-item divided @click="exportRows(selection)"
                  >导出选中</el-dropdown-item
                >
                <el-dropdown-item @click="printVisible = true"
                  >打印选中</el-dropdown-item
                >
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button
            v-if="!isWorkbench"
            :loading="exporting"
            @click="exportRows()"
            >导出</el-button
          >
          <el-button v-if="!isWorkbench" @click="printVisible = true"
            >打印</el-button
          >
          <el-button
            v-if="!isPurchase"
            v-perm="`inv:${bizType}:create`"
            type="primary"
            @click="openCreate"
          >
            <AppIcon name="Plus" />
            <span>{{ createText }}</span>
          </el-button>
          <el-button aria-label="刷新单据" :loading="loading" @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <div v-if="listError" class="purchase-list-error" role="alert">
        <strong>单据列表加载失败</strong>
        <p>当前数据可能已变化，请重新加载后继续处理。</p>
        <el-button @click="load">重新加载</el-button>
      </div>
      <PurchaseWorkbench
        v-else-if="isWorkbench"
        :rows="rows"
        :selected-id="selectedId"
        :loading="loading"
        :detail-loading="detailLoading"
        :detail-error="detailError"
        :has-detail="Boolean(detail)"
        :filtered="hasFilters"
        :mobile-detail="mobileDetail"
        @select="selectOrder"
        @retry="retryDetail"
        @reset="reset"
        @back="mobileDetail = false"
      >
        <template v-if="detail">
          <el-alert
            v-if="reviewResult"
            :title="reviewResult"
            type="success"
            show-icon
            :closable="false"
            class="purchase-result"
          />
          <OrderDetailContent
            :order="detail.row"
            :items="detail.items"
            :party-label="partyLabel"
            :is-purchase="true"
            :picks="picks"
          />
          <div class="purchase-review-actions">
            <div class="purchase-review-actions__secondary">
              <el-button @click="openAttach(detail.row)"
                >附件{{
                  attachCounts[String(detail.row.id)]
                    ? ` (${attachCounts[String(detail.row.id)]})`
                    : ''
                }}</el-button
              >
              <el-button @click="printRef?.open(detail.row)">打印</el-button>
            </div>
            <div class="purchase-review-actions__primary">
              <template v-if="detail.row.status === 1">
                <el-button
                  v-perm="'inv:purchase:update'"
                  :disabled="loading"
                  @click="openEdit(detail.row)"
                  >编辑采购单</el-button
                >
                <el-button
                  v-perm="'inv:purchase:finish'"
                  type="primary"
                  :disabled="loading"
                  @click="openApprove(detail.row)"
                  >审核并入库</el-button
                >
              </template>
              <el-button
                v-else
                type="primary"
                :disabled="loading"
                @click="nextOrder"
                >{{ rows.length ? '处理下一张' : '返回待办' }}</el-button
              >
            </div>
          </div>
        </template>
      </PurchaseWorkbench>
      <el-table
        v-else
        ref="tableRef"
        v-loading="loading"
        :data="rows"
        stripe
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="44" />
        <el-table-column prop="orderNo" label="单号" width="150">
          <template #default="{ row }"
            ><el-button
              link
              type="primary"
              @click="openDetail(row as InvOrderRow)"
              >{{ row.orderNo }}</el-button
            ></template
          >
        </el-table-column>
        <el-table-column
          :label="partyLabel"
          min-width="120"
          show-overflow-tooltip
        >
          <template #default="{ row }">{{ row.partyName }}</template>
        </el-table-column>
        <el-table-column
          prop="orderDate"
          label="单据日期"
          :width="isPurchase ? 112 : 100"
        >
          <template #default="{ row }">{{ formatDay(row.orderDate) }}</template>
        </el-table-column>
        <el-table-column
          prop="dueDate"
          :label="dueDateLabel"
          :width="isPurchase ? 120 : 100"
        >
          <template #default="{ row }">{{ formatDay(row.dueDate) }}</template>
        </el-table-column>
        <el-table-column
          label="明细"
          :width="isPurchase ? 72 : 52"
          align="center"
        >
          <template #default="{ row }">{{ row.itemCount }} 行</template>
        </el-table-column>
        <el-table-column label="金额" width="112" align="right">
          <template #default="{ row }">
            <strong>{{ formatMoney(row.totalAmount) }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="已结算" width="100" align="right">
          <template #default="{ row }">
            <span class="text-secondary">{{
              formatMoney(row.settledAmount)
            }}</span>
          </template>
        </el-table-column>
        <el-table-column
          :label="isPurchase ? '付款状态' : '结算'"
          :width="isPurchase ? 132 : 84"
          align="center"
        >
          <template #default="{ row }">
            <span
              v-if="isPurchase && row.status !== 2"
              class="text-secondary"
              >{{ row.status === 1 ? '审核后形成应付' : '—' }}</span
            >
            <el-tag
              v-else
              :type="settleMeta[row.settleStatus]?.type"
              size="small"
              effect="light"
            >
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
            <el-tag
              :type="statusMeta[row.status]?.type"
              size="small"
              effect="light"
            >
              {{ statusMeta[row.status]?.text }}
            </el-tag>
          </template>
        </el-table-column>
        <!-- 宽度按最多按钮的状态算（待处理单：详情/打印/编辑/审核 4 个，
             已审核单：详情/打印/反审核 3 个），写小了按钮会折成两行 -->
        <el-table-column
          label="操作"
          :width="isPurchase ? 210 : 250"
          fixed="right"
          :align="isPurchase ? 'left' : 'center'"
        >
          <template #default="{ row }">
            <div :class="{ 'purchase-row-actions': isPurchase }">
              <el-button
                v-if="!isPurchase"
                link
                type="primary"
                @click="openDetail(row as InvOrderRow)"
                >查看</el-button
              >
              <el-button
                v-if="!isPurchase"
                link
                type="primary"
                @click="openAttach(row as InvOrderRow)"
              >
                附件{{
                  attachCounts[String(row.id)]
                    ? `(${attachCounts[String(row.id)]})`
                    : ''
                }}
              </el-button>
              <el-button
                v-if="!isPurchase"
                link
                type="primary"
                @click="printRef?.open(row as InvOrderRow)"
                >打印</el-button
              >
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
                {{ isPurchase ? '审核并入库' : '审核' }}
              </el-button>
              <el-dropdown v-if="isPurchase" trigger="click">
                <el-button link type="primary"
                  >更多<AppIcon name="ArrowDown"
                /></el-button>
                <template #dropdown
                  ><el-dropdown-menu>
                    <el-dropdown-item @click="openAttach(row as InvOrderRow)"
                      >附件{{
                        attachCounts[String(row.id)]
                          ? ` (${attachCounts[String(row.id)]})`
                          : ''
                      }}</el-dropdown-item
                    >
                    <el-dropdown-item
                      @click="printRef?.open(row as InvOrderRow)"
                      >打印</el-dropdown-item
                    >
                    <el-dropdown-item
                      v-if="row.status === 2"
                      v-perm="'inv:purchase:finish'"
                      divided
                      :disabled="row.settledAmount > 0"
                      @click="reverseOrder(row as InvOrderRow)"
                    >
                      {{
                        row.settledAmount > 0
                          ? '已有付款，无法反审核'
                          : '反审核'
                      }}
                    </el-dropdown-item>
                  </el-dropdown-menu></template
                >
              </el-dropdown>
              <el-button
                v-if="row.status === 2 && !isPurchase"
                v-perm="`inv:${bizType}:finish`"
                link
                type="warning"
                @click="reverseOrder(row as InvOrderRow)"
              >
                反审核
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="!listError" class="table-pager">
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
      :before-close="beforeFormClose"
      destroy-on-close
    >
      <el-alert
        v-if="saveError"
        title="保存失败，本次输入已保留。请检查提示后重新保存。"
        type="error"
        :closable="false"
        show-icon
        class="purchase-result"
      />
      <section
        v-if="isPurchase"
        class="purchase-entry-help"
        aria-label="采购录入说明"
      >
        <div>
          <strong>{{
            dialogMode === 'create' ? '先录单，再核对入库' : '修改采购信息'
          }}</strong>
          <p>
            选择供应商和商品，填写数量与单价。保存只记录单据，不会增加库存或登记付款。
          </p>
        </div>
        <div class="purchase-entry-help__tools">
          <router-link
            v-if="
              userStore.hasAnyPerm(['inv:supplier:create']) &&
              router.hasRoute('InvSupplier')
            "
            to="/inv/supplier"
            target="_blank"
            rel="noopener"
            >维护供应商（新标签页）</router-link
          >
          <router-link
            v-if="
              userStore.hasAnyPerm(['inv:product:create']) &&
              router.hasRoute('InvProduct')
            "
            to="/inv/product"
            target="_blank"
            rel="noopener"
            >维护商品（新标签页）</router-link
          >
          <el-button :loading="optionsLoading" @click="refreshOptions"
            >刷新可选资料</el-button
          >
        </div>
        <p class="purchase-entry-help__hint">
          新增资料后回到这里刷新，当前填写的单据会保留。
        </p>
        <p v-if="optionsError" role="alert">
          资料加载失败，请刷新后再选择供应商和商品。
        </p>
        <p
          v-else-if="!optionsLoading && (!parties.length || !picks.length)"
          role="status"
        >
          {{ !parties.length ? '暂无可选供应商。' : ''
          }}{{ !picks.length ? '暂无可选商品规格与单位。' : '' }}
          请先维护缺少的资料；没有维护权限时，请联系管理员。
        </p>
      </section>
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-width="isPurchase ? '106px' : '88px'"
        novalidate
      >
        <el-row :gutter="16">
          <el-col :xs="24" :sm="12" :lg="6">
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
          <el-col :xs="24" :sm="12" :lg="6">
            <el-form-item :label="partyLabel" prop="partyId">
              <el-select
                v-model="form.partyId"
                filterable
                placeholder="请选择"
                style="width: 100%"
              >
                <el-option
                  v-for="p in parties"
                  :key="p.id"
                  :label="p.name"
                  :value="p.id"
                />
              </el-select>
              <div v-if="partyTruncated" class="pick-hint">
                仅显示前 {{ partyLimit }} 个{{ partyLabel }}，请用关键字搜索其余
              </div>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12" :lg="6">
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
          <el-col :xs="24" :sm="12" :lg="6">
            <el-form-item :label="dueDateLabel">
              <el-date-picker
                v-model="form.dueDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12" :lg="6">
            <!-- 业务员是自由文本而不是人员下拉：跑业务的常常没有后台账号
                 （门店、外包、已离职但历史单据还要能查），挂账号后这些人就录不进单 -->
            <el-form-item label="业务员">
              <el-input
                v-model="form.salesman"
                maxlength="64"
                placeholder="选填"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="明细">
          <div v-if="picksTruncated" class="pick-hint">
            商品超过 {{ pickLimit }} 个，下拉只加载了一部分 ——
            可以先在「商品档案」里按关键字找到它
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
            <div
              v-for="(item, index) in form.items"
              :key="index"
              class="item-editor__row"
            >
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
                  :label="`${s.spec || '—'}（库存 ${formatQty(s.stock)}）`"
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
              <el-input-number
                v-model="item.qty"
                class="col-num"
                :min="0.01"
                :precision="3"
                :controls="false"
              />
              <span class="col-base">
                {{ formatQty(qtyBase(item)) }}
                {{ unitOptions(item).find((u) => u.isBase)?.unit || '' }}
              </span>
              <el-input-number
                v-model="item.price"
                class="col-num"
                :min="0"
                :precision="2"
                :controls="false"
              />
              <span class="col-num amount">{{
                formatMoney((item.qty || 0) * (item.price || 0))
              }}</span>
              <el-button
                class="col-act"
                link
                type="danger"
                @click="removeItem(index)"
                >删除</el-button
              >
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
        <el-button :disabled="submitting" @click="requestCloseForm"
          >取消</el-button
        >
        <el-button type="primary" :loading="submitting" @click="submitForm">{{
          isPurchase ? '保存采购单' : '保存销售单'
        }}</el-button>
      </template>
    </el-dialog>

    <!-- 详情：与新增 / 编辑同宽同档（同一份数据的三种看法，宽度不该跳）。
         版式与「新增商品」等弹窗共用 form-section / form-grid，
         全站的弹窗因此是同一套视觉语言。 -->
    <el-dialog
      v-model="detailVisible"
      :title="`${title}详情`"
      width="1280px"
      top="6vh"
    >
      <template v-if="detail">
        <OrderDetailContent
          :order="detail.row"
          :items="detail.items"
          :party-label="partyLabel"
          :is-purchase="isPurchase"
          :picks="picks"
        />
      </template>

      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
        <el-button v-if="isPurchase && detail" @click="openAttach(detail.row)"
          >附件</el-button
        >
        <el-button
          :type="isPurchase ? 'default' : 'primary'"
          @click="detail && printRef?.open(detail.row)"
          >打印单据</el-button
        >
        <template v-if="isPurchase && detail?.row.status === 1">
          <el-button
            v-perm="'inv:purchase:update'"
            @click="detail && openEdit(detail.row)"
            >编辑采购单</el-button
          >
          <el-button
            v-perm="'inv:purchase:finish'"
            type="primary"
            @click="detail && openApprove(detail.row)"
            >审核并入库</el-button
          >
        </template>
      </template>
    </el-dialog>

    <!-- 打印 -->
    <OrderPrintDialog
      ref="printRef"
      :biz-type="bizType"
      :party-label="partyLabel"
    />

    <!-- 附件抽屉：采购单与销售单共用，bizType 决定挂在哪类单据上 -->
    <AttachmentDrawer
      v-model="attachVisible"
      :biz-type="bizType as InvAttachBizType"
      :biz-id="attachBizId"
      :title="attachTitle"
    />

    <!-- 列表打印：有勾选打勾选的，没勾选打当前页 -->
    <ListPrintDialog
      v-model="printVisible"
      :title="title"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />

    <!-- 审核确认：可勾选同时结清全额款项 -->
    <el-dialog
      v-model="approveVisible"
      :title="isPurchase ? '审核并入库' : '审核确认'"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      :close-on-press-escape="!approveRunning"
      :show-close="!approveRunning"
    >
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        class="approve-alert"
        :title="
          isPurchase
            ? `将为「${approveRow?.orderNo}」生成入库单、增加库存，并形成应付。请确认已收到货物。`
            : `审核通过后将为「${approveRow?.orderNo}」生成出入库单并调整库存。`
        "
      />
      <el-checkbox v-model="approveSettleNow">
        同时结清全额款项
        <span class="text-muted"
          >（{{
            partyLabel === '客户' ? '登记一笔收款' : '登记一笔付款'
          }}，适合一手交钱一手交货）</span
        >
      </el-checkbox>
      <template #footer>
        <el-button :disabled="approveRunning" @click="approveVisible = false"
          >取消</el-button
        >
        <el-button
          type="primary"
          :loading="approveRunning"
          @click="confirmApprove"
          >{{ isPurchase ? '确认审核并入库' : '确认审核' }}</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.purchase-entry-help {
  margin-bottom: 24px;
  padding: 16px;
  background: var(--app-surface-subtle);
  border-radius: var(--radius-md);
  strong {
    font-size: 14px;
    font-weight: 600;
  }
  p {
    margin: 8px 0 0;
    line-height: 1.6;
    color: var(--app-text-secondary);
  }
  &__tools {
    display: flex;
    gap: 16px;
    align-items: center;
    flex-wrap: wrap;
    margin-top: 12px;
  }
  a {
    color: var(--brand-primary);
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  a:hover {
    color: var(--app-text);
  }
  a:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 3px;
  }
  a:active {
    opacity: 0.8;
  }
  &__hint {
    font-size: 12px;
  }
}

.purchase-header {
  display: flex;
  align-items: center;
  gap: 24px;
  margin-bottom: 12px;
  min-height: 40px;
  h1 {
    margin: 0;
    flex: none;
    font-size: 18px;
    line-height: 32px;
    font-weight: 600;
  }
}
.purchase-tabs {
  flex: 1;
  min-width: 0;
  :deep(.el-tabs__header) {
    margin: 0;
  }
  :deep(.el-tabs__nav-wrap::after) {
    display: none;
  }
}
.purchase-page {
  .page-card {
    padding: 16px;
  }
  .list-head {
    grid-template-columns: minmax(0, 1fr) auto;
    margin-bottom: 16px;
    padding-bottom: 16px;
    border-bottom: 1px solid var(--app-border-light);
  }
  :deep(.el-table .cell) {
    white-space: nowrap;
  }
  .list-head__title {
    line-height: 24px;
    grid-column: 1 / -1;
    width: 100%;
    color: var(--app-text);
  }
  .list-head .filter-form {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    gap: 12px 16px;
  }
  .list-head .filter-form :deep(.el-form-item) {
    width: 100%;
    margin: 0;
  }
  .list-head .filter-form :deep(.el-input),
  .list-head .filter-form :deep(.el-select),
  .list-head .filter-form :deep(.el-date-editor) {
    width: 100%;
    min-width: 0;
  }
  .list-head__actions {
    flex-wrap: wrap;
  }
}
.purchase-row-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 24px;
  :deep(.el-button) {
    margin: 0;
    padding: 0;
    min-height: 24px;
    line-height: 24px;
  }
  :deep(.el-dropdown) {
    display: inline-flex;
    align-items: center;
    vertical-align: middle;
  }
}
.purchase-review-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  padding-top: 20px;
  border-top: 1px solid var(--app-border-light);
  &__primary,
  &__secondary {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }
  :deep(.el-button + .el-button) {
    margin-left: 0;
  }
}
.purchase-result {
  margin-bottom: 20px;
}
.purchase-list-error {
  min-height: 280px;
  padding: 48px 20px;
  text-align: center;
}
.purchase-list-error p {
  color: var(--app-text-secondary);
  line-height: 1.6;
}
@media (max-width: 800px) {
  .purchase-header {
    gap: 8px 16px;
    flex-wrap: wrap;
  }
  .purchase-header h1 {
    font-size: 18px;
  }
  .purchase-header .purchase-tabs {
    order: 3;
    flex-basis: 100%;
  }
  .purchase-header > .el-button {
    margin-left: auto;
  }
  .purchase-page .page-card {
    padding: 16px;
  }
  .purchase-page .list-head {
    grid-template-columns: minmax(0, 1fr);
    margin-bottom: 20px;
  }
  .purchase-page .list-head .filter-form {
    grid-template-columns: minmax(0, 1fr);
  }
  .table-pager {
    overflow-x: auto;
    justify-content: flex-start;
  }
}
.approve-alert {
  margin-bottom: 14px;
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
  overflow-x: auto;

  &__head,
  &__row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 1050px;
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
</style>
