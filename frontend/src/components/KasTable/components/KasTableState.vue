<script setup lang="ts">
defineProps<{ error: boolean; emptyText: string; errorText: string }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div v-if="error" class="kas-table-state" role="alert">
    <p>{{ errorText }}</p>
    <el-button @click="$emit('retry')">重新加载</el-button>
  </div>
  <el-empty v-else :description="emptyText" :image-size="64" />
</template>

<style scoped lang="scss">
.kas-table-state {
  padding: 24px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}
</style>
