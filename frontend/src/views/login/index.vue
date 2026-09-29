<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { usePermissionStore, useUserStore, useAppStore } from '@/store'
import AppIcon from '@/components/AppIcon.vue'
import type { LoginParams } from '@/types/api'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const permissionStore = usePermissionStore()
const appStore = useAppStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive<LoginParams>({ username: '', password: '' })

const rules: FormRules<LoginParams> = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度为 3 ~ 64 个字符', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, max: 64, message: '密码长度为 6 ~ 64 个字符', trigger: 'blur' },
  ],
}

async function handleLogin(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await userStore.login({ ...form })
    // 登录后权限缓存需要重新生成
    permissionStore.reset()
    ElMessage.success(`欢迎回来，${userStore.displayName}`)
    const redirect = (route.query.redirect as string) || '/'
    await router.replace(redirect)
  } catch {
    // 具体错误提示已由请求层统一处理
  } finally {
    loading.value = false
  }
}

/** 一键填入演示账号 */
function fillDemo(): void {
  form.username = 'demo'
  form.password = 'demo123456'
}
</script>

<template>
  <div class="login">
    <!-- 主题切换放在登录页也能用，避免暗色用户每次进来都被闪一下 -->
    <el-tooltip
      :content="appStore.isDark ? '切换到亮色主题' : '切换到暗色主题'"
      placement="bottom"
    >
      <button
        class="login__theme"
        type="button"
        :aria-label="appStore.isDark ? '切换到亮色主题' : '切换到暗色主题'"
        @click="appStore.toggleTheme()"
      >
        <AppIcon :key="appStore.theme" :name="appStore.isDark ? 'Sunny' : 'Moon'" />
      </button>
    </el-tooltip>

    <!-- 左侧品牌区 -->
    <section class="login__brand">
      <div class="login__brand-inner">
        <div class="login__logo">N</div>
        <h1 class="login__title">Nova Admin</h1>
        <p class="login__slogan">面向业务的现代化后台管理系统</p>

        <ul class="login__features">
          <li>
            <AppIcon name="CircleCheck" />
            <span>统一权限体系：用户 · 角色 · 菜单 · 部门</span>
          </li>
          <li>
            <AppIcon name="CircleCheck" />
            <span>菜单驱动动态路由，前后端权限一次配置</span>
          </li>
          <li>
            <AppIcon name="CircleCheck" />
            <span>操作日志全量留痕，关键变更可追溯</span>
          </li>
          <li>
            <AppIcon name="CircleCheck" />
            <span>Go + Vue3 + PostgreSQL，接口规范清晰</span>
          </li>
        </ul>
      </div>
      <div class="login__glow login__glow--a" />
      <div class="login__glow login__glow--b" />
    </section>

    <!-- 右侧登录表单 -->
    <section class="login__panel">
      <div class="login__card">
        <header class="login__card-head">
          <h2>账号登录</h2>
          <p class="text-secondary">请输入您的账号信息以继续</p>
        </header>

        <el-form ref="formRef" :model="form" :rules="rules" size="large" @keyup.enter="handleLogin">
          <el-form-item prop="username">
            <el-input v-model="form.username" placeholder="用户名" clearable autocomplete="username">
              <template #prefix>
                <AppIcon name="User" />
              </template>
            </el-input>
          </el-form-item>

          <el-form-item prop="password">
            <el-input
              v-model="form.password"
              type="password"
              placeholder="密码"
              show-password
              autocomplete="current-password"
            >
              <template #prefix>
                <AppIcon name="Lock" />
              </template>
            </el-input>
          </el-form-item>

          <el-form-item>
            <el-button type="primary" class="login__submit" :loading="loading" @click="handleLogin">
              登 录
            </el-button>
          </el-form-item>
        </el-form>

        <div class="login__hint">
          <span class="text-muted">演示账号</span>
          <el-tag size="small" type="info" effect="plain">demo / demo123456</el-tag>
          <el-button link type="primary" size="small" @click="fillDemo">一键填入</el-button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped lang="scss">
.login {
  position: relative;
  display: flex;
  height: 100%;
  background: var(--app-surface);

  &__theme {
    position: absolute;
    top: 18px;
    right: 20px;
    z-index: 6;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    font-size: 16px;
    color: var(--app-text-secondary);
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: 50%;
    cursor: pointer;
    transition: var(--transition-base);

    &:hover {
      color: var(--brand-primary);
      border-color: var(--brand-primary);
    }

    :deep(.app-icon) {
      animation: login-theme-turn 0.42s cubic-bezier(0.4, 0, 0.2, 1);
    }
  }

  &__brand {
    position: relative;
    display: none;
    flex: 1.05;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    background: linear-gradient(140deg, #1e3a8a 0%, #2563eb 52%, #3b82f6 100%);

    @media (min-width: 900px) {
      display: flex;
    }
  }

  &__brand-inner {
    position: relative;
    z-index: 2;
    max-width: 420px;
    padding: 0 32px;
    color: #fff;
  }

  &__logo {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 50px;
    height: 50px;
    margin-bottom: 24px;
    font-size: 24px;
    font-weight: 700;
    background: rgba(255, 255, 255, 0.18);
    border: 1px solid rgba(255, 255, 255, 0.32);
    border-radius: 14px;
    backdrop-filter: blur(6px);
  }

  &__title {
    margin: 0 0 10px;
    font-size: 30px;
    font-weight: 700;
    letter-spacing: 0.5px;
  }

  &__slogan {
    margin: 0 0 34px;
    font-size: 14px;
    color: rgba(255, 255, 255, 0.82);
  }

  &__features {
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 0;
    margin: 0;
    list-style: none;

    li {
      display: flex;
      align-items: center;
      gap: 10px;
      font-size: 13.5px;
      color: rgba(255, 255, 255, 0.92);
    }

    :deep(.app-icon) {
      font-size: 16px;
      color: #bfdbfe;
    }
  }

  &__glow {
    position: absolute;
    border-radius: 50%;
    filter: blur(70px);

    &--a {
      top: -70px;
      right: -60px;
      width: 280px;
      height: 280px;
      background: rgba(96, 165, 250, 0.45);
    }

    &--b {
      bottom: -80px;
      left: -60px;
      width: 320px;
      height: 320px;
      background: rgba(59, 130, 246, 0.35);
    }
  }

  &__panel {
    display: flex;
    flex: 1;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }

  &__card {
    width: 100%;
    max-width: 380px;
  }

  &__card-head {
    margin-bottom: 28px;

    h2 {
      margin: 0 0 8px;
      font-size: 22px;
      font-weight: 700;
      color: var(--app-text);
    }

    p {
      margin: 0;
      font-size: 13px;
    }
  }

  &__submit {
    width: 100%;
    font-size: 15px;
    letter-spacing: 2px;
  }

  &__hint {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-top: 16px;
    margin-top: 4px;
    font-size: 12px;
    border-top: 1px dashed var(--app-border);
  }
}

@keyframes login-theme-turn {
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
