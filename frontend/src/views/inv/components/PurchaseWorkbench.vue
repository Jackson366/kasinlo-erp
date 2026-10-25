<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { InvOrderRow } from '@/types/api'
import { formatDay, formatMoney } from '@/utils/format'
import AppIcon from '@/components/AppIcon.vue'

const props = defineProps<{
  rows: InvOrderRow[]
  selectedId: number | null
  loading: boolean
  detailLoading: boolean
  detailError: boolean
  hasDetail: boolean
  filtered: boolean
  mobileDetail: boolean
}>()
const reviewPanel = ref<HTMLElement>()
const queuePanel = ref<HTMLElement>()
watch(
  () => props.mobileDetail,
  async (open) => {
    await nextTick()
    if (window.matchMedia('(max-width: 800px)').matches) {
      if (open) reviewPanel.value?.focus()
      else
        queuePanel.value
          ?.querySelector<HTMLButtonElement>('[aria-pressed="true"]')
          ?.focus()
    }
  },
)
defineEmits<{
  select: [row: InvOrderRow]
  retry: []
  reset: []
  back: []
}>()
</script>

<template>
  <div class="purchase-workbench" :class="{ 'is-detail-open': mobileDetail }">
    <aside ref="queuePanel" class="purchase-queue" aria-label="待处理采购单">
      <div class="purchase-queue__head">
        <strong>待办队列</strong>
        <span>选择单据开始核对</span>
      </div>
      <div
        v-loading="loading"
        class="purchase-queue__items"
        :aria-busy="loading"
      >
        <button
          v-for="row in rows"
          :key="row.id"
          type="button"
          class="purchase-queue__item"
          :class="{ 'is-selected': row.id === selectedId }"
          :aria-pressed="row.id === selectedId"
          :disabled="loading"
          @click="$emit('select', row)"
        >
          <span class="purchase-queue__item-top">
            <span class="purchase-queue__number">{{ row.orderNo }}</span>
            <span class="purchase-queue__status">待处理</span>
          </span>
          <strong class="purchase-queue__supplier">{{ row.partyName }}</strong>
          <span class="purchase-queue__item-bottom">
            <span
              >{{ formatDay(row.orderDate) }} · {{ row.itemCount }} 项明细</span
            >
            <strong>{{ formatMoney(row.totalAmount) }}</strong>
          </span>
        </button>
        <el-empty
          v-if="!loading && !rows.length"
          :image-size="64"
          :description="
            filtered ? '没有符合条件的采购单' : '当前没有待处理采购单'
          "
        >
          <el-button v-if="filtered" @click="$emit('reset')"
            >清除筛选</el-button
          >
        </el-empty>
      </div>
    </aside>

    <section
      ref="reviewPanel"
      tabindex="-1"
      class="purchase-review"
      aria-label="采购单核对"
      :aria-busy="detailLoading"
    >
      <div class="purchase-review__heading">
        <el-button class="purchase-review__back" @click="$emit('back')">
          <AppIcon name="ArrowLeft" />返回待办
        </el-button>
        <span class="purchase-review__eyebrow">采购核对</span>
        <span>确认采购事实，再完成入库</span>
      </div>
      <div v-loading="detailLoading" class="purchase-review__body">
        <div v-if="detailError" class="purchase-review__error" role="alert">
          <h3>采购单加载失败</h3>
          <p>请重新加载后再核对，避免使用不完整的信息。</p>
          <el-button @click="$emit('retry')">重新加载</el-button>
        </div>
        <slot v-else-if="hasDetail && !detailLoading" />
        <el-empty
          v-else-if="!detailLoading"
          :image-size="88"
          description="选择一张采购单开始核对"
        />
      </div>
    </section>
  </div>
</template>

<style scoped lang="scss">
.purchase-workbench {
  display: grid;
  grid-template-columns: 296px minmax(0, 1fr);
  align-items: start;
  gap: 24px;
}
.purchase-queue {
  min-width: 0;
  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 0 12px;
    strong {
      font-size: 14px;
      font-weight: 600;
    }
    > span {
      font-size: 12px;
      color: var(--app-text-secondary);
    }
  }
  &__items {
    min-height: 240px;
    max-height: 680px;
    overflow-y: auto;
    scrollbar-gutter: stable;
    display: grid;
    align-content: start;
    grid-auto-rows: max-content;
    gap: 10px;
    padding: 3px 6px 3px 3px;
  }
  &__item {
    appearance: none;
    width: 100%;
    padding: 16px;
    text-align: left;
    font: inherit;
    color: var(--app-text);
    background: var(--app-surface-subtle);
    border: 1px solid var(--app-border-light);
    border-radius: var(--radius-md);
    cursor: pointer;
    &:hover {
      background: var(--app-fill-hover);
      border-color: var(--app-primary-soft-border);
    }
    &:focus-visible {
      outline: 2px solid var(--brand-primary);
      outline-offset: 3px;
    }
    &:active {
      background: var(--app-primary-soft);
    }
    &:disabled {
      opacity: 0.6;
      cursor: wait;
    }
    &.is-selected {
      background: var(--app-primary-soft);
      border-color: var(--brand-primary);
      box-shadow: inset 3px 0 var(--brand-primary);
    }
  }
  &__item-top,
  &__item-bottom {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }
  &__number {
    font-size: 12px;
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }
  &__status {
    flex: none;
    color: var(--tone-amber);
    font-size: 12px;
  }
  &__supplier {
    display: block;
    margin: 10px 0 16px;
    font-size: 15px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  &__item-bottom {
    font-size: 12px;
    color: var(--app-text-secondary);
    strong {
      font-size: 14px;
      color: var(--app-text);
      font-weight: 600;
      font-variant-numeric: tabular-nums;
    }
  }
}
.purchase-review {
  min-width: 0;
  border-left: 1px solid var(--app-border-light);
  padding-left: 24px;
  &__heading {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
    color: var(--app-text-secondary);
    font-size: 12px;
    flex-wrap: wrap;
  }
  &__eyebrow {
    color: var(--app-text);
    font-weight: 600;
    font-size: 14px;
  }
  &__back {
    display: none;
  }
  &__body {
    min-height: 360px;
  }
  &__error {
    padding: 48px 16px;
    text-align: center;
    h3 {
      font-size: 16px;
    }
    p {
      color: var(--app-text-secondary);
      line-height: 1.6;
    }
  }
}
@media (max-width: 1100px) {
  .purchase-workbench {
    grid-template-columns: 248px minmax(0, 1fr);
    gap: 16px;
  }
  .purchase-review {
    padding-left: 16px;
  }
  .purchase-queue__item {
    padding: 12px;
  }
  .purchase-queue__item-bottom {
    flex-wrap: wrap;
  }
}
@media (max-width: 800px) {
  .purchase-workbench {
    display: block;
  }
  .purchase-queue__items {
    max-height: none;
  }
  .purchase-review {
    display: none;
    padding: 0;
    border: 0;
  }
  .is-detail-open {
    .purchase-queue {
      display: none;
    }
    .purchase-review {
      display: block;
    }
  }
  .purchase-review__back {
    display: inline-flex;
  }
}
</style>
