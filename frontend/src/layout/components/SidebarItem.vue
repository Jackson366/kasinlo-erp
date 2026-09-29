<script setup lang="ts">
import { computed } from 'vue'
import type { RouteRecord } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'

// 递归渲染菜单：目录 -> el-sub-menu，叶子 -> el-menu-item
const props = defineProps<{ item: RouteRecord }>()

const visibleChildren = computed(() => (props.item.children ?? []).filter((child) => !child.meta?.hidden))
const hasChildren = computed(() => visibleChildren.value.length > 0)
</script>

<template>
  <el-sub-menu v-if="hasChildren" :index="item.path || String(item.id)">
    <template #title>
      <AppIcon :name="item.meta?.icon" />
      <span>{{ item.meta?.title }}</span>
    </template>
    <SidebarItem v-for="child in visibleChildren" :key="child.id" :item="child" />
  </el-sub-menu>

  <el-menu-item v-else :index="item.path">
    <AppIcon :name="item.meta?.icon" />
    <template #title>
      <span>{{ item.meta?.title }}</span>
    </template>
  </el-menu-item>
</template>
