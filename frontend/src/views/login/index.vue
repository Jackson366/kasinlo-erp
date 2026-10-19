<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormInstance, FormRules } from 'element-plus'
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
    // 触发时机补 change：只挂 blur 的话，用户「输入 → 直接回车」这条最常见的路径
    // 全程不会失焦，长度不足的错误提示要等到点登录之后的往返才出现。
    // 用户名同理，回车提交时同样处于未失焦状态。
    { min: 6, max: 64, message: '密码长度为 6 ~ 64 个字符', trigger: ['blur', 'change'] },
  ],
}

async function handleLogin(): Promise<void> {
  // 前置守卫：validate 是异步的，从点击到 loading 置位之间有一小段空窗，
  // 连点两下会并发发出两次登录请求（后者还可能把前者的跳转结果覆盖掉）。
  if (loading.value) return

  // formRef 可能因挂载时序尚未就绪，此时 validate 是 undefined。
  // 原写法 `if (!valid) return` 会把这个 undefined 当成「校验未通过」静默吞掉——
  // 首屏点登录按钮毫无反应，也没有任何提示。必须显式兜底。
  if (!formRef.value) {
    ElMessage.warning('表单尚未就绪，请稍后重试')
    return
  }
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    // 校验失败时把焦点交给第一个出错的字段，
    // 否则用户看到红字还要自己找是哪一栏错了。
    focusFirstInvalidField()
    return
  }

  loading.value = true
  try {
    await userStore.login({ ...form })
    // 登录后权限缓存需要重新生成
    permissionStore.reset()
    // 标签栏属于「上一个登录人」的浏览现场：换账号登录时若不清理，
    // 新账号会看到（甚至能点回）旧账号访问过的页面。登出路径有同样的
    // 三件套清理，但登录这条路径绕过了它，这里必须补上。
    appStore.clearTags()
    ElMessage.success(`欢迎回来，${userStore.displayName}`)
    const redirect = (route.query.redirect as string) || '/'
    await router.replace(redirect)
  } catch {
    // 具体错误提示已由请求层统一处理
  } finally {
    loading.value = false
  }
}

/** 演示账号入口只在开发构建可见 */
const showDemo = import.meta.env.DEV

/** 一键填入演示账号 */
function fillDemo(): void {
  form.username = 'demo'
  form.password = 'demo123456'
  // 填值后清掉已有的校验状态：否则上一轮输错留下的红字会挂在输入框上，
  // 而框里的内容其实已经换成合法值了。
  formRef.value?.clearValidate()
}

/**
 * 校验失败时把焦点交给第一个出错的字段。
 *
 * 不用 `nextTick` 之外的手段：element-plus 在校验失败后同步就把 `is-error` 类打上了，
 * 这里只需要拿到对应输入框的 DOM 节点。
 */
function focusFirstInvalidField(): void {
  const item = document.querySelector('.login .el-form-item.is-error .el-input__inner')
  if (item instanceof HTMLInputElement) {
    item.focus()
    item.select()
  }
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
        <h1 class="login__title">Nova ERP</h1>
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

        <!-- 演示账号只在开发构建里出现：生产构建带出去等于把默认凭据印在登录页上 -->
        <div v-if="showDemo" class="login__hint">
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
