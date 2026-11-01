<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { usePermissionStore } from '@/store'
import { navigationMenus, navigationTrail } from '../navigation'
import AppIcon from '@/components/AppIcon.vue'
const route = useRoute()
const permissionStore = usePermissionStore()
const homePath = computed(() => permissionStore.homePath())
const items = computed(() => {
  if (route.path === homePath.value) return []
  const trail = navigationTrail(navigationMenus(permissionStore.menus), route.path)
  return trail.length
    ? trail
    : [{ path: route.path, meta: { title: route.meta?.title || '当前页面' } }]
})
</script>

<template>
  <nav class="breadcrumb" aria-label="面包屑导航">
    <ol>
      <li>
        <router-link v-if="items.length" :to="homePath" aria-label="首页"
          ><AppIcon name="House"
        /></router-link>
        <span v-else aria-current="page"
          ><AppIcon name="House" /><span class="breadcrumb__home">首页</span></span
        >
      </li>
      <li v-for="(item, index) in items" :key="item.path">
        <span class="breadcrumb__separator" aria-hidden="true">/</span>
        <span :aria-current="index === items.length - 1 ? 'page' : undefined">{{
          item.meta.title
        }}</span>
      </li>
    </ol>
  </nav>
</template>

<style scoped lang="scss">
.breadcrumb {
  color: var(--app-text-secondary);
  font-size: 13px;
  ol {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 24px;
    padding: 0;
    margin: 0;
    list-style: none;
    flex-wrap: nowrap;
    overflow: hidden;
  }
  li,
  li > span {
    display: inline-flex;
    align-items: center;
  }
  li {
    gap: 10px;
    min-width: 0;
    white-space: nowrap;
    > span:last-child { overflow: hidden; text-overflow: ellipsis; display: block; }
  }
  a {
    display: inline-flex;
    color: var(--app-text-secondary);
    border-radius: var(--radius-sm);
  }
  a:hover {
    color: var(--brand-primary);
  }
  a:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 4px;
  }
  :deep(.el-icon) {
    font-size: 20px;
  }
  [aria-current='page'] {
    color: var(--app-text);
    font-weight: 500;
  }
  &__separator {
    color: var(--app-text-muted);
  }
  &__home {
    margin-left: 10px;
  }
}
</style>
