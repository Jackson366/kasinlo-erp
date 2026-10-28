<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

const props = defineProps<{
  modelValue: boolean
  page: boolean
  title: string
  busy: boolean
  beforeClose: (done: () => void) => void
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; back: [] }>()
const heading = ref<HTMLElement>()
watch(
  () => props.modelValue,
  async (visible) => {
    if (visible && props.page) {
      await nextTick()
      heading.value?.focus()
    }
  },
  { immediate: true },
)
</script>

<template>
  <section
    v-if="page && modelValue"
    class="purchase-editor"
    role="region"
    :aria-label="title"
  >
    <header class="purchase-editor__header">
      <el-button text :disabled="busy" @click="emit('back')"
        >返回采购工作台</el-button
      >
      <div ref="heading" tabindex="-1" class="purchase-editor__heading">
        <slot
          name="header"
          title-id="purchase-editor-title"
          title-class="purchase-editor__title"
        />
      </div>
    </header>
    <div class="purchase-editor__body"><slot /></div>
    <footer class="purchase-editor__footer"><slot name="footer" /></footer>
  </section>
  <el-dialog
    v-else-if="!page"
    :model-value="modelValue"
    :title="title"
    width="1280px"
    top="6vh"
    :close-on-click-modal="false"
    :before-close="beforeClose"
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
  >
    <slot />
    <template #footer><slot name="footer" /></template>
  </el-dialog>
</template>

<style scoped lang="scss">
.purchase-editor {
  max-width: 1440px;
  margin: 0 auto;
  color: var(--app-text);
  &__header {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px 24px;
    margin-bottom: 16px;
  }
  &__heading {
    min-width: 0;
  }
  :deep(.purchase-editor__title) {
    margin: 0;
    font-size: 18px;
    font-weight: 600;
    line-height: 1.6;
  }
  &__body {
    padding: 20px;
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--radius-md);
  }
  &__footer {
    position: sticky;
    bottom: 0;
    z-index: 5;
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    align-items: center;
    gap: 12px;
    padding: 12px 20px;
    background: var(--app-surface);
    border-top: 1px solid var(--app-border);
  }
  &__footer :deep(.el-button + .el-button) {
    margin-left: 0;
  }
}
@media (max-width: 800px) {
  .purchase-editor__body {
    padding: 16px;
  }
  .purchase-editor__footer {
    padding: 12px 16px;
  }
}
</style>
