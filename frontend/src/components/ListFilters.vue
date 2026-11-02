<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  useId,
  watch,
} from 'vue'
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
const common = ref<HTMLElement>()
const actionRow = ref<HTMLElement>()
const wrapped = ref(false)
let observer: ResizeObserver | undefined

function measureActionRow(): void {
  if (
    !common.value ||
    !actionRow.value ||
    !actionRow.value.getClientRects().length
  )
    return
  wrapped.value =
    actionRow.value.getBoundingClientRect().top >
    common.value.getBoundingClientRect().top + 2
}
onMounted(() => {
  observer = new ResizeObserver(measureActionRow)
  if (actionRow.value) {
    observer.observe(actionRow.value)
    if (actionRow.value.parentElement)
      observer.observe(actionRow.value.parentElement)
  }
  if (common.value) observer.observe(common.value)
  measureActionRow()
})
watch(expanded, () => void nextTick(measureActionRow))
onBeforeUnmount(() => observer?.disconnect())

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
    <div ref="common" class="list-filters__common"><slot /></div>
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
    <div
      ref="actionRow"
      class="list-filters__action-row"
      :class="{ 'is-wrapped': wrapped }"
    >
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
      <div v-if="$slots.tools" class="list-head__actions list-filters__tools">
        <slot name="tools" />
      </div>
    </div>
  </el-form>
</template>

<style scoped lang="scss">
.list-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  min-width: 0;
  &__common,
  &__advanced {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    flex: 0 0 auto;
    gap: 8px 12px;
    min-width: 0;
    max-width: 100%;
  }
  &__actions,
  &__tools {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    min-height: 32px;
    min-width: 0;
    max-width: 100%;
    flex: 0 0 auto;
  }
  &__action-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    flex: 1 0 auto;
    min-width: 0;
    max-width: 100%;
  }
  &__tools {
    margin-left: auto;
    justify-content: flex-end;
  }
  &__action-row.is-wrapped &__tools {
    margin-left: 0;
  }
  :deep(.el-form-item) {
    margin: 0;
    min-width: 0;
    flex: 0 1 220px;
    width: 220px;
    max-width: 100%;
  }
  &__common :deep(.el-form-item:first-child) {
    flex-basis: 300px;
    width: 300px;
  }
  // Date ranges need room for both dates; keep the exception in the shared owner.
  :deep(.el-form-item:has(.el-date-editor--daterange)) {
    flex-basis: 340px;
    width: 340px;
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
  .list-filters {
    &__common,
    &__advanced {
      flex-basis: 100%;
      width: 100%;
    }
    :deep(.el-form-item) {
      flex-basis: 100%;
      width: 100%;
    }
  }
}
</style>
