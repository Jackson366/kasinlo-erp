<script setup lang="ts">
/**
 * 采购 / 销售单打印版式（纯展示组件）。
 *
 * 与 StockSheet 同一套设计语言：固定黑白灰纸面（不跟主题）、宋体、
 * 表头信息条 + 明细表 + 备注 + 签字栏 + 页脚。差异是带单价 / 金额列，
 * 且表头多一行往来单位（供应商 / 客户）。
 *
 * 打印弹窗与模板设计器共用：前者喂真实单据，后者喂示例数据做实时预览。
 */
import { computed } from 'vue'
import type { InvOrderDetail } from '@/types/api'
import { formatDay } from '@/utils/format'
import type { PrintContentConfig } from './printConfig'

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
  if (props.cfg.showOperator && d.operator) {
    rows.push({ label: '制单人', value: d.operator })
  }
  if (props.cfg.showPrintTime) {
    rows.push({ label: '打印时间', value: new Date().toLocaleString('zh-CN', { hour12: false }) })
  }
  return rows
})

/** 明细表可见列数（不含 # 列），用于合计行的 colspan */
const colspan = computed(() => {
  let n = 3 // 商品名称 + 数量 + 合计值所在列
  if (props.cfg.showCode) n++
  if (props.cfg.showSpec) n++
  if (props.cfg.showUnit) n++
  if (props.cfg.showPrice) n++
  if (props.cfg.showAmount) n++
  return n - 1
})
</script>

<template>
  <div
    class="sheet"
    :class="`sheet--${paper}`"
    :style="{ width: sheetWidth, fontSize: `${cfg.fontSize}pt` }"
  >
    <h1 class="sheet__title">{{ title }}</h1>
    <div class="sheet__direction">{{ partyLabel }}：{{ detail.partyName || '-' }}</div>

    <div class="sheet__meta">
      <span v-for="m in metaItems" :key="m.label" class="sheet__meta-item">
        <b>{{ m.label }}：</b>{{ m.value }}
      </span>
    </div>

    <table class="sheet__table">
      <thead>
        <tr>
          <th class="col-index">#</th>
          <th v-if="cfg.showCode" class="col-code">商品编码</th>
          <th>商品名称</th>
          <th v-if="cfg.showSpec">规格</th>
          <th v-if="cfg.showUnit" class="col-unit">单位</th>
          <th class="col-qty">数量</th>
          <th v-if="cfg.showPrice" class="col-money">单价</th>
          <th v-if="cfg.showAmount" class="col-money">金额</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(it, i) in detail.items" :key="i">
          <td class="col-index">{{ i + 1 }}</td>
          <td v-if="cfg.showCode" class="col-code">{{ it.productCode || '-' }}</td>
          <td>{{ it.productName }}</td>
          <td v-if="cfg.showSpec">{{ it.spec || '-' }}</td>
          <td v-if="cfg.showUnit" class="col-unit">{{ it.unit || '-' }}</td>
          <td class="col-qty">
            {{ it.qty }}
            <!-- 非基本单位时把换算关系写出来：纸面上的「6 米」入账是 1.0002 个基本单位，
                 不写清楚，对账时没人知道库存为什么不是整数 -->
            <span v-if="it.rate !== 1" class="qty-sub">×{{ it.rate }} = {{ it.qtyBase }}</span>
          </td>
          <td v-if="cfg.showPrice" class="col-money">{{ it.price.toFixed(2) }}</td>
          <td v-if="cfg.showAmount" class="col-money">{{ it.amount.toFixed(2) }}</td>
        </tr>
        <tr v-if="detail.items.length === 0">
          <td :colspan="colspan + 1" class="sheet__empty">无明细</td>
        </tr>
      </tbody>
      <tfoot>
        <tr>
          <td :colspan="colspan" class="col-qty-label">合计金额（元）</td>
          <td class="col-money">{{ detail.totalAmount.toFixed(2) }}</td>
        </tr>
      </tfoot>
    </table>

    <div v-if="detail.remark" class="sheet__remark"><b>备注：</b>{{ detail.remark }}</div>

    <div class="sheet__sign">
      <span>制单：____________</span>
      <span>审核：____________</span>
      <span>批准：____________</span>
    </div>

    <div class="sheet__footer">{{ cfg.footer }}</div>
  </div>
</template>

<style scoped lang="scss">
.sheet {
  margin: 0 auto;
  padding: 24px 28px;
  background: #fff;
  color: #1a1a1a;
  font-family: 'SimSun', 'Songti SC', serif;
  line-height: 1.6;
  box-shadow: 0 2px 12px rgb(0 0 0 / 18%);
  box-sizing: border-box;

  &__title {
    margin: 0;
    font-size: 1.6em;
    text-align: center;
    letter-spacing: 0.35em;
    text-indent: 0.35em; // 抵消最后一个字的字距，让标题真正居中
  }

  &__direction {
    text-align: center;
    font-size: 0.9em;
    color: #666;
    margin-bottom: 14px;
  }

  &__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 22px;
    border-top: 2px solid #1a1a1a;
    border-bottom: 1px solid #999;
    padding: 8px 2px;
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
      background: #f2f2f2;
      font-weight: 600;
      white-space: nowrap;
    }

    tfoot td {
      border-top: 2px solid #1a1a1a;
      font-weight: 600;
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
    text-align: right;
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
    text-align: right;
    white-space: nowrap;
  }

  .col-qty-label {
    text-align: right;
  }

  &__empty {
    text-align: center;
    color: #999;
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
