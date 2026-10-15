<script setup lang="ts">
/**
 * 报表中心：按时间范围汇总经营指标，并提供趋势、品类占比与销售排行。
 */
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { EChartsCoreOption } from 'echarts/core'
import { invApi } from '@/api'
import type { InvReportCategoryRow, InvReportRankRow, InvReportSummary, InvReportTrendPoint } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import EChart from '@/components/EChart.vue'
import ReportPrintDialog from '../components/ReportPrintDialog.vue'
import { formatMoney } from '@/utils/format'
import { chartPalette } from '@/utils/theme'
import { useAppStore } from '@/store'

defineOptions({ name: 'InvReport' })

const appStore = useAppStore()
const palette = computed(() => {
  void appStore.theme
  return chartPalette()
})

const printRef = ref<InstanceType<typeof ReportPrintDialog> | null>(null)

const ranges = [
  { label: '今日', value: 'today' },
  { label: '本周', value: 'week' },
  { label: '本月', value: 'month' },
  { label: '本季', value: 'quarter' },
  { label: '本年', value: 'year' },
]
const activeRange = ref('month')

/**
 * 趋势图的天数。自定义区间时由区间跨度推导（见 filterParams），
 * 用预设范围时固定 30 天 —— 趋势图本身不随 range 变，这是刻意的：
 * 「本月」的汇总配 30 天趋势才看得出走势，配 1 天的趋势没有意义。
 */
const trendDays = ref(30)

const loading = ref(true)
const summary = ref<InvReportSummary | null>(null)
const trend = ref<InvReportTrendPoint[]>([])
const ranking = ref<InvReportRankRow[]>([])
const categories = ref<InvReportCategoryRow[]>([])

const kpis = computed(() => {
  const s = summary.value
  return [
    { label: '营业收入', value: formatMoney(s?.salesAmount ?? 0), tone: 'blue' },
    { label: '销售成本', value: formatMoney(s?.costAmount ?? 0), tone: 'amber' },
    { label: '毛利', value: formatMoney(s?.grossProfit ?? 0), tone: 'green' },
    { label: '毛利率', value: `${s?.grossRate ?? 0}%`, tone: 'violet' },
  ]
})

async function load(): Promise<void> {
  loading.value = true
  try {
    const p = filterParams()
    const [sm, tr, rk, ct] = await Promise.all([
      invApi.reportSummary(p),
      invApi.reportTrend({ ...p, days: trendDays.value }),
      invApi.reportRanking({ ...p, limit: 5 }),
      invApi.reportCategory(p),
    ])
    summary.value = sm
    trend.value = tr
    ranking.value = rk
    categories.value = ct
  } finally {
    loading.value = false
  }
}

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

function changeRange(value: string): void {
  activeRange.value = value
  // 切预设时间就清掉自定义区间：两个都留着会让用户分不清当前生效的是哪个
  customFrom.value = ''
  customTo.value = ''
  load()
}

/** 当前生效的查询参数。自定义 from/to 优先于 range 预设 */
function filterParams(): Record<string, string | number> {
  const p: Record<string, string | number> = {}
  if (customFrom.value) {
    p.from = customFrom.value
    if (customTo.value) p.to = customTo.value
    // 趋势天数跟着自定义区间走，不然图还是 30 天、与汇总对不上
    trendDays.value = customTo.value ? daysBetween(customFrom.value, customTo.value) : 30
  } else {
    p.range = activeRange.value
    if (customTo.value) p.to = customTo.value
    trendDays.value = 30
  }
  if (activeCat.value) p.categoryId = activeCat.value
  return p
}

/** 两个日期的间隔天数，夹在 1~90（后端上限） */
function daysBetween(from: string, to: string): number {
  const a = new Date(from).getTime()
  const b = new Date(to).getTime()
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return 30
  return Math.min(Math.max(Math.round((b - a) / 86400000) + 1, 1), 90)
}

/** 品类筛选：命中自身 + 全部子孙分类，所以下拉用树 */
const catOptions = ref<Array<{ id: number; name: string }>>([])
const activeCat = ref(0)

async function loadCatOptions(): Promise<void> {
  const list = await invApi.categoryOptions()
  catOptions.value = list.map((c) => ({ id: c.id, name: c.name }))
}

/** 自定义区间：起始 / 结束日期 */
const customFrom = ref('')
const customTo = ref('')

function applyCustomRange(): void {
  if (!customFrom.value && !customTo.value) {
    ElMessage.warning('请先选择起始或结束日期')
    return
  }
  activeRange.value = ''
  load()
}

/** 当前生效范围的文字描述，显示在 KPI 旁边，避免用户不知道自己在看哪段时间 */
const rangeLabel = computed(() => {
  if (customFrom.value || customTo.value) {
    return `${customFrom.value || '不限'} ~ ${customTo.value || '不限'}`
  }
  return ranges.find((r) => r.value === activeRange.value)?.label ?? ''
})

const catLabel = computed(() => {
  if (!activeCat.value) return ''
  return catOptions.value.find((c) => c.id === activeCat.value)?.name ?? ''
})

onMounted(() => {
  load()
  loadCatOptions()
})
</script>

<template>
  <div v-loading="loading" class="report-page">
    <div class="page-card range-bar">
      <div class="range-tabs">
        <button
          v-for="item in ranges"
          :key="item.value"
          class="range-tab"
          :class="activeRange === item.value ? 'is-active' : ''"
          @click="changeRange(item.value)"
        >
          {{ item.label }}
        </button>
      </div>

      <!-- 自定义时间与品类筛选。from/to 优先于上面的预设 tab，
           所以选了自定义之后 tab 会全部取消高亮 —— 当前生效的是哪个一眼看得出 -->
      <div class="range-extra">
        <el-date-picker
          v-model="customFrom"
          type="date"
          value-format="YYYY-MM-DD"
          placeholder="起始日期"
          style="width: 150px"
        />
        <span class="range-extra__sep">至</span>
        <el-date-picker
          v-model="customTo"
          type="date"
          value-format="YYYY-MM-DD"
          placeholder="结束日期"
          style="width: 150px"
        />
        <el-button type="primary" @click="applyCustomRange">查询</el-button>
        <el-button v-if="customFrom || customTo" @click="changeRange('month')">重置</el-button>
        <el-select v-model="activeCat" placeholder="全部品类" clearable style="width: 180px" @change="load">
          <el-option v-for="c in catOptions" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
      </div>
      <div class="range-hint">
        当前范围：<b>{{ rangeLabel }}</b>
        <template v-if="catLabel"> · 品类：<b>{{ catLabel }}</b>（含子分类）</template>
      </div>
      <div class="range-bar__actions">
        <el-button @click="printRef?.open()">
          <AppIcon name="Printer" />
          <span>打印</span>
        </el-button>
        <el-button @click="load">
          <AppIcon name="Refresh" />
          <span>刷新</span>
        </el-button>
      </div>
    </div>

    <div class="kpi-row">
      <div v-for="kpi in kpis" :key="kpi.label" class="kpi-card" :class="`tone-${kpi.tone}`">
        <span class="kpi-card__label">{{ kpi.label }}</span>
        <strong class="kpi-card__value">{{ kpi.value }}</strong>
      </div>
    </div>

    <div class="chart-row">
      <div class="page-card chart-main">
        <header class="card-head">
          <h3>销售与采购趋势</h3>
          <span class="text-muted">近 30 天 · 单位：元</span>
        </header>
        <EChart :option="trendOption" height="300px" />
      </div>
      <div class="page-card chart-side">
        <header class="card-head">
          <h3>品类销售占比</h3>
          <span class="text-muted">{{ summary?.range }}</span>
        </header>
        <EChart v-if="categories.length" :option="categoryOption" height="300px" />
        <el-empty v-else description="暂无销售数据" :image-size="80" />
      </div>
    </div>

    <div class="page-card">
      <header class="card-head">
        <h3>销售排行</h3>
        <span class="text-muted">{{ summary?.range }} · 共 {{ summary?.salesOrders ?? 0 }} 张销售单</span>
      </header>
      <el-table :data="ranking" stripe>
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

    <!-- 打印当前报表（汇总 + 排行 + 品类） -->
    <ReportPrintDialog ref="printRef" :summary="summary" :ranking="ranking" :categories="categories" />
  </div>
</template>

<style scoped lang="scss">
.report-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.range-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

// 自定义时间 + 品类筛选：与预设 tab 同一行，但允许换行（窄屏不挤成一团）
.range-extra {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;

  &__sep {
    color: var(--app-text-muted);
    font-size: 13px;
  }
}

// 当前生效范围：自定义区间与预设 tab 并存时必须说清「现在看的是哪段」，
// 否则用户改了日期却以为还在看本月
.range-hint {
  width: 100%;
  font-size: 12px;
  color: var(--app-text-muted);
}

.range-bar__actions {
  display: flex;
  align-items: center;

  .el-button + .el-button {
    margin-left: 0;
  }
}

.range-tabs {
  display: inline-flex;
  gap: 4px;
  padding: 3px;
  background: var(--app-surface-subtle);
  border-radius: var(--radius-sm);
}

.range-tab {
  padding: 6px 16px;
  border: none;
  border-radius: calc(var(--radius-sm) - 2px);
  background: transparent;
  font-size: 13px;
  color: var(--app-text-secondary);
  cursor: pointer;
  transition: var(--transition-base);

  &:hover {
    color: var(--app-text);
  }

  &.is-active {
    color: #fff;
    background: var(--brand-gradient);
  }
}

.kpi-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.kpi-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 18px 20px;
  background: var(--app-surface);
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-card);

  &__label {
    font-size: 12.5px;
    color: var(--app-text-secondary);
  }

  &__value {
    font-size: 22px;
    font-weight: 700;
    color: var(--app-text);
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
