<script setup lang="ts">
import { computed } from 'vue'
import type { RouteRecord } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
const props = withDefaults(
  defineProps<{ item: RouteRecord; activePath: string; depth?: number }>(),
  { depth: 0 },
)
const visibleChildren = computed(() =>
  (props.item.children ?? []).filter((child) => !child.meta?.hidden),
)
</script>

<template>
  <el-sub-menu
    v-if="visibleChildren.length"
    :index="item.path || String(item.id)"
    popper-class="sidebar-menu-popup"
  >
    <template #title>
      <AppIcon v-if="depth === 0" :name="item.meta?.icon" />
      <span>{{ item.meta?.title }}</span>
    </template>
    <SidebarItem
      v-for="child in visibleChildren"
      :key="child.path"
      :item="child"
      :depth="depth + 1"
      :active-path="activePath"
    />
  </el-sub-menu>
  <a
    v-else-if="item.meta?.isExternal"
    class="el-menu-item sidebar-item__external"
    :href="item.path"
    target="_blank"
    rel="noopener noreferrer"
    :aria-label="item.meta?.title"
  >
    <AppIcon v-if="depth === 0" :name="item.meta?.icon" />
    <span class="sidebar-item__label">{{ item.meta?.title }}</span>
  </a>
  <el-menu-item
    v-else
    :index="item.path"
    :aria-current="item.path === activePath ? 'page' : undefined"
  >
    <AppIcon v-if="depth === 0" :name="item.meta?.icon" />
    <span v-else class="sidebar-item__dot" aria-hidden="true" />
    <template #title
      ><span>{{ item.meta?.title }}</span></template
    >
  </el-menu-item>
</template>

<style scoped lang="scss">
.sidebar-item__external {
  text-decoration: none;
}
.sidebar-item__dot {
  flex: 0 0 5px;
  width: 5px;
  height: 5px;
  margin-right: 16px;
  border-radius: 50%;
  background: transparent;
}
.is-active > .sidebar-item__dot {
  background: var(--brand-primary);
}
</style>
