<script setup lang="ts">
// 供应商管理页：复用往来单位页面主体。
import PartyPage from '../components/PartyPage.vue'

defineOptions({ name: 'InvSupplier' })
</script>

<template>
  <PartyPage
    biz-type="purchase"
    title="供应商管理"
    subtitle="供应商档案与采购协作"
    create-text="新增供应商"
    balance-label="应付余额"
  />
</template>
