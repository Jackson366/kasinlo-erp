<script setup lang="ts">
import { nextTick, onBeforeUnmount, onDeactivated, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import AppIcon from '@/components/AppIcon.vue'
import { usePurchaseGuideStore } from '@/store/purchaseGuide'
import { purchaseTourSteps, type GuideOrder, type PurchaseTourStep } from './purchaseGuidePresentation'

const props = defineProps<{ order?: GuideOrder | null; entering?: boolean; editing?: boolean; busy?: boolean; failed?: boolean }>()
const guide = usePurchaseGuideStore()
const root = ref<HTMLElement>()
const toggle = ref<{ $el: HTMLButtonElement }>()
// Open state is deliberately local: saved preferences never auto-open a tour.
const open = ref(false)
const current = ref(0)
const steps = ref<(PurchaseTourStep & { target?: HTMLElement })[]>([])

async function start(): Promise<void> {
  const scope = root.value?.closest('.el-dialog, .order-page, .purchase-start')
  const visibleTarget = (selector: string) => {
    const element = scope?.querySelector<HTMLElement>(selector)
    return element?.getClientRects().length ? element : undefined
  }
  const overview = scope?.classList.contains('purchase-start') ?? false
  steps.value = purchaseTourSteps(props.order, props.entering, props.editing, overview)
    .flatMap(step => {
      const target = visibleTarget(step.anchor)
      return target ? [{ ...step, target }] : []
    })
  if (!steps.value.length || props.busy || props.failed) {
    steps.value = [{
      anchor: '', title: props.busy ? '正在读取单据' : props.failed ? '请先重新加载单据' : '从实际采购开始',
      description: props.busy || props.failed
        ? '确认单据状态后，再从这里重新开启操作引导。'
        : '当前没有可引导的操作区域。请先选择单据；只有具备权限的操作才会显示。',
    }]
  }
  current.value = 0
  open.value = true
}

async function close(): Promise<void> {
  open.value = false
  if (!guide.setMode('closed'))
    ElMessage.warning('本次已退出，但浏览器未能记住，下次进入可能再次显示首次入口。')
  await nextTick()
  toggle.value?.$el.focus()
}

// A changing record invalidates old targets and guidance, including async refreshes.
watch(() => [props.order?.orderNo, props.order?.status, props.order?.settleStatus, props.busy, props.failed], () => {
  if (open.value) void close()
})
onDeactivated(() => { if (open.value) void close() })
onBeforeUnmount(() => { open.value = false })
</script>

<template>
  <span ref="root" class="purchase-guide">
    <el-button ref="toggle" text size="small" :aria-expanded="open" aria-haspopup="dialog" @click="start">
      <AppIcon name="InfoFilled" />{{ guide.mode === 'offered' ? '开始引导' : '采购操作指南' }}
    </el-button>
    <el-tour
      v-if="open"
      v-model="open"
      v-model:current="current"
      class="purchase-tour"
      role="dialog"
      aria-modal="true"
      aria-label="采购操作引导"
      :show-close="true"
      :target-area-clickable="false"
      :content-style="{ width: 'min(360px, calc(100vw - 32px))', maxHeight: 'calc(100dvh - 32px)', overflowY: 'auto' }"
      :scroll-into-view-options="{ block: 'center', behavior: 'instant' }"
      @close="close"
    >
      <template #indicators="{ current: index, total }">
        <span class="purchase-tour__count" aria-live="polite" :aria-label="`引导第 ${index + 1} 步，共 ${total} 步`">{{ index + 1 }}/{{ total }}</span>
      </template>
      <el-tour-step
        v-for="(step, index) in steps" :key="step.anchor || 'fallback'"
        :target="step.target" :title="step.title"
        :prev-button-props="{ children: '上一步', text: true }"
        :next-button-props="{ children: index === steps.length - 1 ? '结束引导' : '下一步' }"
      >
        <p>{{ step.description }}</p>
        <p v-if="step.note" class="purchase-tour__note">{{ step.note }}</p>
      </el-tour-step>
    </el-tour>
  </span>
</template>

<style scoped lang="scss">
.purchase-guide {
  display: inline-flex;
  flex: none;
  vertical-align: middle;
  :deep(.el-button) { color: var(--app-text-secondary); }
  :deep(.el-button:hover) { color: var(--brand-primary); }
  :deep(.el-button:focus-visible) { outline: 2px solid var(--brand-primary); outline-offset: 2px; }
}
/* ElTour teleports its content to body; scope the shared-library overrides by owner. */
:global(.purchase-tour .el-tour__content) {
  background: var(--app-surface-overlay);
  color: var(--app-text);
  border: 1px solid var(--app-border);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-popup);
}
:global(.purchase-tour .el-tour__title) { font-size: 14px; font-weight: 600; }
:global(.purchase-tour p) { margin: 0; font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
:global(.purchase-tour .purchase-tour__note) { margin-top: 8px; color: var(--app-text-secondary); }
:global(.purchase-tour .purchase-tour__count) { color: var(--app-text-secondary); font-size: 12px; font-variant-numeric: tabular-nums; }
:global(.purchase-tour .el-tour__closebtn:focus-visible) { outline: 2px solid var(--brand-primary); outline-offset: 2px; }
@media (prefers-reduced-motion: reduce) {
  /* Disable library transitions as well as app-owned motion. */
  :global(.purchase-tour *) { transition: none !important; animation: none !important; }
}
</style>
