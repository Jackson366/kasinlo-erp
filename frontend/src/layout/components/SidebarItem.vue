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

  <!-- 外链不能走 el-menu-item：el-menu 开了 router 模式，点击会把外链地址
       当作内部路径 push，得到一个 404。外链没有注册内部路由（buildRoutes 跳过），
       这里用原生 <a> 新窗口打开；挂 el-menu-item 类是为了复用 Sidebar 的菜单项样式。 -->
  <a
    v-else-if="item.meta?.isExternal"
    class="el-menu-item sidebar-item__external"
    :href="item.path"
    target="_blank"
    rel="noopener noreferrer"
  >
    <AppIcon :name="item.meta?.icon" />
    <span class="sidebar-item__label">{{ item.meta?.title }}</span>
  </a>

  <el-menu-item v-else :index="item.path">
    <AppIcon :name="item.meta?.icon" />
    <template #title>
      <span>{{ item.meta?.title }}</span>
    </template>
  </el-menu-item>
</template>

<style scoped lang="scss">
// <a> 默认有下划线与链接色，覆盖成与普通菜单项一致
.sidebar-item__external {
  text-decoration: none;
}

// 侧栏折叠时隐藏文字，与 el-menu-item 组件的折叠表现保持一致
:global(.el-menu--collapse) .sidebar-item__label {
  display: none;
}
</style>
