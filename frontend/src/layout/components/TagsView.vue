<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import AppIcon from '@/components/AppIcon.vue'
import type { TagView } from '@/store'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

const listRef = ref<HTMLElement>()
const homePath = computed(() => permissionStore.homePath())

// 进入页面时登记标签页；隐藏路由（如个人中心）不登记
watch(
  () => route.fullPath,
  () => {
    if (route.meta?.hidden || !route.meta?.title) return
    appStore.addTag({
      path: route.path,
      fullPath: route.fullPath,
      title: route.meta.title as string,
      closable: route.path !== homePath.value,
    })
    void nextTick(() =>
      listRef.value
        ?.querySelector('[aria-current="page"]')
        ?.scrollIntoView({ block: 'nearest', inline: 'nearest' }),
    )
  },
  { immediate: true },
)

function isActive(tag: TagView): boolean {
  return tag.path === route.path
}

async function closeTag(tag: TagView): Promise<void> {
  if (tag.path === route.path) {
    const index = appStore.tags.findIndex((item) => item.path === tag.path)
    const next = appStore.tags[index + 1] ?? appStore.tags[index - 1]
    // Keep the tag until form/navigation guards have accepted leaving.
    const failure = await router.push(next?.fullPath ?? next?.path ?? homePath.value)
    if (failure) return
  }
  appStore.removeTag(tag.path, route.path)
  await nextTick()
  listRef.value
    ?.querySelector<HTMLAnchorElement>('[aria-current="page"]')
    ?.focus({ preventScroll: true })
}

async function handleCommand(command: string): Promise<void> {
  if (command === 'others') {
    appStore.removeOtherTags(route.path)
    return
  }
  if (command === 'close') {
    const current = appStore.tags.find((tag) => tag.path === route.path)
    if (current?.closable) {
      await closeTag(current)
    }
    return
  }
  if (command === 'home') {
    const failure = route.path === homePath.value ? undefined : await router.push(homePath.value)
    if (failure) return
    appStore.removeOtherTags(homePath.value)
  }
}
</script>

<template>
  <nav class="tags-view" aria-label="已打开的页面">
    <el-scrollbar class="tags-view__scroll">
      <div ref="listRef" class="tags-view__list">
        <div
          v-for="tag in appStore.tags"
          :key="tag.path"
          class="tags-view__item"
          :class="{ 'is-active': isActive(tag) }"
        >
          <router-link
            :to="tag.fullPath || tag.path"
            :aria-current="isActive(tag) ? 'page' : undefined"
            class="tags-view__link"
          >
            {{ tag.title }}
          </router-link>
          <button
            v-if="tag.closable"
            type="button"
            class="tags-view__close"
            :aria-label="`关闭${tag.title}标签`"
            @click="closeTag(tag)"
          >
            <AppIcon name="Close" />
          </button>
        </div>
      </div>
    </el-scrollbar>
    <el-dropdown class="tags-view__more" trigger="click" @command="handleCommand">
      <button class="tags-view__more-btn" type="button" aria-label="标签页操作">
        <AppIcon name="More" />
      </button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item command="home">关闭全部标签</el-dropdown-item>
          <el-dropdown-item command="others">关闭其它标签</el-dropdown-item>
          <el-dropdown-item command="close" divided>关闭当前标签</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </nav>
</template>

<style scoped lang="scss">
.tags-view {
  display: flex;
  align-items: center;
  height: var(--layout-tags-height);
  gap: 12px;
  &__scroll {
    flex: 1;
    min-width: 0;
  }
  &__list {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
  }
  &__item {
    display: inline-flex;
    flex: none;
    align-items: center;
    height: 38px;
    padding-right: 8px;
    color: var(--app-text);
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--radius-sm);
    white-space: nowrap;
    transition:
      background-color var(--transition-base),
      border-color var(--transition-base);
    &:hover {
      background: var(--app-fill);
    }
    &.is-active {
      color: var(--brand-primary);
      background: var(--app-primary-soft);
      border-color: var(--app-primary-soft);
      font-weight: 600;
    }
  }
  &__link {
    display: inline-flex;
    align-items: center;
    height: 100%;
    padding: 0 12px;
    color: inherit;
    font-size: 14px;
    border-radius: var(--radius-sm);
  }
  &__close,
  &__more-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--app-text-muted);
    cursor: pointer;
    font-size: 13px;
    &:hover {
      background: var(--app-fill-hover);
      color: var(--brand-primary);
    }
    &:active {
      background: var(--app-primary-soft);
    }
  }
  &__more-btn {
    width: 32px;
    height: 32px;
    font-size: 18px;
  }
  &__link:focus-visible,
  &__close:focus-visible,
  &__more-btn:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: -2px;
  }
  .is-active &__close {
    color: var(--brand-primary);
  }
}
@media (prefers-reduced-motion: reduce) {
  .tags-view__item {
    transition: none;
  }
}
</style>
