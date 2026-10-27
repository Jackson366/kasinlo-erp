<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useUserStore } from '@/store'
import { usePurchaseHandoff } from '@/composables/usePurchaseHandoff'
import AppIcon from '@/components/AppIcon.vue'

const { returnTo } = usePurchaseHandoff()
const user = useUserStore()
const router = useRouter()
</script>

<template>
  <router-link
    v-if="
      returnTo &&
      user.hasAnyPerm(['inv:purchase:list']) &&
      router.hasRoute('InvPurchase')
    "
    :to="returnTo"
    class="purchase-return"
  >
    <AppIcon name="ArrowLeft" />返回采购单
  </router-link>
</template>

<style scoped lang="scss">
.purchase-return {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--brand-primary);
  cursor: pointer;
  &:hover {
    color: var(--brand-primary-hover);
    text-decoration: underline;
  }
  &:active {
    color: var(--brand-primary-active);
  }
  &:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 3px;
  }
}
</style>
