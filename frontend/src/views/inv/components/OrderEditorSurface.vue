<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

const props = defineProps<{
  modelValue: boolean
  title: string
}>()
const heading = ref<HTMLElement>()
watch(
  () => props.modelValue,
  async (visible) => {
    if (visible) {
      await nextTick()
      heading.value?.focus()
    }
  },
  { immediate: true },
)
</script>

<template>
  <section
    v-if="modelValue"
    class="purchase-editor"
    role="region"
    :aria-label="title"
  >
    <header class="purchase-editor__header">
      <div ref="heading" tabindex="-1" class="purchase-editor__heading">
        <slot
          name="header"
          title-id="purchase-editor-title"
          title-class="purchase-editor__title"
        />
      </div>
    </header>
    <slot />
    <footer class="purchase-editor__footer"><slot name="footer" /></footer>
  </section>
</template>

<style scoped lang="scss">
.purchase-editor {
  max-width: 1440px;
  margin: 0 auto;
  color: var(--app-text);
  &__header {
    padding: 0 20px;
    margin-bottom: 16px;
  }
  &__heading {
    min-width: 0;
  }
  // 单行大标题不需要正文的 1.6 行高：字形下方会空出半个字，
  // 把标题和下面的单号摘要撑散。1.4 让两行读成一个块。
  :deep(.purchase-editor__title) {
    margin: 0;
    font-size: 18px;
    font-weight: 600;
    line-height: 1.4;
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
    margin-top: 16px;
    padding: 12px 20px;
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--radius-md);
  }
  &__footer :deep(.el-button + .el-button) {
    margin-left: 0;
  }
}
@media (max-width: 800px) {
  .purchase-editor__header {
    padding: 0 16px;
  }
  .purchase-editor__footer {
    padding: 12px 16px;
  }
}
</style>
