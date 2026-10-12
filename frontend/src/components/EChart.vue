<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

// 按需注册，避免打包整个 echarts
echarts.use([
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  TitleComponent,
  CanvasRenderer,
])

// 曾经有一个 loading prop（转发到 echarts.showLoading），但没有任何调用方
// 传过它——页面级 loading 统一由 v-loading 指令负责。留着一段永远走不到
// 的分支，还要为它维护 readToken 的色值，删掉。
const props = withDefaults(
  defineProps<{
    /** ECharts 配置项 */
    option: echarts.EChartsCoreOption
    /** 图表高度 */
    height?: string
  }>(),
  { height: '320px' },
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
