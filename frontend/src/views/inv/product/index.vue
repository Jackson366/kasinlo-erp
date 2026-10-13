<script setup lang="ts">
/**
 * 商品管理页：商品档案的完整维护入口，粒度是「商品 → 规格 → 计量单位」三层。
 *
 * 为什么单独一页而不是并进库存页：库存回答「现在有多少」，档案回答
 * 「这是什么、按什么单位卖、卖多少钱」。前者随业务天天在动，后者是主数据，
 * 变更频率低但影响面大（单位与换算率会直接进单据与打印纸面）。混在一起做，
 * 库存页会变成「一个列表两种职责」，权限也没法分开给（仓管改销售价？）。
 *
 * 三个刻意的设计取舍：
 *  1. **库存只读**。它由出入库单据驱动，表单里没有这一栏。允许直接改，
 *     等于凭空多出一套与单据无关的账，盘点对不上时无从追溯。
 *  2. **规格是库存的归属层**。库存记在规格上（按基本单位计数），
 *     所以「φ20 剩 30 根」永远只有一个数，不会出现商品层与规格层两套账。
 *  3. **规格与单位按「比对后增删改」保存，ID 保持不变**。规格 ID 被单据明细
 *     的 spec_id 引用着，全删重建会让历史单据指向不存在的行 —— 那张单再
 *     打开保存就会被打回。所以改名要把原 id 一并回传（区分「改名」与
 *     「删旧建新」），被移除且已被引用的规格由后端降级为停用。
 *
 * 录入查重是**提示而非拦截**：同名不同规格是合法场景（两家供应商的「纸箱」），
 * 硬拦会挡住正常录入，所以只把可疑行摆出来，确认不是同一个仍可保存。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvCategoryOption,
  InvProductDupRow,
  InvProductPayload,
  InvProductRow,
  InvProductSpecPayload,
  Status,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import StatusTag from '@/components/StatusTag.vue'
import { formatDate, formatMoney, formatQty } from '@/utils/format'
import { useCrudDialog } from '@/composables/useCrudDialog'
import { useTableQuery } from '@/composables/useTableQuery'

defineOptions({ name: 'InvProduct' })

interface ProductQuery {
  page?: number
  pageSize?: number
  keyword?: string
  status?: Status
  categoryId?: number
}

const { query, loading, rows, total, load, search, reset } = useTableQuery<
  ProductQuery,
  InvProductRow
>({ keyword: '', status: undefined, categoryId: undefined }, (params) =>
  invApi.products({
    page: params.page,
    pageSize: params.pageSize,
    keyword: params.keyword || undefined,
    status: params.status ?? undefined,
    categoryId: params.categoryId ?? undefined,
  }),
)

/**
 * 分类下拉有**两份**，刻意不同：
 *
 * - `categories`（筛选用）：走全量列表接口，**包含已停用分类**。
 *   只给启用分类的话，一旦某个分类被停用，属于它的那批商品就再也筛不出来 ——
 *   筛选器变成一个「看起来正常但永远筛不到」的死选项，比没有还糟。
 * - `categoryOptions`（表单用）：只给启用分类，编辑时再把当前值补回去。
 */
const categories = ref<InvCategoryOption[]>([])

/** 当前编辑商品的分类（可能已停用）—— 打开弹窗时记下来，避免依赖列表行还在不在 */
const editingCategory = ref<InvCategoryOption | undefined>()

async function loadCategories(): Promise<void> {
  const [all, enabled] = await Promise.all([
    invApi.categories({ pageSize: 500 }),
    invApi.categoryOptions(),
  ])
  categories.value = all.list.map((c) => ({ id: c.id, name: c.name }))
  enabledOptions.value = enabled
}

const enabledOptions = ref<InvCategoryOption[]>([])

// ---------------- 新增 / 编辑 ----------------
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<InvProductPayload>({
  formRef,
  create: (payload) => invApi.createProduct(payload),
  update: (id, payload) => invApi.updateProduct(id, payload),
  messages: { create: '新增成功', update: '修改成功' },
})
const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog

/** 新建一个空规格：默认带一条基本单位，「一个规格至少一个单位」是硬约束 */
function blankSpec(): InvProductSpecPayload {
  return {
    spec: '',
    warnStock: 0,
    status: 1,
    units: [{ unit: '', rate: 1, isBase: true, purchasePrice: 0, salePrice: 0 }],
  }
}

const form = reactive<InvProductPayload>({
  code: '',
  name: '',
  categoryId: undefined,
  status: 1,
  remark: '',
  specs: [blankSpec()],
})

// ---------------- 录入查重 ----------------
// 名称失焦（或停止输入 400ms）后拉一次同名商品，只提示不拦截。
const duplicates = ref<InvProductDupRow[]>([])
let dupTimer: ReturnType<typeof setTimeout> | undefined

async function checkDuplicates(): Promise<void> {
  const name = form.name.trim()
  if (name.length < 2) {
    duplicates.value = []
    return
  }
  try {
    duplicates.value = await invApi.productDuplicates({
      name,
      excludeId: editingId.value ?? undefined,
    })
  } catch {
    // 查重只是辅助提示，接口异常不该把录入流程卡住
    duplicates.value = []
  }
}

function onNameInput(): void {
  if (dupTimer) clearTimeout(dupTimer)
  dupTimer = setTimeout(checkDuplicates, 400)
}

// ---------------- 规格 / 单位 编辑动作 ----------------
function addSpec(): void {
  form.specs.push(blankSpec())
}

function removeSpec(index: number): void {
  if (form.specs.length <= 1) {
    ElMessage.warning('至少保留一个规格')
    return
  }
  const spec = form.specs[index]
  // 有历史引用的规格由后端降级为停用而不是删除（会返回 409 或直接保留），
  // 这里先给一句可读的提醒：规格一旦被单据引用，历史单据的库存口径就没了出处。
  const persisted = spec.id !== undefined
  const hint = persisted
    ? `「${spec.spec || '未命名规格'}」若已被单据引用将无法删除，删除失败时会给出提示。确定继续吗？`
    : `确定删除规格「${spec.spec || '未命名'}」吗？`
  ElMessageBox.confirm(hint, '删除确认', { type: 'warning', confirmButtonText: '删除' })
    .then(() => {
      form.specs.splice(index, 1)
    })
    .catch(() => undefined)
}

function addUnit(spec: InvProductSpecPayload): void {
  // 换算率留空由用户填：给个 1 会让人以为「1 箱 = 1 个」是默认正确的
  spec.units.push({ unit: '', rate: 1, isBase: false, purchasePrice: 0, salePrice: 0 })
}

function removeUnit(spec: InvProductSpecPayload, index: number): void {
  if (spec.units.length <= 1) {
    ElMessage.warning('每个规格至少保留一个计量单位')
    return
  }
  const removed = spec.units[index]
  spec.units.splice(index, 1)
  // 删掉的正好是基本单位：把第一个提升为基本单位，避免出现「没有基本单位」的中间态
  if (removed?.isBase && spec.units.length) {
    spec.units[0].isBase = true
    spec.units[0].rate = 1
  }
}

/** 基本单位在规格内的下标；-1 表示还没指定 */
function baseIndex(spec: InvProductSpecPayload): number {
  return spec.units.findIndex((u) => u.isBase)
}

/**
 * 指定基本单位。基本单位是库存的计数单位，一个规格有且只能有一个，
 * 且它的换算率恒为 1（「1 个 = 1 个」），所以在这里一并钉死，
 * 免得留下「基本单位 rate = 3」这种自相矛盾的档案。
 */
function setBaseUnit(spec: InvProductSpecPayload, index: number): void {
  spec.units.forEach((u, i) => {
    u.isBase = i === index
    if (i === index) u.rate = 1
  })
}

/** 规格行里「1 箱 = N 个」的提示语，让换算率一眼能读懂 */
function rateHint(spec: InvProductSpecPayload, index: number): string {
  const unit = spec.units[index]?.unit || '该单位'
  const base = spec.units.find((u) => u.isBase)?.unit || '基本单位'
  const rate = spec.units[index]?.rate
  if (!rate || rate <= 0) return ''
  if (spec.units[index]?.isBase) return `库存按${base}计`
  return `1 ${unit} = ${rate} ${base}`
}

// ---------------- 提交校验 ----------------
// 逐字段必填交给 el-form-item 的嵌套 prop（错误信息贴着控件显示）；
// 这里只管**跨行约束**：唯一基本单位、规格名/单位名不重复、基本单位换算率恒为 1。
// 空值刻意不在这里报 —— 否则同一处会同时出现「第 1 个规格：请填写规格名称」
// 与「请填写规格名称」两条，看着像重复提交了。
function validateSpecs(_rule: unknown, _value: unknown, callback: (e?: Error) => void): void {
  const errs: string[] = []
  if (!form.specs.length) errs.push('至少需要一个规格')

  const seenSpec = new Set<string>()
  form.specs.forEach((s, si) => {
    const label = `第 ${si + 1} 个规格`
    const specName = s.spec.trim()
    if (specName) {
      if (seenSpec.has(specName)) errs.push(`规格「${specName}」重复了`)
      else seenSpec.add(specName)
    }

    if (!s.units.length) {
      errs.push(`${label}：至少需要一个计量单位`)
      return
    }
    const baseCount = s.units.filter((u) => u.isBase).length
    if (baseCount === 0) errs.push(`${label}：请指定一个基本单位`)
    else if (baseCount > 1) errs.push(`${label}：只能有一个基本单位`)

    const seenUnit = new Set<string>()
    s.units.forEach((u) => {
      const name = u.unit.trim()
      if (name) {
        if (seenUnit.has(name)) errs.push(`${label}：单位「${name}」重复了`)
        else seenUnit.add(name)
      }
      if (!u.rate || u.rate <= 0) {
        errs.push(`${label}：单位「${name || '?'}」的换算率必须大于 0`)
      }
      // 基本单位的换算率由 setBaseUnit 钉成 1，走不到这里；
      // 留着是为了兜住「手工改过 rate 又没重新点基本单位」的情况
      if (u.isBase && Math.abs(u.rate - 1) > 1e-6) {
        errs.push(`${label}：基本单位「${name}」的换算率必须是 1`)
      }
    })
  })

  callback(errs.length ? new Error(errs[0]) : undefined)
}

const rules: FormRules<InvProductPayload> = {
  code: [{ required: true, message: '请输入商品编码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入商品名称', trigger: 'blur' }],
  specs: [{ validator: validateSpecs, trigger: 'change' }],
}

function openCreate(): void {
  Object.assign(form, {
    code: '',
    name: '',
    categoryId: undefined,
    status: 1,
    remark: '',
    specs: [blankSpec()],
  })
  duplicates.value = []
  editingCategory.value = undefined
  dialog.openCreate()
}

function openEdit(row: InvProductRow): void {
  Object.assign(form, {
    code: row.code,
    name: row.name,
    categoryId: row.categoryId || undefined,
    status: row.status,
    remark: row.remark,
    // 深拷贝：直接把列表行的 specs 塞进表单，用户取消编辑也会改到列表数据
    specs: row.specs.map((s) => ({
      id: s.id,
      spec: s.spec,
      warnStock: s.warnStock,
      status: s.status,
      units: s.units.map((u) => ({
        unit: u.unit,
        rate: u.rate,
        isBase: u.isBase,
        purchasePrice: u.purchasePrice,
        salePrice: u.salePrice,
        status: u.status,
      })),
    })),
  })
  duplicates.value = []
  editingCategory.value = row.categoryId
    ? { id: row.categoryId, name: row.categoryName || '' }
    : undefined
  dialog.openEdit(row.id)
}

async function submitForm(): Promise<void> {
  await dialog.submit(
    () => {
      load()
      loadCategories()
    },
    () => ({
      code: form.code.trim(),
      name: form.name.trim(),
      categoryId: form.categoryId ?? 0,
      status: form.status,
      remark: form.remark,
      specs: form.specs.map((s) => ({
        id: s.id,
        spec: s.spec.trim(),
        warnStock: s.warnStock ?? 0,
        status: s.status,
        units: s.units.map((u) => ({
          unit: u.unit.trim(),
          rate: u.rate,
          isBase: u.isBase,
          purchasePrice: u.purchasePrice ?? 0,
          salePrice: u.salePrice ?? 0,
          status: u.status,
        })),
      })),
    }),
  )
}

async function removeRow(row: InvProductRow): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确定删除「${row.name}」吗？已被单据明细引用时无法删除 —— 建议改为停用。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除' },
    )
  } catch {
    return
  }
  await invApi.deleteProduct(row.id)
  ElMessage.success('删除成功')
  load()
  loadCategories()
}

/** 打开弹窗时清掉上一条的校验残留 */
function onDialogOpen(): void {
  formRef.value?.clearValidate()
}

/** 表单里的分类下拉：只给启用项，但要把当前值补回去，否则老商品显示成空白 */
const categoryOptions = computed<InvCategoryOption[]>(() => {
  const cur = editingCategory.value
  if (!cur) return enabledOptions.value
  if (enabledOptions.value.some((c) => c.id === cur.id)) return enabledOptions.value
  return [cur, ...enabledOptions.value]
})

/** 列表行展开时规格摘要，例如「φ20 · φ25」 */
function specSummary(row: InvProductRow): string {
  return row.specs.map((s) => s.spec).join(' · ') || '-'
}

onMounted(() => {
  load()
  loadCategories()
})
</script>

<template>
  <div class="product-page">
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="编码 / 名称 / 规格"
            clearable
            style="width: 220px"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="query.categoryId" placeholder="全部" clearable style="width: 160px">
            <el-option v-for="c in categories" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="query.status" placeholder="全部" clearable style="width: 140px">
            <el-option label="启用" :value="1" />
            <el-option label="停用" :value="0" />
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
          <h3>商品管理</h3>
          <span class="text-muted">
            商品档案与价格维护 · 共 {{ total }} 条 · 展开行可看规格与单位
          </span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="'inv:product:create'" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>新增商品</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="spec-detail">
              <div v-for="s in (row as InvProductRow).specs" :key="s.id" class="spec-detail__item">
                <div class="spec-detail__head">
                  <span class="spec-detail__name">{{ s.spec }}</span>
                  <span class="spec-detail__stock" :class="s.warn ? 'stock-warn' : ''">
                    库存 {{ formatQty(s.stock) }}
                    {{ s.units.find((u) => u.isBase)?.unit || '' }}
                  </span>
                  <span class="text-muted">预警下限 {{ formatQty(s.warnStock) }}</span>
                  <StatusTag v-if="s.status === 0" :status="s.status" />
                </div>
                <el-table :data="s.units" size="small" class="unit-table">
                  <el-table-column prop="unit" label="计量单位" width="120">
                    <template #default="{ row: u }">
                      {{ u.unit }}
                      <el-tag v-if="u.isBase" size="small" effect="plain" type="success">基本</el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="换算率" width="160">
                    <template #default="{ row: u }">
                      <span v-if="u.isBase" class="text-muted">库存计数单位</span>
                      <span v-else>1 {{ u.unit }} = {{ u.rate }} 基本单位</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="采购价" align="right" width="120">
                    <template #default="{ row: u }">{{ formatMoney(u.purchasePrice) }}</template>
                  </el-table-column>
                  <el-table-column label="销售价" align="right" width="120">
                    <template #default="{ row: u }">{{ formatMoney(u.salePrice) }}</template>
                  </el-table-column>
                </el-table>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="code" label="编码" width="110" />
        <el-table-column prop="name" label="商品名称" min-width="150" show-overflow-tooltip />
        <el-table-column label="分类" width="110">
          <template #default="{ row }">{{ row.categoryName || '-' }}</template>
        </el-table-column>
        <el-table-column label="规格" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ specSummary(row as InvProductRow) }}</template>
        </el-table-column>
        <el-table-column label="规格数" width="90" align="center">
          <template #default="{ row }">{{ row.specCount }}</template>
        </el-table-column>
        <el-table-column label="单位数" width="90" align="center">
          <template #default="{ row }">{{ row.unitCount }}</template>
        </el-table-column>
        <el-table-column label="总库存" width="120" align="right">
          <template #default="{ row }">
            <span :class="row.warn ? 'stock-warn' : ''">{{ formatQty(row.totalStock) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <StatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="120">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatDate(row.createdAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right" align="center">
          <template #default="{ row }">
            <el-button
              v-perm="'inv:product:update'"
              link
              type="primary"
              @click="openEdit(row as InvProductRow)"
            >
              编辑
            </el-button>
            <el-button
              v-perm="'inv:product:delete'"
              link
              type="danger"
              @click="removeRow(row as InvProductRow)"
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

    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增商品' : '编辑商品'"
      width="960px"
      top="5vh"
      :close-on-click-modal="false"
      destroy-on-close
      @open="onDialogOpen"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="商品编码" prop="code">
              <el-input v-model="form.code" placeholder="如 SP001" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="商品名称" prop="name">
              <el-input
                v-model="form.name"
                placeholder="如 热轧钢板"
                @input="onNameInput"
                @blur="checkDuplicates"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-alert
          v-if="duplicates.length"
          type="warning"
          :closable="false"
          show-icon
          class="dup-alert"
        >
          <template #title>
            发现 {{ duplicates.length }} 个同名商品，确认不是同一个再继续保存
          </template>
          <div class="dup-list">
            <div v-for="d in duplicates" :key="d.id" class="dup-list__item">
              <span class="dup-list__code">{{ d.code }}</span>
              <span class="dup-list__name">{{ d.name }}</span>
              <span class="text-muted">{{ d.categoryName || '未分类' }}</span>
              <span class="text-muted">规格：{{ d.specSummary || '-' }}</span>
              <StatusTag :status="d.status" />
            </div>
          </div>
        </el-alert>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="商品分类">
              <el-select
                v-model="form.categoryId"
                placeholder="未分类（可在「商品分类」页维护）"
                clearable
                style="width: 100%"
              >
                <el-option v-for="c in categoryOptions" :key="c.id" :label="c.name" :value="c.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="状态">
              <el-switch
                :model-value="form.status === 1"
                @update:model-value="(v) => (form.status = v ? 1 : 0)"
              />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="选填" />
        </el-form-item>

        <el-divider content-position="left">
          <span class="divider-title">规格与计量单位</span>
        </el-divider>
        <p class="spec-tip">
          同一商品可维护多个规格，每个规格下可维护多个计量单位；库存按<strong>基本单位</strong>计数，
          出入库时按所选单位的换算率折算。
        </p>

        <!-- 跨行约束（唯一基本单位、规格名/单位名不重复、基本单位换算率恒为 1）
             由 validateSpecs 一次判完。它挂在 specs 这个「整数组」上，必须有一个
             绑到 prop="specs" 的 form-item 才会被 form.validate() 调到 ——
             否则 rules.specs 就是一段死配置，空规格能直接提交出去。
             逐字段必填仍贴着各自的控件显示，这里只承载「整组」的结论。 -->
        <el-form-item prop="specs" class="spec-errors" label-width="0" />

        <!-- 规格列表：跨行约束（唯一基本单位、名称不重复）由 validateSpecs 一次判完，
             这里只按嵌套 prop 做逐字段必填，错误信息贴着控件显示 -->
        <div class="spec-list">
          <div v-for="(spec, si) in form.specs" :key="si" class="spec-card">
            <div class="spec-card__head">
              <span class="spec-card__index">规格 {{ si + 1 }}</span>
              <el-form-item
                :prop="`specs.${si}.spec`"
                :rules="[{ required: true, message: '请填写规格名称', trigger: 'blur' }]"
                label="规格"
                class="spec-card__field"
              >
                <el-input v-model="spec.spec" placeholder="如 φ20 / Q235B 6mm / 500ml" />
              </el-form-item>
              <el-form-item
                :prop="`specs.${si}.warnStock`"
                label="预警下限"
                class="spec-card__field spec-card__field--narrow"
              >
                <el-input-number
                  v-model="spec.warnStock"
                  :min="0"
                  :precision="2"
                  :controls="false"
                  style="width: 100%"
                />
              </el-form-item>
              <!-- 规格级停用：被单据引用过的规格删不掉（只能停用），
                   没有这个开关就只能干瞪眼 —— 想让它退出新单据却无从下手。
                   新增中的规格不显示，因为「新建一个停用的规格」没有意义。 -->
              <el-form-item v-if="spec.id" label="启用" class="spec-card__field--narrow">
                <el-switch
                  :model-value="(spec.status ?? 1) === 1"
                  @update:model-value="(v) => (spec.status = v ? 1 : 0)"
                />
              </el-form-item>
              <el-button link type="danger" @click="removeSpec(si)">
                <AppIcon name="Delete" />
                <span>删除规格</span>
              </el-button>
            </div>

            <div class="unit-table">
              <div class="unit-table__row unit-table__row--head">
                <span class="unit-col unit-col--unit">计量单位</span>
                <span class="unit-col unit-col--rate">换算率（1 该单位 = ? 基本单位）</span>
                <span class="unit-col unit-col--base">基本单位</span>
                <span class="unit-col unit-col--price">采购价</span>
                <span class="unit-col unit-col--price">销售价</span>
                <span class="unit-col unit-col--op"></span>
              </div>
              <div v-for="(u, ui) in spec.units" :key="ui" class="unit-table__row">
                <div class="unit-col unit-col--unit">
                  <el-form-item
                    :prop="`specs.${si}.units.${ui}.unit`"
                    :rules="[{ required: true, message: '请填写单位', trigger: 'blur' }]"
                    class="unit-form-item"
                  >
                    <el-input v-model="u.unit" placeholder="根 / 米 / 箱" />
                  </el-form-item>
                </div>
                <div class="unit-col unit-col--rate">
                  <el-form-item class="unit-form-item">
                    <div class="rate-cell">
                      <el-input-number
                        v-model="u.rate"
                        :min="0.0001"
                        :precision="4"
                        :controls="false"
                        :disabled="u.isBase"
                        style="width: 100%"
                      />
                      <span class="rate-hint">{{ rateHint(spec, ui) }}</span>
                    </div>
                  </el-form-item>
                </div>
                <div class="unit-col unit-col--base">
                  <el-radio-group
                    :model-value="baseIndex(spec)"
                    @update:model-value="(v) => setBaseUnit(spec, Number(v))"
                  >
                    <el-radio :value="ui" />
                  </el-radio-group>
                </div>
                <div class="unit-col unit-col--price">
                  <el-form-item class="unit-form-item">
                    <el-input-number
                      v-model="u.purchasePrice"
                      :min="0"
                      :precision="2"
                      :controls="false"
                      style="width: 100%"
                    />
                  </el-form-item>
                </div>
                <div class="unit-col unit-col--price">
                  <el-form-item class="unit-form-item">
                    <el-input-number
                      v-model="u.salePrice"
                      :min="0"
                      :precision="2"
                      :controls="false"
                      style="width: 100%"
                    />
                  </el-form-item>
                </div>
                <div class="unit-col unit-col--op">
                  <el-button link type="danger" @click="removeUnit(spec, ui)">
                    <AppIcon name="Delete" />
                  </el-button>
                </div>
              </div>
              <div class="unit-table__foot">
                <el-button link type="primary" @click="addUnit(spec)">
                  <AppIcon name="Plus" />
                  <span>添加计量单位</span>
                </el-button>
              </div>
            </div>
          </div>
        </div>

        <el-button class="add-spec" @click="addSpec">
          <AppIcon name="Plus" />
          <span>添加规格</span>
        </el-button>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>
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

// ---------------- 列表展开行 ----------------
.spec-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px 12px 12px 48px;
}

.spec-detail__head {
  display: flex;
  align-items: center;
  gap: 16px;
}

.spec-detail__name {
  font-weight: 600;
  color: var(--app-text);
}

.spec-detail__stock {
  color: var(--app-text-secondary);
}

.unit-table {
  margin-left: 8px;
}

// ---------------- 弹窗内的规格编辑 ----------------
.dup-alert {
  margin-bottom: 12px;
}

.dup-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}

.dup-list__item {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}

.dup-list__code {
  font-family: var(--app-font-mono, monospace);
  color: var(--app-text-secondary);
}

.dup-list__name {
  font-weight: 600;
}

.divider-title {
  font-weight: 600;
  color: var(--app-text);
}

.spec-tip {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-muted);
}

.spec-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

// 整组校验的错误行：没有标签也没有控件，只有一行提示，
// 所以要把 form-item 的默认下边距与左缩进收掉，避免下方空出一截
.spec-errors {
  margin-bottom: 0;

  :deep(.el-form-item__content) {
    margin-left: 0 !important;
    line-height: 1.6;
  }

  &:has(:deep(.el-form-item__error)) {
    margin-bottom: 8px;
  }
}

.spec-card {
  padding: 12px;
  border: 1px solid var(--app-border);
  border-radius: 6px;
  background: var(--app-bg-subtle, transparent);
}

.spec-card__head {
  display: flex;
  align-items: flex-start;
  gap: 12px;

  // 卡片头一行的 form-item 去掉默认下边距，避免卡片被撑高
  :deep(.el-form-item) {
    margin-bottom: 0;
  }
}

.spec-card__index {
  flex: none;
  padding-top: 8px;
  font-weight: 600;
  color: var(--app-text);
}

.spec-card__field {
  flex: 1;
  min-width: 0;
}

.spec-card__field--narrow {
  flex: none;
  width: 140px;
}

.unit-table__row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 4px 0;
}

.unit-table__row--head {
  padding-bottom: 6px;
  font-size: 12px;
  color: var(--app-text-muted);
  border-bottom: 1px solid var(--app-border);
}

.unit-col--unit {
  width: 150px;
  flex: none;
}

.unit-col--rate {
  flex: 1;
  min-width: 0;
}

.unit-col--base {
  width: 88px;
  flex: none;
  display: flex;
  justify-content: center;
  padding-top: 6px;
}

.unit-col--price {
  width: 130px;
  flex: none;
}

.unit-col--op {
  width: 44px;
  flex: none;
  display: flex;
  justify-content: center;
  padding-top: 4px;
}

// 单位行嵌在自由布局里，form-item 若保留默认下边距会把行高撑成两倍
.unit-form-item {
  margin-bottom: 0;
}

.rate-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
}

.rate-hint {
  font-size: 12px;
  line-height: 1.4;
  color: var(--app-text-muted);
}

.unit-table__foot {
  padding-top: 4px;
}

.add-spec {
  width: 100%;
  margin-top: 12px;
  border-style: dashed;
}
</style>
