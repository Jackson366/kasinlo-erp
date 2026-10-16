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
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvCategoryOption,
  InvCategoryPayload,
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
 * - `enabledOptions`（表单用）：只给启用分类，编辑时再把当前值补回去。
 */
const categories = ref<InvCategoryOption[]>([])
const enabledOptions = ref<InvCategoryOption[]>([])

/** 当前编辑商品的分类（可能已停用）—— 打开弹窗时记下来，避免依赖列表行还在不在 */
const editingCategory = ref<InvCategoryOption | undefined>()

async function loadCategories(): Promise<void> {
  const [all, enabled] = await Promise.all([
    invApi.categories({ pageSize: 500 }),
    invApi.categoryOptions(),
  ])
  categories.value = all.list.map((c) => ({ id: c.id, parentId: c.parentId, name: c.name }))
  enabledOptions.value = enabled
}

// ---------------- 新增 / 编辑 ----------------
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<InvProductPayload>({
  formRef,
  create: (payload) => invApi.createProduct(payload),
  update: (id, payload) => invApi.updateProduct(id, payload),
  messages: { create: '新增成功', update: '修改成功' },
})
const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog

/**
 * 新建一个空规格：默认带一条基本单位。
 *
 * 规格名与单位名都可以**留空** —— 保存时由后端补成「默认 / 个」。
 * 表单里预置一行是为了让想填的人有地方填，而不是因为它是必填项。
 */
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
    ElMessage.warning('至少留一个规格（留空不填也可以，系统会按「默认」处理）')
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
    ElMessage.warning('至少留一个计量单位（留空不填也可以，系统会按「个」处理）')
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
// 这里只管**真正的错误**：规格重名、同一规格下单位重名。
//
// 规格名 / 单位名 / 换算率 / 基本单位都**不再是必填**：留空由后端
// normalizeSpecs 兜底成「默认」「个」「1」，并把第一个单位提为基本单位。
// 理由是这些约束对录一个「办公用品-签字笔」的人来说纯属负担 ——
// 真正的约束（库存要有归属、单据要能折算）由服务端守住，
// 不应该转嫁成用户面前的必填项。
//
// 唯一不能让步的是重名：两行「φ20」会把库存劈成两半、各自都不预警；
// 同一规格下两个「箱」会让单据选单位时不知道看哪一行的价格。
function validateSpecs(_rule: unknown, _value: unknown, callback: (e?: Error) => void): void {
  const errs: string[] = []

  const seenSpec = new Set<string>()
  form.specs.forEach((s, si) => {
    const label = `第 ${si + 1} 个规格`
    // 按「补完之后的名字」判重：两个都留空的规格，后端都会补成「默认」，同属重名
    const specName = s.spec.trim() || '默认'
    if (seenSpec.has(specName)) errs.push(`规格「${specName}」重复了`)
    else seenSpec.add(specName)

    const seenUnit = new Set<string>()
    s.units.forEach((u) => {
      const name = u.unit.trim() || '个'
      if (seenUnit.has(name)) errs.push(`${label}：单位「${name}」重复了`)
      else seenUnit.add(name)
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
    ? { id: row.categoryId, parentId: 0, name: row.categoryName || '' }
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

/**
 * 分类下拉（树形）：只给启用项，但要把当前值补回去，否则老商品显示成空白。
 *
 * 刻意**不用 el-tree-select**：表单里选分类是一次性动作，下拉比树省空间，
 * 而且「当前值补回」在树控件里要做额外展开定位，不值当。
 * 真正需要树的地方是左侧分类栏 —— 那里要的是「浏览 + 维护」，不是「选一个」。
 */
/**
 * 表单里「商品分类」下拉的选项：**扁平化 + 层级路径**。
 *
 * 两个必须扁平化的理由：
 *  1. `el-option` 用 v-for 只遍历数组顶层 —— 直接喂树结构（带 children）
 *     的结果是**只有根分类能被选到**，子分类全部消失。分类多级化之后，
 *     这一条会让「钢材 / 螺纹钢」这类子分类彻底选不了。
 *  2. 扁平化之后 label 是完整路径（`钢材 / 螺纹钢`），配合 `filterable`
 *     默认按 label 匹配，输入「钢材」或「螺纹」都能命中 ——
 *     与销售单里商品下拉的搜索体验一致。
 *
 * 刻意**不用 el-tree-select**：表单里选分类是一次性动作，下拉比树省空间。
 * 真正需要树的地方是左侧分类栏 —— 那里要的是「浏览 + 维护」，不是「选一个」。
 * 路径分隔符用 ` / ` 而不是树控件的缩进，避免在窄下拉里把文字挤没。
 */
const categoryFlatOptions = computed(() => {
  const out: Array<{ id: number; path: string }> = []
  const walk = (parentId: number, prefix: string) => {
    for (const c of enabledOptions.value.filter((x) => x.parentId === parentId)) {
      const path = prefix ? `${prefix} / ${c.name}` : c.name
      out.push({ id: c.id, path })
      walk(c.id, path)
    }
  }
  walk(0, '')
  // 当前编辑项的分类可能已停用（不在启用列表里），补回列表头部，
  // 否则老商品打开编辑时分类显示成空白、看起来像「分类丢了」
  const cur = editingCategory.value
  if (cur && !out.some((o) => o.id === cur.id)) {
    out.unshift({ id: cur.id, path: `${cur.name}（已停用）` })
  }
  return out
})

/** 列表行展开时规格摘要，例如「φ20 · φ25」 */
function specSummary(row: InvProductRow): string {
  return row.specs.map((s) => s.spec).join(' · ') || '-'
}

// ============================================================
// 左侧分类树（原独立「商品分类」页合并到这里）
// ============================================================

interface CatNode {
  id: number
  parentId: number
  name: string
  sort: number
  status: number
  remark: string
  productCount: number
  childCount: number
}

const catLoading = ref(false)
const catRows = ref<CatNode[]>([])
/** 树当前选中的分类 id；0 = 全部商品 */
const activeCat = ref(0)
/** 分类树默认展开的节点 */
const expandedCat = ref<number[]>([])

async function loadCatTree(): Promise<void> {
  catLoading.value = true
  try {
    // 商品页的分类树要看得见全部（含停用）：停用分类下的商品仍在库里，
    // 树上看不到就等于那批商品从这一页消失了。
    const res = await invApi.categories({ page: 1, pageSize: 500 })
    catRows.value = res.list as unknown as CatNode[]
    expandedCat.value = catRows.value.filter((c) => c.parentId > 0).map((c) => c.parentId)
  } finally {
    catLoading.value = false
  }
}

interface CatTreeNode {
  id: number
  label: string
  productCount: number
  childCount: number
  status: number
  children: CatTreeNode[]
}

/** 平表 → 树。按 sort 排序，父节点缺失的节点挂到根下，不丢数据 */
const catTree = computed<CatTreeNode[]>(() => {
  const byParent = new Map<number, CatNode[]>()
  for (const c of catRows.value) {
    const arr = byParent.get(c.parentId) || []
    arr.push(c)
    byParent.set(c.parentId, arr)
  }
  const build = (parentId: number): CatTreeNode[] =>
    (byParent.get(parentId) || [])
      .slice()
      .sort((a, b) => a.sort - b.sort || a.id - b.id)
      .map((c) => ({
        id: c.id,
        label: c.name,
        productCount: c.productCount,
        childCount: c.childCount,
        status: c.status,
        children: build(c.id),
      }))
  return build(0)
})

/** 树节点文字：带上商品数，选之前就知道这一分类下有多少货 */
function catNodeLabel(node: CatTreeNode): string {
  return node.productCount > 0 ? `${node.label}（${node.productCount}）` : node.label
}

/** 选分类 → 右侧商品列表按 categoryId 过滤；点「全部」则清空 */
function onCatClick(node: { id: number } | null): void {
  activeCat.value = node ? node.id : 0
  query.categoryId = node ? node.id : undefined
  search()
}

// ---------------- 分类的增改删 ----------------
const catDialogVisible = ref(false)
const catSubmitting = ref(false)
const catEditingId = ref(0)
const catForm = reactive<InvCategoryPayload>({ parentId: 0, name: '', sort: 0, status: 1, remark: '' })
const catFormRef = ref<FormInstance>()
const catRules: FormRules = {
  name: [{ required: true, message: '请输入分类名称', trigger: 'blur' }],
}

/**
 * 上级分类下拉。不能选自己与自己的子孙 —— parent_id 一成环，
 * 前端按 parent_id 组装树会无限递归，页面直接卡死。
 * 这里过滤只是体验，真正的防线在后端 categoryCycleFree。
 */
const parentOptions = computed<InvCategoryOption[]>(() => {
  const flat: InvCategoryOption[] = [{ id: 0, parentId: 0, name: '（根分类）' }]
  const walk = (parentId: number, depth: number) => {
    for (const c of catRows.value.filter((x) => x.parentId === parentId)) {
      if (c.id === catEditingId.value) continue
      flat.push({ id: c.id, parentId: c.parentId, name: '　'.repeat(depth) + c.name })
      walk(c.id, depth + 1)
    }
  }
  walk(0, 0)
  return flat
})

function openCatCreate(parentId: number): void {
  catEditingId.value = 0
  Object.assign(catForm, { parentId, name: '', sort: 0, status: 1, remark: '' })
  catDialogVisible.value = true
}

function openCatEdit(id: number): void {
  const c = catRows.value.find((x) => x.id === id)
  if (!c) return
  catEditingId.value = id
  Object.assign(catForm, {
    parentId: c.parentId,
    name: c.name,
    sort: c.sort,
    status: c.status,
    remark: c.remark,
  })
  catDialogVisible.value = true
}

async function submitCat(): Promise<void> {
  await catFormRef.value?.validate()
  catSubmitting.value = true
  try {
    if (catEditingId.value) {
      await invApi.updateCategory(catEditingId.value, { ...catForm })
      ElMessage.success('分类已保存')
    } else {
      await invApi.createCategory({ ...catForm })
      ElMessage.success('分类已新增')
    }
    catDialogVisible.value = false
    await Promise.all([loadCatTree(), loadCategories(), load()])
  } finally {
    catSubmitting.value = false
  }
}

async function removeCat(id: number): Promise<void> {
  const c = catRows.value.find((x) => x.id === id)
  if (!c) return
  if (c.childCount > 0) {
    ElMessageBox.alert(
      `「${c.name}」下还有 ${c.childCount} 个子分类。请先把子分类移走或删除，否则它们会集体变成根分类。`,
      '不能删除',
      { type: 'warning', confirmButtonText: '知道了' },
    ).catch(() => undefined)
    return
  }
  if (c.productCount > 0) {
    ElMessageBox.alert(
      `「${c.name}」下还有 ${c.productCount} 个商品，删除会让这些商品的分类悬空 —— 报表品类占比也会凭空少一块。\n\n请先把商品改到其他分类，或在这里改为停用。`,
      '不能删除',
      { type: 'warning', confirmButtonText: '知道了' },
    ).catch(() => undefined)
    return
  }
  try {
    await ElMessageBox.confirm(`确定删除分类「${c.name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await invApi.deleteCategory(id)
  ElMessage.success('分类已删除')
  if (activeCat.value === id) onCatClick(null)
  await Promise.all([loadCatTree(), loadCategories()])
}

onMounted(() => {
  load()
  loadCategories()
  loadCatTree()
})
</script>

<template>
  <div class="product-page">
    <div class="product-layout">
      <!-- 左栏：分类树。原「商品分类」独立页合并到这里 ——
           维护分类时本来就要盯着它下面的商品，分两页等于同一个动作开两个入口 -->
      <div class="page-card cat-pane">
        <div class="cat-pane__head">
          <span class="cat-pane__title">商品分类</span>
          <el-tooltip content="新增根分类" placement="top">
            <el-button v-perm="'inv:category:create'" link type="primary" @click="openCatCreate(0)">
              <AppIcon name="Plus" />
            </el-button>
          </el-tooltip>
        </div>
        <el-scrollbar v-loading="catLoading" class="cat-pane__tree">
          <el-tree
            :data="catTree"
            node-key="id"
            :expand-on-click-node="false"
            :default-expanded-keys="expandedCat"
            highlight-current
            @current-change="onCatClick"
          >
            <template #default="{ data }">
              <span class="cat-node" :class="{ 'cat-node--off': data.status === 0 }">
                <span class="cat-node__label">{{ catNodeLabel(data) }}</span>
                <span class="cat-node__ops" @click.stop>
                  <el-button
                    v-perm="'inv:category:create'"
                    link
                    type="primary"
                    title="新增子分类"
                    @click="openCatCreate(data.id)"
                  >
                    <AppIcon name="Plus" />
                  </el-button>
                  <el-button
                    v-perm="'inv:category:update'"
                    link
                    type="primary"
                    title="编辑分类"
                    @click="openCatEdit(data.id)"
                  >
                    <AppIcon name="Edit" />
                  </el-button>
                  <el-button
                    v-perm="'inv:category:delete'"
                    link
                    type="danger"
                    title="删除分类"
                    @click="removeCat(data.id)"
                  >
                    <AppIcon name="Delete" />
                  </el-button>
                </span>
              </span>
            </template>
          </el-tree>
        </el-scrollbar>
        <div class="cat-pane__foot">
          <el-button size="small" :type="activeCat === 0 ? 'primary' : 'default'" @click="onCatClick(null)">
            全部商品
          </el-button>
        </div>
      </div>

      <!-- 右栏：商品表格 -->
      <div class="product-main">
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
              <el-select
                v-model="query.categoryId"
                placeholder="全部"
                clearable
                style="width: 160px"
                @change="search"
              >
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
    </div>
    </div>

    <!-- 分类弹窗：新增 / 编辑。上级分类过滤掉自己与子孙，防止 parent_id 成环 -->
    <el-dialog
      v-model="catDialogVisible"
      :title="catEditingId ? '编辑分类' : '新增分类'"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="catFormRef" :model="catForm" :rules="catRules" label-width="88px">
        <el-form-item label="上级分类" prop="parentId">
          <el-select v-model="catForm.parentId" style="width: 100%">
            <el-option
              v-for="c in parentOptions"
              :key="c.id"
              :label="c.name"
              :value="c.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="分类名称" prop="name">
          <el-input v-model="catForm.name" placeholder="如 螺纹钢" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="catForm.sort" :min="0" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch :model-value="catForm.status === 1" @update:model-value="(v) => (catForm.status = v ? 1 : 0)" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="catForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="catDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="catSubmitting" @click="submitCat">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增商品' : '编辑商品'"
      width="1100px"
      top="6vh"
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
              <!-- 可输入搜索，与销售单里选商品同一套交互：
                   分类多了（尤其多级）靠翻列表是找不到的 -->
              <el-select
                v-model="form.categoryId"
                filterable
                clearable
                placeholder="输入分类名搜索，如 钢材 / 螺纹钢"
                style="width: 100%"
              >
                <el-option v-for="c in categoryFlatOptions" :key="c.id" :label="c.path" :value="c.id" />
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
          <span class="divider-title">规格与计量单位（可留空）</span>
        </el-divider>
        <p class="spec-tip">
          简单商品<strong>留空即可</strong>，保存时自动按「默认 / 个」处理。
          需要区分时才填：一个商品可维护多个规格，每个规格下可维护多个计量单位；
          库存按<strong>基本单位</strong>计数，出入库时按所选单位的换算率折算。
        </p>

        <!-- 跨行约束只剩「重名」一项，由 validateSpecs 判完。它挂在 specs 这个
             「整数组」上，必须有一个绑到 prop="specs" 的 form-item 才会被
             form.validate() 调到 —— 否则 rules.specs 就是一段死配置。 -->
        <el-form-item prop="specs" class="spec-errors" label-width="0" />

        <!-- 规格列表：规格名 / 单位名 / 换算率都不再是必填，
             错误信息只在「重名」时从整组校验里给出 -->
        <div class="spec-list">
          <div v-for="(spec, si) in form.specs" :key="si" class="spec-card">
            <div class="spec-card__head">
              <span class="spec-card__index">规格 {{ si + 1 }}</span>
              <el-form-item
                :prop="`specs.${si}.spec`"
                label="规格"
                class="spec-card__field"
              >
                <el-input v-model="spec.spec" placeholder="选填，如 φ20 / 500ml（留空按「默认」）" />
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
                    class="unit-form-item"
                  >
                    <el-input v-model="u.unit" placeholder="选填，如 根 / 箱（留空按「个」）" />
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
// ---------------- 左右分栏 ----------------
.product-layout {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.cat-pane {
  width: 248px;
  flex: none;
  padding: 12px;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 8px;
    border-bottom: 1px solid var(--app-border-light);
  }

  &__title {
    font-size: 14px;
    font-weight: 600;
  }

  // 树要能单独滚：分类几十条时不至于把整页拉长
  &__tree {
    max-height: calc(100vh - 260px);
  }

  &__foot {
    padding-top: 10px;
    margin-top: 8px;
    border-top: 1px solid var(--app-border-light);
  }
}

.product-main {
  flex: 1;
  min-width: 0; // 否则表格会把弹性盒撑破，出现横向滚动
}

// 分类节点：名称占满剩余宽度，操作按钮**常显**。
//
// 此前按钮做成 hover 才出，结果一个都看不到 —— el-tree 的 .el-tree-node__content
// 是 flex 容器，节点内容前面还有展开箭头与层级缩进；这里再写 width: 100%
// 会让内容区比容器还宽，三个按钮被推到可视区之外裁掉，hover 也够不到。
// 常显 + flex: 1 + min-width: 0 才是可靠做法，顺带省掉了「要先把鼠标放上去」
// 这层发现成本 —— 用户看不到按钮就会以为没有这个功能。
.cat-node {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 4px;
  flex: 1;
  min-width: 0;
  padding-right: 2px;

  &__label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__ops {
    display: inline-flex;
    align-items: center;
    flex: none;

    :deep(.el-button) {
      padding: 0 3px;
      height: auto;
      font-size: 13px;
      line-height: 1;
    }

    // Element 默认给相邻按钮 12px 左间距，三个图标在 248px 的树里会占掉一大截
    :deep(.el-button + .el-button) {
      margin-left: 0;
    }
  }

  // 停用分类压灰，但仍在树上：它下面的商品还在库里
  &--off &__label {
    color: var(--app-text-muted);
    text-decoration: line-through;
  }
}

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
