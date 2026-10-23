<script setup lang="ts">
/**
 * 报表中心：按「经营总览 / 销售排行 / 采购排行 / 库存排行 / 销售明细 / 采购明细 /
 * 合同执行 / 合同台账」八个报表区组织，前六个共用顶部时间与品类筛选栏。
 *
 * 各报表区数据懒加载、切 tab 时按需请求。排行一律**全量**返回（名次一直排下去，
 * 不截成「前 5 名」），明细是单据明细粒度（一行一个商品）。
 *
 * 合同两个区**不吃时间范围**：合同是跨月跨年的长期台账，
 * 按「本月」筛签订日期，看到的往往是一份合同都没有 —— 它们自带阶段/类型筛选。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { EChartsCoreOption } from 'echarts/core'
import { invApi } from '@/api'
import type {
  InvContractBizType,
  InvContractStage,
  InvReportCategoryRow,
  InvReportContractLedgerRow,
  InvReportContractResult,
  InvReportDetailResult,
  InvReportRankRow,
  InvReportStockRankRow,
  InvReportSummary,
  InvReportTrendPoint,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import EChart from '@/components/EChart.vue'
import ReportPrintDialog from '../components/ReportPrintDialog.vue'
import { formatDay, formatMoney } from '@/utils/format'
import { chartPalette } from '@/utils/theme'
import { useAppStore } from '@/store'
import { useLatestGuard } from '@/composables/useLatestGuard'

defineOptions({ name: 'InvReport' })

const appStore = useAppStore()
const palette = computed(() => {
  void appStore.theme
  return chartPalette()
})

type TabKey =
  | 'overview'
  | 'salesRank'
  | 'purchaseRank'
  | 'stockRank'
  | 'salesDetail'
  | 'purchaseDetail'
  | 'contractExec'
  | 'contractLedger'
type RankDimension = 'customer' | 'product'
type PurchaseDimension = 'supplier' | 'product'

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'overview', label: '经营总览' },
  { key: 'salesRank', label: '销售排行' },
  { key: 'purchaseRank', label: '采购排行' },
  { key: 'stockRank', label: '库存排行' },
  { key: 'salesDetail', label: '销售明细' },
  { key: 'purchaseDetail', label: '采购明细' },
  { key: 'contractExec', label: '合同执行' },
  { key: 'contractLedger', label: '合同台账' },
]
const activeTab = ref<TabKey>('overview')
/** 合同两个区用独立筛选：合同是长期台账，跟着「本月」筛没有意义 */
const isContractTab = computed(() => activeTab.value === 'contractExec' || activeTab.value === 'contractLedger')
const contractBiz = ref<InvContractBizType | ''>('')
const contractStage = ref<InvContractStage | ''>('')
const contractKeyword = ref('')
const rankDimension = ref<RankDimension>('customer')
const purchaseDimension = ref<PurchaseDimension>('supplier')

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
/** 销售排行（当前维度，全量） */
const ranking = ref<InvReportRankRow[]>([])
/** 采购排行（当前维度，全量） */
const purchaseRanking = ref<InvReportRankRow[]>([])
/** 库存排行（按库存金额降序，全量） */
const stockRanking = ref<InvReportStockRankRow[]>([])
/** 明细：销售 / 采购共用一个容器，按当前 tab 取数 */
const detail = ref<InvReportDetailResult | null>(null)
/** 合同执行报表 */
const contractReport = ref<InvReportContractResult | null>(null)
/** 合同收付款台账 */
const contractLedger = ref<InvReportContractLedgerRow[]>([])
/**
 * 明细的往来单位关键字（销售 = 客户，采购 = 供应商）。
 * 只作用于明细：排行榜本身按往来单位聚合，再按名字过滤它没有意义。
 */
const detailParty = ref('')

// ---------------- 各报表区加载 ----------------

const guard = useLatestGuard()

/** 概览区：汇总 + 趋势 + 品类占比 + 客户排行（打印用） */
async function loadOverview(ticket: number): Promise<void> {
  const p = filterParams()
  const [sm, tr, ct, rk] = await Promise.all([
    invApi.reportSummary(p),
    invApi.reportTrend({ ...p, days: trendDays.value }),
    invApi.reportCategory(p),
    invApi.reportRanking({ ...p, dimension: 'customer', biz: 'sales', limit: 10 }),
  ])
  // summary / trend 与「采购分析」共享写入：快速切 tab 时，
  // 后返回的旧 tab 会覆盖新 tab，必须按票据丢弃过期响应。
  if (!guard.isCurrent(ticket)) return
  summary.value = sm
  trend.value = tr
  categories.value = ct
  customerRanking.value = rk
}

/** 销售排行：limit 0 = 全量，名次一直排下去 */
async function loadSalesRank(ticket: number): Promise<void> {
  const rk = await invApi.reportRanking({
    ...filterParams(),
    dimension: rankDimension.value,
    biz: 'sales',
    limit: 0,
  })
  if (!guard.isCurrent(ticket)) return
  ranking.value = rk
}

/** 采购排行：按供应商（party 聚合）/ 按商品（明细聚合） */
async function loadPurchaseRank(ticket: number): Promise<void> {
  const rk = await invApi.reportRanking({
    ...filterParams(),
    dimension: purchaseDimension.value === 'supplier' ? 'customer' : 'product',
    biz: 'purchase',
    limit: 0,
  })
  if (!guard.isCurrent(ticket)) return
  purchaseRanking.value = rk
}

/** 库存排行：共享主数据，不随时间 / 品类筛选变化 */
async function loadStockRank(ticket: number): Promise<void> {
  const rk = await invApi.reportStockRanking()
  if (!guard.isCurrent(ticket)) return
  stockRanking.value = rk
}

/** 明细：销售 / 采购共用，按当前 tab 决定 biz；往来单位是明细专属筛选 */
async function loadDetail(ticket: number): Promise<void> {
  const biz = activeTab.value === 'purchaseDetail' ? 'purchase' : 'sales'
  const p: Record<string, string | number> = { ...filterParams(), biz }
  if (detailParty.value) p.party = detailParty.value
  const r = await invApi.reportDetail(p)
  if (!guard.isCurrent(ticket)) return
  detail.value = r
}

/** 合同两个区的公共筛选参数（不吃时间范围，见文件头注释） */
function contractParams(): Record<string, string | number> {
  const p: Record<string, string | number> = {}
  if (contractBiz.value) p.bizType = contractBiz.value
  // 「已完成 / 已终止」不是日期条件，后端按 status 判（与合同列表同一套映射）
  if (contractStage.value === 'finished') p.status = 2
  else if (contractStage.value === 'terminated') p.status = 3
  else if (contractStage.value) p.stage = contractStage.value
  if (contractKeyword.value.trim()) p.keyword = contractKeyword.value.trim()
  return p
}

/** 合同执行：一份合同一行，带收付进度与质保金余额 */
async function loadContractExec(ticket: number): Promise<void> {
  const r = await invApi.reportContract(contractParams())
  if (!guard.isCurrent(ticket)) return
  contractReport.value = r
}

/** 合同台账：一行一笔实际发生的收付款 */
async function loadContractLedger(ticket: number): Promise<void> {
  const r = await invApi.reportContractLedger(contractParams())
  if (!guard.isCurrent(ticket)) return
  contractLedger.value = r
}

/** 按当前 tab 加载对应数据 */
async function loadTab(): Promise<void> {
  const ticket = guard.start()
  loading.value = true
  try {
    switch (activeTab.value) {
      case 'salesRank':
        await loadSalesRank(ticket)
        break
      case 'purchaseRank':
        await loadPurchaseRank(ticket)
        break
      case 'stockRank':
        await loadStockRank(ticket)
        break
      case 'salesDetail':
      case 'purchaseDetail':
        await loadDetail(ticket)
        break
      case 'contractExec':
        await loadContractExec(ticket)
        break
      case 'contractLedger':
        await loadContractLedger(ticket)
        break
      default:
        await loadOverview(ticket)
    }
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    if (guard.isCurrent(ticket)) loading.value = false
  }
}

watch(activeTab, () => loadTab())
// 维度切换只在自己那个 tab 内生效，切了维度立即重查
watch(rankDimension, () => {
  if (activeTab.value === 'salesRank') loadTab()
})
watch(purchaseDimension, () => {
  if (activeTab.value === 'purchaseRank') loadTab()
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
// 用 undefined 而不是 0 表示「全部品类」：0 不在选项里，el-select 会把 0 当成
// 已选值直接显示出来（下拉框里出现一个孤零零的「0」）
const activeCat = ref<number | undefined>(undefined)

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

/** 导出当前报表区的数据为 xlsx（导出的是当前筛选范围的全量，不是屏幕上的前 N 条） */
const exporting = ref(false)
/**
 * 哪些报表区支持导出。
 *
 * 导出走服务端生成的工作簿，覆盖总览 / 两个排行 / 两个明细；
 * 库存排行本身就是一张按金额排序的表，不另做导出。
 */
const canExport = computed(() =>
  ['overview', 'salesRank', 'purchaseRank', 'salesDetail', 'purchaseDetail', 'contractExec'].includes(
    activeTab.value,
  ),
)

async function exportReport(): Promise<void> {
  exporting.value = true
  try {
    // 合同执行走「合同台账」导出：后端那一份列里已经带上了已收付 / 未收付 /
    // 阶段 / 验收 / 质保到期，与屏幕上的报表是同一套查询
    if (activeTab.value === 'contractExec') {
      await invApi.exportList({ entity: 'contract', ...contractParams() }, '合同执行.xlsx')
      ElMessage.success('导出成功')
      return
    }
    const params: Record<string, unknown> = { ...filterParams(), type: 'overview' }
    if (activeTab.value === 'salesRank') {
      params.type = 'ranking'
      params.biz = 'sales'
      params.dimension = rankDimension.value
    } else if (activeTab.value === 'purchaseRank') {
      params.type = 'ranking'
      params.biz = 'purchase'
      params.dimension = purchaseDimension.value === 'supplier' ? 'customer' : 'product'
    } else if (activeTab.value === 'salesDetail' || activeTab.value === 'purchaseDetail') {
      // 明细导出跟着屏幕上的筛选走：换了往来单位再导出，拿到的必须是过滤后的那份
      params.type = 'detail'
      params.biz = activeTab.value === 'purchaseDetail' ? 'purchase' : 'sales'
      if (detailParty.value) params.party = detailParty.value
    }
    await invApi.reportExport(params)
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  loadTab()
  loadCatOptions()
})
</script>

<template>
  <div v-loading="loading" class="report-page">
    <!-- 共享筛选栏：时间范围 + 品类，前六个报表区通用 -->
    <div v-if="!isContractTab" class="page-card range-bar">
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
        <el-button v-if="canExport" :loading="exporting" @click="exportReport">
          <AppIcon name="Download" />
          <span>导出</span>
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

    <!-- 合同两个区：不吃时间范围，用阶段/类型/关键字筛 -->
    <div v-else class="page-card range-bar">
      <el-form class="filter-form" inline @submit.prevent>
        <el-form-item label="类型">
          <el-select v-model="contractBiz" placeholder="全部" clearable style="width: 140px" @change="loadTab">
            <el-option label="销售合同" value="sales" />
            <el-option label="采购合同" value="purchase" />
          </el-select>
        </el-form-item>
        <el-form-item label="阶段">
          <el-select v-model="contractStage" placeholder="全部" clearable style="width: 140px" @change="loadTab">
            <el-option label="待验收" value="pending" />
            <el-option label="已验收" value="accepted" />
            <el-option label="质保中" value="warranty" />
            <el-option label="质保到期" value="warranty_due" />
            <el-option label="已完成" value="finished" />
            <el-option label="已终止" value="terminated" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键字">
          <el-input
            v-model="contractKeyword"
            placeholder="合同号 / 名称 / 往来单位"
            clearable
            style="width: 220px"
            @keyup.enter="loadTab"
          />
        </el-form-item>
      </el-form>
      <div class="range-bar__actions">
        <el-button type="primary" @click="loadTab">
          <AppIcon name="Search" />
          <span>查询</span>
        </el-button>
        <el-button v-if="canExport" :loading="exporting" @click="exportReport">
          <AppIcon name="Download" />
          <span>导出</span>
        </el-button>
        <el-button @click="loadTab">
          <AppIcon name="Refresh" />
          <span>刷新</span>
        </el-button>
      </div>
      <div class="range-hint">
        合同是跨期台账，不随时间范围筛选；金额口径为<b>付款计划</b>（未设计划则为 0）
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

    <!-- ② 销售排行（全量，不截名次） -->
    <div v-else-if="activeTab === 'salesRank'" class="page-card">
      <header class="card-head">
        <h3>销售排行</h3>
        <div class="head-actions">
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
              按商品
            </button>
          </div>
        </div>
      </header>

      <el-table :data="ranking" stripe max-height="620">
        <el-table-column label="排名" width="80" align="center">
          <template #default="{ row }">
            <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
          </template>
        </el-table-column>
        <el-table-column
          prop="name"
          :label="rankDimension === 'customer' ? '客户' : '商品'"
          min-width="180"
          show-overflow-tooltip
        />
        <el-table-column label="销售额" width="160" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column v-if="rankDimension === 'product'" label="销量（基本单位）" width="150" align="right">
          <template #default="{ row }">{{ row.quantity }}</template>
        </el-table-column>
        <el-table-column label="单据数" width="100" align="right">
          <template #default="{ row }">{{ row.orderCount }}</template>
        </el-table-column>
        <el-table-column label="占比" min-width="160">
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

    <!-- ③ 采购排行（全量） -->
    <div v-else-if="activeTab === 'purchaseRank'" class="page-card">
      <header class="card-head">
        <h3>采购排行</h3>
        <div class="head-actions">
          <div class="dim-switch">
            <button
              class="dim-btn"
              :class="purchaseDimension === 'supplier' ? 'is-active' : ''"
              @click="purchaseDimension = 'supplier'"
            >
              按供应商
            </button>
            <button
              class="dim-btn"
              :class="purchaseDimension === 'product' ? 'is-active' : ''"
              @click="purchaseDimension = 'product'"
            >
              按商品
            </button>
          </div>
        </div>
      </header>

      <el-table :data="purchaseRanking" stripe max-height="620">
        <el-table-column label="排名" width="80" align="center">
          <template #default="{ row }">
            <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
          </template>
        </el-table-column>
        <el-table-column
          prop="name"
          :label="purchaseDimension === 'supplier' ? '供应商' : '商品'"
          min-width="180"
          show-overflow-tooltip
        />
        <el-table-column label="采购额" width="160" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column v-if="purchaseDimension === 'product'" label="采购量（基本单位）" width="160" align="right">
          <template #default="{ row }">{{ row.quantity }}</template>
        </el-table-column>
        <el-table-column label="单据数" width="100" align="right">
          <template #default="{ row }">{{ row.orderCount }}</template>
        </el-table-column>
        <el-table-column label="占比" min-width="160">
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

    <!-- ④ 库存排行（按库存金额降序，全量） -->
    <div v-else-if="activeTab === 'stockRank'" class="page-card">
      <header class="card-head">
        <h3>库存排行</h3>
        <span class="text-muted">按库存金额降序 · 计价 = 库存数量 × 基本单位采购价</span>
      </header>

      <el-table :data="stockRanking" stripe max-height="640">
        <el-table-column label="排名" width="80" align="center">
          <template #default="{ row }">
            <span class="rank" :class="row.rank <= 3 ? 'rank--top' : ''">{{ row.rank }}</span>
          </template>
        </el-table-column>
        <el-table-column label="商品" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.productName }}
            <span class="text-muted">{{ row.productCode }}</span>
          </template>
        </el-table-column>
        <el-table-column label="规格" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ row.spec || '—' }}</template>
        </el-table-column>
        <el-table-column label="单位" width="80" align="center">
          <template #default="{ row }">{{ row.unit || '—' }}</template>
        </el-table-column>
        <el-table-column label="库存数量" width="120" align="right">
          <template #default="{ row }">{{ row.stock }}</template>
        </el-table-column>
        <el-table-column label="库存金额" width="150" align="right">
          <template #default="{ row }">{{ formatMoney(row.value) }}</template>
        </el-table-column>
        <el-table-column label="占比" min-width="160">
          <template #default="{ row }">
            <div class="ratio">
              <el-progress :percentage="row.ratio" :stroke-width="8" :show-text="false" />
              <span>{{ row.ratio }}%</span>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!stockRanking.length" description="暂无库存数据" :image-size="80" />
    </div>

    <!-- ⑤⑥ 销售 / 采购明细（单据明细粒度） -->
    <div v-else-if="activeTab === 'salesDetail' || activeTab === 'purchaseDetail'" class="page-card">
      <header class="card-head">
        <h3>{{ activeTab === 'salesDetail' ? '销售明细' : '采购明细' }}</h3>
        <div class="detail-head__right">
          <span class="text-muted">
            共 {{ detail?.total ?? 0 }} 行 · 合计 {{ formatMoney(detail?.amount ?? 0) }}
            <template v-if="detail?.truncated">（行数过多，仅展示前 {{ detail.rows.length }} 行）</template>
          </span>
          <el-input
            v-model="detailParty"
            :placeholder="`${activeTab === 'salesDetail' ? '客户' : '供应商'}名称，可模糊`"
            clearable
            style="width: 200px"
            @keyup.enter="loadTab"
            @clear="loadTab"
          >
            <template #prefix>
              <AppIcon name="Search" />
            </template>
          </el-input>
          <el-button type="primary" plain @click="loadTab">查询</el-button>
        </div>
      </header>

      <el-table :data="detail?.rows ?? []" stripe max-height="640">
        <el-table-column label="日期" width="110">
          <template #default="{ row }">{{ formatDay(row.orderDate) }}</template>
        </el-table-column>
        <el-table-column prop="orderNo" label="单号" width="160" show-overflow-tooltip />
        <el-table-column prop="partyName" :label="activeTab === 'salesDetail' ? '客户' : '供应商'" min-width="150" show-overflow-tooltip />
        <el-table-column prop="productName" label="商品" min-width="160" show-overflow-tooltip />
        <el-table-column label="规格" min-width="110" show-overflow-tooltip>
          <template #default="{ row }">{{ row.spec || '—' }}</template>
        </el-table-column>
        <el-table-column label="单位" width="70" align="center">
          <template #default="{ row }">{{ row.unit || '—' }}</template>
        </el-table-column>
        <el-table-column label="数量" width="100" align="right">
          <template #default="{ row }">{{ row.qty }}</template>
        </el-table-column>
        <el-table-column label="单价" width="110" align="right">
          <template #default="{ row }">{{ formatMoney(row.price, false) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="130" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!(detail?.rows ?? []).length" description="暂无明细数据" :image-size="80" />
    </div>

    <!-- ⑦ 合同执行 -->
    <div v-else-if="activeTab === 'contractExec'" class="page-card">
      <div class="kpi-row">
        <div class="kpi-card tone-blue">
          <span class="kpi-card__label">合同金额</span>
          <strong class="kpi-card__value">{{ formatMoney(contractReport?.amount ?? 0) }}</strong>
        </div>
        <div class="kpi-card tone-green">
          <span class="kpi-card__label">已收付</span>
          <strong class="kpi-card__value">{{ formatMoney(contractReport?.settledAmount ?? 0) }}</strong>
        </div>
        <div class="kpi-card tone-amber">
          <span class="kpi-card__label">未收付</span>
          <strong class="kpi-card__value">{{ formatMoney(contractReport?.outstanding ?? 0) }}</strong>
        </div>
        <div class="kpi-card tone-red">
          <span class="kpi-card__label">质保金未收</span>
          <strong class="kpi-card__value">{{ formatMoney(contractReport?.retentionOutstanding ?? 0) }}</strong>
        </div>
      </div>

      <header class="card-head">
        <h3>合同执行</h3>
        <span class="text-muted">
          共 {{ contractReport?.count ?? 0 }} 份 · 计划合计 {{ formatMoney(contractReport?.planAmount ?? 0) }}
          <template v-if="contractReport?.truncated">（已超过 1000 份，仅展示前 1000 份）</template>
        </span>
      </header>

      <el-table :data="contractReport?.rows ?? []" stripe max-height="640">
        <el-table-column prop="contractNo" label="合同编号" width="130" show-overflow-tooltip />
        <el-table-column prop="name" label="合同名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.bizType === 'purchase' ? 'warning' : 'success'" effect="plain">
              {{ row.bizType === 'purchase' ? '采购' : '销售' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="partyName" label="往来单位" min-width="140" show-overflow-tooltip />
        <el-table-column label="签订日期" width="104">
          <template #default="{ row }">{{ formatDay(row.signDate) }}</template>
        </el-table-column>
        <el-table-column label="合同金额" width="116" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="计划" width="112" align="right">
          <template #default="{ row }">
            {{ row.planCount ? formatMoney(row.planAmount) : '—' }}
          </template>
        </el-table-column>
        <el-table-column label="已收付" width="112" align="right">
          <template #default="{ row }">{{ formatMoney(row.settledAmount) }}</template>
        </el-table-column>
        <el-table-column label="未收付" width="112" align="right">
          <template #default="{ row }">
            <strong :class="row.outstanding > 0 ? 'text-warning' : ''">{{ formatMoney(row.outstanding) }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="质保金未收" width="112" align="right">
          <template #default="{ row }">{{ formatMoney(row.retentionOutstanding) }}</template>
        </el-table-column>
        <el-table-column label="阶段" width="96">
          <template #default="{ row }">
            <el-tag size="small" effect="light">{{ row.stageLabel }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="质保到期" width="104">
          <template #default="{ row }">{{ row.warrantyEndAt ? formatDay(row.warrantyEndAt) : '—' }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!(contractReport?.rows ?? []).length" description="暂无合同数据" :image-size="80" />
    </div>

    <!-- ⑧ 合同收付款台账 -->
    <div v-else class="page-card">
      <header class="card-head">
        <h3>合同收付款台账</h3>
        <span class="text-muted">
          共 {{ contractLedger.length }} 笔 · 合计
          {{ formatMoney(contractLedger.reduce((s, r) => s + r.amount, 0)) }}
        </span>
      </header>
      <el-table :data="contractLedger" stripe max-height="640">
        <el-table-column label="收付日期" width="110">
          <template #default="{ row }">{{ formatDay(row.settledAt) }}</template>
        </el-table-column>
        <el-table-column prop="contractNo" label="合同编号" width="130" show-overflow-tooltip />
        <el-table-column prop="name" label="合同名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.bizType === 'purchase' ? 'warning' : 'success'" effect="plain">
              {{ row.bizType === 'purchase' ? '付款' : '收款' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="partyName" label="往来单位" min-width="140" show-overflow-tooltip />
        <el-table-column label="期次" width="150" show-overflow-tooltip>
          <template #default="{ row }">第{{ row.planSeq }}期 {{ row.planName }}</template>
        </el-table-column>
        <el-table-column label="金额" width="126" align="right">
          <template #default="{ row }">
            <strong>{{ formatMoney(row.amount) }}</strong>
          </template>
        </el-table-column>
        <el-table-column prop="method" label="方式" width="110" />
        <el-table-column prop="remark" label="备注" min-width="140" show-overflow-tooltip />
      </el-table>
      <el-empty v-if="!contractLedger.length" description="暂无收付款记录" :image-size="80" />
    </div>

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

/* 还有未收付金额时的高亮（合同执行报表用） */
.text-warning {
  color: var(--el-color-warning);
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
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 12px;

  h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--app-text);
  }
}

// 明细卡片头右侧：统计 + 往来单位筛选，一行排开（窄屏整体换行）
.detail-head__right {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
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
