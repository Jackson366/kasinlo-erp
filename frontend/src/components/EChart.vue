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

// 按需注册，避免打包整个 echarts。
//
// 注册表必须覆盖全部实际用到的图表类型：ECharts 对未注册的类型不报错、
// 只是打印一条 warning 然后把整个 series 丢掉 —— 页面不白屏，但那块图
// 就是不出来。数据概览与报表中心的「品类占比」都是 pie，漏注册的代价
// 是两处饼图长期空白（运行期验证脚本的控制台检查会抓到这条 warning）。
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
