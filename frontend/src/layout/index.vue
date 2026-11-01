<script setup lang="ts">
import { computed, nextTick, provide, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { pageLeaveGuardsKey } from '@/composables/useUnsavedChanges'
import { useAppStore, usePermissionStore } from '@/store'
import Sidebar from './components/Sidebar.vue'
import Navbar from './components/Navbar.vue'
import TagsView from './components/TagsView.vue'
import Breadcrumb from './components/Breadcrumb.vue'

const route = useRoute()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

// 页面刷新：把当前页面从 keep-alive 的缓存名单里摘掉一帧，逼它重建。
//
// 不要用「把 refreshKey 拼进 :key」的做法：那会让每次刷新都产生一个**新的 key**，
// 而 keep-alive 是按 key 存缓存的——旧 key 的实例永远留在缓存里，既不会被复用，
// 也不会被回收，反复刷新页面就会持续泄漏组件实例（连带它们的定时器、监听、DOM 引用）。
const mobileMenuOpen = ref(false)
watch(
  () => route.fullPath,
  () => {
    mobileMenuOpen.value = false
  },
)
const refreshing = ref(false)
const pageLeaveGuards = new Set<() => Promise<boolean>>()
provide(pageLeaveGuardsKey, pageLeaveGuards)
provide('pageRefresh', async () => {
  for (const canLeave of pageLeaveGuards) {
    if (!(await canLeave())) return
  }
  refreshing.value = true
  nextTick(() => {
    refreshing.value = false
  })
})

/** 需要缓存的页面组件名（与后端菜单 keepAlive 及组件 defineOptions name 对应） */
const cachedViews = computed(() =>
  permissionStore.routes
    .filter((item) => item.meta?.keepAlive && item.name)
    .map((item) => item.name as string),
)

// 刷新时不能把 include 清空成 []：那会把**所有**页面的 keep-alive 缓存一并丢弃，
// 其他页面未保存的表单、滚动位置全部丢失。只把当前页从名单里摘掉一帧，其余保留。
const aliveIncludes = computed(() =>
  refreshing.value && route.name
    ? cachedViews.value.filter((name) => name !== route.name)
    : cachedViews.value,
)

// key 只用 path：稳定标识，keep-alive 才能正确复用与回收。
// 刷新靠上面的 refreshing 开关实现，不污染 key。
const viewKey = computed(() => route.path)
</script>

<template>
  <div class="layout" :class="{ 'is-collapsed': appStore.primarySidebarCollapsed }">
    <Navbar class="layout__header" @open-menu="mobileMenuOpen = true" />
    <aside class="layout__aside"><Sidebar /></aside>
    <section class="layout__body">
      <div class="layout__navigation">
        <Breadcrumb />
        <TagsView />
      </div>
      <main id="main-content" class="layout__content" aria-label="工作区">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <keep-alive :include="aliveIncludes">
              <component :is="Component" :key="viewKey" />
            </keep-alive>
          </transition>
        </router-view>
      </main>
    </section>
    <el-drawer
      v-model="mobileMenuOpen"
      title="菜单"
      direction="ltr"
      size="280px"
      class="layout-menu-drawer"
    >
      <Sidebar expanded />
    </el-drawer>
  </div>
</template>

<style scoped lang="scss">
.layout {
  --sidebar-width: var(--layout-sidebar-width);
  display: grid;
  grid-template-columns: var(--sidebar-width) minmax(0, 1fr);
  grid-template-rows: var(--layout-navbar-height) minmax(0, 1fr);
  height: 100%;
  background: var(--app-bg);
  transition: grid-template-columns var(--transition-base);
  &.is-collapsed {
    --sidebar-width: var(--layout-sidebar-collapsed);
  }
  &__header {
    grid-column: 1 / -1;
  }
  &__aside {
    min-height: 0;
  }
  &__body {
    display: flex;
    flex-direction: column;
    gap: 20px;
    min-width: 0;
    min-height: 0;
    padding: 4px 24px 24px 8px;
  }
  &__navigation {
    flex: none;
    min-width: 0;
    padding: 18px 24px 10px;
    background: var(--app-surface);
    border-radius: var(--radius-lg);
  }
  &__content {
    flex: 1;
    min-height: 0;
    overflow: auto;
    scrollbar-gutter: stable;
    scroll-padding-block: 16px;
  }
}
@media (max-width: 800px) {
  .layout {
    grid-template-columns: minmax(0, 1fr);
    &__aside {
      display: none;
    }
    &__body {
      gap: 12px;
      padding: 0 12px 12px;
    }
    &__navigation {
      padding: 14px 16px 6px;
    }
  }
}
@media (prefers-reduced-motion: reduce) {
  .layout {
    transition: none;
  }
}
</style>
