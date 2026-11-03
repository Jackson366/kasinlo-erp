<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
  unit: string
  resultLabel: string
  total: number
  selectedCount: number
}>()
defineEmits<{ clearSelection: [] }>()
</script>

<template>
  <div class="list-head">
    <span class="list-head__title">
      {{ title }}<em>{{ subtitle ? `${subtitle} · ` : '' }}{{ resultLabel }} {{ total }} {{ unit }}</em>
      <em v-if="selectedCount">
        已选 {{ selectedCount }} 项
        <el-button link type="primary" @click="$emit('clearSelection')">清空</el-button>
      </em>
    </span>
    <slot />
  </div>
</template>
