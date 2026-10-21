<script setup lang="ts">
/**
 * 单据打印纸面（纯展示组件，采购销售 / 出入库共用）。
 *
 * 版式按纸质送货单重做：
 *
 *   　　　　　　　　　　销货出库单
 *   客户：xxx　　　　单据日期：2026-07-14　　　　单据编号：XHCD0582
 *   ┌──────────┬────────┬────┬────┬────┬────┬──────┐
 *   │ 商品名称  │ 规格型号│单位│单价│数量│金额│备注信息│
 *   ├──────────┼────────┼────┼────┼────┼────┼──────┤
 *   │ xxx      │        │ 个 │300 │ 1  │300 │      │
 *   └──────────┴────────┴────┴────┴────┴────┴──────┘
 *   信阳市全程电子有限公司 联系电话：135…        合计金额：300
 *   本人确认已收到上述产品，经核对数量、型号、外观均无误，款未付。　客户签字：
 *
 * 与旧版的差别（都是有意的）：
 *   1. 抬头上不再压公司名与地址，落款放到明细表下方——签收人第一眼要看到的是货和钱；
 *   2. 信息条没有上下横线，三个字段横向铺开，与表格左对齐；
 *   3. 表头无灰色底衬、细实线边框，黑白打印不发灰；
 *   4. 合计金额从表格里挪到表尾行右侧，与公司信息同一行；
 *   5. 三联「制单 / 审核 / 批准」签收栏换成「一行收货声明 + 签字处」，仍可切回。
 *
 * 版式完全由模板配置驱动：明细列（显隐 / 标题 / 对齐）、信息条（显隐 / 改名 /
 * 排序）、公司信息与 Logo、页边距与行高字号、声明与签字——
 * 组件本身不写死任何一列、任何一个信息条字段。
 *
 * 页边距：配置是毫米，预览按 96dpi 换算模拟，打印时注入 @page（applyPrintPage），
 * 两边数值一致——所见即所得。打印方向由 @page size 钉死为竖版。
 */
import { computed, onMounted, watch } from 'vue'
import type { InvOrderDetail, InvStockOrderDetail } from '@/types/api'
import { formatDay, formatQty } from '@/utils/format'
import {
  amountToCN,
  applyPrintPage,
  defaultDeclaration,
  defaultMetaFields,
  defaultSignItems,
  defaultSignLabel,
  MM_PX,
  visibleCols,
  type PrintContentConfig,
  type PrintMetaField,
  type SheetMode,
} from './printConfig'

const props = defineProps<{
  mode: SheetMode
  detail: InvOrderDetail | InvStockOrderDetail
  cfg: PrintContentConfig
  /** 最终展示的标题（配置标题留空时由父级按业务线取「采购单 / 销售单 / 入库单 / 出库单」） */
  title: string
  paper: 'A4' | 'A5'
  /** 往来单位标签：供应商 / 客户（仅 order 模式使用） */
  partyLabel?: string
}>()

/** 预览宽度按 96dpi 近似：A4 210mm≈794px、A5 148mm≈560px，打印时由 @media print 接管 */
const sheetWidth = computed(() => (props.paper === 'A5' ? '560px' : '794px'))

/** 页边距（配置毫米 → 预览像素）。打印时该值注入 @page，纸面 padding 归零 */
const framePad = computed(() => `${((props.cfg.page.marginMm || 10) * MM_PX).toFixed(1)}px`)

/** 打印页设置跟随纸面：竖版 + 页边距。挂载与配置变化时同步注入 */
onMounted(applyPage)
watch([() => props.paper, () => props.cfg.page.marginMm], applyPage)
function applyPage(): void {
  applyPrintPage(props.paper, props.cfg.page.marginMm)
}

const bizType = computed(() => (props.detail as InvOrderDetail).bizType)
const direction = computed(() => (props.detail as InvStockOrderDetail).direction)

const sourceLabel = computed(() => {
  const st = (props.detail as InvStockOrderDetail).sourceType
  return st === 'purchase' ? '采购单号' : st === 'sales' ? '销售单号' : '来源单号'
})

/** 信息条字段取值。value 为空且 skipIfEmpty 的字段（来源单号 / 业务员 / 制单人）不输出 */
interface MetaValue {
  label: string
  value: string
  skipIfEmpty?: boolean
}

const metaPool = computed<Record<string, MetaValue>>(() => {
  const d = props.detail
  if (props.mode === 'order') {
    const o = d as InvOrderDetail
    const pool: Record<string, MetaValue> = {
      party: { label: props.partyLabel || '往来单位', value: o.partyName || '-' },
      orderDate: { label: '单据日期', value: formatDay(o.orderDate) },
      orderNo: { label: '单据编号', value: o.orderNo },
      salesman: { label: '业务员', value: o.salesman || '', skipIfEmpty: true },
      operator: { label: '制单人', value: o.operator || '', skipIfEmpty: true },
    }
    return pool
  }
  const s = d as InvStockOrderDetail
  const pool: Record<string, MetaValue> = {
    orderDate: { label: '单据日期', value: formatDay(s.orderDate) },
    orderNo: { label: '单据编号', value: s.orderNo },
    source: { label: sourceLabel.value, value: s.sourceOrderNo || '', skipIfEmpty: true },
    salesman: { label: '业务员', value: s.salesman || '', skipIfEmpty: true },
    operator: { label: '制单人', value: s.operator || '', skipIfEmpty: true },
  }
  return pool
})

const metaItems = computed(() => {
  const cfgMeta = props.cfg.meta?.length ? props.cfg.meta : defaultMetaFields(props.mode, props.partyLabel || '')
  const pool = metaPool.value
  const items = cfgMeta
    .filter((f: PrintMetaField) => f.visible && pool[f.key])
    .map((f: PrintMetaField) => ({ ...pool[f.key], label: f.label || pool[f.key].label }))
    .filter((m) => !(m.skipIfEmpty && !m.value))
  // 采购 / 销售单永远带往来单位：模板可能固化过出入库模式的信息条
  // （出入库默认池里没有「往来单位」字段），被销售/采购单复用就会丢客户/供应商。
  // 这里兜底强制补上 —— 往来单位是单据的主体，比「可配置」优先级更高。
  if (props.mode === 'order' && !items.some((m) => m.label === (props.partyLabel || '往来单位'))) {
    const f = pool.party
    if (f) items.unshift({ ...f, label: props.partyLabel || f.label })
  }
  return items
})

const cols = computed(() => visibleCols(props.cfg, props.mode))
/** 空明细时的占位列数 */
const colCount = computed(() => Math.max(cols.value.length, 1))

const amountCN = computed(() => (props.mode === 'order' ? amountToCN((props.detail as InvOrderDetail).totalAmount) : ''))

/** 表尾合计：销售/采购打金额，出入库打数量 */
const totalLabel = computed(() => (props.mode === 'order' ? '合计金额' : '合计数量'))
const totalValue = computed(() =>
  props.mode === 'order'
    ? (props.detail as InvOrderDetail).totalAmount.toFixed(2)
    // 数量是 numeric(14,4)，直接打出来会是 1.0002 这种账本精度，纸面上要读成人话
    : formatQty((props.detail as InvStockOrderDetail).totalQty),
)

/** 表尾左侧的公司落款（配置了「公司信息位置 = 表尾」才显示） */
const companyBottom = computed(() => props.cfg.header.placement !== 'top')
const companyParts = computed(() => {
  const h = props.cfg.header
  const parts: string[] = []
  if (h.companyName) parts.push(h.companyName)
  if (h.phone) parts.push(`联系电话：${h.phone}`)
  if (h.address) parts.push(h.address)
  if (h.bank) parts.push(h.bank)
  return parts
})

const declaration = computed(
  () =>
    props.cfg.footer.declaration ||
    defaultDeclaration(props.mode, bizType.value, direction.value),
)
const signLabel = computed(
  () =>
    props.cfg.footer.signLabel ||
    defaultSignLabel(props.mode, props.partyLabel || '', direction.value),
)

type AnyItem = Record<string, unknown>

/** 单元格取值。把模板配置的列键翻成单据行上的字面值 */
function cell(it: AnyItem, key: string): string {
  switch (key) {
    case 'code':
      return String(it.productCode || '-')
    case 'name':
      return String(it.productName ?? '-')
    case 'spec':
      return String(it.spec || '')
    case 'unit':
      return String(it.unit || '-')
    case 'qtyBase':
      return formatQty(Number(it.qtyBase))
    case 'rate':
      return String(it.rate ?? '-')
    case 'price':
      return Number(it.price ?? 0).toFixed(2)
    case 'amount':
      return Number(it.amount ?? 0).toFixed(2)
    // 备注列是留给手写的空白格：纸质单据上的「备注信息」本来就要现场填
    case 'note':
      return ''
    default:
      return '-'
  }
}

/** 签收栏项目：配置为 null（未自定义）时按单据类型取默认 */
const signItems = computed(() =>
  props.cfg.footer.signItems?.length ? props.cfg.footer.signItems : defaultSignItems(props.mode),
)
</script>

<template>
  <div
    class="sheet"
    :class="`sheet--${paper}`"
    :style="{
      width: sheetWidth,
      fontSize: `${cfg.page.fontSize}pt`,
      padding: framePad,
      '--row-h': `${cfg.page.rowHeight}px`,
      '--th-bg': cfg.page.headerBg || 'transparent',
    }"
  >
    <img v-if="cfg.header.logoUrl && !companyBottom" class="sheet__logo" :src="cfg.header.logoUrl" alt="logo" />
    <h1 v-if="cfg.header.showTitle" class="sheet__title" :style="{ fontSize: `${cfg.page.titleFontSize}pt` }">
      {{ title }}
    </h1>
    <!-- 公司信息放在表头时（少数模板习惯）才在标题下重复一遍 -->
    <template v-if="!companyBottom">
      <div v-if="cfg.header.companyName" class="sheet__company">{{ cfg.header.companyName }}</div>
      <div v-if="cfg.header.address || cfg.header.phone" class="sheet__company-sub">
        {{ cfg.header.address }}{{ cfg.header.address && cfg.header.phone ? ' · ' : '' }}{{ cfg.header.phone }}
      </div>
      <div v-if="cfg.header.bank" class="sheet__company-sub">{{ cfg.header.bank }}</div>
    </template>

    <div class="sheet__meta">
      <span v-for="m in metaItems" :key="m.label" class="sheet__meta-item">
        <em>{{ m.label }}：</em>{{ m.value }}
      </span>
    </div>

    <table class="sheet__table">
      <thead>
        <tr>
          <!-- 表头一律居中：纸质单据的表头是「栏名」，与单元格数据的对齐方式无关 -->
          <th v-for="c in cols" :key="c.key" :class="`col-${c.key}`">
            {{ c.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(it, i) in (detail as any).items" :key="i">
          <td v-for="c in cols" :key="c.key" :class="`col-${c.key}`" :style="{ textAlign: c.align }">
            <template v-if="c.key === 'index'">{{ i + 1 }}</template>
            <template v-else-if="c.key === 'qty'">
              {{ formatQty(it.qty) }}
              <!-- 非基本单位时把折算关系写出来：纸面上的「2 箱」入账是 20 个基本单位，
                   不写清楚，对账时没人知道库存为什么不是整数 -->
              <span v-if="Number(it.rate) !== 1" class="qty-sub">×{{ formatQty(it.rate) }} = {{ formatQty(it.qtyBase) }}{{ it.unitBase ? ` ${it.unitBase}` : '' }}</span>
            </template>
            <template v-else>{{ cell(it, c.key) }}</template>
          </td>
        </tr>
        <tr v-if="(detail as any).items.length === 0">
          <td :colspan="colCount" class="sheet__empty">无明细</td>
        </tr>
      </tbody>
    </table>

    <div class="sheet__summary">
      <div class="sheet__summary-left">
        <span v-for="p in companyParts" :key="p">{{ p }}</span>
      </div>
      <div class="sheet__summary-right">
        {{ totalLabel }}：<b>{{ totalValue }}</b>
      </div>
    </div>

    <div v-if="cfg.footer.showAmountCn && amountCN" class="sheet__amount-cn">{{ amountCN }}</div>

    <div v-if="declaration || signLabel" class="sheet__declare">
      <span class="sheet__declare-text">{{ declaration }}</span>
      <span class="sheet__declare-sign">{{ signLabel }}：<i /></span>
    </div>

    <div v-if="cfg.footer.showRemark && (detail as any).remark" class="sheet__remark">
      <b>备注：</b>{{ (detail as any).remark }}
    </div>
    <div v-if="cfg.footer.customText" class="sheet__remark sheet__custom">{{ cfg.footer.customText }}</div>

    <!-- 旧版三联签收栏：默认关闭，模板里打开才出现 -->
    <div v-if="cfg.footer.showSign" class="sheet__sign">
      <span v-for="s in signItems" :key="s">{{ s }}：____________</span>
    </div>

    <div class="sheet__footer">{{ cfg.footer.text }}</div>
  </div>
</template>

<style scoped lang="scss">
.sheet {
  margin: 0 auto;
  background: #fff;
  color: #1a1a1a;
  font-family: 'SimSun', 'Songti SC', serif;
  line-height: 1.6;
  box-shadow: 0 2px 12px rgb(0 0 0 / 18%);
  box-sizing: border-box;

  &__logo {
    display: block;
    max-height: 56px;
    margin: 0 auto 8px;
  }

  // 标题：居中、略加字距（纸质单据的习惯）；末尾的空白用 text-indent 抵掉，保证真正居中
  &__title {
    margin: 4px 0 14px;
    text-align: center;
    font-weight: 700;
    letter-spacing: 0.18em;
    text-indent: 0.18em;
  }

  &__company {
    text-align: center;
    font-size: 1.05em;
    font-weight: 600;
    margin-top: 2px;
  }

  &__company-sub {
    text-align: center;
    font-size: 0.82em;
    color: #555;
  }

  // 信息条：三五个字段横向铺开，与表格左右边界对齐；不画线（纸质单据的写法）
  &__meta {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: baseline;
    gap: 4px 24px;
    margin-bottom: 6px;
    font-size: 1em;

    em {
      font-style: normal;
      font-weight: 400;
    }
  }

  &__table {
    width: 100%;
    border-collapse: collapse;
    font-size: 1em;

    th,
    td {
      border: 1px solid #000;
      padding: 3px 6px;
      vertical-align: middle;
      word-break: break-all;
    }

    // 表头不加粗、不填底衬：细线框的白表头在黑白打印下最干净
    th {
      background: var(--th-bg);
      font-weight: 400;
      text-align: center;
      white-space: nowrap;
    }

    tbody td {
      height: var(--row-h, 30px);
    }
  }

  .col-index {
    width: 38px;
  }

  .col-code {
    width: 92px;
  }

  .col-name {
    min-width: 150px;
  }

  .col-spec {
    min-width: 96px;
  }

  .col-unit {
    width: 64px;
  }

  .col-qty,
  .col-qtybase,
  .col-price,
  .col-money,
  .col-amount,
  .col-rate {
    width: 74px;
    white-space: nowrap;
  }

  .col-note {
    min-width: 84px;
  }

  // 折算行是「数量的解释」而不是另一个数量：降一号字、压灰，
  // 让主数量仍然是一眼能读到的那个数
  .qty-sub {
    display: block;
    font-size: 0.85em;
    color: #666;
  }

  &__empty {
    text-align: center;
    color: #999;
  }

  // 表尾行：左侧公司落款，右侧合计金额，与表格同宽
  &__summary {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
    margin-top: 8px;
    font-size: 1em;

    &-left {
      display: flex;
      flex-wrap: wrap;
      gap: 2px 22px;
      min-width: 0;
    }

    &-right {
      flex: none;
      white-space: nowrap;

      b {
        font-weight: 700;
      }
    }
  }

  &__amount-cn {
    margin-top: 2px;
    text-align: right;
    font-size: 0.92em;
  }

  // 声明行：左一句话，右签字处，中间留白给手写
  &__declare {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 24px;
    margin-top: 14px;
    font-size: 1em;

    &-text {
      min-width: 0;
    }

    &-sign {
      flex: none;
      white-space: nowrap;
      display: inline-flex;
      align-items: baseline;

      i {
        display: inline-block;
        width: 110px;
        border-bottom: 1px solid #666;
      }
    }
  }

  &__remark {
    margin-top: 8px;
    font-size: 0.92em;
  }

  &__sign {
    display: flex;
    justify-content: space-between;
    margin-top: 28px;
    font-size: 0.92em;
  }

  &__footer {
    margin-top: 10px;
    text-align: center;
    font-size: 0.85em;
    color: #666;
    white-space: pre-wrap;

    &:empty {
      display: none;
    }
  }
}
</style>
