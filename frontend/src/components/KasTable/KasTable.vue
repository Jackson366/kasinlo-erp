<script setup lang="ts" generic="Row extends object">
import { ref } from 'vue'
import type { TableInstance } from 'element-plus'
import ListPanel from '../ListPanel.vue'
import Pagination from '../Pagination.vue'
import KasTableToolbar from './components/KasTableToolbar.vue'
import KasTableState from './components/KasTableState.vue'
import type { KasTableProps } from './types'

withDefaults(defineProps<KasTableProps<Row>>(), { selectedCount: 0 })
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
  <ListPanel class="kas-table">
    <KasTableToolbar
      :title="title"
      :subtitle="subtitle"
      :total="total"
      :selected-count="selectedCount"
      @clear-selection="$emit('clearSelection')"
    >
      <slot name="filters" />
    </KasTableToolbar>
    <!-- 保持为 ListPanel 的直接子节点，沿用表体与页脚的尺寸契约。 -->
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
        <KasTableState :error="error" @retry="$emit('retry')" />
      </template>
    </el-table>
    <div class="table-pager">
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
