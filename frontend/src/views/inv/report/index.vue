<script setup lang="ts">
/**
 * 报表中心：按「经营总览 / 销售排行 / 采购分析 / 库存分析」四个报表区组织，
 * 顶部共享时间与品类筛选栏。
 *
 * 各报表区数据懒加载、切 tab 时按需请求；共享主数据（库存）不随筛选变化，
 * 其余报表跟着筛选栏走。排行支持「客户 / 商品」两个维度。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { EChartsCoreOption } from 'echarts/core'
import { invApi } from '@/api'
import type {
  InvReportCategoryRow,
  InvReportInventory,
  InvReportRankRow,
  InvReportSummary,
  InvReportTrendPoint,
} from '@/types/api'
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

type TabKey = 'overview' | 'ranking' | 'purchase' | 'inventory'
type RankDimension = 'customer' | 'product'

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'overview', label: '经营总览' },
  { key: 'ranking', label: '销售排行' },
  { key: 'purchase', label: '采购分析' },
  { key: 'inventory', label: '库存分析' },
]
const activeTab = ref<TabKey>('overview')
const rankDimension = ref<RankDimension>('customer')
/** 排行条数：排行 tab 可调，采购供应商排行固定 10 */
const rankLimit = ref(10)

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
const categories = ref<InvReportCategoryRow[]>([])
/** 客户销售排行：概览区展示 + 打印共用（排行 tab 另有一份随维度变化的 ranking） */
const customerRanking = ref<InvReportRankRow[]>([])
/** 排行 tab 当前维度的排行数据 */
const ranking = ref<InvReportRankRow[]>([])
/** 采购供应商排行 */
const purchaseRanking = ref<InvReportRankRow[]>([])
const inventory = ref<InvReportInventory | null>(null)

// ---------------- 各报表区加载 ----------------

/** 概览区：汇总 + 趋势 + 品类占比 + 客户排行（打印用） */
async function loadOverview(): Promise<void> {
  const p = filterParams()
  const [sm, tr, ct, rk] = await Promise.all([
    invApi.reportSummary(p),
    invApi.reportTrend({ ...p, days: trendDays.value }),
    invApi.reportCategory(p),
    invApi.reportRanking({ ...p, dimension: 'customer', biz: 'sales', limit: 10 }),
  ])
  summary.value = sm
  trend.value = tr
  categories.value = ct
  customerRanking.value = rk
}

/** 排行区：按当前维度查销售排行 */
async function loadRanking(): Promise<void> {
  ranking.value = await invApi.reportRanking({
    ...filterParams(),
    dimension: rankDimension.value,
    biz: 'sales',
    limit: rankLimit.value,
  })
}

/** 采购区：采购汇总 + 采购趋势 + 供应商排行 */
async function loadPurchase(): Promise<void> {
  const p = filterParams()
  const [sm, tr, rk] = await Promise.all([
    invApi.reportSummary(p),
    invApi.reportTrend({ ...p, days: trendDays.value }),
    invApi.reportRanking({ ...p, dimension: 'customer', biz: 'purchase', limit: 10 }),
  ])
  summary.value = sm
  trend.value = tr
  purchaseRanking.value = rk
}

/** 库存区：库存分析（共享主数据，不随筛选变化） */
async function loadInventory(): Promise<void> {
  inventory.value = await invApi.reportInventory()
}

/** 按当前 tab 加载对应数据 */
async function loadTab(): Promise<void> {
  loading.value = true
  try {
    switch (activeTab.value) {
      case 'ranking':
        await loadRanking()
        break
      case 'purchase':
        await loadPurchase()
        break
      case 'inventory':
        await loadInventory()
        break
      default:
        await loadOverview()
    }
  } finally {
    loading.value = false
  }
}

watch(activeTab, () => loadTab())
// 维度切换只在排行 tab 内生效，切了维度立即重查
watch(rankDimension, () => {
  if (activeTab.value === 'ranking') loadTab()
})

// ---------------- KPI 卡片 ----------------

const overviewKpis = computed(() => {
  const s = summary.value
  return [
    { label: '营业收入', value: formatMoney(s?.salesAmount ?? 0), tone: 'blue' },
    { label: '销售成本', value: formatMoney(s?.costAmount ?? 0), tone: 'amber' },
    { label: '毛利', value: formatMoney(s?.grossProfit ?? 0), tone: 'green' },
    { label: '毛利率', value: `${s?.grossRate ?? 0}%`, tone: 'violet' },
  ]
})

const purchaseKpis = computed(() => {
  const s = summary.value
  return [
    { label: '采购总额', value: formatMoney(s?.purchases ?? 0), tone: 'blue' },
    { label: '采购单数', value: `${s?.purchaseOrders ?? 0} 张`, tone: 'green' },
  ]
})

const inventoryKpis = computed(() => {
  const inv = inventory.value
  return [
    { label: '库存总金额', value: formatMoney(inv?.totalValue ?? 0), tone: 'blue' },
    { label: '库存规格数', value: `${inv?.specCount ?? 0} 个`, tone: 'violet' },
    { label: '低库存预警', value: `${inv?.warnCount ?? 0} 项`, tone: inv?.warnCount ? 'amber' : 'green' },
  ]
})

// ---------------- 图表 ----------------

/** 趋势图：概览区显示销售 + 采购两条线，采购区只显示采购额 */
function buildTrendOption(showSales: boolean, showPurchase: boolean): EChartsCoreOption {
  const p = palette.value
  const series: Array<Record<string, unknown>> = []
  if (showSales) {
    series.push({
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
    })
  }
  if (showPurchase) {
    series.push({
      name: '采购额',
      type: 'line',
      smooth: true,
      symbol: 'circle',
      symbolSize: 5,
      data: trend.value.map((it) => it.purchaseAmount),
      itemStyle: { color: p.success },
      lineStyle: { width: 2.4, color: p.success },
    })
  }
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
    series,
  }
}

const overviewTrendOption = computed(() => buildTrendOption(true, true))
const purchaseTrendOption = computed(() => buildTrendOption(false, true))

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

/** 品类库存金额分布：横向条形图（品类多时纵向排得下） */
const inventoryOption = computed<EChartsCoreOption>(() => {
  const p = palette.value
  const data = inventory.value?.categoryValue ?? []
  return {
    grid: { left: 8, right: 30, top: 10, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: p.overlay,
      borderColor: p.border,
      textStyle: { color: p.textStrong, fontSize: 12 },
      formatter: (params: Array<{ name: string; value: number }>) => {
        const it = params[0]
        return `${it.name}<br/>库存金额：¥${it.value.toLocaleString()}`
      },
    },
    xAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: p.split } },
      axisLabel: { color: p.text, fontSize: 11, formatter: '{value}元' },
    },
    yAxis: {
      type: 'category',
      data: data.map((it) => it.category),
      axisLine: { lineStyle: { color: p.axis } },
      axisTick: { show: false },
      axisLabel: { color: p.text, fontSize: 12 },
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 20,
        data: data.map((it) => ({ value: it.value, itemStyle: { color: p.primary, borderRadius: [0, 4, 4, 0] } })),
      },
    ],
  }
})

// ---------------- 筛选 ----------------

function changeRange(value: string): void {
  activeRange.value = value
  // 切预设时间就清掉自定义区间：两个都留着会让用户分不清当前生效的是哪个
  customFrom.value = ''
  customTo.value = ''
  loadTab()
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
  loadTab()
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

function openPrint(): void {
  printRef.value?.open()
}

onMounted(() => {
  loadTab()
  loadCatOptions()
})
</script>

<template>
  <div v-loading="loading" class="report-page">
    <!-- 共享筛选栏：时间范围 + 品类，四个报表区通用 -->
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
        <el-select v-model="activeCat" placeholder="全部品类" clearable style="width: 180px" @change="loadTab">
          <el-option v-for="c in catOptions" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
      </div>

      <div class="range-bar__actions">
        <el-button v-if="activeTab === 'overview'" @click="openPrint">
          <AppIcon name="Printer" />
          <span>打印</span>
        </el-button>
        <el-button @click="loadTab">
          <AppIcon name="Refresh" />
          <span>刷新</span>
        </el-button>
      </div>

      <div class="range-hint">
        当前范围：<b>{{ rangeLabel }}</b>
        <template v-if="catLabel"> · 品类：<b>{{ catLabel }}</b>（含子分类）</template>
      </div>
    </div>

    <!-- 报表区导航 -->
    <div class="report-tabs">
      <button
        v-for="t in tabs"
        :key="t.key"
        class="report-tab"
        :class="activeTab === t.key ? 'is-active' : ''"
        @click="activeTab = t.key"
      >
        {{ t.label }}
      </button>
    </div>

    <!-- ① 经营总览 -->
    <template v-if="activeTab === 'overview'">
      <div class="kpi-row">
        <div v-for="kpi in overviewKpis" :key="kpi.label" class="kpi-card" :class="`tone-${kpi.tone}`">
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
          <EChart :option="overviewTrendOption" height="300px" />
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
          <h3>客户销售排行</h3>
          <span class="text-muted">{{ summary?.range }} · 共 {{ summary?.salesOrders ?? 0 }} 张销售单</span>
        </header>
        <el-table :data="customerRanking" stripe>
          <el-table-column label="排名" width="80" align="center">
            <template #default="{ row }">
              <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="name" label="客户" min-width="160" show-overflow-tooltip />
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
        <el-empty v-if="!customerRanking.length" description="暂无排行数据" :image-size="80" />
      </div>
    </template>

    <!-- ② 销售排行 -->
    <div v-else-if="activeTab === 'ranking'" class="page-card">
      <header class="card-head">
        <h3>销售排行</h3>
        <div class="head-actions">
          <el-select v-model="rankLimit" style="width: 110px" @change="loadRanking">
            <el-option :value="5" label="前 5 名" />
            <el-option :value="10" label="前 10 名" />
            <el-option :value="20" label="前 20 名" />
          </el-select>
          <div class="dim-switch">
            <button
              class="dim-btn"
              :class="rankDimension === 'customer' ? 'is-active' : ''"
              @click="rankDimension = 'customer'"
            >
              按客户
            </button>
            <button
              class="dim-btn"
              :class="rankDimension === 'product' ? 'is-active' : ''"
              @click="rankDimension = 'product'"
            >
              按产品
            </button>
          </div>
        </div>
      </header>

      <el-table :data="ranking" stripe>
        <el-table-column label="排名" width="80" align="center">
          <template #default="{ row }">
            <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="name" :label="rankDimension === 'customer' ? '客户' : '商品'" min-width="180" show-overflow-tooltip />
        <el-table-column label="销售额" width="160" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column v-if="rankDimension === 'product'" label="销量" width="140" align="right">
          <template #default="{ row }">{{ row.quantity }}</template>
        </el-table-column>
        <el-table-column label="订单数" width="110" align="right">
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

    <!-- ③ 采购分析 -->
    <template v-else-if="activeTab === 'purchase'">
      <div class="kpi-row kpi-row--two">
        <div v-for="kpi in purchaseKpis" :key="kpi.label" class="kpi-card" :class="`tone-${kpi.tone}`">
          <span class="kpi-card__label">{{ kpi.label }}</span>
          <strong class="kpi-card__value">{{ kpi.value }}</strong>
        </div>
      </div>

      <div class="page-card">
        <header class="card-head">
          <h3>采购金额趋势</h3>
          <span class="text-muted">近 30 天 · 单位：元</span>
        </header>
        <EChart :option="purchaseTrendOption" height="300px" />
      </div>

      <div class="page-card">
        <header class="card-head">
          <h3>供应商采购排行</h3>
          <span class="text-muted">{{ summary?.range }} · 共 {{ summary?.purchaseOrders ?? 0 }} 张采购单</span>
        </header>
        <el-table :data="purchaseRanking" stripe>
          <el-table-column label="排名" width="80" align="center">
            <template #default="{ row }">
              <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="name" label="供应商" min-width="180" show-overflow-tooltip />
          <el-table-column label="采购额" width="160" align="right">
            <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
          </el-table-column>
          <el-table-column label="订单数" width="110" align="right">
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
        <el-empty v-if="!purchaseRanking.length" description="暂无采购数据" :image-size="80" />
      </div>
    </template>

    <!-- ④ 库存分析 -->
    <template v-else>
      <div class="kpi-row">
        <div v-for="kpi in inventoryKpis" :key="kpi.label" class="kpi-card" :class="`tone-${kpi.tone}`">
          <span class="kpi-card__label">{{ kpi.label }}</span>
          <strong class="kpi-card__value">{{ kpi.value }}</strong>
        </div>
      </div>

      <div class="chart-row chart-row--inventory">
        <div class="page-card chart-main">
          <header class="card-head">
            <h3>品类库存金额分布</h3>
            <span class="text-muted">按基本单位采购价计价</span>
          </header>
          <EChart v-if="inventory?.categoryValue.length" :option="inventoryOption" height="300px" />
          <el-empty v-else description="暂无库存数据" :image-size="80" />
        </div>
        <div class="page-card chart-side">
          <header class="card-head">
            <h3>低库存预警</h3>
            <span class="text-muted">共 {{ inventory?.warnCount ?? 0 }} 项</span>
          </header>
          <div class="warn-list">
            <div v-for="w in inventory?.warnList" :key="w.specId" class="warn-item">
              <div class="warn-item__main">
                <span class="warn-item__name" :title="`${w.productName} ${w.spec}`">{{ w.productName }} · {{ w.spec }}</span>
                <span class="warn-item__code">{{ w.productCode }}</span>
              </div>
              <div class="warn-item__stock">
                <span class="warn-item__num">{{ w.stock }}</span>
                <span class="warn-item__sep">/</span>
                <span class="warn-item__limit">预警 {{ w.warnStock }}{{ w.unit }}</span>
              </div>
            </div>
            <el-empty v-if="!inventory?.warnList.length" description="没有低库存商品" :image-size="80" />
          </div>
        </div>
      </div>
    </template>

    <!-- 打印经营概览（汇总 + 客户排行 + 品类占比） -->
    <ReportPrintDialog ref="printRef" :summary="summary" :ranking="customerRanking" :categories="categories" />
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

// 报表区导航：四个 tab，与筛选栏的 range-tabs 视觉同族但更醒目
.report-tabs {
  display: inline-flex;
  gap: 6px;
  padding: 4px;
  background: var(--app-surface);
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  align-self: flex-start;
  box-shadow: var(--shadow-card);
}

.report-tab {
  padding: 8px 20px;
  border: none;
  border-radius: calc(var(--radius-md) - 4px);
  background: transparent;
  font-size: 14px;
  color: var(--app-text-secondary);
  cursor: pointer;
  transition: var(--transition-base);

  &:hover {
    color: var(--app-text);
  }

  &.is-active {
    color: #fff;
    font-weight: 600;
    background: var(--brand-gradient);
  }
}

// 排行 tab 头部：条数选择 + 维度切换
.head-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.dim-switch {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  background: var(--app-surface-subtle);
  border-radius: var(--radius-sm);
}

.dim-btn {
  padding: 5px 14px;
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

  &--two {
    grid-template-columns: repeat(2, minmax(200px, 1fr));
    max-width: 640px;
  }
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

// 低库存预警清单：紧凑列表，缺货程度已在后端排序，最缺的在前
.warn-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 300px;
  overflow-y: auto;
}

.warn-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-sm);
  background: var(--app-surface-subtle);

  &__main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  &__name {
    font-size: 13px;
    color: var(--app-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__code {
    font-size: 12px;
    color: var(--app-text-muted);
  }

  &__stock {
    display: flex;
    align-items: baseline;
    gap: 4px;
    flex: none;
  }

  &__num {
    font-size: 16px;
    font-weight: 700;
    color: var(--tone-amber);
  }

  &__sep {
    color: var(--app-text-muted);
  }

  &__limit {
    font-size: 12px;
    color: var(--app-text-secondary);
  }
}
</style>
