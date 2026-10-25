<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import type { RouteRecord } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import SidebarItem from './SidebarItem.vue'

const emit = defineEmits<{ 'secondary-change': [visible: boolean] }>()
const route = useRoute()
const appStore = useAppStore()
const permissionStore = usePermissionStore()
const menus = computed(() => permissionStore.menus.filter(item => !item.meta?.hidden))
const activePath = computed<string>(() => (route.meta?.activeMenu as string) || route.path)
const selectedGroup = ref<number | null>(null)
const childrenOf = (item: RouteRecord) => (item.children ?? []).filter(child => !child.meta?.hidden)
function contains(item: RouteRecord, path: string): boolean {
  return item.path === path || childrenOf(item).some(child => contains(child, path))
}
const currentGroup = computed(() => menus.value.find(item => item.id === selectedGroup.value))
const secondaryItems = computed(() => currentGroup.value ? childrenOf(currentGroup.value) : [])
const defaultOpenGroups = computed(() => secondaryItems.value.filter(item => childrenOf(item).length).map(item => item.path || String(item.id)))
watch([menus, activePath], () => {
  selectedGroup.value = menus.value.find(item => contains(item, activePath.value))?.id ?? menus.value[0]?.id ?? null
}, { immediate: true })
watch(() => secondaryItems.value.length > 0, value => emit('secondary-change', value), { immediate: true })
function openGroup(item: RouteRecord): void {
  selectedGroup.value = item.id
  if (appStore.sidebarCollapsed) appStore.toggleSidebar()
}
</script>

<template>
  <aside class="sidebar" :class="{ 'is-secondary-collapsed': appStore.sidebarCollapsed }" aria-label="主导航">
    <button v-if="secondaryItems.length" type="button" class="sidebar__toggle"
      :aria-label="appStore.sidebarCollapsed ? '展开二级菜单' : '收起二级菜单'"
      :title="appStore.sidebarCollapsed ? '展开二级菜单' : '收起二级菜单'"
      :aria-expanded="!appStore.sidebarCollapsed" aria-controls="sidebar-secondary" @click="appStore.toggleSidebar()">
      <AppIcon :name="appStore.sidebarCollapsed ? 'ArrowRight' : 'ArrowLeft'" />
    </button>
    <div class="sidebar__rail">
      <div class="sidebar__brand"><span class="sidebar__logo">K</span><span>Kasinlo</span></div>
      <el-scrollbar class="sidebar__scroll">
        <nav class="sidebar__primary" aria-label="业务分组">
          <template v-for="item in menus" :key="item.id">
            <button v-if="childrenOf(item).length" type="button" class="sidebar__primary-item"
              :class="{ 'is-active': selectedGroup === item.id }"
              :aria-expanded="selectedGroup === item.id && !appStore.sidebarCollapsed"
              :aria-controls="selectedGroup === item.id && !appStore.sidebarCollapsed ? 'sidebar-secondary' : undefined"
              @click="openGroup(item)">
              <AppIcon :name="item.meta?.icon" /><span>{{ item.meta?.title }}</span>
            </button>
            <a v-else-if="item.meta?.isExternal" class="sidebar__primary-item" :href="item.path" target="_blank" rel="noopener noreferrer">
              <AppIcon :name="item.meta?.icon" /><span>{{ item.meta?.title }}</span>
            </a>
            <router-link v-else :to="item.path" class="sidebar__primary-item"
              :class="{ 'is-active': contains(item, activePath) }" :aria-current="contains(item, activePath) ? 'page' : undefined">
              <AppIcon :name="item.meta?.icon" /><span>{{ item.meta?.title }}</span>
            </router-link>
          </template>
        </nav>
      </el-scrollbar>
    </div>
    <section v-if="secondaryItems.length" v-show="!appStore.sidebarCollapsed" id="sidebar-secondary" class="sidebar__secondary" :aria-label="currentGroup?.meta?.title">
      <h2 class="sidebar__heading">{{ currentGroup?.meta?.title }}</h2>
      <el-scrollbar class="sidebar__scroll">
        <el-menu :key="currentGroup?.id" :default-active="activePath" :default-openeds="defaultOpenGroups" router class="sidebar__menu">
          <SidebarItem v-for="item in secondaryItems" :key="item.id" :item="item" />
        </el-menu>
      </el-scrollbar>
    </section>
  </aside>
</template>

<style scoped lang="scss">
.sidebar { position: relative; display: flex; height: 100%; min-width: 0; background: var(--app-surface-subtle); border-right: 1px solid var(--app-border-light); }
.sidebar__rail { width: var(--layout-sidebar-primary-width); flex: none; display: flex; flex-direction: column; min-height: 0; }
.sidebar__brand { display: flex; align-items: center; gap: 7px; height: var(--layout-navbar-height); padding: 0 12px; flex: none; font-size: 13px; font-weight: 600; white-space: nowrap; color: var(--app-text); }
.sidebar__logo { display: grid; place-items: center; width: 22px; height: 24px; border-radius: var(--radius-sm); color: var(--brand-primary); background: var(--app-primary-soft); font-size: 15px; }
.sidebar__scroll { flex: 1; min-height: 0; }
.sidebar__primary { display: grid; gap: 4px; padding: 8px; }
.sidebar__primary-item {
  appearance: none; display: flex; align-items: center; gap: 9px; width: 100%; min-height: 38px; padding: 8px; border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--app-text-secondary); text-align: left; font: inherit; font-size: 13px; line-height: 20px; cursor: pointer;
  span { overflow-wrap: anywhere; }
  :deep(.el-icon) { flex: none; font-size: 15px; }
  &:hover { background: var(--app-fill-hover); color: var(--brand-primary); }
  &:active { background: var(--app-primary-soft); }
  &:focus-visible { outline: 2px solid var(--brand-primary); outline-offset: -2px; }
  &.is-active { background: var(--app-primary-soft); color: var(--brand-primary); font-weight: 600; }
}
.is-secondary-collapsed .sidebar__rail { width: var(--layout-sidebar-collapsed); }
.sidebar__toggle {
  position: absolute; top: 80px; right: -12px; z-index: 2;
  display: grid; place-items: center; width: 24px; height: 40px;
  padding: 0; border: 1px solid var(--app-border); border-radius: 999px;
  color: var(--app-text-secondary); background: var(--app-surface); box-shadow: var(--shadow-card); cursor: pointer;
  :deep(.el-icon) { font-size: 12px; }
  &:hover { color: var(--brand-primary); border-color: var(--brand-primary); background: var(--app-fill-hover); }
  &:active { background: var(--app-primary-soft); }
  &:focus-visible { outline: 2px solid var(--brand-primary); outline-offset: 2px; }
}
.sidebar__secondary { display: flex; flex: 1; flex-direction: column; min-width: 0; border-left: 1px solid var(--app-border-light); }
.sidebar__heading { margin: 0; display: flex; align-items: center; height: var(--layout-navbar-height); flex: none; padding: 0 20px; font-size: 15px; font-weight: 600; color: var(--app-text); }
.sidebar__menu { --el-menu-bg-color: transparent; --el-menu-hover-bg-color: var(--app-fill-hover); --el-menu-active-color: var(--brand-primary); --el-menu-text-color: var(--app-text-secondary); border: 0; padding: 8px; }
.sidebar__menu :deep(.el-menu-item), .sidebar__menu :deep(.el-sub-menu__title) { height: auto; min-height: 36px; line-height: 20px; padding-top: 8px; padding-bottom: 8px; margin-bottom: 2px; border-radius: var(--radius-sm); white-space: normal; font-size: 13px; }
.sidebar__menu :deep(.el-menu-item.is-active) { color: var(--brand-primary); background: var(--app-primary-soft); font-weight: 600; }
.sidebar__menu :deep(.el-sub-menu.is-active > .el-sub-menu__title) { color: var(--app-text); font-weight: 600; }
</style>
