<script setup lang="ts">
// 客户管理页：复用往来单位页面主体。
import PartyPage from '../components/PartyPage.vue'

defineOptions({ name: 'InvCustomer' })
</script>

<template>
  <PartyPage
    biz-type="sales"
    title="客户管理"
    subtitle="客户档案与信用管理"
    create-text="新增客户"
    balance-label="应收余额"
  />
</template>
