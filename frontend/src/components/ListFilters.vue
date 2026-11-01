<script setup lang="ts">
import { ref, useId } from 'vue'
import type { ActiveFilter } from '@/composables/useAdvancedFilters'
import AppIcon from './AppIcon.vue'

withDefaults(defineProps<{ activeFilters?: ActiveFilter[] }>(), {
  activeFilters: () => [],
})
const emit = defineEmits<{ clear: [key: string] }>()
const expanded = ref(false)
const panelId = `list-filters-${useId()}`
const toggleButton = ref<{ $el: HTMLButtonElement }>()

function clearFilter(key: string): void {
  emit('clear', key)
  // 标签会在本次更新中移除，把键盘位置留在可继续操作的入口。
  toggleButton.value?.$el.focus()
}
</script>

<template>
  <el-form class="filter-form list-filters" label-width="76px" novalidate @submit.prevent>
    <div class="list-filters__primary">
      <div class="list-filters__fields"><slot /></div>
      <div class="list-filters__actions">
        <slot name="actions" />
        <el-button
          ref="toggleButton"
          text
          :type="activeFilters.length ? 'primary' : 'default'"
          :aria-expanded="expanded"
          :aria-controls="panelId"
          @click="expanded = !expanded"
        >
          {{ expanded ? '收起筛选' : '更多筛选'
          }}<span v-if="activeFilters.length">（{{ activeFilters.length }}）</span>
          <AppIcon
            name="ArrowDown"
            class="list-filters__arrow"
            :class="{ 'is-expanded': expanded }"
          />
        </el-button>
      </div>
    </div>
    <div
      :id="panelId"
      v-show="expanded"
      class="list-filters__advanced"
      role="group"
      aria-label="更多筛选条件"
    >
      <slot name="advanced" />
    </div>
    <div
      v-if="!expanded && activeFilters.length"
      class="list-filters__selection"
      aria-label="已选的更多筛选条件"
    >
      <span class="list-filters__selection-label">已选条件</span>
      <button
        v-for="filter in activeFilters"
        :key="filter.key"
        type="button"
        class="list-filters__chip"
        :aria-label="`清除${filter.label}`"
        @click="clearFilter(filter.key)"
      >
        {{ filter.label }}<AppIcon name="Close" />
      </button>
    </div>
  </el-form>
</template>

<style scoped lang="scss">
.filter-form.list-filters {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  grid-column: 1 / -1;
  gap: 12px;
  min-width: 0;
  margin: 0;
  padding: 0;

  .list-filters__primary {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 12px 16px;
  }

  .list-filters__fields {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr));
    flex: 1 1 480px;
    max-width: 720px;
    gap: 10px 16px;
    min-width: 0;
  }

  .list-filters__actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    min-height: 32px;
    :deep(.el-button + .el-button) {
      margin-left: 0;
    }
  }

  .list-filters__advanced {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 280px), 1fr));
    gap: 12px 20px;
    padding-top: 12px;
    border-top: 1px solid var(--app-border-light);
  }

  :deep(.el-form-item) {
    margin: 0;
    min-width: 0;
  }
  :deep(.el-form-item__label) {
    flex-shrink: 0;
  }
  :deep(.el-form-item__content) {
    min-width: 0;
  }
  :deep(.el-input),
  :deep(.el-select),
  :deep(.el-date-editor) {
    width: 100%;
    min-width: 0;
  }
}

.list-filters__arrow.is-expanded {
  transform: rotate(180deg);
}
.list-filters__selection {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 8px;
  font-size: 12px;
}
.list-filters__selection-label {
  color: var(--app-text-secondary);
}
.list-filters__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 4px 8px;
  font: inherit;
  text-align: left;
  overflow-wrap: anywhere;
  color: var(--app-text);
  background: var(--app-surface-subtle);
  border: 1px solid var(--app-border);
  border-radius: var(--radius-sm);
  cursor: pointer;
  .app-icon {
    flex-shrink: 0;
  }
  &:hover {
    color: var(--brand-primary);
    border-color: var(--brand-primary);
  }
  &:active {
    background: var(--app-primary-soft);
  }
  &:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 2px;
  }
}
</style>
