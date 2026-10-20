<script setup lang="ts">
/**
 * 单据打印纸面（纯展示组件，采购销售 / 出入库共用）。
 *
 * 与 ReportPrintDialog 的报表版式同一套设计语言：固定黑白灰纸面（不跟主题）、
 * 宋体，纸头区 + 信息条 + 明细表 + 底部区域。差异由 mode 决定：
 * order 带单价金额与合计金额，stock 合计数量且无金额列。
 *
 * 版式完全由模板配置驱动：明细列（显隐 / 标题 / 对齐）、信息条（显隐 / 改名 /
 * 排序）、公司信息与 Logo、页边距与行高字号、签收栏与底部区域——
 * 组件本身不写死任何一列、任何一个信息条字段。
 *
 * 页边距：配置是毫米，预览按 96dpi 换算模拟，打印时注入 @page（applyPrintPage），
 * 两边数值一致——所见即所得。打印方向由 @page size 钉死为竖版。
 *
 * 打印弹窗与模板设计器共用：前者喂真实单据，后者喂示例数据做实时预览。
 */
import { computed, onMounted, watch } from 'vue'
import type { InvOrderDetail, InvStockOrderDetail } from '@/types/api'
import { formatDay } from '@/utils/format'
import {
  amountToCN,
  applyPrintPage,
  defaultMetaFields,
  defaultSignItems,
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
      orderNo: { label: '单据编号', value: o.orderNo },
      orderDate: { label: '单据日期', value: formatDay(o.orderDate) },
      party: { label: props.partyLabel || '往来单位', value: o.partyName || '-' },
      salesman: { label: '业务员', value: o.salesman || '', skipIfEmpty: true },
      operator: { label: '制单人', value: o.operator || '', skipIfEmpty: true },
    }
    return pool
  }
  const s = d as InvStockOrderDetail
  const pool: Record<string, MetaValue> = {
    orderNo: { label: '单据编号', value: s.orderNo },
    orderDate: { label: '单据日期', value: formatDay(s.orderDate) },
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

const cols = computed(() => visibleCols(props.cfg))
/** 合计行的 colspan = 可见列数 - 1（值自己占一格） */
const colspan = computed(() => Math.max(cols.value.length - 1, 1))

const amountCN = computed(() => (props.mode === 'order' ? amountToCN((props.detail as InvOrderDetail).totalAmount) : ''))

type AnyItem = Record<string, unknown>

/** 单元格取值。index / qty 两列有专属渲染，其余列按配置 */
function cell(it: AnyItem, key: string): string {
  switch (key) {
    case 'code':
      return String(it.productCode || '-')
    case 'name':
      return String(it.productName ?? '-')
    case 'spec':
      return String(it.spec || '-')
    case 'unit':
      return String(it.unit || '-')
    case 'qtyBase':
      return String(it.qtyBase ?? '-')
    case 'rate':
      return String(it.rate ?? '-')
    case 'price':
      return Number(it.price ?? 0).toFixed(2)
    case 'amount':
      return Number(it.amount ?? 0).toFixed(2)
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
    <img v-if="cfg.header.logoUrl" class="sheet__logo" :src="cfg.header.logoUrl" alt="logo" />
    <h1 v-if="cfg.header.showTitle" class="sheet__title" :style="{ fontSize: `${cfg.page.titleFontSize}pt` }">
      {{ title }}
    </h1>
    <div v-if="cfg.header.companyName" class="sheet__company">{{ cfg.header.companyName }}</div>
    <div v-if="cfg.header.address || cfg.header.phone" class="sheet__company-sub">
      {{ cfg.header.address }}{{ cfg.header.address && cfg.header.phone ? ' · ' : '' }}{{ cfg.header.phone }}
    </div>
    <div v-if="cfg.header.bank" class="sheet__company-sub">{{ cfg.header.bank }}</div>

    <div class="sheet__meta">
      <span v-for="m in metaItems" :key="m.label" class="sheet__meta-item">
        <b>{{ m.label }}：</b>{{ m.value }}
      </span>
    </div>

    <table class="sheet__table">
      <thead>
        <tr>
          <th
            v-for="c in cols"
            :key="c.key"
            :class="`col-${c.key}`"
            :style="{ textAlign: c.align }"
          >
            {{ c.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(it, i) in (detail as any).items" :key="i">
          <td v-for="c in cols" :key="c.key" :class="`col-${c.key}`" :style="{ textAlign: c.align }">
            <template v-if="c.key === 'index'">{{ i + 1 }}</template>
            <template v-else-if="c.key === 'qty'">
              {{ it.qty }}
              <!-- 非基本单位时把折算关系写出来：纸面上的「2 箱」入账是 20 个基本单位，
                   不写清楚，对账时没人知道库存为什么不是整数 -->
              <span v-if="Number(it.rate) !== 1" class="qty-sub">×{{ it.rate }} = {{ it.qtyBase }}{{ it.unitBase ? ` ${it.unitBase}` : '' }}</span>
            </template>
            <template v-else>{{ cell(it, c.key) }}</template>
          </td>
        </tr>
        <tr v-if="(detail as any).items.length === 0">
          <td :colspan="colspan" class="sheet__empty">无明细</td>
        </tr>
      </tbody>
      <tfoot>
        <tr v-if="mode === 'order'">
          <td :colspan="colspan" class="col-qty-label">合计金额（元）</td>
          <td class="col-money">{{ (detail as InvOrderDetail).totalAmount.toFixed(2) }}</td>
        </tr>
        <tr v-else>
          <td :colspan="colspan" class="col-qty-label">合计数量</td>
          <td class="col-qty">{{ (detail as InvStockOrderDetail).totalQty }}</td>
        </tr>
      </tfoot>
    </table>

    <div v-if="cfg.footer.showAmountCn && amountCN" class="sheet__amount-cn">{{ amountCN }}</div>
    <div v-if="cfg.footer.showRemark && (detail as any).remark" class="sheet__remark">
      <b>备注：</b>{{ (detail as any).remark }}
    </div>
    <div v-if="cfg.footer.customText" class="sheet__remark sheet__custom">{{ cfg.footer.customText }}</div>

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

  &__title {
    margin: 0;
    text-align: center;
    letter-spacing: 0.35em;
    text-indent: 0.35em; // 抵消最后一个字的字距，让标题真正居中
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

  &__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 22px;
    border-top: 2px solid #1a1a1a;
    border-bottom: 1px solid #999;
    padding: 8px 2px;
    margin-top: 10px;
    font-size: 0.92em;

    b {
      font-weight: 600;
    }
  }

  &__table {
    width: 100%;
    margin-top: 10px;
    border-collapse: collapse;
    font-size: 0.95em;

    th,
    td {
      border: 1px solid #444;
      padding: 5px 8px;
      text-align: left;
      word-break: break-all;
    }

    th {
      background: var(--th-bg);
      font-weight: 600;
      white-space: nowrap;
    }

    // 行高可调：紧凑适合条目多的单据，宽松适合手写补录
    tbody td {
      height: var(--row-h, 30px);
    }

    tfoot td {
      border-top: 2px solid #1a1a1a;
      font-weight: 600;
      height: auto;
    }
  }

  .col-index {
    width: 34px;
    text-align: center;
  }

  .col-code {
    width: 88px;
  }

  .col-unit,
  .col-rate {
    width: 48px;
    text-align: center;
  }

  .col-qty,
  .col-qtybase,
  .col-money {
    width: 80px;
    white-space: nowrap;
  }

  // 折算行是「数量的解释」而不是另一个数量：降一号字、压灰，
  // 让主数量仍然是一眼能读到的那个数
  .qty-sub {
    display: block;
    font-size: 0.85em;
    color: #666;
  }

  .col-qty-label {
    text-align: right;
  }

  &__empty {
    text-align: center;
    color: #999;
  }

  &__amount-cn {
    margin-top: 8px;
    text-align: right;
    font-size: 0.92em;
    font-weight: 600;
  }

  &__remark {
    margin-top: 10px;
    font-size: 0.92em;
  }

  &__sign {
    display: flex;
    justify-content: space-between;
    margin-top: 34px;
    font-size: 0.92em;
  }

  &__footer {
    margin-top: 14px;
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
