<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import AppIcon from './AppIcon.vue'

const props = withDefaults(defineProps<{ activeValues?: unknown[] }>(), {
  activeValues: () => [],
})
const activeCount = computed(
  () =>
    props.activeValues.filter((value) =>
      Array.isArray(value)
        ? value.length > 0
        : value !== undefined && value !== null && value !== '',
    ).length,
)
const expanded = ref(false)
const advancedId = useId()
const toggle = ref<HTMLButtonElement>()

function collapse(event: KeyboardEvent): void {
  if (event.key !== 'Escape' || !expanded.value) return
  event.stopPropagation()
  expanded.value = false
  toggle.value?.focus()
}
</script>

<template>
  <el-form
    class="list-filters"
    label-width="auto"
    novalidate
    @submit.prevent
    @keydown="collapse"
  >
    <div class="list-filters__common"><slot /></div>
    <div class="list-filters__actions">
      <slot name="actions" />
      <button
        v-if="$slots.advanced"
        ref="toggle"
        type="button"
        class="list-filters__toggle"
        :aria-expanded="expanded"
        :aria-controls="advancedId"
        @click="expanded = !expanded"
      >
        {{ expanded ? '收起筛选' : '更多筛选'
        }}{{ activeCount ? `（${activeCount}）` : '' }}
        <AppIcon :name="expanded ? 'ArrowUp' : 'ArrowDown'" />
      </button>
    </div>
    <div
      v-if="$slots.advanced"
      v-show="expanded"
      :id="advancedId"
      class="list-filters__advanced"
      role="group"
      aria-label="更多筛选条件"
    >
      <slot name="advanced" />
    </div>
  </el-form>
</template>

<style scoped lang="scss">
.list-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 12px 16px;
  min-width: 0;
  &__common,
  &__advanced {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 260px), 1fr));
    gap: 12px 16px;
    min-width: 0;
  }
  &__common {
    flex: 1 1 500px;
  }
  &__advanced {
    flex: 1 0 100%;
    padding-top: 12px;
    border-top: 1px solid var(--app-border-light);
  }
  &__actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    min-height: 32px;
    margin-left: auto;
  }
  :deep(.el-form-item) {
    margin: 0;
    min-width: 0;
  }
  :deep(.el-form-item__label) {
    flex: none;
    padding-right: 8px;
  }
  :deep(.el-form-item__content) {
    min-width: 0;
  }
  :deep(.el-input),
  :deep(.el-select),
  :deep(.el-date-editor),
  :deep(.el-tree-select) {
    width: 100%;
    min-width: 0;
  }
  :deep(.el-button + .el-button) {
    margin-left: 0;
  }
  &__toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-height: 32px;
    padding: 0 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--app-text-secondary);
    font: inherit;
    font-size: 13px;
    cursor: pointer;
    &:hover {
      color: var(--brand-primary);
      background: var(--app-fill-hover);
    }
    &:active {
      background: var(--app-primary-soft);
    }
    &:focus-visible {
      outline: 2px solid var(--brand-primary);
      outline-offset: 2px;
    }
  }
}
@media (max-width: 600px) {
  .list-filters__actions {
    margin-left: 0;
  }
}
</style>
