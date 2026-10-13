<script setup lang="ts">
// 销售管理页：复用单据页面主体，传入销售业务线。
import OrderPage from '../components/OrderPage.vue'

defineOptions({ name: 'InvSales' })
</script>

<template>
  <OrderPage biz-type="sales" title="销售单据" create-text="新增销售单" party-label="客户" />
</template>
