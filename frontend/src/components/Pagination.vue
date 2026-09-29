<script setup lang="ts">
import { computed } from 'vue'

// 分页组件：与后端 PageData 结构保持一致（page 从 1 开始）
const props = withDefaults(
  defineProps<{
    page: number
    pageSize: number
    total: number
    pageSizes?: number[]
  }>(),
  { pageSizes: () => [10, 20, 50, 100] },
)

const emit = defineEmits<{
  (e: 'update:page', value: number): void
  (e: 'update:pageSize', value: number): void
  (e: 'change'): void
}>()

const currentPage = computed({
  get: () => props.page,
  set: (value: number) => {
    emit('update:page', value)
    emit('change')
  },
})

const size = computed({
  get: () => props.pageSize,
  set: (value: number) => {
    emit('update:pageSize', value)
    emit('update:page', 1)
    emit('change')
  },
})
</script>

<template>
  <el-pagination
    v-model:current-page="currentPage"
    v-model:page-size="size"
    :total="total"
    :page-sizes="pageSizes"
    layout="total, sizes, prev, pager, next, jumper"
    background
    :hide-on-single-page="false"
  />
</template>
