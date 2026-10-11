<script setup lang="ts">
import { computed, ref } from 'vue'

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

/**
 * 同一 tick 内的 change 只发一次。
 *
 * 改每页条数时，el-pagination 自己会因总页数变化重算并写回 current-page，
 * 于是 `size` 与 `currentPage` 两个 setter 在同一轮交互里**各发一次** change，
 * 父组件的 @change 被触发两次 —— 于是同一个查询条件发出两次列表请求。
 * 「最后一页只有 3 条、每页改为 10 条」这类场景尤其明显：响应几乎同时到达，
 * 白白多一次往返，还可能让 loading 闪烁两次。
 *
 * 用 pending 标记 + 微任务归并，而不是加防抖：
 * 防抖会把「翻页点击」也延后，手感变差；这里只合并同一轮交互里的重复信号。
 */
const changePending = ref(false)

function emitChangeOnce(): void {
  if (changePending.value) return
  changePending.value = true
  void Promise.resolve().then(() => {
    changePending.value = false
    emit('change')
  })
}

const currentPage = computed({
  get: () => props.page,
  set: (value: number) => {
    emit('update:page', value)
    emitChangeOnce()
  },
})

const size = computed({
  get: () => props.pageSize,
  set: (value: number) => {
    // 与页码相同时不重复写：值没变却触发父组件更新，等于白跑一次请求
    if (value !== props.pageSize) {
      emit('update:pageSize', value)
      emit('update:page', 1)
    }
    emitChangeOnce()
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
