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
const menus = computed(() =>
  permissionStore.menus.filter((item) => !item.meta?.hidden),
)
const activePath = computed<string>(
  () => (route.meta?.activeMenu as string) || route.path,
)
const selectedGroup = ref<number | null>(null)
const childrenOf = (item: RouteRecord) =>
  (item.children ?? []).filter((child) => !child.meta?.hidden)
function contains(item: RouteRecord, path: string): boolean {
  return (
    item.path === path ||
    childrenOf(item).some((child) => contains(child, path))
  )
}
const currentGroup = computed(() =>
  menus.value.find((item) => item.id === selectedGroup.value),
)
const secondaryItems = computed(() =>
  currentGroup.value ? childrenOf(currentGroup.value) : [],
)
const defaultOpenGroups = computed(() =>
  secondaryItems.value
    .filter((item) => childrenOf(item).length)
    .map((item) => item.path || String(item.id)),
)
watch(
  [menus, activePath],
  () => {
    selectedGroup.value =
      menus.value.find((item) => contains(item, activePath.value))?.id ??
      menus.value[0]?.id ??
      null
  },
  { immediate: true },
)
watch(
  () => secondaryItems.value.length > 0,
  (value) => emit('secondary-change', value),
  { immediate: true },
)
function openGroup(item: RouteRecord): void {
  selectedGroup.value = item.id
  if (appStore.sidebarCollapsed) appStore.toggleSidebar()
}
</script>

<template>
  <aside
    class="sidebar"
    :class="{ 'is-primary-collapsed': appStore.primarySidebarCollapsed }"
    aria-label="主导航"
  >
    <button
      v-if="secondaryItems.length"
      type="button"
      class="sidebar__toggle"
      :aria-label="appStore.sidebarCollapsed ? '展开二级菜单' : '收起二级菜单'"
      :title="appStore.sidebarCollapsed ? '展开二级菜单' : '收起二级菜单'"
      :aria-expanded="!appStore.sidebarCollapsed"
      aria-controls="sidebar-secondary"
      @click="appStore.toggleSidebar()"
    >
      <AppIcon :name="appStore.sidebarCollapsed ? 'ArrowRight' : 'ArrowLeft'" />
    </button>
    <div class="sidebar__rail">
      <div class="sidebar__brand" aria-label="Kasinlo ERP">
        <img
          class="sidebar__logo"
          src="/brand/kasinlo-mark.png"
          alt=""
          width="30"
          height="30"
        />
        <span
          v-show="!appStore.primarySidebarCollapsed"
          class="sidebar__wordmark"
          >Kasinlo</span
        >
      </div>
      <el-scrollbar class="sidebar__scroll">
        <nav
          id="sidebar-primary"
          class="sidebar__primary"
          aria-label="业务分组"
        >
          <el-tooltip
            v-for="item in menus"
            :key="item.id"
            :disabled="!appStore.primarySidebarCollapsed"
            :content="item.meta?.title"
            :trigger="['hover', 'focus']"
            :trigger-keys="[]"
            placement="right"
            :show-after="150"
          >
            <button
              v-if="childrenOf(item).length"
              type="button"
              class="sidebar__primary-item"
              :aria-label="item.meta?.title"
              :class="{ 'is-active': selectedGroup === item.id }"
              :aria-expanded="
                selectedGroup === item.id && !appStore.sidebarCollapsed
              "
              :aria-controls="
                selectedGroup === item.id && !appStore.sidebarCollapsed
                  ? 'sidebar-secondary'
                  : undefined
              "
              @click="openGroup(item)"
            >
              <AppIcon :name="item.meta?.icon" />
              <span v-show="!appStore.primarySidebarCollapsed">{{
                item.meta?.title
              }}</span>
            </button>
            <a
              v-else-if="item.meta?.isExternal"
              class="sidebar__primary-item"
              :aria-label="item.meta?.title"
              :href="item.path"
              target="_blank"
              rel="noopener noreferrer"
            >
              <AppIcon :name="item.meta?.icon" />
              <span v-show="!appStore.primarySidebarCollapsed">{{
                item.meta?.title
              }}</span>
            </a>
            <router-link
              v-else
              :to="item.path"
              class="sidebar__primary-item"
              :aria-label="item.meta?.title"
              :class="{ 'is-active': contains(item, activePath) }"
              :aria-current="contains(item, activePath) ? 'page' : undefined"
            >
              <AppIcon :name="item.meta?.icon" />
              <span v-show="!appStore.primarySidebarCollapsed">{{
                item.meta?.title
              }}</span>
            </router-link>
          </el-tooltip>
        </nav>
      </el-scrollbar>
      <el-tooltip
        :disabled="!appStore.primarySidebarCollapsed"
        content="展开一级菜单"
        :trigger="['hover', 'focus']"
        :trigger-keys="[]"
        placement="right"
      >
        <button
          type="button"
          class="sidebar__primary-item sidebar__primary-toggle"
          :aria-label="
            appStore.primarySidebarCollapsed ? '展开一级菜单' : '收起一级菜单'
          "
          :aria-expanded="!appStore.primarySidebarCollapsed"
          aria-controls="sidebar-primary"
          @click="appStore.togglePrimarySidebar()"
        >
          <AppIcon
            :name="appStore.primarySidebarCollapsed ? 'Expand' : 'Fold'"
          />
          <span v-show="!appStore.primarySidebarCollapsed">收起菜单</span>
        </button>
      </el-tooltip>
    </div>
    <section
      v-if="secondaryItems.length"
      v-show="!appStore.sidebarCollapsed"
      id="sidebar-secondary"
      class="sidebar__secondary"
      :aria-label="currentGroup?.meta?.title"
    >
      <h2 class="sidebar__heading">{{ currentGroup?.meta?.title }}</h2>
      <el-scrollbar class="sidebar__scroll">
        <el-menu
          :key="currentGroup?.id"
          :default-active="activePath"
          :default-openeds="defaultOpenGroups"
          router
          class="sidebar__menu"
        >
          <SidebarItem
            v-for="item in secondaryItems"
            :key="item.id"
            :item="item"
          />
        </el-menu>
      </el-scrollbar>
    </section>
  </aside>
</template>

<style scoped lang="scss">
.sidebar {
  --sidebar-header-height: var(--layout-navbar-height);
  position: relative;
  display: flex;
  height: 100%;
  min-width: 0;
  background: var(--app-surface-subtle);
  border-right: 1px solid var(--app-border-light);
}

.sidebar__rail {
  width: var(--sidebar-rail-width);
  flex: none;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.sidebar__brand {
  display: flex;
  align-items: center;
  gap: 8px;
  height: var(--sidebar-header-height);
  padding: 0 17px;
  flex: none;
  color: var(--app-text);
  overflow: hidden;
  white-space: nowrap;
}

.sidebar__logo {
  display: block;
  flex: none;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  object-fit: contain;
}

.sidebar__wordmark {
  flex: none;
  font-size: 13px;
  font-weight: 600;
  line-height: 20px;
  letter-spacing: 0.2px;
}

.sidebar__scroll {
  flex: 1;
  min-height: 0;
}
.sidebar__primary {
  display: grid;
  gap: 4px;
  padding: 8px;
}
.sidebar__primary-item {
  position: relative;
  appearance: none;
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 40px;
  padding: 10px 15px;
  border: 0;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--app-text-secondary);
  text-align: left;
  text-decoration: none;
  font: inherit;
  font-size: 13px;
  line-height: 20px;
  cursor: pointer;
  transition:
    background-color var(--transition-base),
    color var(--transition-base);

  span {
    flex: none;
    white-space: nowrap;
  }
  :deep(.el-icon) {
    flex: 0 0 18px;
    width: 18px;
    height: 18px;
    font-size: 18px;
  }
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
  &.is-active {
    background: var(--app-primary-soft);
    color: var(--tone-blue);
    font-weight: 600;

    &::before {
      content: '';
      position: absolute;
      left: -8px;
      top: 12px;
      bottom: 12px;
      width: 3px;
      border-radius: 0 3px 3px 0;
      background: var(--brand-primary);
    }
  }
}

.sidebar__primary-toggle {
  flex: none;
  width: calc(100% - 16px);
  margin: 8px;
}

.sidebar__toggle {
  position: absolute;
  top: 80px;
  right: -12px;
  z-index: 2;
  display: grid;
  place-items: center;
  width: 24px;
  height: 36px;
  padding: 0;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  color: var(--app-text-secondary);
  background: var(--app-surface);
  box-shadow: var(--shadow-card);
  cursor: pointer;

  :deep(.el-icon) {
    font-size: 12px;
  }
  &:hover {
    color: var(--brand-primary);
    border-color: var(--brand-primary);
    background: var(--app-fill-hover);
  }
  &:active {
    background: var(--app-primary-soft);
  }
  &:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 2px;
  }
}

.sidebar__secondary {
  display: flex;
  flex: none;
  width: var(--layout-sidebar-secondary-width);
  flex-direction: column;
  min-width: 0;
  background: var(--app-surface);
  border-left: 1px solid var(--app-border-light);
}

.sidebar__heading {
  margin: 0;
  display: flex;
  align-items: center;
  height: var(--sidebar-header-height);
  flex: none;
  padding: 0 20px;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text);
}

.sidebar__menu {
  --el-menu-bg-color: transparent;
  --el-menu-hover-bg-color: var(--app-fill);
  --el-menu-active-color: var(--tone-blue);
  --el-menu-text-color: var(--app-text-secondary);
  --el-menu-base-level-padding: 12px;
  --el-menu-level-padding: 12px;
  border: 0;
  padding: 8px;

  :deep(.el-menu-item),
  :deep(.el-sub-menu__title) {
    position: relative;
    height: auto;
    min-height: 40px;
    line-height: 20px;
    padding-top: 10px;
    padding-bottom: 10px;
    margin-bottom: 4px;
    border-radius: var(--radius-sm);
    white-space: normal;
    font-size: 13px;

    .el-icon {
      flex: 0 0 18px;
      width: 18px;
      height: 18px;
      margin-right: 8px;
      font-size: 18px;
    }
    .el-sub-menu__icon-arrow {
      width: 12px;
      margin-right: 0;
      font-size: 12px;
    }
    span {
      min-width: 0;
      overflow-wrap: anywhere;
    }
    &:focus-visible {
      outline: 2px solid var(--brand-primary);
      outline-offset: -2px;
    }
  }

  :deep(.el-menu-item.is-active) {
    color: var(--tone-blue);
    background: var(--app-primary-soft);
    font-weight: 600;
  }
  :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
    color: var(--app-text);
    font-weight: 500;
  }
}

@media (prefers-reduced-motion: reduce) {
  .sidebar__primary-item {
    transition: none;
  }
}
</style>
