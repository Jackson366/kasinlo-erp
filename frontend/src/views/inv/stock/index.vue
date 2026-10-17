<script setup lang="ts">
/**
 * 库存管理页：出入库单据 + 库存查询两个视图。
 *
 * 两个视图的粒度不同，这是刻意的：
 *  - 出入库单据记的是「一次收发的动作」，明细要到**规格 + 计量单位**（2 箱、12 米）；
 *  - 库存查询的行是**规格**，数量按基本单位计。
 * 库存只有一个数（记在规格上的基本单位数量），单据上的折算是入口的换算，
 * 两者不是两套账，而是「输入」与「结果」。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvInventoryRow,
  InvPickSpec,
  InvPickUnit,
  InvProductPick,
  InvStockOrderPayload,
  InvStockOrderRow,
  InvStockDirection,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import { formatDay, formatMoney, formatQty } from '@/utils/format'
import { useTableQuery } from '@/composables/useTableQuery'
import { useListTools } from '@/composables/useListTools'
import StockPrintDialog from '../components/StockPrintDialog.vue'

defineOptions({ name: 'InvStock' })

const activeTab = ref('orders')
const printRef = ref<InstanceType<typeof StockPrintDialog> | null>(null)

// ---------------- 出入库单据 ----------------
interface StockQuery {
  page?: number
  pageSize?: number
  keyword?: string
  direction?: InvStockDirection
}

const {
  query,
  loading,
  rows,
  total,
  load,
  search,
  reset,
} = useTableQuery<StockQuery, InvStockOrderRow>(
  { keyword: '', direction: undefined },
  (params) =>
    invApi.stockOrders({
      page: params.page,
      pageSize: params.pageSize,
      keyword: params.keyword || undefined,
      direction: params.direction ?? undefined,
    }),
)

// ---------------- 库存查询 ----------------
const inventory = ref<InvInventoryRow[]>([])
const inventoryLoading = ref(false)
const inventoryKeyword = ref('')

async function loadInventory(): Promise<void> {
  inventoryLoading.value = true
  try {
    const res = await invApi.inventory({
      pageSize: 200,
      keyword: inventoryKeyword.value || undefined,
    })
    inventory.value = res.list
  } finally {
    inventoryLoading.value = false
  }
}

const warnCount = computed(() => inventory.value.filter((it) => it.warn).length)
const inventoryTotal = computed(() =>
  inventory.value.reduce((sum, it) => sum + it.stock * it.purchasePrice, 0),
)

// ---------------- 新增出入库单 ----------------
/** 商品 → 规格 → 单位 级联选项（与采购 / 销售单同一份数据源） */
const picks = ref<InvProductPick[]>([])
/** 商品数超过单次上限：下拉里只是一部分，必须说清楚 */
const picksTruncated = ref(false)
/** 单次上限的实际数值，用于提示文案（后端 pickLimit） */
const pickLimit = 1000
const formRef = ref<FormInstance>()
const dialogVisible = ref(false)
const submitting = ref(false)

interface StockFormItem {
  productId: number | undefined
  specId: number | undefined
  unitId: number | undefined
  qty: number
}

const form = reactive<{
  direction: InvStockDirection
  orderDate: string
  salesman: string
  remark: string
  items: StockFormItem[]
}>({
  direction: 'in',
  orderDate: '',
  salesman: '',
  remark: '',
  items: [],
})

const rules: FormRules = {
  orderDate: [{ required: true, message: '请选择单据日期', trigger: 'change' }],
}

function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function openCreate(): void {
  Object.assign(form, {
    direction: 'in',
    orderDate: today(),
    salesman: '',
    remark: '',
    items: [blankItem()],
  })
  dialogVisible.value = true
}

function blankItem(): StockFormItem {
  return { productId: undefined, specId: undefined, unitId: undefined, qty: 1 }
}

function addItem(): void {
  form.items.push(blankItem())
}

function removeItem(index: number): void {
  form.items.splice(index, 1)
}

// ---------------- 三级联动 ----------------
function specOptions(item: StockFormItem): InvPickSpec[] {
  if (!item.productId) return []
  return picks.value.find((p) => p.id === item.productId)?.specs ?? []
}

function unitOptions(item: StockFormItem): InvPickUnit[] {
  return specOptions(item).find((s) => s.id === item.specId)?.units ?? []
}

function unitOf(item: StockFormItem): InvPickUnit | undefined {
  return unitOptions(item).find((u) => u.id === item.unitId)
}

/** 折算成基本单位的数量：入库时仓管按「箱」点货，账上按基本单位记 */
function qtyBase(item: StockFormItem): number {
  return (Number(item.qty) || 0) * (unitOf(item)?.rate ?? 1)
}

function onProductChange(item: StockFormItem): void {
  item.specId = undefined
  item.unitId = undefined
  const specs = specOptions(item)
  if (!specs.length) return
  item.specId = specs[0].id
  syncUnit(item)
}

function onSpecChange(item: StockFormItem): void {
  syncUnit(item)
}

/** 规格一变，单位回到该规格的基本单位 */
function syncUnit(item: StockFormItem): void {
  const units = unitOptions(item)
  item.unitId = (units.find((u) => u.isBase) ?? units[0])?.id
}

async function submitForm(): Promise<void> {
  const valid = formRef.value ? await formRef.value.validate().catch(() => false) : false
  if (!valid) return
  const items = form.items.filter((it) => it.productId && it.specId && it.unitId)
  if (items.length === 0) {
    ElMessage.warning('请至少添加一条明细')
    return
  }
  if (items.length !== form.items.length) {
    ElMessage.warning('有明细行还没选完规格与单位，请补全后再提交')
    return
  }
  const payload: InvStockOrderPayload = {
    direction: form.direction,
    sourceType: 'manual',
    orderDate: form.orderDate,
    salesman: form.salesman.trim() || undefined,
    remark: form.remark,
    items: items.map((it) => ({
      productId: it.productId as number,
      specId: it.specId as number,
      unitId: it.unitId as number,
      qty: Number(it.qty),
    })),
  }
  submitting.value = true
  try {
    await invApi.createStockOrder(payload)
    ElMessage.success('已过账，库存已更新')
    dialogVisible.value = false
    load()
    loadInventory()
  } finally {
    submitting.value = false
  }
}

async function voidRow(row: InvStockOrderRow): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确定作废单据「${row.orderNo}」吗？系统会按反方向回滚库存。`,
      '作废确认',
      { type: 'warning', confirmButtonText: '作废' },
    )
  } catch {
    return
  }
  await invApi.voidStockOrder(row.id)
  ElMessage.success('已作废')
  load()
  loadInventory()
}

async function loadPicks(): Promise<void> {
  const res = await invApi.productPick()
  picks.value = res.items
  picksTruncated.value = res.truncated
  applyPickFilter(pickQuery.value)
}

// ---------------- 商品下拉的模糊查找 ----------------
/**
 * 下拉里当前显示的商品。
 *
 * 不用 el-select 的 `remote`：**remote 模式下 el-select 不做本地过滤**，
 * 它原样显示你给的全部 options。之前把服务端搜索结果合并进选项后，下拉里
 * 仍是全部商品（含不匹配的），用户看到的就像「搜索没生效」。
 * 改用 `filterable` + `filter-method`，过滤结果自己维护。
 */
const filteredPicks = ref<InvProductPick[]>([])
const pickQuery = ref('')

/** 三字段模糊匹配，与后端 ProductList 口径一致（现场报「φ20」比编码更常见） */
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
  // 已选中的商品必须始终留在选项里，否则 el-select 反查不到 label、显示成数字
  const picked = new Set(form.items.map((it) => it.productId).filter(Boolean) as number[])
  const missing = all.filter((p) => picked.has(p.id) && !matched.some((m) => m.id === p.id))
  filteredPicks.value = missing.length ? [...missing, ...matched] : matched
}

function filterProducts(query: string): void {
  applyPickFilter(query)
}

function onProductPickChange(item: StockFormItem): void {
  onProductChange(item)
  applyPickFilter('')
}

function onPickVisible(open: boolean): void {
  if (open) applyPickFilter('')
}

/** 商品下拉文案：名称 + 编码 + 规格数（具体规格在下一级选） */
function productLabel(p: InvProductPick): string {
  // 有码就带上：扫码枪敲完一串数字，用户要能一眼确认「选中的是这件货」
  const key = p.barcode ? `${p.code} / ${p.barcode}` : p.code
  return `${p.name}（${key}）· ${p.specs.length} 个规格`
}

function onTabChange(name: string | number): void {
  if (name === 'inventory') loadInventory()
}

/** 来源文案（模板与打印共用一份，避免两处各写一遍映射） */
function sourceLabel(t: string): string {
  return { purchase: '采购', sales: '销售', manual: '手工' }[t] || '-'
}

// ---------------- 导出 / 打印 / 批量（出入库单据） ----------------
const tableRef = ref()
const {
  selection,
  exporting,
  batchRunning,
  onSelectionChange,
  clearSelection,
  exportRows,
  runBatch,
} = useListTools<InvStockOrderRow>({
  exportUrl: '/inv/export',
  exportParams: () => ({
    entity: 'stock',
    keyword: query.keyword || undefined,
    direction: query.direction || undefined,
  }),
  fallbackName: '出入库单据.xlsx',
})

const printVisible = ref(false)
const printRows = computed(() => (selection.value.length ? selection.value : rows.value))

const printColumns = computed<ListPrintColumn<InvStockOrderRow>[]>(() => [
  { label: '单号', prop: 'orderNo' },
  { label: '方向', value: (r) => (r.direction === 'in' ? '入库' : '出库') },
  { label: '来源', value: (r) => sourceLabel(r.sourceType) },
  { label: '单据日期', value: (r) => formatDay(r.orderDate) },
  { label: '明细', value: (r) => `${r.itemCount} 行`, align: 'center' },
  { label: '总数量', value: (r) => formatQty(r.totalQty), align: 'right' },
  { label: '业务员', value: (r) => r.salesman || '-' },
  { label: '操作人员', value: (r) => r.operator || '-' },
  { label: '备注', value: (r) => r.remark || '-' },
])

const printSubtitle = computed(() => {
  const parts: string[] = []
  if (query.keyword) parts.push(`单号「${query.keyword}」`)
  if (query.direction) parts.push(query.direction === 'in' ? '入库' : '出库')
  if (selection.value.length) parts.push('仅打印勾选')
  return parts.join(' · ') || '全部出入库单据'
})

/** 批量作废：逐条回滚库存，失败项汇总提示（已被引用的单据会拒绝） */
async function batchVoid(): Promise<void> {
  if (!selection.value.length) return
  const done = await runBatch(
    '批量作废',
    selection.value,
    (r) => invApi.voidStockOrder(r.id),
    (r) => r.orderNo,
  )
  if (done) {
    clearSelection(tableRef.value)
    load()
    loadInventory()
  }
}

onMounted(() => {
  load()
  loadInventory()
  loadPicks()
})
</script>

<template>
  <div class="stock-page">
    <!-- 筛选区与其它列表一致：独立卡片 + 网格；随 tab 切换内容 -->
    <div class="page-card search-bar">
      <el-form v-if="activeTab === 'orders'" class="filter-form" label-width="76px" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="单号"
            clearable
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="query.direction" placeholder="全部方向" clearable>
            <el-option label="入库" value="in" />
            <el-option label="出库" value="out" />
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
      <el-form v-else class="filter-form" label-width="76px" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="inventoryKeyword"
            placeholder="商品编码 / 名称"
            clearable
            @keyup.enter="loadInventory"
          />
        </el-form-item>
        <div class="filter-form__actions">
          <el-button type="primary" @click="loadInventory">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="inventoryKeyword = ''; loadInventory()">重置</el-button>
        </div>
      </el-form>
    </div>

    <div class="page-card">
      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <!-- 出入库单据 -->
        <el-tab-pane label="出入库单据" name="orders">
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>出入库单据</h3>
              <span class="text-muted">共 {{ total }} 条</span>
              <span v-if="selection.length" class="selection-tip">
                已选 {{ selection.length }} 项
                <a @click="clearSelection(tableRef)">清空</a>
              </span>
            </div>
            <div class="toolbar__actions">
              <el-button v-if="selection.length" :loading="batchRunning" @click="batchVoid">
                批量作废
              </el-button>
              <el-button :loading="exporting" @click="exportRows(selection.length ? selection : undefined)">
                导出{{ selection.length ? '选中' : '' }}
              </el-button>
              <el-button @click="printVisible = true">打印</el-button>
              <el-button v-perm="'inv:stock:create'" type="primary" @click="openCreate">
                <AppIcon name="Plus" />
                <span>新增出入库单</span>
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
            <el-table-column prop="orderNo" label="单号" width="150" />
            <el-table-column label="方向" width="90" align="center">
              <template #default="{ row }">
                <el-tag :type="row.direction === 'in' ? 'success' : 'warning'" size="small" effect="light">
                  {{ row.direction === 'in' ? '入库' : '出库' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="来源" width="110">
              <template #default="{ row }">
                {{ sourceLabel(row.sourceType) }}
              </template>
            </el-table-column>
            <el-table-column prop="orderDate" label="单据日期" width="130">
              <template #default="{ row }">{{ formatDay(row.orderDate) }}</template>
            </el-table-column>
            <el-table-column label="明细" width="80" align="center">
              <template #default="{ row }">{{ row.itemCount }} 行</template>
            </el-table-column>
            <el-table-column label="总数量" width="110" align="right">
              <template #default="{ row }">{{ formatQty(row.totalQty) }}</template>
            </el-table-column>
            <el-table-column label="业务员" width="90" show-overflow-tooltip>
              <template #default="{ row }">{{ row.salesman || '-' }}</template>
            </el-table-column>
            <el-table-column label="操作人员" width="100" show-overflow-tooltip>
              <template #default="{ row }">{{ row.operator || '-' }}</template>
            </el-table-column>
            <!-- 备注只是辅助信息，不该按主数据列的宽度占位 -->
            <el-table-column prop="remark" label="备注" min-width="100" show-overflow-tooltip>
              <template #default="{ row }">{{ row.remark || '-' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="130" fixed="right" align="center">
              <template #default="{ row }">
                <el-button link type="primary" @click="printRef?.open(row as InvStockOrderRow)">
                  打印
                </el-button>
                <el-button
                  v-perm="'inv:stock:delete'"
                  link
                  type="danger"
                  @click="voidRow(row as InvStockOrderRow)"
                >
                  作废
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
        </el-tab-pane>

        <!-- 库存查询 -->
        <el-tab-pane label="库存查询" name="inventory">
          <div class="toolbar">
            <div class="toolbar__title">
              <h3>库存查询</h3>
              <span class="text-muted">
                共 {{ inventory.length }} 条规格 · 预警 {{ warnCount }} 项
              </span>
            </div>
            <div class="toolbar__actions">
              <span class="text-secondary">库存总值 {{ formatMoney(inventoryTotal) }}</span>
            </div>
          </div>

          <el-table v-loading="inventoryLoading" :data="inventory" stripe>
            <el-table-column prop="productCode" label="编码" width="100" />
            <el-table-column
              prop="productName"
              label="商品"
              min-width="140"
              show-overflow-tooltip
            />
            <el-table-column prop="spec" label="规格" width="130" />
            <!-- 单位单列出来：它是库存的**计数单位**（基本单位），
                 决定「12」是 12 张还是 12 吨；塞在库存数字后面会被当成数量的一部分读过去 -->
            <el-table-column prop="unit" label="基本单位" width="100" align="center">
              <template #default="{ row }">{{ row.unit || '-' }}</template>
            </el-table-column>
            <el-table-column prop="categoryName" label="分类" width="110">
              <template #default="{ row }">{{ row.categoryName || '-' }}</template>
            </el-table-column>
            <el-table-column label="库存" width="120" align="right">
              <template #default="{ row }">
                <span :class="row.warn ? 'stock-warn' : ''">{{ formatQty(row.stock) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="预警下限" width="110" align="right">
              <template #default="{ row }">{{ formatQty(row.warnStock) }}</template>
            </el-table-column>
            <el-table-column label="库存成本" width="130" align="right">
              <template #default="{ row }">
                {{ formatMoney(row.stock * row.purchasePrice) }}
              </template>
            </el-table-column>
            <el-table-column label="状态" width="100" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.warn" type="danger" size="small" effect="light">库存预警</el-tag>
                <el-tag v-else type="success" size="small" effect="light">正常</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- 新增出入库单 -->
    <el-dialog
      v-model="dialogVisible"
      title="新增出入库单"
      width="1280px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="6">
            <el-form-item label="方向">
              <el-radio-group v-model="form.direction">
                <el-radio-button value="in">入库</el-radio-button>
                <el-radio-button value="out">出库</el-radio-button>
              </el-radio-group>
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
            <!-- 与采购 / 销售单一致：业务员是自由文本，不挂账号 -->
            <el-form-item label="业务员">
              <el-input v-model="form.salesman" maxlength="64" placeholder="选填" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="备注">
              <el-input v-model="form.remark" placeholder="选填" />
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
                <el-option v-for="p in filteredPicks" :key="p.id" :label="productLabel(p)" :value="p.id" />
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
              <el-button class="col-act" link type="danger" @click="removeItem(index)">删除</el-button>
            </div>
            <div class="item-editor__foot">
              <el-button link type="primary" @click="addItem">
                <AppIcon name="Plus" />
                <span>添加明细</span>
              </el-button>
            </div>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <StockPrintDialog ref="printRef" />

    <ListPrintDialog
      v-model="printVisible"
      title="出入库单据"
      :subtitle="printSubtitle"
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

.stock-warn {
  color: var(--tone-red);
  font-weight: 600;
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
    margin-top: 4px;
  }

  // 商品列要给足宽度（下拉里是「名称（编码）· N 个规格」），
  // 其余列压到「刚好放下内容」，把省下的空间全给商品列
  .col-product {
    flex: 1;
    min-width: 260px;
  }

  .col-spec {
    width: 140px;
    flex: none;
  }

  .col-unit {
    width: 84px;
    flex: none;
  }

  .col-num {
    width: 96px;
    flex: none;
  }

  // 折算列是计算结果而非输入，用居中 + 次要色与可编辑列区分开
  .col-base {
    width: 104px;
    flex: none;
    text-align: center;
    font-size: 12px;
    color: var(--app-text-secondary);
  }

  .col-act {
    width: 52px;
    flex: none;
  }
}
</style>
