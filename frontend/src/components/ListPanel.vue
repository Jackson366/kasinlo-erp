<script setup lang="ts">
import { useKasTableLayout } from './KasTable/hooks/useKasTableLayout'

const props = withDefaults(defineProps<{ fit?: boolean; surface?: boolean }>(), {
  fit: true,
  surface: true,
})
const { panel, height, minimum } = useKasTableLayout(() => props.fit)
</script>

<template>
  <div
    ref="panel"
    class="list-panel"
    :class="{ 'page-card': surface, 'list-panel--fit': fit }"
    :style="fit ? { height, minHeight: minimum } : undefined"
  >
    <slot />
  </div>
</template>

<style lang="scss">
.list-panel--fit {
  display: flex;
  flex-direction: column;
  min-width: 0;
  > * {
    flex: none;
  }
  > .el-table {
    flex: 1 1 0;
    min-height: 200px;
    width: 100%;
  }
  &.list-panel > .table-pager {
    margin-top: 0;
    padding-top: 12px;
    min-height: 48px;
    border-top: 1px solid var(--app-border-light);
    background: var(--app-surface);
  }
}
</style>
