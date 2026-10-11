<script setup lang="ts">
import { computed, nextTick, provide, ref } from 'vue'
import { useRoute } from 'vue-router'
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
const refreshing = ref(false)
provide('pageRefresh', () => {
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

// key 只用 path：稳定标识，keep-alive 才能正确复用与回收。
// 刷新靠上面的 refreshing 开关实现，不污染 key。
const viewKey = computed(() => route.path)
</script>

<template>
  <div class="layout" :class="{ 'is-collapsed': appStore.sidebarCollapsed }">
    <aside class="layout__aside">
      <Sidebar />
    </aside>

    <section class="layout__body">
      <Navbar />
      <TagsView />
      <main class="layout__content">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <keep-alive :include="refreshing ? [] : cachedViews">
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
  display: flex;
  height: 100%;
  overflow: hidden;

  &__aside {
    width: var(--layout-sidebar-width);
    flex-shrink: 0;
    transition: width var(--transition-base);
  }

  &.is-collapsed &__aside {
    width: var(--layout-sidebar-collapsed);
  }

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
</style>
