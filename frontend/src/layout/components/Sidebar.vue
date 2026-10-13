<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import SidebarItem from './SidebarItem.vue'

const route = useRoute()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

// 隐藏项不进入菜单
const menus = computed(() => permissionStore.menus.filter((item) => !item.meta?.hidden))
// 支持页面声明高亮某个菜单项（如详情页高亮列表菜单）
const activePath = computed<string>(() => (route.meta?.activeMenu as string) || route.path)
</script>

<template>
  <div class="sidebar">
    <div class="sidebar__brand">
      <div class="sidebar__logo">N</div>
      <span v-show="!appStore.sidebarCollapsed" class="sidebar__name">Nova ERP</span>
    </div>

    <el-scrollbar class="sidebar__scroll">
      <el-menu
        :default-active="activePath"
        :collapse="appStore.sidebarCollapsed"
        :collapse-transition="false"
        unique-opened
        router
        class="sidebar__menu"
      >
        <SidebarItem v-for="item in menus" :key="item.id" :item="item" />
      </el-menu>
    </el-scrollbar>
  </div>
</template>

<style scoped lang="scss">
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--app-surface);
  border-right: 1px solid var(--app-border-light);

  &__brand {
    display: flex;
    align-items: center;
    gap: 10px;
    height: var(--layout-navbar-height);
    padding: 0 16px;
    flex-shrink: 0;
    border-bottom: 1px solid var(--app-border-light);
    overflow: hidden;
  }

  &__logo {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    flex-shrink: 0;
    font-size: 16px;
    font-weight: 700;
    color: #fff;
    background: var(--brand-gradient);
    border-radius: 8px;
    box-shadow: 0 2px 6px rgba(37, 99, 235, 0.35);
  }

  &__name {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: 0.2px;
    color: var(--app-text);
    white-space: nowrap;
  }

  &__scroll {
    flex: 1;
    min-height: 0;
  }

  &__menu {
    // 让菜单容器融入侧栏底色（Element Plus 默认用 --el-fill-color-blank，
    // 暗色下会比卡片略亮，出现一条可见的色差带）
    --el-menu-bg-color: transparent;
    --el-menu-hover-bg-color: var(--app-fill-hover);
    --el-menu-active-color: var(--brand-primary);
    --el-menu-text-color: var(--app-text-secondary);

    padding: 10px;
    border-right: none;

    &:not(.el-menu--collapse) {
      width: var(--layout-sidebar-width);
    }
  }
}

// 菜单项样式：圆角、悬停浅底、选中主色
:deep(.el-menu-item),
:deep(.el-sub-menu__title) {
  height: 42px;
  line-height: 42px;
  margin-bottom: 2px;
  border-radius: var(--radius-sm);
  color: var(--app-text-secondary);
  font-weight: 500;

  &:hover {
    color: var(--brand-primary);
    background: var(--app-fill-hover);
  }
}

:deep(.el-menu-item.is-active) {
  color: var(--brand-primary);
  background: var(--app-primary-soft);
  font-weight: 600;
}

:deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: var(--brand-primary);
}

:deep(.el-menu--collapse) {
  .el-menu-item,
  .el-sub-menu__title {
    padding-left: 0 !important;
  }
}
</style>
