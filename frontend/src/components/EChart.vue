<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { readToken } from '@/utils/theme'

// 按需注册，避免打包整个 echarts
echarts.use([
  LineChart,
  BarChart,
  PieChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  TitleComponent,
  CanvasRenderer,
])

const props = withDefaults(
  defineProps<{
    /** ECharts 配置项 */
    option: echarts.EChartsCoreOption
    /** 图表高度 */
    height?: string
    /** 是否显示加载态 */
    loading?: boolean
  }>(),
  { height: '320px', loading: false },
)

const container = ref<HTMLDivElement>()
const chart = shallowRef<echarts.ECharts>()
let observer: ResizeObserver | null = null

function render(): void {
  if (!container.value) return
  if (!chart.value) {
    chart.value = echarts.init(container.value)
  }
  chart.value.setOption(props.option, true)
}

function resize(): void {
  chart.value?.resize()
}

onMounted(() => {
  render()
  if (container.value) {
    observer = new ResizeObserver(() => resize())
    observer.observe(container.value)
  }
})

watch(
  () => props.option,
  () => render(),
  { deep: true },
)

watch(
  () => props.loading,
  (value) => {
    if (value) {
      // 加载态由 ECharts 自己画在 canvas 上，只能用具体色值，故从令牌读取
      chart.value?.showLoading('default', {
        text: '加载中',
        color: readToken('--brand-primary', '#2563eb'),
        textColor: readToken('--app-text-secondary', '#6b7280'),
        maskColor: readToken('--app-chart-mask', 'rgba(255, 255, 255, 0.65)'),
      })
    } else {
      chart.value?.hideLoading()
    }
  },
)

onBeforeUnmount(() => {
  observer?.disconnect()
  chart.value?.dispose()
  chart.value = undefined
})
</script>

<template>
  <div ref="container" class="chart" :style="{ height }" />
</template>

<style scoped>
.chart {
  width: 100%;
}
</style>
