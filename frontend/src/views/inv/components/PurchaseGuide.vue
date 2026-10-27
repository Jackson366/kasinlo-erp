<script setup lang="ts">
import { computed, nextTick, ref, useId } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import AppIcon from '@/components/AppIcon.vue'
import { usePurchaseGuideStore } from '@/store/purchaseGuide'
import { purchaseGuidePresentation, type GuideOrder } from './purchaseGuidePresentation'

const props = defineProps<{ order?: GuideOrder | null; entering?: boolean; busy?: boolean; failed?: boolean }>()
const guide = usePurchaseGuideStore()
const contentId = useId()
const toggle = ref<{ $el: HTMLButtonElement }>()
const current = computed(() => purchaseGuidePresentation(props.order, props.entering))
const steps = [
  { title: '录入采购', description: '保存单据，尚未入库' },
  { title: '核对并入库', description: '确认收货后审核，形成应付' },
  { title: '登记付款', description: '按实际付款结算未付金额' },
]
async function change(active: boolean): Promise<void> {
  const persisted = guide.setMode(active ? 'active' : 'closed')
  if (!active)
    ElMessage.info('已退出引导，当前输入仍保留。可从「采购操作指南」重新开启。')
  if (!persisted)
    ElMessage.warning('本次选择已生效，但浏览器未能记住，下次进入可能再次显示。')
  await nextTick()
  toggle.value?.$el.focus()
}
</script>

<template>
  <section class="purchase-guide" :class="`is-${guide.mode}`" aria-label="采购操作指南">
    <header class="purchase-guide__header">
      <div v-if="guide.mode !== 'closed'" class="purchase-guide__heading">
        <span class="purchase-guide__icon"><AppIcon name="Document" /></span>
        <div>
          <h2>采购操作指南</h2>
          <p v-if="guide.mode === 'offered'">了解三个办理环节，随时可以退出。</p>
          <p v-else>{{ entering ? '当前录入' : order ? `当前单据 · ${order.orderNo}` : '录单 → 入库 → 付款' }}</p>
        </div>
      </div>
      <div class="purchase-guide__actions">
        <el-button
          ref="toggle"
          :type="guide.mode === 'active' ? 'default' : 'primary'"
          :text="guide.mode !== 'offered'"
          size="small"
          :aria-expanded="guide.mode === 'active'"
          :aria-controls="guide.mode === 'active' ? contentId : undefined"
          @click="change(guide.mode !== 'active')"
        >
          <AppIcon v-if="guide.mode === 'closed'" name="InfoFilled" />
          {{ guide.mode === 'active' ? '退出引导' : guide.mode === 'offered' ? '开始引导' : '采购操作指南' }}
        </el-button>
        <el-button v-if="guide.mode === 'offered'" text size="small" @click="change(false)">直接办理</el-button>
      </div>
    </header>
    <div v-if="guide.mode === 'active'" :id="contentId" class="purchase-guide__content" :aria-busy="busy">
      <div class="purchase-guide__progress-heading" aria-live="polite">
        <span>{{ busy ? '正在核验当前单据…' : failed ? '单据状态未能加载' : current.label }}</span>
        <span v-if="!busy && !failed && current.measurable">已完成 {{ current.completed }} / 3 个环节</span>
        <span v-else-if="!busy && !failed">三个办理环节</span>
      </div>
      <el-progress
        v-if="!busy && !failed && current.measurable"
        :percentage="Math.round(current.completed / 3 * 100)"
        :show-text="false"
        :stroke-width="4"
        stroke-linecap="square"
        :status="current.completed === 3 ? 'success' : undefined"
        aria-label="当前采购单办理进度"
        :aria-valuetext="`已完成 ${current.completed} / 3 个环节`"
      />
      <el-steps
        class="purchase-guide__steps"
        :active="!busy && !failed && current.measurable ? current.completed : -1"
        finish-status="success"
        role="list"
        aria-label="采购办理顺序"
      >
        <el-step v-for="(step, index) in steps" :key="step.title"
          :title="step.title" :description="step.description" role="listitem"
          :aria-current="!busy && !failed && current.measurable && current.completed === index ? 'step' : undefined"
        />
      </el-steps>
      <div class="purchase-guide__instruction" aria-live="polite">
        <template v-if="busy || failed">
          <strong>{{ busy ? '请稍候，正在读取单据状态' : '请先重新加载单据' }}</strong>
          <p>状态确认前不显示办理进度，请使用下方单据区的重新加载入口。</p>
        </template>
        <template v-else>
          <strong>{{ current.title }}</strong>
          <p>{{ current.instruction }}</p>
          <p class="purchase-guide__consequence"><AppIcon name="InfoFilled" />{{ current.consequence }}</p>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped lang="scss">
.purchase-guide {
  margin-bottom: 16px;
  padding: 16px 20px;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-left: 3px solid var(--brand-primary);
  border-radius: var(--radius-sm);
  min-width: 0;
  &.is-closed { padding: 0; border: 0; background: transparent; }
  &__header, &__heading, &__actions { display: flex; align-items: center; gap: 12px; }
  &__header { justify-content: space-between; gap: 12px 24px; flex-wrap: wrap; }
  &__heading { min-width: 0; }
  &__heading > div { min-width: 0; }
  &__icon { display: grid; place-items: center; width: 32px; height: 32px; flex: none; color: var(--brand-primary); background: var(--app-primary-soft); }
  &__icon .app-icon { font-size: 18px; }
  h2 { margin: 0; color: var(--app-text); font-size: 14px; line-height: 1.5; font-weight: 600; }
  p { margin: 4px 0 0; font-size: 13px; line-height: 1.6; color: var(--app-text-secondary); overflow-wrap: anywhere; }
  &__heading p { font-size: 12px; }
  &__actions { gap: 4px; flex: none; }
  :deep(.el-button + .el-button) { margin-left: 0; }
  :deep(.el-button:focus-visible) { outline: 2px solid var(--brand-primary); outline-offset: 3px; }
  &__content { margin-top: 20px; }
  &__progress-heading { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 4px 16px; margin-bottom: 8px; font-size: 12px; color: var(--app-text-secondary); }
  &__progress-heading > span:first-child { color: var(--app-text); font-weight: 500; }
  &__steps { margin-top: 20px; }
  :deep(.el-step__title) { font-size: 13px; line-height: 24px; font-weight: 500; }
  :deep(.el-step__description) { padding-right: 16px; font-size: 12px; line-height: 1.6; }
  :deep(.el-step__head.is-wait), :deep(.el-step__title.is-wait), :deep(.el-step__description.is-wait) { color: var(--app-text-secondary); border-color: var(--app-border); }
  :deep(.el-step__head.is-process), :deep(.el-step__title.is-process) { color: var(--brand-primary); border-color: var(--brand-primary); }
  &__instruction { margin-top: 20px; padding: 12px 16px; background: var(--app-surface-subtle); border-radius: var(--radius-sm); }
  &__instruction strong { color: var(--app-text); font-size: 14px; font-weight: 600; }
  &__consequence { display: flex; gap: 6px; align-items: baseline; }
  &__consequence .app-icon { flex: none; color: var(--app-info); }
}
@media (max-width: 800px) {
  .purchase-guide {
    padding: 12px;
    &__steps { display: block; }
    :deep(.el-step.is-horizontal) { display: block; width: 100%; padding-bottom: 16px; }
    :deep(.el-step__head) { float: left; width: 24px; }
    :deep(.el-step__main) { padding-left: 36px; }
    :deep(.el-step__line) { display: none; }
    :deep(.el-step__description) { padding-right: 0; }
    &__instruction { margin-top: 0; padding: 12px; }
  }
}
@media (prefers-reduced-motion: reduce) {
  /* Disable Element Plus progress/step transitions for reduced motion. */
  .purchase-guide :deep(*) { transition: none !important; animation: none !important; }
}
</style>
