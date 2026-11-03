<script setup lang="ts" generic="Row extends object">
import { ref } from 'vue'
import type { TableInstance } from 'element-plus'
import ListPanel from '../ListPanel.vue'
import Pagination from '../Pagination.vue'
import KasTableToolbar from './components/KasTableToolbar.vue'
import KasTableState from './components/KasTableState.vue'
import type { KasTableProps } from './types'

withDefaults(defineProps<KasTableProps<Row>>(), {
  selectedCount: 0,
  unit: '条',
  resultLabel: '共',
  fit: true,
  surface: true,
  showPagination: true,
  emptyText: '暂无数据',
  errorText: '列表加载失败，请重新加载后查看。',
})
defineEmits<{
  'update:page': [value: number]
  'update:pageSize': [value: number]
  change: []
  retry: []
  clearSelection: []
  selectionChange: [rows: Row[]]
}>()
const table = ref<TableInstance>()
function clearSelection(): void {
  table.value?.clearSelection()
}
defineExpose({ clearSelection })
</script>

<template>
  <ListPanel class="kas-table" :fit="fit" :surface="surface">
    <slot name="before" />
    <KasTableToolbar
      :title="title"
      :subtitle="subtitle"
      :unit="unit"
      :result-label="resultLabel"
      :total="total"
      :selected-count="selectedCount"
      @clear-selection="$emit('clearSelection')"
    >
      <slot name="filters" />
    </KasTableToolbar>
    <!-- 保持为 ListPanel 的直接子节点，沿用表体与页脚的尺寸契约。 -->
    <slot name="body">
      <el-table
        ref="table"
        v-loading="loading"
        height="100%"
        :data="error ? [] : rows"
        stripe
        @selection-change="$emit('selectionChange', $event)"
      >
        <slot />
        <template #empty>
          <KasTableState
            :error="error"
            :empty-text="emptyText"
            :error-text="errorText"
            @retry="$emit('retry')"
          />
        </template>
      </el-table>
    </slot>
    <div v-if="showPagination" class="table-pager">
      <Pagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        @update:page="$emit('update:page', $event)"
        @update:page-size="$emit('update:pageSize', $event)"
        @change="$emit('change')"
      />
    </div>
  </ListPanel>
</template>
