<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import Sidebar from './components/Sidebar.vue'
import Navbar from './components/Navbar.vue'
import TagsView from './components/TagsView.vue'

const route = useRoute()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

// 页面刷新：改变 key 强制重建当前组件
const refreshKey = ref(0)
provide('pageRefresh', () => {
  refreshKey.value += 1
})

/** 需要缓存的页面组件名（与后端菜单 keepAlive 及组件 defineOptions name 对应） */
const cachedViews = computed(() =>
  permissionStore.routes
    .filter((item) => item.meta?.keepAlive && item.name)
    .map((item) => item.name as string),
)

const viewKey = computed(() => `${route.path}#${refreshKey.value}`)
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
            <keep-alive :include="cachedViews">
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
