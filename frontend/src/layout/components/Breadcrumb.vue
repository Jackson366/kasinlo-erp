<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { usePermissionStore } from '@/store'
import type { RouteRecord } from '@/types/api'

const route = useRoute()
const permissionStore = usePermissionStore()

/** 在菜单树中查找当前路径的完整链路（含目录层级） */
function findTrail(records: RouteRecord[], path: string): RouteRecord[] {
  for (const record of records) {
    if (record.path === path && !record.children?.length) {
      return [record]
    }
    if (record.children?.length) {
      const sub = findTrail(record.children, path)
      if (sub.length) return [record, ...sub]
    }
  }
  return []
}

const items = computed(() => {
  const trail = findTrail(permissionStore.menus, route.path)
  if (trail.length) {
    return trail.map((record) => ({ title: record.meta?.title || record.name, path: record.path }))
  }
  // 未匹配到菜单时退化为当前路由标题（如个人中心）
  const title = (route.meta?.title as string) || ''
  return title ? [{ title, path: route.path }] : []
})
</script>

<template>
  <el-breadcrumb separator="/" class="breadcrumb">
    <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
    <el-breadcrumb-item v-for="(item, index) in items" :key="item.path">
      <span :class="{ 'is-current': index === items.length - 1 }">{{ item.title }}</span>
    </el-breadcrumb-item>
  </el-breadcrumb>
</template>

<style scoped lang="scss">
.breadcrumb {
  font-size: 13px;

  :deep(.el-breadcrumb__inner) {
    color: var(--app-text-muted);
    font-weight: 400;
  }

  :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
    color: var(--app-text);
    font-weight: 600;
  }
}

.is-current {
  color: var(--app-text);
}
</style>
