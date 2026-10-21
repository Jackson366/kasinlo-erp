<script setup lang="ts">
/**
 * 打印模板设计器（自包含弹窗）。
 *
 * 左侧编辑版式，右侧用示例数据实时预览；保存时把配置序列化成 JSON
 * 写进模板的 content 字段。设计器分四组：
 *
 *   1. 纸张与标题   标题、纸张规格（A4/A5 竖版）、标题是否显示
 *   2. 公司信息与 Logo  公司名 / 电话 / 地址 / 开户行；可放表尾落款（默认）或表头居中
 *   3. 信息条       往来单位 / 日期 / 单号等字段：可改名、显隐、排序
 *   4. 明细列       显隐、列标题可改、对齐方式、可上下移动调顺序
 *                  （序号 / 编码 / 基本数量 / 换算率 / 备注都在列池里，可自由组合）
 *   5. 页面与底部   页边距（毫米，直接驱动打印 @page）/ 行高 / 字号 / 表头底衬；
 *                  收货声明与签字处可自定义，备注 / 大写 / 页脚、旧版三联签收栏
 *
 * 页边距单位是毫米并与打印 @page 同源（PrintSheet 挂载时注入），
 * 预览按同一数值模拟 —— 设计器里看到多宽的留白，打印出来就是多宽。
 *
 * 纸面渲染统一收在 PrintSheet（采购销售 / 出入库共用），新增数据列只需
 * 在 printConfig 的 ALL_COLUMNS 加一项并在 PrintSheet.cell() 补一个取值。
 */
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { invApi } from '@/api'
import type { InvOrderDetail, InvPrintTemplateRow, InvStockOrderDetail } from '@/types/api'
import {
  ALL_COLUMNS,
  DEFAULT_COLUMNS,
  defaultDeclaration,
  defaultMetaFields,
  defaultSignItems,
  defaultSignLabel,
  parseContent,
  type ColumnKey,
  type PrintColumn,
  type PrintContentConfig,
  type SheetMode,
} from './printConfig'
import PrintSheet from './PrintSheet.vue'

defineOptions({ name: 'TemplateDesigner' })

const emit = defineEmits<{ saved: [] }>()

const visible = ref(false)
const saving = ref(false)
const row = ref<InvPrintTemplateRow | null>(null)
const paper = ref<'A4' | 'A5'>('A4')
/** 左侧分组默认展开哪一个（单据组，最常用） */
const activeGroup = ref('doc')

const cfg = reactive<PrintContentConfig>(parseContent(''))

/** 设计器预览用示例数据——字段结构与真实打印详情一致 */
const sample: InvStockOrderDetail = {
  id: 0,
  orderNo: 'CK202610130001',
  direction: 'in',
  sourceType: 'purchase',
  sourceOrderNo: 'CG202610010001',
  orderDate: '2026-10-13T00:00:00Z',
  remark: '示例备注：按采购单批量入库，已验收。',
  salesman: '李四',
  operator: '张三',
  totalQty: 148,
  items: [
    // 第 2 行刻意用非基本单位（1 箱 = 10 盒），让预览能看出折算行
    { productCode: 'SP001', productName: '高强度螺栓 M12', spec: 'M12×40', unit: '盒', rate: 1, qty: 120, qtyBase: 120, unitBase: '盒' },
    { productCode: 'SP002', productName: '角钢 50×50', spec: '50×50×5mm', unit: '箱', rate: 10, qty: 2, qtyBase: 20, unitBase: '根' },
    { productCode: 'SP003', productName: '密封胶条', spec: '10m/卷', unit: '卷', rate: 1, qty: 8, qtyBase: 8, unitBase: '卷' },
  ],
}

/** 采购 / 销售单预览用示例数据（带单价金额） */
const sampleOrder: InvOrderDetail = {
  id: 0,
  orderNo: 'CG202610130001',
  bizType: 'purchase',
  partyId: 0,
  partyName: '信阳市宏达钢材有限公司',
  orderDate: '2026-10-13T00:00:00Z',
  dueDate: null, // 到期日是内部结算字段，不进纸面；示例数据同样留空
  status: 1,
  totalAmount: 15440,
  settledAmount: 0,
  settleStatus: 'unpaid',
  itemCount: 3,
  remark: '示例备注：含运费，货到付款。',
  createdAt: '2026-10-13T08:00:00Z',
  salesman: '李四',
  operator: '张三',
  items: [
    { id: 1, productId: 1, specId: 1, unitId: 1, productCode: 'SP001', productName: '高强度螺栓 M12', spec: 'M12×40', unit: '盒', rate: 1, qty: 120, qtyBase: 120, price: 45, amount: 5400 },
    // 第 2 行用「箱」成交（1 箱 = 10 根），单价按箱计，纸面会多出一行换算说明
    { id: 2, productId: 2, specId: 2, unitId: 3, productCode: 'SP002', productName: '角钢 50×50', spec: '50×50×5mm', unit: '箱', rate: 10, qty: 8, qtyBase: 80, price: 180, amount: 1440 },
    { id: 3, productId: 3, specId: 3, unitId: 4, productCode: 'SP003', productName: '密封胶条', spec: '10m/卷', unit: '卷', rate: 1, qty: 80, qtyBase: 80, price: 107.5, amount: 8600 },
  ],
}

/** 模板的单据类型指向采购 / 销售单时，预览切换到对应的纸面版式 */
const isOrderDoc = computed(() => /采购|销售/.test(row.value?.docType || ''))
const mode = computed<SheetMode>(() => (isOrderDoc.value ? 'order' : 'stock'))
const orderPartyLabel = computed(() =>
  /销售/.test(row.value?.docType || '') ? '客户' : '供应商',
)

function open(r: InvPrintTemplateRow): void {
  row.value = r
  paper.value = r.paper === 'A5' ? 'A5' : 'A4'
  Object.assign(cfg, parseContent(r.content))
  // 出入库单没有成交价：把单价 / 金额列从配置里摘掉，设计器列表与纸面保持一致
  if (mode.value === 'stock') {
    cfg.columns = cfg.columns.filter((c) => c.key !== 'price' && c.key !== 'amount')
  }
  // 信息条没配置过（老模板）就按当前单据类型初始化，保存后固化
  if (!cfg.meta.length) {
    cfg.meta = defaultMetaFields(mode.value, orderPartyLabel.value)
  }
  visible.value = true
}

/** 采购 / 销售单预览用示例数据（带单价金额） */
const isSalesDoc = computed(() => /销售/.test(row.value?.docType || ''))

/**
 * 预览用的示例单据。销售类模板要给「销售」口径的示例：
 * 声明话术（款未付 / 货款按约定结算）与往来单位标签都跟着业务线走，
 * 否则设计器里看到的纸面和真实打印出来的一句话不一样。
 */
const previewDetail = computed<InvOrderDetail | InvStockOrderDetail>(() =>
  isOrderDoc.value
    ? {
        ...sampleOrder,
        bizType: (isSalesDoc.value ? 'sales' : 'purchase') as InvOrderDetail['bizType'],
        partyName: isSalesDoc.value ? '信阳市宏达商贸有限公司' : '信阳市宏达钢材有限公司',
      }
    : sample,
)

const previewTitle = computed(() => {
  if (cfg.title) return cfg.title
  // 纸张抬头用模板的单据类型（「销售订单 / 销货出库单」），比「销售单」更像纸面
  const dt = row.value?.docType || ''
  if (/采购|销售/.test(dt)) return dt
  return sample.direction === 'in' ? '入库单' : '出库单'
})

/** 列表项上下移（明细列与信息条共用） */
function moveItem<T>(arr: T[], index: number, dir: -1 | 1): void {
  const to = index + dir
  if (to < 0 || to >= arr.length) return
  const [it] = arr.splice(index, 1)
  arr.splice(to, 0, it)
}

// ---------------- 明细列编辑 ----------------
/**
 * 可添加的列池。出入库单没有单价 / 金额（成本不归仓管看），
 * 所以那两个键在库存单据下不进池子 —— 加进去纸面也没有数据可渲染。
 */
const columnPool = computed<PrintColumn[]>(() =>
  ALL_COLUMNS.filter(
    (c) =>
      !(mode.value === 'stock' && (c.key === 'price' || c.key === 'amount')) &&
      !cfg.columns.some((x) => x.key === c.key),
  ),
)

function addColumn(key: ColumnKey): void {
  const tpl = ALL_COLUMNS.find((c) => c.key === key)
  if (tpl) cfg.columns.push({ ...tpl })
}

function removeCol(index: number): void {
  const c = cfg.columns[index]
  // 名称与数量是表格的主体，删了纸面就没有可读内容
  if (c.key === 'name' || c.key === 'qty') {
    ElMessage.warning(`「${c.label}」是明细表的主体列，不能删除（可以改为不显示）`)
    return
  }
  cfg.columns.splice(index, 1)
}

/** 恢复默认列集合：用户改乱了之后的一条退路（出入库单没有单价 / 金额） */
function resetColumns(): void {
  cfg.columns = DEFAULT_COLUMNS.filter(
    (c) => !(mode.value === 'stock' && (c.key === 'price' || c.key === 'amount')),
  ).map((c) => ({ ...c }))
}

// ---------------- 信息条编辑 ----------------
function addMeta(): void {
  cfg.meta.push({ key: `field${Date.now() % 1000}`, label: '自定义字段', visible: false })
}

function removeMeta(index: number): void {
  cfg.meta.splice(index, 1)
}

/** 默认声明 / 签字标签的提示文案（占位符里显示「留空会打成什么」） */
const declHint = computed(() =>
  defaultDeclaration(mode.value, isOrderDoc.value ? (isSalesDoc.value ? 'sales' : 'purchase') : undefined, sample.direction),
)
const signHint = computed(() => defaultSignLabel(mode.value, orderPartyLabel.value, sample.direction))

// ---------------- 签收栏编辑 ----------------
/**
 * 签收栏项目：配置为 null（未自定义）时显示当前单据类型的默认项；
 * 用户动过就存实际数组，与默认完全一致时归一回 null（跨单据类型仍跟随默认）。
 */
const signItemsView = computed<string[]>({
  get: () => cfg.footer.signItems ?? defaultSignItems(mode.value),
  set: (v) => {
    cfg.footer.signItems = v
  },
})

function sameSign(a: string[] | null, b: string[]): boolean {
  return (a ?? []).join('\u0001') === b.join('\u0001')
}

async function save(): Promise<void> {
  if (!row.value) return
  saving.value = true
  try {
    const signItems = sameSign(cfg.footer.signItems, defaultSignItems(mode.value)) ? null : signItemsView.value
    await invApi.updatePrintTemplate(row.value.id, {
      code: row.value.code,
      name: row.value.name,
      docType: row.value.docType,
      paper: paper.value,
      content: JSON.stringify({ ...cfg, footer: { ...cfg.footer, signItems } }),
      status: row.value.status,
      remark: row.value.remark,
    })
    ElMessage.success('版式已保存')
    emit('saved')
    visible.value = false
  } finally {
    saving.value = false
  }
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`设计版式 — ${row?.name || ''}`"
    width="1280px"
    top="4vh"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div class="designer">
      <div class="designer__panel">
        <el-form label-width="82px" label-position="left" size="small">
          <el-collapse v-model="activeGroup">
            <el-collapse-item title="纸张与标题" name="doc">
              <el-form-item label="单据标题">
                <el-input v-model="cfg.title" placeholder="留空则按单据类型显示默认标题" />
              </el-form-item>
              <el-form-item label="显示标题">
                <el-switch v-model="cfg.header.showTitle" />
              </el-form-item>
              <el-form-item label="纸张规格">
                <el-radio-group v-model="paper">
                  <el-radio-button value="A4">A4 竖版</el-radio-button>
                  <el-radio-button value="A5">A5 竖版</el-radio-button>
                </el-radio-group>
              </el-form-item>
            </el-collapse-item>

            <el-collapse-item title="公司信息与 Logo" name="company">
              <el-form-item label="显示位置">
                <el-radio-group v-model="cfg.header.placement">
                  <el-radio-button value="bottom">表尾落款</el-radio-button>
                  <el-radio-button value="top">表头居中</el-radio-button>
                </el-radio-group>
              </el-form-item>
              <el-form-item label="公司名称">
                <el-input v-model="cfg.header.companyName" placeholder="如：信阳市全程电子有限公司" />
              </el-form-item>
              <el-form-item label="电话">
                <el-input v-model="cfg.header.phone" placeholder="如：13598586346" />
              </el-form-item>
              <el-form-item label="地址">
                <el-input v-model="cfg.header.address" />
              </el-form-item>
              <el-form-item label="开户行">
                <el-input v-model="cfg.header.bank" placeholder="选填" />
              </el-form-item>
              <el-form-item label="Logo 地址">
                <el-input v-model="cfg.header.logoUrl" placeholder="图片 URL，留空不显示（仅表头模式）" />
              </el-form-item>
              <div class="col-editor__foot text-muted">
                「表尾落款」是纸质送货单的常规写法：公司名与电话压在明细表下方、合计金额左侧。
              </div>
            </el-collapse-item>

            <el-collapse-item title="信息条（单据编号 / 日期等）" name="meta">
              <div class="col-editor">
                <div class="col-editor__row col-editor__row--head">
                  <span class="col-editor__move" />
                  <span class="col-editor__label">显示文案</span>
                  <span class="col-editor__show">显示</span>
                  <span class="col-editor__op" />
                </div>
                <div v-for="(m, i) in cfg.meta" :key="m.key + i" class="col-editor__row">
                  <span class="col-editor__move">
                    <el-button link size="small" :disabled="i === 0" @click="moveItem(cfg.meta, i, -1)">↑</el-button>
                    <el-button
                      link
                      size="small"
                      :disabled="i === cfg.meta.length - 1"
                      @click="moveItem(cfg.meta, i, 1)"
                    >
                      ↓
                    </el-button>
                  </span>
                  <span class="col-editor__label">
                    <el-input v-model="m.label" size="small" />
                  </span>
                  <span class="col-editor__show">
                    <el-checkbox v-model="m.visible" />
                  </span>
                  <span class="col-editor__op">
                    <el-button link type="danger" size="small" @click="removeMeta(i)">删</el-button>
                  </span>
                </div>
              </div>
              <div class="col-editor__foot">
                <el-button link type="primary" size="small" @click="addMeta">+ 添加自定义项</el-button>
                <el-button
                  link
                  size="small"
                  @click="cfg.meta = defaultMetaFields(mode, orderPartyLabel)"
                >
                  恢复默认
                </el-button>
                <div class="text-muted">空值的项不会打到纸面（如未填业务员的单据）</div>
              </div>
            </el-collapse-item>

            <el-collapse-item title="明细列（可改名 / 排序）" name="columns">
              <div class="col-editor">
                <div class="col-editor__row col-editor__row--head">
                  <span class="col-editor__move" />
                  <span class="col-editor__label">列标题</span>
                  <span class="col-editor__align">对齐</span>
                  <span class="col-editor__show">显示</span>
                  <span class="col-editor__op" />
                </div>
                <div v-for="(c, i) in cfg.columns" :key="c.key" class="col-editor__row">
                  <span class="col-editor__move">
                    <el-button link size="small" :disabled="i === 0" @click="moveItem(cfg.columns, i, -1)">↑</el-button>
                    <el-button
                      link
                      size="small"
                      :disabled="i === cfg.columns.length - 1"
                      @click="moveItem(cfg.columns, i, 1)"
                    >
                      ↓
                    </el-button>
                  </span>
                  <span class="col-editor__label">
                    <el-input v-model="c.label" size="small" />
                  </span>
                  <span class="col-editor__align">
                    <el-select v-model="c.align" size="small">
                      <el-option label="左" value="left" />
                      <el-option label="中" value="center" />
                      <el-option label="右" value="right" />
                    </el-select>
                  </span>
                  <span class="col-editor__show">
                    <el-checkbox v-model="c.visible" />
                  </span>
                  <span class="col-editor__op">
                    <el-button link type="danger" size="small" @click="removeCol(i)">删</el-button>
                  </span>
                </div>
              </div>
              <div class="col-editor__foot">
                <span class="text-muted">可添加：</span>
                <el-button
                  v-for="p in columnPool"
                  :key="p.key"
                  link
                  type="primary"
                  size="small"
                  @click="addColumn(p.key)"
                >
                  {{ p.label }}
                </el-button>
                <el-button link size="small" @click="resetColumns">恢复默认列</el-button>
              </div>
            </el-collapse-item>

            <el-collapse-item title="页面与底部" name="page">
              <el-form-item label="页边距">
                <el-input-number
                  v-model="cfg.page.marginMm"
                  :min="0"
                  :max="30"
                  size="small"
                  controls-position="right"
                />
                <span class="unit">mm（打印留白，所见即所得）</span>
              </el-form-item>
              <el-form-item label="明细行高">
                <el-input-number v-model="cfg.page.rowHeight" :min="20" :max="80" size="small" controls-position="right" />
                <span class="unit">px</span>
              </el-form-item>
              <el-form-item label="正文字号">
                <el-select v-model="cfg.page.fontSize" size="small" style="width: 110px">
                  <el-option v-for="n in [9, 10, 11, 12, 13, 14]" :key="n" :value="n" :label="`${n} pt`" />
                </el-select>
              </el-form-item>
              <el-form-item label="标题字号">
                <el-select v-model="cfg.page.titleFontSize" size="small" style="width: 110px">
                  <el-option v-for="n in [16, 18, 20, 22, 24, 26]" :key="n" :value="n" :label="`${n} pt`" />
                </el-select>
              </el-form-item>
              <el-form-item label="表头底色">
                <el-input v-model="cfg.page.headerBg" size="small" placeholder="如 #f2f2f2，留空为无" />
              </el-form-item>
              <el-form-item label="收货声明">
                <el-input
                  v-model="cfg.footer.declaration"
                  type="textarea"
                  :rows="2"
                  :placeholder="`留空用默认：${declHint}`"
                />
              </el-form-item>
              <el-form-item label="签字处">
                <el-input v-model="cfg.footer.signLabel" :placeholder="`默认：${signHint}`" />
              </el-form-item>
              <el-form-item label="三联签收栏">
                <el-switch v-model="cfg.footer.showSign" />
              </el-form-item>
              <el-form-item v-if="cfg.footer.showSign" label="签收项目">
                <el-select
                  v-model="signItemsView"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                  placeholder="输入后回车添加"
                  style="width: 100%"
                />
              </el-form-item>
              <el-form-item label="单据备注">
                <el-switch v-model="cfg.footer.showRemark" />
              </el-form-item>
              <el-form-item v-if="isOrderDoc" label="合计大写">
                <el-switch v-model="cfg.footer.showAmountCn" />
              </el-form-item>
              <el-form-item label="页脚文字">
                <el-input v-model="cfg.footer.text" placeholder="如：货物品以当面验收为准" maxlength="60" />
              </el-form-item>
              <el-form-item label="附加文字">
                <el-input
                  v-model="cfg.footer.customText"
                  type="textarea"
                  :rows="2"
                  placeholder="显示在备注之后，可写账期、验收标准等"
                />
              </el-form-item>
            </el-collapse-item>
          </el-collapse>
        </el-form>
      </div>

      <div class="designer__preview">
        <PrintSheet
          :mode="mode"
          :detail="previewDetail"
          :cfg="cfg"
          :title="previewTitle"
          :paper="paper"
          :party-label="orderPartyLabel"
        />
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存版式</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.designer {
  display: flex;
  gap: 20px;

  &__panel {
    width: 360px;
    flex: none;
    max-height: 74vh;
    overflow: auto;
  }

  &__preview {
    flex: 1;
    max-height: 74vh;
    overflow: auto;
    padding: 4px;
    background: var(--app-bg-muted, transparent);
    border-radius: var(--radius-sm);
  }
}

.unit {
  margin-left: 6px;
  color: var(--app-text-muted);
  font-size: 12px;
}

.col-editor {
  &__row {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 4px;

    &--head {
      font-size: 12px;
      color: var(--app-text-muted);
      margin-bottom: 6px;
    }
  }

  &__move {
    width: 56px;
    flex: none;
  }

  &__label {
    flex: 1;
  }

  &__align {
    width: 72px;
    flex: none;
  }

  &__show {
    width: 40px;
    flex: none;
    text-align: center;
  }

  &__op {
    width: 28px;
    flex: none;
  }

  &__foot {
    margin-top: 8px;
    font-size: 12px;
    line-height: 1.8;
  }
}
</style>
