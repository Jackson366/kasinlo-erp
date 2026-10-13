<script setup lang="ts">
/**
 * 采购 / 销售单据页面主体。两张页面的字段、列、交互完全一致，
 * 差异只有「走哪条业务线」「往来单位是什么」——收敛成一个组件，两个薄壳页面传 bizType。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvOrderItem,
  InvOrderPayload,
  InvOrderRow,
  InvOrderStatus,
  InvPartyRow,
  InvPickSpec,
  InvPickUnit,
  InvProductPick,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import OrderPrintDialog from './OrderPrintDialog.vue'
import { formatDay, formatMoney, formatQty } from '@/utils/format'
import { useTableQuery } from '@/composables/useTableQuery'

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
}

const { query, loading, rows, total, load, search, reset } = useTableQuery<OrderQuery, InvOrderRow>(
  { keyword: '', status: undefined },
  (params) =>
    invApi.orders({
      bizType: props.bizType,
      page: params.page,
      pageSize: params.pageSize,
      keyword: params.keyword || undefined,
      status: params.status ?? undefined,
    }),
)

// ---------------- 备选数据 ----------------
const parties = ref<InvPartyRow[]>([])
/**
 * 商品 → 规格 → 计量单位 的级联选项。
 *
 * 为什么不用商品列表：明细的可选项是「规格 + 单位」，商品列表里没有这一层。
 * 一次拉全量（现场商品量级在几百条内）比在每一行里按需请求简单得多，
 * 也避免了三级下拉各自的异步竞态。
 */
const picks = ref<InvProductPick[]>([])
/** 商品数超过单次上限：下拉里只是一部分，必须提示用关键字搜，否则用户以为没录这个商品 */
const picksTruncated = ref(false)
/** 远程搜索进行中（el-select 的 loading 态） */
const picking = ref(false)

async function loadOptions(): Promise<void> {
  const [partyRes, pickRes] = await Promise.all([
    isPurchase.value ? invApi.suppliers({ pageSize: 200 }) : invApi.customers({ pageSize: 200 }),
    invApi.productPick(),
  ])
  parties.value = partyRes.list
  picks.value = pickRes.items
  picksTruncated.value = pickRes.truncated
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

/**
 * 远程搜索商品并**合并**进可选项（不替换）。
 *
 * 为什么不替换：替换掉整个列表后，已经选好商品的那几行会因为找不到
 * 自己的 specId 而显示成空白 —— 搜一次把别人的行弄坏，这种缺陷最难复现。
 * 合并则只增不减，已选的行永远拿得到自己的规格与单位。
 */
async function remoteSearchProducts(keyword: string): Promise<void> {
  const kw = keyword.trim()
  if (!kw) return
  picking.value = true
  try {
    const res = await invApi.productPick({ keyword: kw })
    const seen = new Set(picks.value.map((p) => p.id))
    for (const p of res.items) {
      if (!seen.has(p.id)) picks.value.push(p)
    }
  } finally {
    picking.value = false
  }
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
const printRef = ref<InstanceType<typeof OrderPrintDialog> | null>(null)

const form = reactive<{
  partyId: number | undefined
  orderDate: string
  dueDate: string
  remark: string
  items: FormItem[]
}>({
  partyId: undefined,
  orderDate: '',
  dueDate: '',
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
  return `${p.name}（${p.code}）· ${p.specs.length} 个规格`
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

function onSpecChange(item: FormItem): void {
  syncUnit(item)
}

function onUnitChange(item: FormItem): void {
  applyPrice(item)
}

function openCreate(): void {
  Object.assign(form, { partyId: undefined, orderDate: today(), dueDate: '', remark: '', items: [] })
  addItem()
  dialogMode.value = 'create'
  editingId.value = null
  dialogVisible.value = true
}

async function openEdit(row: InvOrderRow): Promise<void> {
  const data = await invApi.orderDetail(row.id)
  // 到期日走 formatDay 归一：历史数据里的 0001-01-01 要当成「未填」，
  // 否则日期选择器会显示一个荒唐的年份，用户一保存就把脏值又写回去
  const due = formatDay(data.dueDate)
  Object.assign(form, {
    partyId: data.partyId,
    orderDate: formatDay(data.orderDate),
    dueDate: due === '-' ? '' : due,
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
    partyId: form.partyId as number,
    orderDate: form.orderDate,
    dueDate: form.dueDate || undefined,
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

async function changeStatus(row: InvOrderRow, status: InvOrderStatus): Promise<void> {
  if (status === 2) {
    try {
      await ElMessageBox.confirm(
        `确认审核单据「${row.orderNo}」？审核通过后将生成出入库单并调整库存。`,
        '审核确认',
        { type: 'warning', confirmButtonText: '审核' },
      )
    } catch {
      return
    }
  }
  await invApi.changeOrderStatus(row.id, status)
  ElMessage.success('操作成功')
  load()
}

async function removeRow(row: InvOrderRow): Promise<void> {
  try {
    await ElMessageBox.confirm(`确定删除单据「${row.orderNo}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await invApi.deleteOrder(row.id)
  ElMessage.success('删除成功')
  load()
}

async function openDetail(row: InvOrderRow): Promise<void> {
  const data = await invApi.orderDetail(row.id)
  detail.value = { row: data, items: data.items }
  detailVisible.value = true
}

onMounted(() => {
  load()
  loadOptions()
})
</script>

<template>
  <div class="order-page">
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="单号 / 往来单位"
            clearable
            style="width: 220px"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="query.status" placeholder="全部" clearable style="width: 140px">
            <el-option label="待处理" :value="1" />
            <el-option label="已审核" :value="2" />
            <el-option label="已取消" :value="3" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <h3>{{ title }}</h3>
          <span class="text-muted">共 {{ total }} 张单据</span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="`inv:${bizType}:create`" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>{{ createText }}</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="orderNo" label="单号" width="150" />
        <el-table-column :label="partyLabel" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">{{ row.partyName }}</template>
        </el-table-column>
        <el-table-column prop="orderDate" label="单据日期" width="120">
          <template #default="{ row }">{{ formatDay(row.orderDate) }}</template>
        </el-table-column>
        <el-table-column prop="dueDate" label="到期日" width="120">
          <template #default="{ row }">{{ formatDay(row.dueDate) }}</template>
        </el-table-column>
        <el-table-column label="明细" width="70" align="center">
          <template #default="{ row }">{{ row.itemCount }} 行</template>
        </el-table-column>
        <el-table-column label="金额" width="130" align="right">
          <template #default="{ row }">
            <strong>{{ formatMoney(row.totalAmount) }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="已结算" width="130" align="right">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatMoney(row.settledAmount) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="结算" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="settleMeta[row.settleStatus]?.type" size="small" effect="light">
              {{ settleMeta[row.settleStatus]?.text }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="statusMeta[row.status]?.type" size="small" effect="light">
              {{ statusMeta[row.status]?.text }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="272" fixed="right" align="center">
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
              v-if="row.status === 1"
              v-perm="`inv:${bizType}:finish`"
              link
              type="warning"
              @click="changeStatus(row as InvOrderRow, 3)"
            >
              取消
            </el-button>
            <el-button
              v-if="row.status !== 2"
              v-perm="`inv:${bizType}:delete`"
              link
              type="danger"
              @click="removeRow(row as InvOrderRow)"
            >
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
      :title="dialogMode === 'create' ? createText : '编辑单据'"
      width="1040px"
      top="5vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item :label="partyLabel" prop="partyId">
              <el-select v-model="form.partyId" filterable placeholder="请选择" style="width: 100%">
                <el-option v-for="p in parties" :key="p.id" :label="p.name" :value="p.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
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
          <el-col :span="8">
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
        </el-row>

        <el-form-item label="明细">
          <div v-if="picksTruncated" class="pick-hint">
            商品较多，下拉只加载了一部分 —— 找不到想要的那个请直接输入编码或名称搜索
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
                remote
                reserve-keyword
                :remote-method="remoteSearchProducts"
                :loading="picking"
                placeholder="选择商品（可输入编码 / 名称搜索）"
                @change="onProductChange(item)"
              >
                <el-option
                  v-for="p in picks"
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
              <el-input-number v-model="item.qty" class="col-num" :min="0.01" :controls="false" />
              <span class="col-base">
                {{ formatQty(qtyBase(item)) }}
                {{ unitOptions(item).find((u) => u.isBase)?.unit || '' }}
              </span>
              <el-input-number v-model="item.price" class="col-num" :min="0" :controls="false" />
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

    <!-- 详情 -->
    <el-dialog v-model="detailVisible" title="单据详情" width="720px">
      <template v-if="detail">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="单号">{{ detail.row.orderNo }}</el-descriptions-item>
          <el-descriptions-item :label="partyLabel">{{ detail.row.partyName }}</el-descriptions-item>
          <el-descriptions-item label="单据日期">{{ formatDay(detail.row.orderDate) }}</el-descriptions-item>
          <el-descriptions-item label="到期日">{{ formatDay(detail.row.dueDate) }}</el-descriptions-item>
          <el-descriptions-item label="结算状态">
            {{ settleMeta[detail.row.settleStatus]?.text }}
          </el-descriptions-item>
          <el-descriptions-item label="单据状态">{{ statusMeta[detail.row.status]?.text }}</el-descriptions-item>
          <el-descriptions-item label="备注" :span="2">{{ detail.row.remark || '-' }}</el-descriptions-item>
        </el-descriptions>
        <el-table :data="detail.items" size="small" class="mt-16">
          <el-table-column prop="productName" label="商品" min-width="130" />
          <el-table-column prop="spec" label="规格" width="100" />
          <el-table-column label="数量" width="110" align="right">
            <template #default="{ row }">{{ formatQty(row.qty) }} {{ row.unit }}</template>
          </el-table-column>
          <el-table-column label="折算基本数量" width="140" align="right">
            <template #default="{ row }">
              <span v-if="row.rate === 1" class="text-muted">—</span>
              <span v-else>
                ×{{ row.rate }} = {{ formatQty(row.qtyBase) }}
                {{ baseUnitNameOf(row.productId, row.specId) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="单价" width="100" align="right">
            <template #default="{ row }">{{ formatMoney(row.price) }}</template>
          </el-table-column>
          <el-table-column label="金额" width="110" align="right">
            <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
          </el-table-column>
        </el-table>
        <div class="detail-total">
          合计：<strong>{{ formatMoney(detail.row.totalAmount) }}</strong>
        </div>
      </template>
    </el-dialog>

    <!-- 打印 -->
    <OrderPrintDialog ref="printRef" :biz-type="bizType" :party-label="partyLabel" />
  </div>
</template>

<style scoped lang="scss">
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

  .col-product {
    flex: 1.2;
    min-width: 0;
  }

  .col-spec {
    width: 150px;
    flex: none;
  }

  .col-unit {
    width: 96px;
    flex: none;
  }

  .col-num {
    width: 104px;
    flex: none;
    text-align: right;
  }

  // 折算列是计算结果而非输入，用居中 + 次要色和可编辑列区分开
  .col-base {
    width: 120px;
    flex: none;
    text-align: center;
    font-size: 12px;
    color: var(--app-text-secondary);
  }

  .col-act {
    width: 56px;
    flex: none;
  }

  .amount {
    color: var(--app-text);
    font-weight: 600;
  }
}

.detail-total {
  margin-top: 12px;
  text-align: right;
  font-size: 14px;
  color: var(--app-text-secondary);

  strong {
    color: var(--app-text);
    font-size: 16px;
  }
}
</style>
