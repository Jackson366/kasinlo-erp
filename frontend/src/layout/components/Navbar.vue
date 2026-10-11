<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAppStore, usePermissionStore, useUserStore } from '@/store'
import AppIcon from '@/components/AppIcon.vue'
import Breadcrumb from './Breadcrumb.vue'

const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()
const permissionStore = usePermissionStore()

const fullscreen = ref(false)
const collapsed = computed(() => appStore.sidebarCollapsed)

// 由 layout 注入的页面刷新方法；未注入时退化为整页刷新
const refreshPage = inject<() => void>('pageRefresh', () => window.location.reload())

function toggleFullscreen(): void {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen().catch(() => {
      ElMessage.warning('当前浏览器不支持全屏')
    })
  } else {
    document.exitFullscreen().catch(() => undefined)
  }
}

// 监听器必须随组件卸载而移除：Navbar 挂在 Layout 上，
// 「退出登录 -> 再登录」会重新 setup 并再挂一个监听器，
// 旧的监听器持有已卸载组件的 ref，持续泄漏。
function onFullscreenChange(): void {
  fullscreen.value = !!document.fullscreenElement
}
onMounted(() => document.addEventListener('fullscreenchange', onFullscreenChange))
onBeforeUnmount(() => document.removeEventListener('fullscreenchange', onFullscreenChange))

async function handleLogout(): Promise<void> {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '退出',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return // 用户取消
  }
  await userStore.logout()
  permissionStore.reset()
  appStore.clearTags()
  ElMessage.success('已退出登录')
  router.push('/login')
}

function handleCommand(command: string): void {
  if (command === 'logout') {
    handleLogout()
  } else if (command === 'profile') {
    router.push('/profile')
  }
}
</script>

<template>
  <div class="navbar">
    <div class="navbar__left">
      <el-tooltip :content="collapsed ? '展开菜单' : '收起菜单'" placement="bottom">
        <button class="navbar__icon-btn" type="button" @click="appStore.toggleSidebar()">
          <AppIcon :name="collapsed ? 'Expand' : 'Fold'" />
        </button>
      </el-tooltip>
      <Breadcrumb />
    </div>

    <div class="navbar__right">
      <el-tooltip content="刷新页面" placement="bottom">
        <button class="navbar__icon-btn" type="button" @click="refreshPage()">
          <AppIcon name="Refresh" />
        </button>
      </el-tooltip>

      <el-tooltip :content="fullscreen ? '退出全屏' : '全屏显示'" placement="bottom">
        <button class="navbar__icon-btn" type="button" @click="toggleFullscreen">
          <AppIcon name="FullScreen" />
        </button>
      </el-tooltip>

      <el-tooltip :content="appStore.isDark ? '切换到亮色主题' : '切换到暗色主题'" placement="bottom">
        <button
          class="navbar__icon-btn navbar__icon-btn--theme"
          type="button"
          :aria-label="appStore.isDark ? '切换到亮色主题' : '切换到暗色主题'"
          @click="appStore.toggleTheme()"
        >
          <!-- key 让两个图标在切换时重新挂载，从而播放旋转过渡 -->
          <AppIcon :key="appStore.theme" :name="appStore.isDark ? 'Sunny' : 'Moon'" />
        </button>
      </el-tooltip>

      <el-dropdown trigger="click" @command="handleCommand">
        <div class="navbar__user">
          <el-avatar :size="30" :src="userStore.avatar || undefined" class="navbar__avatar">
            {{ userStore.displayName.slice(0, 1) }}
          </el-avatar>
          <span class="navbar__username">{{ userStore.displayName }}</span>
          <AppIcon name="ArrowDown" class="navbar__caret" />
        </div>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="profile">
              <AppIcon name="User" />
              <span>个人中心</span>
            </el-dropdown-item>
            <el-dropdown-item command="logout" divided>
              <AppIcon name="SwitchButton" />
              <span>退出登录</span>
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>
</template>

<style scoped lang="scss">
.navbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--layout-navbar-height);
  padding: 0 16px 0 8px;
  background: var(--app-surface);
  border-bottom: 1px solid var(--app-border-light);

  &__left,
  &__right {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  &__icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    font-size: 16px;
    color: var(--app-text-secondary);
    background: transparent;
    border: none;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: var(--transition-base);

    &:hover {
      color: var(--brand-primary);
      background: var(--app-fill-hover);
    }

    // 主题按钮：切换时图标重挂载，借动画表达「翻转」
    &--theme :deep(.app-icon) {
      animation: navbar-theme-turn 0.42s cubic-bezier(0.4, 0, 0.2, 1);
    }
  }

  &__user {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 38px;
    padding: 0 10px 0 6px;
    border-radius: 999px;
    cursor: pointer;
    outline: none;
    transition: var(--transition-base);

    &:hover {
      background: var(--app-fill-hover);
    }
  }

  &__avatar {
    background: var(--brand-gradient);
    color: #fff;
    font-weight: 600;
  }

  &__username {
    max-width: 132px;
    overflow: hidden;
    font-size: 13px;
    font-weight: 500;
    color: var(--app-text);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__caret {
    font-size: 12px;
    color: var(--app-text-muted);
  }
}

@keyframes navbar-theme-turn {
  from {
    opacity: 0;
    transform: rotate(-90deg) scale(0.6);
  }

  to {
    opacity: 1;
    transform: rotate(0) scale(1);
  }
}
</style>
