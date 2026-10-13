<script setup lang="ts">
// 采购管理页：复用单据页面主体，传入采购业务线。
import OrderPage from '../components/OrderPage.vue'

defineOptions({ name: 'InvPurchase' })
</script>

<template>
  <OrderPage biz-type="purchase" title="采购单据" create-text="新增采购单" party-label="供应商" />
</template>
