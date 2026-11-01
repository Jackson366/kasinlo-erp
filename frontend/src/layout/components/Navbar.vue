<script setup lang="ts">
import { inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
// 值一律从子路径导入：element-plus 根入口引用全部组件，会禁用 tree-shaking
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { useAppStore, usePermissionStore, useUserStore } from '@/store'
import { useLogout } from '@/composables/useLogout'
import AppIcon from '@/components/AppIcon.vue'
const emit = defineEmits<{ 'open-menu': [] }>()

const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()
const permissionStore = usePermissionStore()
const { logout } = useLogout()

const fullscreen = ref(false)

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
  // 清理（卸载动态路由、清标签、跳登录页）都在 composable 里，
  // 两个退出入口共用同一套，避免再出现「漏清标签」的半截会话
  await logout()
  ElMessage.success('已退出登录')
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
  <header class="navbar">
    <div class="navbar__left">
      <button
        class="navbar__icon-btn navbar__mobile-menu"
        type="button"
        aria-label="打开菜单"
        @click="emit('open-menu')"
      >
        <AppIcon name="Menu" />
      </button>
      <router-link
        :to="permissionStore.homePath()"
        class="navbar__brand"
        aria-label="Kasinlo ERP 首页"
      >
        <img src="/brand/kasinlo-mark.png" alt="" width="32" height="32" />
        <span>Kasinlo ERP</span>
      </router-link>
      <span class="navbar__badge">SaaS</span>
    </div>

    <div class="navbar__right">
      <el-tooltip content="刷新页面" placement="bottom">
        <button class="navbar__icon-btn" type="button" aria-label="刷新页面" @click="refreshPage()">
          <AppIcon name="Refresh" />
        </button>
      </el-tooltip>

      <el-tooltip :content="fullscreen ? '退出全屏' : '全屏显示'" placement="bottom">
        <button
          class="navbar__icon-btn"
          type="button"
          :aria-label="fullscreen ? '退出全屏' : '全屏显示'"
          @click="toggleFullscreen"
        >
          <AppIcon name="FullScreen" />
        </button>
      </el-tooltip>

      <el-tooltip
        :content="appStore.isDark ? '切换到亮色主题' : '切换到暗色主题'"
        placement="bottom"
      >
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
        <button type="button" class="navbar__user" aria-label="账户菜单">
          <el-avatar :size="30" :src="userStore.avatar || undefined" class="navbar__avatar">
            {{ userStore.displayName.slice(0, 1) }}
          </el-avatar>
          <span class="navbar__username">{{ userStore.displayName }}</span>
          <AppIcon name="ArrowDown" class="navbar__caret" />
        </button>
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
  </header>
</template>

<style scoped lang="scss">
.navbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--layout-navbar-height);
  padding: 0 28px;
  background: transparent;

  &__left,
  &__right {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  &__icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 38px;
    height: 38px;
    font-size: 21px;
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
    border-radius: var(--radius-sm);
    cursor: pointer;
    border: 0;
    background: transparent;
    font: inherit;
    transition: var(--transition-base);

    &:hover {
      background: var(--app-fill-hover);
    }
  }

  &__brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    color: var(--app-text);
    font-size: 20px;
    font-weight: 650;
    white-space: nowrap;
    img {
      display: block;
      object-fit: contain;
    }
  }
  &__badge {
    padding: 3px 9px;
    border-radius: var(--radius-sm);
    background: var(--app-primary-soft);
    color: var(--brand-primary);
    font-size: 12px;
  }
  &__mobile-menu {
    display: none;
  }
  &__icon-btn:focus-visible,
  &__user:focus-visible,
  &__brand:focus-visible {
    outline: 2px solid var(--brand-primary);
    outline-offset: 2px;
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
@media (max-width: 800px) {
  .navbar {
    padding: 0 12px;
    &__mobile-menu {
      display: inline-flex;
    }
    &__brand {
      font-size: 16px;
      gap: 8px;
    }
    &__left,
    &__right {
      gap: 4px;
    }
    &__badge,
    &__username {
      display: none;
    }
    &__icon-btn {
      width: 32px;
      height: 36px;
      font-size: 18px;
    }
    &__user {
      padding-inline: 4px;
    }
  }
}
@media (max-width: 480px) {
  .navbar__right > .navbar__icon-btn:not(.navbar__icon-btn--theme) {
    display: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .navbar__icon-btn--theme :deep(.app-icon) {
    animation: none;
  }
}
</style>
