<script setup lang="ts">
import { computed } from 'vue'
import { fallbackIcon, getIcon } from './icons'

// 通用图标组件：按名称渲染 element-plus 图标，未登记时回退到默认图标
const props = withDefaults(
  defineProps<{
    name?: string
    /** 未找到图标时是否使用默认图标 */
    fallback?: boolean
  }>(),
  { name: '', fallback: true },
)

const icon = computed(() => getIcon(props.name) || (props.fallback ? fallbackIcon : null))
</script>

<template>
  <el-icon v-if="icon" class="app-icon">
    <component :is="icon" />
  </el-icon>
</template>

<style scoped>
.app-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
</style>
