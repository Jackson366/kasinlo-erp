<script setup lang="ts">
// 应用根组件：只保留一个出口，布局逻辑放在 layout/index.vue
// locale 从 app.use(ElementPlus) 迁到 el-config-provider（按需引入后的官方姿势）
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>

<template>
  <el-config-provider :locale="zhCn">
    <router-view />
  </el-config-provider>
</template>
