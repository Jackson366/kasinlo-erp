<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { EChartsCoreOption } from 'echarts/core'
import { dashboardApi } from '@/api'
import type { DashboardOverview, LoginLogItem, ModuleStat, TrendPoint } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import EChart from '@/components/EChart.vue'
import { fromNow } from '@/utils/format'
import { chartPalette } from '@/utils/theme'
import { useAppStore, useUserStore } from '@/store'

defineOptions({ name: 'Dashboard' })

const appStore = useAppStore()
const userStore = useUserStore()

/**
 * 图表调色板。ECharts 只接受具体色值、不认 CSS 变量，
 * 因此在主题变化时重新读取一次令牌；下面的 option 都是 computed，
 * 会跟着重算并触发图表重绘。
 */
const palette = computed(() => {
  void appStore.theme
  return chartPalette()
})

const loading = ref(true)
const overview = ref<DashboardOverview | null>(null)
const trend = ref<TrendPoint[]>([])
const moduleStats = ref<ModuleStat[]>([])
const recentLogins = ref<LoginLogItem[]>([])

const statCards = computed(() => [
  {
    key: 'users',
    label: '用户总数',
    value: overview.value?.userCount ?? 0,
    icon: 'User',
    tone: 'blue',
    hint: `近 7 天活跃 ${overview.value?.activeUserWeek ?? 0} 人`,
  },
  {
    key: 'logins',
    label: '今日登录',
    value: overview.value?.todayLogins ?? 0,
    icon: 'Key',
    tone: 'green',
    hint: `失败 ${overview.value?.todayFailures ?? 0} 次`,
  },
  {
    key: 'ops',
    label: '今日操作',
    value: overview.value?.todayOps ?? 0,
    icon: 'DataLine',
    tone: 'violet',
    hint: '写操作记录条数',
  },
  {
    key: 'menus',
    label: '菜单与权限点',
    value: overview.value?.menuCount ?? 0,
    icon: 'Menu',
    tone: 'amber',
    hint: `角色 ${overview.value?.roleCount ?? 0} 个 · 部门 ${overview.value?.deptCount ?? 0} 个`,
  },
])

/** 首页问候语 */
const greeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 6) return '凌晨好'
  if (hour < 12) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})

const trendOption = computed<EChartsCoreOption>(() => {
  const p = palette.value
  return {
    grid: { left: 8, right: 16, top: 40, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line' },
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
      data: trend.value.map((item) => item.date.slice(5)),
      axisLine: { lineStyle: { color: p.axis } },
      axisTick: { show: false },
      axisLabel: { color: p.text, fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: p.split } },
      axisLabel: { color: p.text, fontSize: 11 },
    },
    series: [
      {
        name: '操作次数',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        data: trend.value.map((item) => item.opers),
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
        name: '登录次数',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        data: trend.value.map((item) => item.logins),
        itemStyle: { color: p.success },
        lineStyle: { width: 2.4, color: p.success },
      },
    ],
  }
})

const moduleOption = computed<EChartsCoreOption>(() => {
  const p = palette.value
  const sorted = [...moduleStats.value].sort((a, b) => a.total - b.total)
  return {
    grid: { left: 8, right: 24, top: 10, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: p.overlay,
      borderColor: p.border,
      textStyle: { color: p.textStrong, fontSize: 12 },
    },
    xAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: p.split } },
      axisLabel: { color: p.text, fontSize: 11 },
    },
    yAxis: {
      type: 'category',
      data: sorted.map((item) => item.module),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: p.textStrong, fontSize: 12 },
    },
    series: [
      {
        name: '操作次数',
        type: 'bar',
        barWidth: 12,
        data: sorted.map((item) => item.total),
        itemStyle: {
          borderRadius: [0, 6, 6, 0],
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 1,
            y2: 0,
            colorStops: [
              { offset: 0, color: p.barFrom },
              { offset: 1, color: p.primary },
            ],
          },
        },
      },
    ],
  }
})

async function loadAll(): Promise<void> {
  loading.value = true
  try {
    const [overviewData, trendData, moduleData, loginData] = await Promise.all([
      dashboardApi.overview(),
      dashboardApi.trend(7),
      dashboardApi.modules(7, 6),
      dashboardApi.recentLogins(6),
    ])
    overview.value = overviewData
    trend.value = trendData
    moduleStats.value = moduleData
    recentLogins.value = loginData
  } finally {
    loading.value = false
  }
}

onMounted(loadAll)

/** 跳转到日志页查看全部 */
function goLogs(): void {
  window.location.hash = '#/system/log'
}
</script>

<template>
  <div v-loading="loading" class="dashboard">
    <!-- 欢迎语 -->
    <div class="dashboard__hello">
      <div>
        <h2>{{ greeting }}，{{ userStore.displayName }}</h2>
        <p class="text-secondary">
          <template v-if="userStore.roles.length">
            当前角色：{{ userStore.roles.join('、') }}
          </template>
          <template v-else>欢迎使用 Nova Admin</template>
        </p>
      </div>
      <el-button @click="loadAll">
        <AppIcon name="Refresh" />
        <span>刷新数据</span>
      </el-button>
    </div>

    <!-- 指标卡片 -->
    <div class="dashboard__stats">
      <div v-for="card in statCards" :key="card.key" class="stat-card" :class="`stat-card--${card.tone}`">
        <div class="stat-card__icon">
          <AppIcon :name="card.icon" />
        </div>
        <div class="stat-card__body">
          <span class="stat-card__label">{{ card.label }}</span>
          <strong class="stat-card__value">{{ card.value }}</strong>
          <span class="stat-card__hint">{{ card.hint }}</span>
        </div>
      </div>
    </div>

    <!-- 图表区 -->
    <div class="dashboard__charts">
      <div class="page-card dashboard__chart-main">
        <header class="card-head">
          <h3>近 7 天趋势</h3>
          <span class="text-muted">登录与操作次数</span>
        </header>
        <EChart :option="trendOption" height="300px" />
      </div>

      <div class="page-card dashboard__chart-side">
        <header class="card-head">
          <h3>模块操作排行</h3>
          <span class="text-muted">Top 6</span>
        </header>
        <EChart v-if="moduleStats.length" :option="moduleOption" height="300px" />
        <el-empty v-else description="暂无操作数据" :image-size="80" />
      </div>
    </div>

    <!-- 最近登录 -->
    <div class="page-card">
      <header class="card-head">
        <h3>最近登录记录</h3>
        <el-button link type="primary" @click="goLogs">查看全部</el-button>
      </header>
      <el-table :data="recentLogins" size="default" stripe>
        <el-table-column prop="username" label="账号" min-width="120" />
        <el-table-column prop="ip" label="登录 IP" min-width="140" />
        <el-table-column label="结果" width="100">
          <template #default="{ row }">
            <el-tag :type="row.success ? 'success' : 'danger'" size="small" effect="light">
              {{ row.success ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="说明" min-width="120" show-overflow-tooltip />
        <el-table-column label="时间" width="140" align="right">
          <template #default="{ row }">
            <span class="text-secondary">{{ fromNow(row.createdAt) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<style scoped lang="scss">
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 16px;

  &__hello {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 18px 20px;
    background: var(--app-gradient-hello);
    border: 1px solid var(--app-border-light);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-card);

    h2 {
      margin: 0 0 6px;
      font-size: 18px;
      font-weight: 700;
      color: var(--app-text);
    }

    p {
      margin: 0;
      font-size: 13px;
    }
  }

  &__stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 16px;
  }

  &__charts {
    display: grid;
    grid-template-columns: 1.9fr 1fr;
    gap: 16px;

    @media (max-width: 1100px) {
      grid-template-columns: 1fr;
    }
  }
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px;
  background: var(--app-surface);
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-card);
  transition: var(--transition-base);

  &:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow-hover);
  }

  &__icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    flex-shrink: 0;
    font-size: 20px;
    border-radius: 12px;
  }

  &__body {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  &__label {
    font-size: 12.5px;
    color: var(--app-text-secondary);
  }

  &__value {
    margin: 2px 0;
    font-size: 24px;
    font-weight: 700;
    line-height: 1.2;
    color: var(--app-text);
  }

  &__hint {
    font-size: 12px;
    color: var(--app-text-muted);
  }

  &--blue &__icon {
    color: var(--tone-blue);
    background: var(--tone-blue-soft);
  }

  &--green &__icon {
    color: var(--tone-green);
    background: var(--tone-green-soft);
  }

  &--violet &__icon {
    color: var(--tone-violet);
    background: var(--tone-violet-soft);
  }

  &--amber &__icon {
    color: var(--tone-amber);
    background: var(--tone-amber-soft);
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

  span {
    font-size: 12px;
  }
}
</style>
