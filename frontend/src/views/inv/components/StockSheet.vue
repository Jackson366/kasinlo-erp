<script setup lang="ts">
/**
 * 出入库单打印版式（纯展示组件）。
 *
 * 打印弹窗与模板设计器共用：前者喂真实单据，后者喂示例数据做实时预览。
 *
 * 注意：纸面颜色刻意用固定黑白灰而不是主题令牌——打印纸永远是白底黑字，
 * 不能跟随暗色主题；暗色下预览也是「纸上观感」，正好所见即所得。
 */
import { computed } from 'vue'
import type { InvStockOrderDetail } from '@/types/api'
import { formatDay } from '@/utils/format'
import type { PrintContentConfig } from './printConfig'

const props = defineProps<{
  detail: InvStockOrderDetail
  cfg: PrintContentConfig
  /** 最终展示的标题（已含「配置标题留空时按方向取默认」的逻辑，由父级算好传入） */
  title: string
  paper: 'A4' | 'A5'
}>()

/** 预览宽度按 96dpi 近似：A4 210mm≈794px、A5 148mm≈559px，打印时由 @media print 接管 */
const sheetWidth = computed(() => (props.paper === 'A5' ? '560px' : '794px'))

const directionText = computed(() => (props.detail.direction === 'in' ? '入库' : '出库'))

const sourceLabel = computed(() =>
  props.detail.sourceType === 'purchase' ? '采购单号' : props.detail.sourceType === 'sales' ? '销售单号' : '来源单号',
)

const metaItems = computed(() => {
  const d = props.detail
  const rows: Array<{ label: string; value: string }> = [
    { label: '单据编号', value: d.orderNo },
    { label: '单据日期', value: formatDay(d.orderDate) },
  ]
  if (props.cfg.showSource && d.sourceOrderNo) {
    rows.push({ label: sourceLabel.value, value: d.sourceOrderNo })
  }
  if (props.cfg.showOperator && d.operator) {
    rows.push({ label: '制单人', value: d.operator })
  }
  if (props.cfg.showPrintTime) {
    rows.push({ label: '打印时间', value: new Date().toLocaleString('zh-CN', { hour12: false }) })
  }
  return rows
})
</script>

<template>
  <div
    class="sheet"
    :class="`sheet--${paper}`"
    :style="{ width: sheetWidth, fontSize: `${cfg.fontSize}pt` }"
  >
    <h1 class="sheet__title">{{ title }}</h1>
    <div class="sheet__direction">{{ directionText }}单</div>

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
            <!-- 非基本单位时把折算值写出来：仓管照着「2 箱」点货，
                 但账上是 20 个，两个数都在纸面上才对得上 -->
            <span v-if="it.rate !== 1" class="qty-sub">= {{ it.qtyBase }} {{ it.unitBase }}</span>
          </td>
        </tr>
        <tr v-if="detail.items.length === 0">
          <td :colspan="6" class="sheet__empty">无明细</td>
        </tr>
      </tbody>
      <tfoot>
        <tr>
          <td :colspan="5" class="col-qty-label">合计数量</td>
          <td class="col-qty">{{ detail.totalQty }}</td>
        </tr>
      </tfoot>
    </table>

    <div v-if="detail.remark" class="sheet__remark"><b>备注：</b>{{ detail.remark }}</div>

    <div class="sheet__sign">
      <span>制单：____________</span>
      <span>仓管：____________</span>
      <span>审核：____________</span>
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
    width: 96px;
  }

  .col-unit {
    width: 52px;
    text-align: center;
  }

  .col-qty {
    width: 84px;
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
