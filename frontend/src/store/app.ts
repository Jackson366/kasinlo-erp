import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { applyTheme, persistTheme, resolveInitialTheme, type ThemeMode } from '@/utils/theme'

/** 侧边栏访问过的标签页 */
export interface TagView {
  path: string
  fullPath?: string
  title: string
  closable: boolean
}

const PRIMARY_SIDEBAR_KEY = 'nova_sidebar_primary_collapsed'

/** 应用级 UI 状态：侧边栏折叠、标签页、主题 */
export const useAppStore = defineStore('app', () => {
  const primarySidebarCollapsed = ref(localStorage.getItem(PRIMARY_SIDEBAR_KEY) === '1')
  const tags = ref<TagView[]>([])

  /** 当前主题；初始值来自本地存储 / 系统偏好 */
  const theme = ref<ThemeMode>(resolveInitialTheme())

  /** 是否暗色主题，供模板做分支（如切换按钮的图标） */
  const isDark = computed(() => theme.value === 'dark')

  /** Single sidebar collapse preference; narrow-screen drawer always stays expanded. */
  function togglePrimarySidebar(): void {
    primarySidebarCollapsed.value = !primarySidebarCollapsed.value
    localStorage.setItem(PRIMARY_SIDEBAR_KEY, primarySidebarCollapsed.value ? '1' : '0')
  }

  /** 设置主题：同时更新状态、DOM 与本地存储 */
  function setTheme(mode: ThemeMode): void {
    theme.value = mode
    applyTheme(mode)
    persistTheme(mode)
  }

  /** 在亮色 / 暗色之间切换 */
  function toggleTheme(): void {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  /** 新增标签页，重复则忽略 */
  function addTag(tag: TagView): void {
    if (!tag.title) return
    const exist = tags.value.find((item) => item.path === tag.path)
    if (exist) {
      exist.fullPath = tag.fullPath
      return
    }
    tags.value.push(tag)
  }

  /** 关闭标签页，返回关闭后应跳转的路径（若关闭的是当前页） */
  function removeTag(path: string, currentPath: string): string | null {
    const index = tags.value.findIndex((item) => item.path === path)
    if (index === -1) return null
    tags.value.splice(index, 1)
    if (path !== currentPath) return null
    const next = tags.value[index] || tags.value[index - 1]
    return next ? next.path : null
  }

  /** 关闭其它标签 */
  function removeOtherTags(path: string): void {
    tags.value = tags.value.filter((item) => !item.closable || item.path === path)
  }

  /** 清空标签 */
  function clearTags(): void {
    tags.value = []
  }

  return {
    primarySidebarCollapsed,
    tags,
    theme,
    isDark,
    togglePrimarySidebar,
    setTheme,
    toggleTheme,
    addTag,
    removeTag,
    removeOtherTags,
    clearTags,
  }
})
