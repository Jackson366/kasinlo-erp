<script setup lang="ts">
import { computed, nextTick, provide, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pageLeaveGuardsKey } from '@/composables/useUnsavedChanges'
import { useAppStore, usePermissionStore } from '@/store'
import Sidebar from './components/Sidebar.vue'
import Navbar from './components/Navbar.vue'
import TagsView from './components/TagsView.vue'

const route = useRoute()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

// 页面刷新：把当前页面从 keep-alive 的缓存名单里摘掉一帧，逼它重建。
//
// 不要用「把 refreshKey 拼进 :key」的做法：那会让每次刷新都产生一个**新的 key**，
// 而 keep-alive 是按 key 存缓存的——旧 key 的实例永远留在缓存里，既不会被复用，
// 也不会被回收，反复刷新页面就会持续泄漏组件实例（连带它们的定时器、监听、DOM 引用）。
const sidebarHasSecondary = ref(false)
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
  <div class="layout" :class="{ 'is-collapsed': appStore.sidebarCollapsed, 'is-primary-collapsed': appStore.primarySidebarCollapsed }">
    <aside class="layout__aside" :class="{ 'is-rail-only': !sidebarHasSecondary }">
      <Sidebar @secondary-change="sidebarHasSecondary = $event" />
    </aside>

    <section class="layout__body">
      <Navbar />
      <TagsView />
      <main class="layout__content">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <keep-alive :include="aliveIncludes">
              <component :is="Component" :key="viewKey" />
            </keep-alive>
          </transition>
        </router-view>
      </main>
    </section>
  </div>
</template>

<style scoped lang="scss">
.layout {
  --sidebar-rail-width: var(--layout-sidebar-primary-width);
  display: flex;
  height: 100%;
  overflow: hidden;

  &__aside {
    width: calc(var(--sidebar-rail-width) + var(--layout-sidebar-secondary-width));
    flex-shrink: 0;
    transition: width var(--transition-base);
  }

  &.is-collapsed &__aside {
    width: var(--sidebar-rail-width);
  }

  &.is-primary-collapsed { --sidebar-rail-width: var(--layout-sidebar-primary-collapsed); }
  &__aside.is-rail-only { width: var(--sidebar-rail-width); }

  &__body {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }

  &__content {
    flex: 1;
    min-height: 0;
    padding: 16px;
    overflow-y: auto;
    background: var(--app-bg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .layout__aside { transition: none; }
}
</style>
