<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { usePurchaseContext } from '@/composables/usePurchaseContext'
const route = useRoute()
const router = useRouter()
const { purchaseId, purchaseNo, returnTarget } = usePurchaseContext()
</script>
<template>
  <div v-if="purchaseId" class="purchase-context">
    <span
      >来源采购单 <strong>{{ purchaseNo || purchaseId }}</strong></span
    >
    <div class="purchase-context__actions">
      <router-link :to="{ path: route.path, query: { view: route.query.view } }"
        >查看全部记录</router-link
      >
      <router-link v-if="router.hasRoute('InvPurchase')" :to="returnTarget">返回采购单</router-link>
    </div>
  </div>
</template>
<style scoped lang="scss">
.purchase-context {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 24px;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  padding: 12px 16px;
  background: var(--app-surface-subtle);
  border-radius: var(--radius-md);
  color: var(--app-text-secondary);
  font-size: 13px;
  &__actions {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
  }
  strong {
    color: var(--app-text);
    font-weight: 500;
    overflow-wrap: anywhere;
  }
  a {
    color: var(--brand-primary);
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  a:hover {
    color: var(--app-text);
  }
  a:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 3px;
  }
  a:active {
    opacity: 0.8;
  }
}
</style>
