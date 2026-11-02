<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { DropdownInstance } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore, usePermissionStore } from '@/store'
import AppIcon from '@/components/AppIcon.vue'
import type { TagView } from '@/store'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const permissionStore = usePermissionStore()

const listRef = ref<HTMLElement>()
const contextMenu = ref<DropdownInstance>()
const contextTag = ref<TagView>()
const contextTrigger = shallowRef<HTMLAnchorElement>()
const homePath = computed(() => permissionStore.homePath())
let pressTimer: ReturnType<typeof setTimeout> | undefined
let pressOrigin = { x: 0, y: 0 }
let suppressTouchClick = false

function cancelContextPress(): void {
  clearTimeout(pressTimer)
  pressTimer = undefined
}

function startContextPress(event: PointerEvent, tag: TagView): void {
  cancelContextPress()
  suppressTouchClick = false
  if (event.pointerType !== 'touch' || !event.isPrimary) return
  const item = event.currentTarget as HTMLElement
  pressOrigin = { x: event.clientX, y: event.clientY }
  pressTimer = setTimeout(() => {
    suppressTouchClick = true
    void showContextMenu(item, tag)
  }, 550)
}

function moveContextPress(event: PointerEvent): void {
  if (Math.hypot(event.clientX - pressOrigin.x, event.clientY - pressOrigin.y) > 8) {
    cancelContextPress()
  }
}

function onTagClick(event: MouseEvent): void {
  if (!suppressTouchClick) return
  suppressTouchClick = false
  event.preventDefault()
  event.stopPropagation()
  // The release click is outside the popup; keep the long-press menu open for the next tap.
  contextMenu.value?.handleOpen()
}

onBeforeUnmount(cancelContextPress)

// 进入页面时登记标签页；隐藏路由（如个人中心）不登记
watch(
  () => route.fullPath,
  () => {
    contextMenu.value?.handleClose()
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
  await focusActiveTag()
}

/**
 * 焦点回到当前标签；首页把整条导航卡隐藏了，此时焦点交给内容区，
 * 否则焦点会掉到 body，键盘用户得从头 Tab。
 */
async function focusActiveTag(): Promise<void> {
  await nextTick()
  const link = listRef.value?.querySelector<HTMLAnchorElement>('[aria-current="page"]')
  if (link && link.offsetParent !== null) {
    link.focus({ preventScroll: true })
    return
  }
  document.querySelector<HTMLElement>('#main-content')?.focus({ preventScroll: true })
}

function openContextMenu(event: MouseEvent | KeyboardEvent, tag: TagView): void {
  void showContextMenu(event.currentTarget as HTMLElement, tag)
}

async function showContextMenu(item: HTMLElement, tag: TagView): Promise<void> {
  const link = item.querySelector<HTMLAnchorElement>('.tags-view__link')
  if (!link) return
  contextTag.value = tag
  contextTrigger.value = link
  link.focus({ preventScroll: true })
  await nextTick()
  contextMenu.value?.handleOpen()
}

function onTagKeydown(event: KeyboardEvent, tag: TagView): void {
  if ((event.shiftKey && event.key === 'F10') || event.key === 'ContextMenu') {
    event.preventDefault()
    void openContextMenu(event, tag)
  }
}

function dismissContextMenu(): void {
  contextMenu.value?.handleClose()
  contextTrigger.value?.focus({ preventScroll: true })
}

async function onContextVisibility(visible: boolean): Promise<void> {
  if (!visible) return
  // Focus immediately; waiting for the opening transition can drop the first keystroke.
  await nextTick()
  contextMenu.value?.contentRef?.focus({ preventScroll: true })
}

async function handleCommand(command: string): Promise<void> {
  const tag = contextTag.value
  if (!tag) return
  if (command === 'others') {
    // Navigate before removing an active editor, so its unsaved-change guard can veto.
    const failure = isActive(tag) ? undefined : await router.push(tag.fullPath || tag.path)
    if (failure) return
    appStore.removeOtherTags(tag.path)
  } else if (command === 'close' && tag.closable) {
    await closeTag(tag)
  } else if (command === 'home') {
    const failure = route.path === homePath.value ? undefined : await router.push(homePath.value)
    if (failure) return
    appStore.removeOtherTags(homePath.value)
  }
  await focusActiveTag()
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
          @contextmenu.prevent="openContextMenu($event, tag)"
          @keydown="onTagKeydown($event, tag)"
          @pointerdown="startContextPress($event, tag)"
          @pointermove="moveContextPress"
          @pointerup="cancelContextPress"
          @pointercancel="cancelContextPress"
          @pointerleave="cancelContextPress"
          @click.capture="onTagClick"
        >
          <router-link
            :id="contextTag?.path === tag.path ? 'tag-context-trigger' : undefined"
            :to="tag.fullPath || tag.path"
            aria-haspopup="menu"
            aria-keyshortcuts="Shift+F10"
            title="右键或长按打开标签页操作（Shift+F10）"
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
    <el-dropdown
      id="tag-context-trigger"
      ref="contextMenu"
      class="tags-view__context"
      :virtual-ref="contextTrigger"
      virtual-triggering
      :trigger="[]"
      :trigger-keys="[]"
      placement="bottom-start"
      @command="handleCommand"
      @visible-change="onContextVisibility"
    >
      <template #dropdown>
        <el-dropdown-menu @keydown.esc.stop="dismissContextMenu">
          <el-dropdown-item command="home">关闭全部标签</el-dropdown-item>
          <el-dropdown-item
            command="others"
            :disabled="!appStore.tags.some((tag) => tag.closable && tag.path !== contextTag?.path)"
          >
            关闭其它标签
          </el-dropdown-item>
          <el-dropdown-item command="close" :disabled="!contextTag?.closable" divided>
            关闭当前标签
          </el-dropdown-item>
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
    padding: 6px 0;
  }
  &__item {
    display: inline-flex;
    flex: none;
    align-items: center;
    height: 32px;
    padding-right: 8px;
    color: var(--app-text);
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--radius-sm);
    white-space: nowrap;
    user-select: none;
    -webkit-touch-callout: none;
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
    font-size: 13px;
    border-radius: var(--radius-sm);
  }
  &__close {
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
  &__context {
    display: contents;
  }
  &__link:focus-visible,
  &__close:focus-visible {
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
