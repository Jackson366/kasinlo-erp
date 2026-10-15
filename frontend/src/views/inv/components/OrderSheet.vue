<script setup lang="ts">
/**
 * 采购 / 销售单打印版式（纯展示组件）。
 *
 * 与 StockSheet 同一套设计语言：固定黑白灰纸面（不跟主题）、宋体，
 * 表头信息条 + 明细表 + 备注 + 签字栏 + 页脚。差异是带单价 / 金额列，
 * 且表头多一行往来单位（供应商 / 客户）。
 *
 * 版式完全由模板配置驱动：明细列（显隐 / 标题 / 对齐）、公司信息与 Logo、
 * 页边距与行高字号、底部区域，都在 cfg 里。**这里不写死任何列** ——
 * 写死的话设计器改了配置纸面也不变，用户会以为配置没生效。
 *
 * 打印弹窗与模板设计器共用：前者喂真实单据，后者喂示例数据做实时预览。
 */
import { computed } from 'vue'
import type { InvOrderDetail } from '@/types/api'
import { formatDay } from '@/utils/format'
import { amountToCN, visibleCols, type PrintContentConfig } from './printConfig'

const props = defineProps<{
  detail: InvOrderDetail
  cfg: PrintContentConfig
  /** 最终展示的标题（配置标题留空时由父级按业务线取「采购单 / 销售单」） */
  title: string
  paper: 'A4' | 'A5'
  /** 往来单位标签：供应商 / 客户 */
  partyLabel: string
}>()

/** 预览宽度按 96dpi 近似：A4 210mm≈794px、A5 148mm≈559px，打印时由 @media print 接管 */
const sheetWidth = computed(() => (props.paper === 'A5' ? '560px' : '794px'))

const metaItems = computed(() => {
  const d = props.detail
  const rows: Array<{ label: string; value: string }> = [
    { label: '单据编号', value: d.orderNo },
    { label: '单据日期', value: formatDay(d.orderDate) },
    { label: props.partyLabel, value: d.partyName || '-' },
  ]
  // 到期日是**内部结算**字段（用来判定应收应付是否逾期），不进纸面。
  // 它既不是模板设计器里的一项，对供应商 / 客户也没有意义：
  // 双方看的都是单据日期与账期约定，没有哪个采购单会在纸面上写「到期日」。
  // 此前这里按「有值就打印」输出，于是未填写的单据会因为零值时间
  // 被打出一行「到期日：0001-01-01」——模板里没有、纸面上却有，正是这个原因。
  if (d.operator) {
    rows.push({ label: '制单人', value: d.operator })
  }
  return rows
})

const cols = computed(() => visibleCols(props.cfg))
/** 合计行的 colspan = 可见列数 - 1（金额列自己占一格） */
const colspan = computed(() => Math.max(cols.value.length - 1, 1))

const amountCN = computed(() => amountToCN(props.detail.totalAmount))

/** 单元格取值。名称列与数量列永远渲染，其余列按配置 */
function cell(it: InvOrderDetail['items'][number], key: string): string {
  switch (key) {
    case 'code':
      return it.productCode || '-'
    case 'name':
      return it.productName
    case 'spec':
      return it.spec || '-'
    case 'unit':
      return it.unit || '-'
    case 'qty':
      return String(it.qty)
    case 'price':
      return it.price.toFixed(2)
    case 'amount':
      return it.amount.toFixed(2)
    default:
      return '-'
  }
}
</script>

<template>
  <div
    class="sheet"
    :class="`sheet--${paper}`"
    :style="{
      width: sheetWidth,
      fontSize: `${cfg.page.fontSize}pt`,
      padding: `${cfg.page.margin}px`,
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
          <th class="col-index">#</th>
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
        <tr v-for="(it, i) in detail.items" :key="i">
          <td class="col-index">{{ i + 1 }}</td>
          <td v-for="c in cols" :key="c.key" :class="`col-${c.key}`" :style="{ textAlign: c.align }">
            <template v-if="c.key === 'qty'">
              {{ it.qty }}
              <!-- 非基本单位时把换算关系写出来：纸面上的「6 米」入账是 1.0002 个基本单位，
                   不写清楚，对账时没人知道库存为什么不是整数 -->
              <span v-if="it.rate !== 1" class="qty-sub">×{{ it.rate }} = {{ it.qtyBase }}</span>
            </template>
            <template v-else>{{ cell(it, c.key) }}</template>
          </td>
        </tr>
        <tr v-if="detail.items.length === 0">
          <td :colspan="colspan + 1" class="sheet__empty">无明细</td>
        </tr>
      </tbody>
      <tfoot>
        <tr>
          <td :colspan="colspan" class="col-qty-label">合计金额（元）</td>
          <td class="col-amount col-money">{{ detail.totalAmount.toFixed(2) }}</td>
        </tr>
      </tfoot>
    </table>

    <div v-if="cfg.footer.showAmountCn && amountCN" class="sheet__amount-cn">{{ amountCN }}</div>
    <div v-if="cfg.footer.showRemark && detail.remark" class="sheet__remark">
      <b>备注：</b>{{ detail.remark }}
    </div>
    <div v-if="cfg.footer.customText" class="sheet__remark sheet__custom">{{ cfg.footer.customText }}</div>

    <div v-if="cfg.footer.showSign" class="sheet__sign">
      <span>制单：____________</span>
      <span>审核：____________</span>
      <span>批准：____________</span>
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

  .col-unit {
    width: 48px;
    text-align: center;
  }

  .col-qty {
    width: 72px;
    white-space: nowrap;
  }

  // 折算行是「数量的解释」而不是另一个数量：降一号字、压灰，
  // 让主数量仍然是一眼能读到的那个数
  .qty-sub {
    display: block;
    font-size: 0.85em;
    color: #666;
  }

  .col-money {
    width: 88px;
    white-space: nowrap;
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
