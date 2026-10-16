<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import AppIcon from '@/components/AppIcon.vue'
import type { TagView } from '@/store'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

const homePath = computed(() => permissionStore.homePath())

// 进入页面时登记标签页；隐藏路由（如个人中心）不登记
watch(
  () => route.path,
  () => {
    if (route.meta?.hidden || !route.meta?.title) return
    appStore.addTag({
      path: route.path,
      title: route.meta.title as string,
      closable: route.path !== homePath.value,
    })
  },
  { immediate: true },
)

function isActive(tag: TagView): boolean {
  return tag.path === route.path
}

async function closeTag(tag: TagView): Promise<void> {
  const isCurrent = tag.path === route.path
  const next = appStore.removeTag(tag.path, route.path)
  // 只有关掉的是当前标签才需要跳转。removeTag 此时返回「下一个」标签，
  // 为 null 说明标签栏已空：必须回首页，否则路由停在一个不在标签栏的
  // 页面上，watch（path 未变化）也不会把它重新登记回来。
  if (isCurrent) {
    await router.push(next ?? homePath.value)
  }
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
    appStore.clearTags()
    await router.push(homePath.value)
  }
}
</script>

<template>
  <div class="tags-view">
    <el-scrollbar class="tags-view__scroll">
      <div class="tags-view__list">
        <router-link
          v-for="tag in appStore.tags"
          :key="tag.path"
          :to="tag.path"
          class="tags-view__item"
          :class="{ 'is-active': isActive(tag) }"
        >
          <span class="tags-view__dot" />
          <span class="tags-view__label">{{ tag.title }}</span>
          <el-icon
            v-if="tag.closable"
            class="tags-view__close"
            @click.prevent.stop="closeTag(tag)"
          >
            <AppIcon name="Close" />
          </el-icon>
        </router-link>
      </div>
    </el-scrollbar>

    <el-dropdown class="tags-view__more" trigger="click" @command="handleCommand">
      <button class="tags-view__more-btn" type="button">
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
  </div>
</template>

<style scoped lang="scss">
.tags-view {
  display: flex;
  align-items: center;
  height: var(--layout-tags-height);
  padding: 0 8px;
  background: var(--app-surface);
  border-bottom: 1px solid var(--app-border-light);

  &__scroll {
    flex: 1;
    min-width: 0;
  }

  &__list {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 0;
  }

  &__item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 28px;
    padding: 0 10px;
    font-size: 12px;
    color: var(--app-text-secondary);
    background: var(--app-fill);
    border: 1px solid transparent;
    border-radius: 999px;
    white-space: nowrap;
    transition: var(--transition-base);

    &:hover {
      color: var(--brand-primary);
      background: var(--app-fill-hover);
    }

    &.is-active {
      color: var(--brand-primary);
      background: var(--app-primary-soft);
      border-color: var(--app-primary-soft-border);
      font-weight: 600;

      .tags-view__dot {
        background: var(--brand-primary);
      }
    }
  }

  &__dot {
    width: 6px;
    height: 6px;
    background: var(--app-text-muted);
    border-radius: 50%;
  }

  &__close {
    font-size: 12px;
    border-radius: 50%;

    &:hover {
      color: #fff;
      background: var(--brand-primary);
    }
  }

  &__more {
    margin-left: 6px;
  }

  &__more-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    color: var(--app-text-secondary);
    background: var(--app-fill);
    border: none;
    border-radius: 50%;
    cursor: pointer;

    &:hover {
      color: var(--brand-primary);
      background: var(--app-fill-hover);
    }
  }
}
</style>
