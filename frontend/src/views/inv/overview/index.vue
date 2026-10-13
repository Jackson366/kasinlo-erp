<script setup lang="ts">
/**
 * 数据概览（登录落地页）：核心经营指标 + 销售趋势 + 品类占比 + 客户排行。
 */
import { computed, onMounted, ref } from 'vue'
import type { EChartsCoreOption } from 'echarts/core'
import { invApi } from '@/api'
import type { InvOverview, InvReportCategoryRow, InvReportRankRow, InvReportTrendPoint } from '@/types/api'
import EChart from '@/components/EChart.vue'
import { formatMoney } from '@/utils/format'
import { chartPalette } from '@/utils/theme'
import { useAppStore } from '@/store'

defineOptions({ name: 'InvOverview' })

const appStore = useAppStore()
const palette = computed(() => {
  void appStore.theme
  return chartPalette()
})

const loading = ref(true)
const overview = ref<InvOverview | null>(null)
const trend = ref<InvReportTrendPoint[]>([])
const ranking = ref<InvReportRankRow[]>([])
const categories = ref<InvReportCategoryRow[]>([])

const cards = computed(() => {
  const o = overview.value
  return [
    {
      key: 'customer',
      label: '客户总数',
      value: o ? String(o.customerCount) : '-',
      hint: `活跃 ${o?.activeCustomer ?? 0} · 本月新增 ${o?.monthNewCustomer ?? 0}`,
      tone: 'blue',
      icon: 'User',
    },
    {
      key: 'monthSales',
      label: '本月销售额',
      value: formatMoney(o?.monthSales ?? 0),
      hint: `本月采购 ${formatMoney(o?.monthPurchase ?? 0)}`,
      tone: 'green',
      icon: 'TrendCharts',
    },
    {
      key: 'receivable',
      label: '应收总额',
      value: formatMoney(o?.receivableTotal ?? 0),
      hint: `应付 ${formatMoney(o?.payableTotal ?? 0)}`,
      tone: 'amber',
      icon: 'Money',
    },
    {
      key: 'stock',
      label: '库存商品',
      value: o ? String(o.productCount) : '-',
      hint: `预警 ${o?.warnProductCount ?? 0} 项`,
      tone: 'red',
      icon: 'Box',
    },
    {
      key: 'pending',
      label: '待处理单据',
      value: o ? String(o.pendingOrders) : '-',
      hint: '待入库 / 待出库',
      tone: 'violet',
      icon: 'Timer',
    },
  ]
})

const trendOption = computed<EChartsCoreOption>(() => {
  const p = palette.value
  return {
    grid: { left: 8, right: 16, top: 40, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: p.overlay,
      borderColor: p.border,
      textStyle: { color: p.textStrong, fontSize: 12 },
    },
    legend: {
      right: 0,
      top: 0,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: p.textStrong, fontSize: 12 },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: trend.value.map((it) => it.date.slice(5)),
      axisLine: { lineStyle: { color: p.axis } },
      axisTick: { show: false },
      axisLabel: { color: p.text, fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: p.split } },
      axisLabel: { color: p.text, fontSize: 11 },
    },
    series: [
      {
        name: '销售额',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 5,
        data: trend.value.map((it) => it.salesAmount),
        itemStyle: { color: p.primary },
        lineStyle: { width: 2.4, color: p.primary },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: p.areaFrom },
              { offset: 1, color: p.areaTo },
            ],
          },
        },
      },
      {
        name: '采购额',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 5,
        data: trend.value.map((it) => it.purchaseAmount),
        itemStyle: { color: p.success },
        lineStyle: { width: 2.4, color: p.success },
      },
    ],
  }
})

const categoryOption = computed<EChartsCoreOption>(() => {
  const p = palette.value
  const colors = [p.primary, p.success, '#f59e0b', '#7c5cfc', '#ef4444', '#14b8a6']
  return {
    tooltip: {
      trigger: 'item',
      backgroundColor: p.overlay,
      borderColor: p.border,
      textStyle: { color: p.textStrong, fontSize: 12 },
      formatter: '{b}: ¥{c} ({d}%)',
    },
    legend: {
      orient: 'vertical',
      right: 0,
      top: 'center',
      icon: 'circle',
      itemWidth: 8,
      itemHeight: 8,
      textStyle: { color: p.textStrong, fontSize: 12 },
    },
    series: [
      {
        type: 'pie',
        radius: ['52%', '74%'],
        center: ['38%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderColor: p.overlay, borderWidth: 2 },
        label: { show: false },
        data: categories.value.map((it, index) => ({
          name: it.category,
          value: it.amount,
          itemStyle: { color: colors[index % colors.length] },
        })),
      },
    ],
  }
})

async function loadAll(): Promise<void> {
  loading.value = true
  try {
    const [ov, tr, rk, ct] = await Promise.all([
      invApi.overview(),
      invApi.reportTrend({ days: 30 }),
      invApi.reportRanking({ range: 'month', limit: 5 }),
      invApi.reportCategory({ range: 'month' }),
    ])
    overview.value = ov
    trend.value = tr
    ranking.value = rk
    categories.value = ct
  } finally {
    loading.value = false
  }
}

onMounted(loadAll)
</script>

<template>
  <div v-loading="loading" class="overview-page">
    <div class="stat-row">
      <div v-for="card in cards" :key="card.key" class="stat-card" :class="`tone-${card.tone}`">
        <div class="stat-card__body">
          <span class="stat-card__label">{{ card.label }}</span>
          <strong class="stat-card__value">{{ card.value }}</strong>
          <span class="stat-card__hint">{{ card.hint }}</span>
        </div>
      </div>
    </div>

    <div class="chart-row">
      <div class="page-card chart-main">
        <header class="card-head">
          <h3>近 30 天销售与采购趋势</h3>
          <span class="text-muted">单位：元</span>
        </header>
        <EChart :option="trendOption" height="300px" />
      </div>
      <div class="page-card chart-side">
        <header class="card-head">
          <h3>品类销售占比</h3>
          <span class="text-muted">本月</span>
        </header>
        <EChart v-if="categories.length" :option="categoryOption" height="300px" />
        <el-empty v-else description="暂无销售数据" :image-size="80" />
      </div>
    </div>

    <div class="page-card">
      <header class="card-head">
        <h3>本月客户销售排行</h3>
        <span class="text-muted">Top 5</span>
      </header>
      <el-table :data="ranking" size="default" stripe>
        <el-table-column label="排名" width="80" align="center">
          <template #default="{ row }">
            <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="partyName" label="客户" min-width="160" show-overflow-tooltip />
        <el-table-column label="销售额" width="150" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="订单数" width="100" align="right">
          <template #default="{ row }">{{ row.orderCount }}</template>
        </el-table-column>
        <el-table-column label="占比" min-width="180">
          <template #default="{ row }">
            <div class="ratio">
              <el-progress :percentage="row.ratio" :stroke-width="8" :show-text="false" />
              <span>{{ row.ratio }}%</span>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!ranking.length" description="暂无排行数据" :image-size="80" />
    </div>
  </div>
</template>

<style scoped lang="scss">
.overview-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.stat-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.stat-card {
  padding: 18px 20px;
  background: var(--app-surface);
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-card);
  transition: var(--transition-base);

  &:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow-hover);
  }

  &__body {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  &__label {
    font-size: 12.5px;
    color: var(--app-text-secondary);
  }

  &__value {
    font-size: 22px;
    font-weight: 700;
    color: var(--app-text);
  }

  &__hint {
    font-size: 12px;
    color: var(--app-text-muted);
  }

  &.tone-blue &__value {
    color: var(--tone-blue);
  }

  &.tone-green &__value {
    color: var(--tone-green);
  }

  &.tone-amber &__value {
    color: var(--tone-amber);
  }

  &.tone-red &__value {
    color: var(--tone-red);
  }

  &.tone-violet &__value {
    color: var(--tone-violet);
  }
}

.chart-row {
  display: grid;
  grid-template-columns: 1.9fr 1fr;
  gap: 16px;

  @media (max-width: 1100px) {
    grid-template-columns: 1fr;
  }
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;

  h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--app-text);
  }
}

.rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  font-size: 12px;
  color: var(--app-text-secondary);
  background: var(--tone-slate-soft);

  &--top {
    color: #fff;
    background: var(--brand-gradient);
  }
}

.ratio {
  display: flex;
  align-items: center;
  gap: 10px;

  :deep(.el-progress) {
    flex: 1;
  }

  span {
    width: 48px;
    text-align: right;
    font-size: 12px;
    color: var(--app-text-secondary);
  }
}
</style>
