<script setup lang="ts">
/**
 * 报表打印弹窗（自包含）。
 *
 * 报表不走打印模板——报表是「当前页面数据的快照」，版式固定为 A4 纸面：
 * 经营汇总 + 销售排行 + 品类占比三段。数据由父级（报表中心页）直接传入，
 * 所见即所选时间范围的数据，不重复请求。
 */
import { ref } from 'vue'
import type { InvReportCategoryRow, InvReportRankRow, InvReportSummary } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { formatMoney } from '@/utils/format'

defineOptions({ name: 'ReportPrintDialog' })

const props = defineProps<{
  summary: InvReportSummary | null
  ranking: InvReportRankRow[]
  categories: InvReportCategoryRow[]
}>()

const visible = ref(false)

function open(): void {
  visible.value = true
}

const summaryRows = () => {
  const s = props.summary
  return [
    { label: '营业收入（元）', value: formatMoney(s?.salesAmount ?? 0) },
    { label: '销售成本（元）', value: formatMoney(s?.costAmount ?? 0) },
    { label: '毛利（元）', value: formatMoney(s?.grossProfit ?? 0) },
    { label: '毛利率', value: `${s?.grossRate ?? 0}%` },
    { label: '销售单数（张）', value: String(s?.salesOrders ?? 0) },
    { label: '采购额（元）', value: formatMoney(s?.purchases ?? 0) },
  ]
}

function doPrint(): void {
  document.body.classList.add('printing')
  const done = () => document.body.classList.remove('printing')
  window.addEventListener('afterprint', done, { once: true })
  window.print()
  // 部分浏览器（尤其 Firefox 旧版）不触发 afterprint，兜底摘掉类
  setTimeout(done, 2000)
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    title="打印报表"
    width="1280px"
    top="4vh"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div class="print-body">
      <div class="print-body__toolbar">
        <span class="print-body__label">A4 纵向 · 黑白版式</span>
        <el-button type="primary" @click="doPrint">
          <AppIcon name="Printer" />
          <span>打印</span>
        </el-button>
      </div>

      <div class="print-body__preview">
        <div class="sheet sheet--a4">
          <h1 class="sheet__title">经营报表</h1>
          <div class="sheet__meta">
            <span class="sheet__meta-item"><b>统计范围：</b>{{ summary?.range || '-' }}</span>
            <span class="sheet__meta-item">
              <b>打印时间：</b>{{ new Date().toLocaleString('zh-CN', { hour12: false }) }}
            </span>
          </div>

          <table class="sheet__table">
            <thead>
              <tr>
                <th class="col-label">指标</th>
                <th class="col-value">数值</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in summaryRows()" :key="r.label">
                <td>{{ r.label }}</td>
                <td class="col-value">{{ r.value }}</td>
              </tr>
            </tbody>
          </table>

          <h2 class="sheet__section">销售排行</h2>
          <table class="sheet__table">
            <thead>
              <tr>
                <th class="col-index">排名</th>
                <th>客户</th>
                <th class="col-value">销售额（元）</th>
                <th class="col-index">订单数</th>
                <th class="col-value">占比</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in ranking" :key="r.rank">
                <td class="col-index">{{ r.rank }}</td>
                <td>{{ r.name }}</td>
                <td class="col-value">{{ formatMoney(r.amount) }}</td>
                <td class="col-index">{{ r.orderCount }}</td>
                <td class="col-value">{{ r.ratio }}%</td>
              </tr>
              <tr v-if="ranking.length === 0">
                <td colspan="5" class="sheet__empty">暂无数据</td>
              </tr>
            </tbody>
          </table>

          <h2 class="sheet__section">品类销售占比</h2>
          <table class="sheet__table">
            <thead>
              <tr>
                <th>品类</th>
                <th class="col-value">销售额（元）</th>
                <th class="col-value">占比</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in categories" :key="r.category">
                <td>{{ r.category || '未分类' }}</td>
                <td class="col-value">{{ formatMoney(r.amount) }}</td>
                <td class="col-value">{{ r.ratio }}%</td>
              </tr>
              <tr v-if="categories.length === 0">
                <td colspan="3" class="sheet__empty">暂无数据</td>
              </tr>
            </tbody>
          </table>

          <div class="sheet__sign">
            <span>制表：____________</span>
            <span>审核：____________</span>
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped lang="scss">
.print-body {
  min-height: 320px;

  &__toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
  }

  &__label {
    color: var(--app-text-muted);
  }

  &__preview {
    max-height: 72vh;
    overflow: auto;
    padding: 4px;
  }
}

/* 纸面：固定黑白灰，不跟主题（与出入库单纸面同一套设计语言） */
.sheet {
  width: 794px;
  margin: 0 auto;
  padding: 24px 28px;
  background: #fff;
  color: #1a1a1a;
  font-family: 'SimSun', 'Songti SC', serif;
  line-height: 1.6;
  box-shadow: 0 2px 12px rgb(0 0 0 / 18%);
  box-sizing: border-box;

  &__title {
    margin: 0 0 14px;
    font-size: 1.6em;
    text-align: center;
    letter-spacing: 0.35em;
    text-indent: 0.35em;
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

  &__section {
    margin: 18px 0 0;
    font-size: 1.05em;
    font-weight: 600;
  }

  &__table {
    width: 100%;
    margin-top: 8px;
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
  }

  .col-index {
    width: 56px;
    text-align: center;
  }

  .col-label {
    width: 40%;
  }

  .col-value {
    text-align: right;
    white-space: nowrap;
  }

  &__empty {
    text-align: center;
    color: #999;
  }

  &__sign {
    display: flex;
    justify-content: space-between;
    margin-top: 34px;
    font-size: 0.92em;
  }
}
</style>

<style lang="scss">
/* 打印态：纸面以外的整页内容全部隐掉（与单据打印共用同一套全局规则） */
@media print {
  body.printing .print-body__preview {
    max-height: none;
    overflow: visible;
    padding: 0;
  }

  body.printing .sheet {
    position: absolute;
    left: 0;
    top: 0;
    width: auto !important;
    box-shadow: none;
  }
}
</style>
