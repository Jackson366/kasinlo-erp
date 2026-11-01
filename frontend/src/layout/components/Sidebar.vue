<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import { navigationMenus, navigationTrail } from '../navigation'
import AppIcon from '@/components/AppIcon.vue'
import SidebarItem from './SidebarItem.vue'

const props = withDefaults(defineProps<{ expanded?: boolean }>(), {
  expanded: false,
})
const route = useRoute()
const appStore = useAppStore()
const permissionStore = usePermissionStore()
const collapsed = computed(() => !props.expanded && appStore.primarySidebarCollapsed)
const menus = computed(() => navigationMenus(permissionStore.menus))
const activePath = computed(() => (route.meta?.activeMenu as string) || route.path)
const menuRef = ref<{ open: (index: string) => void }>()

// Reveal active ancestors after deep links or leaving icon mode. Browsing never navigates.
watch(
  [activePath, collapsed, menus],
  async () => {
    if (collapsed.value) return
    await nextTick()
    for (const item of navigationTrail(menus.value, activePath.value).slice(0, -1)) {
      menuRef.value?.open(item.path)
    }
  },
  { immediate: true },
)
</script>

<template>
  <nav class="sidebar" :class="{ 'is-collapsed': collapsed }" aria-label="主导航">
    <el-scrollbar class="sidebar__scroll">
      <el-menu
        :id="expanded ? 'sidebar-mobile' : 'sidebar-primary'"
        ref="menuRef"
        :default-active="activePath"
        :collapse="collapsed"
        :collapse-transition="false"
        router
        class="sidebar__menu"
        popper-class="sidebar-menu-popup"
      >
        <SidebarItem
          v-for="item in menus"
          :key="item.path"
          :item="item"
          :active-path="activePath"
        />
      </el-menu>
    </el-scrollbar>
    <button
      v-if="!expanded"
      type="button"
      class="sidebar__collapse"
      :aria-label="appStore.primarySidebarCollapsed ? '展开菜单' : '收起菜单'"
      :title="appStore.primarySidebarCollapsed ? '展开菜单' : '收起菜单'"
      :aria-expanded="!appStore.primarySidebarCollapsed"
      aria-controls="sidebar-primary"
      @click="appStore.togglePrimarySidebar()"
    >
      <AppIcon :name="appStore.primarySidebarCollapsed ? 'Expand' : 'Fold'" />
      <span v-if="!appStore.primarySidebarCollapsed">收起菜单</span>
    </button>
  </nav>
</template>

<style scoped lang="scss">
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 8px 14px 16px;
  &__scroll {
    flex: 1;
    min-height: 0;
  }
  &__menu {
    --el-menu-bg-color: transparent;
    --el-menu-hover-bg-color: var(--app-fill-hover);
    --el-menu-active-color: var(--brand-primary);
    --el-menu-text-color: var(--app-text);
    --el-menu-base-level-padding: 16px;
    --el-menu-level-padding: 20px;
    width: 100%;
    border: 0;
    :deep(.el-menu-item),
    :deep(.el-sub-menu__title) {
      height: 48px;
      margin-bottom: 6px;
      border-radius: var(--radius-md);
      font-size: 15px;
      line-height: 48px;
      .el-icon {
        width: 22px;
        margin-right: 14px;
        font-size: 21px;
      }
      .el-sub-menu__icon-arrow {
        width: 12px;
        margin-right: 0;
        font-size: 12px;
        color: var(--app-text-secondary);
      }
    }
    :deep(.el-sub-menu.is-active > .el-sub-menu__title),
    :deep(> .el-menu-item.is-active) {
      background: var(--app-primary-soft);
      color: var(--brand-primary);
      font-weight: 600;
    }
    :deep(.el-sub-menu .el-menu-item),
    :deep(.el-sub-menu .el-sub-menu__title) {
      height: 40px;
      margin-bottom: 2px;
      font-size: 14px;
      line-height: 40px;
      color: var(--app-text-secondary);
    }
    :deep(.el-sub-menu .el-menu-item.is-active) {
      color: var(--brand-primary);
      background: transparent;
      font-weight: 500;
    }
    :deep(.el-menu-item:focus-visible),
    :deep(.el-sub-menu__title:focus-visible) {
      outline: 2px solid var(--brand-primary);
      outline-offset: -2px;
    }
  }
  &__collapse {
    display: flex;
    align-items: center;
    gap: 14px;
    flex: none;
    min-height: 40px;
    width: 100%;
    padding: 0 16px;
    margin-top: 12px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--app-text-secondary);
    font: inherit;
    font-size: 13px;
    cursor: pointer;
    &:hover {
      background: var(--app-fill-hover);
      color: var(--app-text);
    }
    &:active {
      background: var(--app-primary-soft);
    }
    &:focus-visible {
      outline: 2px solid var(--brand-primary);
      outline-offset: -2px;
    }
    :deep(.el-icon) {
      font-size: 20px;
    }
  }
  &.is-collapsed {
    padding-inline: 10px;
    :deep(.el-menu--collapse .sidebar-item__label) {
      display: none;
    }
    :deep(.el-menu--collapse) {
      width: 56px;
    }
    :deep(.el-menu--collapse > .el-menu-item),
    :deep(.el-menu--collapse > .el-sub-menu > .el-sub-menu__title) {
      padding-inline: 17px;
      .el-icon {
        margin: 0;
      }
    }
  }
}
</style>
