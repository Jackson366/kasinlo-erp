<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormInstance, FormRules } from 'element-plus'
import { usePermissionStore, useUserStore, useAppStore } from '@/store'
import AppIcon from '@/components/AppIcon.vue'
import type { LoginParams } from '@/types/api'
import { ApiError } from '@/utils/request'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const permissionStore = usePermissionStore()
const appStore = useAppStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const passwordVisible = ref(false)
const loginError = ref('')
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
    {
      min: 6,
      max: 64,
      message: '密码长度为 6 ~ 64 个字符',
      trigger: ['blur', 'change'],
    },
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
  loading.value = true
  loginError.value = ''
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    // 校验失败时把焦点交给第一个出错的字段，
    // 否则用户看到红字还要自己找是哪一栏错了。
    focusFirstInvalidField()
    loading.value = false
    return
  }

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
  } catch (error) {
    // 请求层继续统一轻提示，表单就近保留失败原因，便于修正后重试。
    loginError.value =
      error instanceof ApiError ? error.message : '登录失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

/** 演示账号入口只在开发构建可见 */
const showDemo = import.meta.env.DEV

/** 一键填入演示账号 */
function fillDemo(): void {
  if (loading.value) return
  loginError.value = ''
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
  const item = document.querySelector(
    '.login .el-form-item.is-error .el-input__inner',
  )
  if (item instanceof HTMLInputElement) {
    item.focus()
    item.select()
  }
}
</script>

<template>
  <main class="login">
    <div class="login__decoration" aria-hidden="true">
      <div class="login__arc login__arc--top" />
      <div class="login__arc login__arc--left" />
      <div class="login__arc login__arc--bottom" />
    </div>

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
        <AppIcon
          :name="appStore.isDark ? 'Sunny' : 'Moon'"
          aria-hidden="true"
        />
      </button>
    </el-tooltip>

    <div class="login__content">
      <section class="login__brand" aria-labelledby="login-brand-title">
        <div class="login__identity">
          <img src="/brand/kasinlo-mark.png" alt="" width="52" height="52" />
          <span class="login__name">Kasinlo ERP</span>
          <span class="login__badge">SaaS</span>
        </div>
        <h1 id="login-brand-title" class="login__title">
          新一代云端 ERP，<br />让企业运营更高效
        </h1>
        <p class="login__description">
          覆盖采购、销售、库存、财务等核心业务，<br
            class="login__desktop-break"
          />
          数据实时协同，助力企业精细化管理与增长。
        </p>
        <ul class="login__features">
          <li>
            <span class="login__feature-icon"
              ><AppIcon name="Collection" aria-hidden="true" /></span
            >业务一体化
          </li>
          <li>
            <span class="login__feature-icon"
              ><AppIcon name="Lightning" aria-hidden="true" /></span
            >高效协同
          </li>
          <li>
            <span class="login__feature-icon"
              ><AppIcon name="Cloudy" aria-hidden="true" /></span
            >安全可靠
          </li>
        </ul>
      </section>

      <section class="login__card" aria-labelledby="login-heading">
        <header class="login__card-head">
          <h2 id="login-heading">欢迎登录</h2>
          <p>请输入您的账号信息以继续使用 Kasinlo ERP</p>
        </header>

        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          size="large"
          label-width="0"
          novalidate
          :aria-busy="loading"
          @submit.prevent="handleLogin"
        >
          <el-form-item prop="username" label="用户名" for="login-username">
            <el-input
              id="login-username"
              v-model="form.username"
              placeholder="用户名"
              clearable
              autocomplete="username"
              :aria-invalid="
                formRef?.getField('username')?.validateState === 'error'
              "
              :aria-describedby="
                formRef?.getField('username')?.validateState === 'error'
                  ? 'login-username-error'
                  : undefined
              "
            >
              <template #prefix
                ><AppIcon name="User" aria-hidden="true"
              /></template>
            </el-input>
            <template #error="{ error }">
              <div id="login-username-error" class="el-form-item__error">
                {{ error }}
              </div>
            </template>
          </el-form-item>

          <el-form-item prop="password" label="密码" for="login-password">
            <el-input
              id="login-password"
              v-model="form.password"
              :type="passwordVisible ? 'text' : 'password'"
              placeholder="密码"
              autocomplete="current-password"
              :aria-invalid="
                formRef?.getField('password')?.validateState === 'error'
              "
              :aria-describedby="
                formRef?.getField('password')?.validateState === 'error'
                  ? 'login-password-error'
                  : undefined
              "
            >
              <template #prefix
                ><AppIcon name="Lock" aria-hidden="true"
              /></template>
              <template #suffix>
                <button
                  class="login__password-toggle"
                  type="button"
                  :aria-label="passwordVisible ? '隐藏密码' : '显示密码'"
                  :aria-pressed="passwordVisible"
                  aria-controls="login-password"
                  @click="passwordVisible = !passwordVisible"
                >
                  <AppIcon
                    :name="passwordVisible ? 'Hide' : 'View'"
                    aria-hidden="true"
                  />
                </button>
              </template>
            </el-input>
            <template #error="{ error }">
              <div id="login-password-error" class="el-form-item__error">
                {{ error }}
              </div>
            </template>
          </el-form-item>

          <el-button
            type="primary"
            native-type="submit"
            class="login__submit"
            :loading="loading"
            >登 录</el-button
          >
          <div class="login__feedback" role="alert" aria-live="polite">
            <span v-if="loginError">{{ loginError }}</span>
          </div>
        </el-form>

        <!-- 保持原有可见性边界：不把演示凭据带入生产构建。 -->
        <div v-if="showDemo" class="login__hint">
          <span>演示账号</span>
          <span class="login__demo-account">demo / demo123456</span>
          <el-button link type="primary" :disabled="loading" @click="fillDemo"
            >一键填入</el-button
          >
        </div>
      </section>
    </div>
  </main>
</template>

<style scoped lang="scss">
.login {
  position: relative;
  isolation: isolate;
  display: grid;
  align-items: center;
  min-height: 100%;
  padding: 80px 64px;
  background: var(--app-login-bg);

  &__decoration {
    position: absolute;
    z-index: -1;
    inset: 0;
    overflow: hidden;
    pointer-events: none;
  }

  &__arc {
    position: absolute;
    width: 1000px;
    height: 1000px;
    border-radius: 50%;

    &--top {
      top: -180px;
      right: -660px;
      background: var(--app-login-arc-blue);
    }

    &--left {
      bottom: -730px;
      left: -580px;
      background: var(--app-login-arc-blue);
    }

    &--bottom {
      bottom: -860px;
      left: -180px;
      width: 1050px;
      background: var(--app-login-arc-violet);
    }
  }

  &__content {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 674px);
    align-items: center;
    gap: clamp(48px, 5vw, 88px);
    width: 100%;
    max-width: 1352px;
    margin: 0 auto;
  }

  &__identity {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 80px;

    img {
      flex: none;
      object-fit: contain;
      border-radius: var(--radius-md);
    }
  }

  &__name {
    font-size: 32px;
    font-weight: 700;
    letter-spacing: -0.8px;
    white-space: nowrap;
  }

  &__badge {
    padding: 5px 15px;
    font-size: 23px;
    font-weight: 600;
    line-height: 1.4;
    color: var(--brand-primary);
    background: var(--app-primary-soft);
    border-radius: var(--radius-md);
  }

  &__title {
    margin: 0;
    font-size: clamp(36px, 3.1vw, 52px);
    font-weight: 600;
    line-height: 1.45;
    letter-spacing: 1px;
  }

  &__description {
    margin: 26px 0 0;
    font-size: clamp(17px, 1.45vw, 24px);
    line-height: 1.8;
    color: var(--app-login-secondary);
  }

  &__features {
    display: flex;
    flex-wrap: wrap;
    gap: 24px 40px;
    margin: 58px 0 0;
    padding: 0;
    list-style: none;

    li {
      display: flex;
      align-items: center;
      gap: 14px;
      font-size: 20px;
      color: var(--app-text-secondary);
      white-space: nowrap;
    }
  }

  &__feature-icon {
    display: grid;
    place-items: center;
    width: 54px;
    height: 54px;
    font-size: 28px;
    color: var(--brand-primary);
    background: var(--app-login-icon-bg);
    border-radius: var(--radius-md);
  }

  &__card {
    min-width: 0;
    padding: 58px 62px 60px;
    background: var(--app-login-card);
    border: 1px solid var(--app-login-card-border);
    border-radius: 44px;
    box-shadow: var(--app-login-shadow);
  }

  &__card-head {
    margin-bottom: 36px;

    h2 {
      margin: 0 0 8px;
      font-size: 44px;
      font-weight: 600;
      line-height: 1.4;
    }

    p {
      margin: 0;
      font-size: 22px;
      line-height: 1.6;
      color: var(--app-login-secondary);
    }
  }

  :deep(.el-form-item) {
    margin-bottom: 24px;
  }

  :deep(.el-form-item__label) {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  :deep(.el-form-item__error) {
    font-size: 12px;
    line-height: 18px;
    padding-top: 3px;
  }

  :deep(.el-input) {
    --el-input-bg-color: var(--app-login-input);
    --el-input-border-color: transparent;
    --el-input-hover-border-color: var(--app-primary-soft-border);
    --el-input-focus-border-color: var(--brand-primary);
    --el-input-border-radius: 20px;
    --el-input-height: 66px;
    --el-input-text-color: var(--app-text);
    --el-input-placeholder-color: var(--app-login-secondary);
    font-size: 23px;
  }

  :deep(.el-input__wrapper) {
    padding: 0 24px;
  }

  :deep(.el-input__prefix) {
    margin-right: 12px;
    font-size: 26px;
    color: var(--app-login-secondary);
  }

  &__password-toggle,
  &__theme {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    padding: 0;
    color: var(--app-login-secondary);
    border: 0;
    border-radius: var(--radius-sm);
    cursor: pointer;

    &:hover {
      color: var(--brand-primary);
      background: var(--app-primary-soft);
    }

    &:active {
      color: var(--brand-primary-active);
    }

    &:focus-visible {
      outline: 2px solid var(--brand-primary);
      outline-offset: 3px;
    }
  }

  &__password-toggle {
    width: 40px;
    height: 44px;
    font-size: 24px;
    background: transparent;
  }

  &__theme {
    position: absolute;
    top: 24px;
    right: 28px;
    width: 40px;
    height: 40px;
    font-size: 20px;
    background: var(--app-login-card);
  }

  &__submit {
    --el-button-bg-color: transparent;
    --el-button-border-color: transparent;
    --el-button-hover-bg-color: transparent;
    --el-button-hover-border-color: transparent;
    --el-button-active-bg-color: transparent;
    --el-button-active-border-color: transparent;
    --el-button-disabled-bg-color: transparent;
    --el-button-disabled-border-color: transparent;
    --el-button-text-color: var(--app-login-on-action);
    --el-button-hover-text-color: var(--app-login-on-action);
    --el-button-active-text-color: var(--app-login-on-action);
    --el-button-disabled-text-color: var(--app-login-on-action);
    width: 100%;
    height: 62px;
    margin-top: 10px;
    font-size: 24px;
    font-weight: 600;
    letter-spacing: 4px;
    background: var(--app-login-action);
    border-radius: 19px;
    cursor: pointer;

    &:hover {
      background: var(--app-login-action-hover);
    }
    &:active {
      background: var(--app-login-action-active);
    }
    &:focus-visible {
      outline: 3px solid var(--brand-primary);
      outline-offset: 4px;
    }
    &.is-loading {
      cursor: wait;
    }
  }

  &__feedback {
    min-height: 24px;
    padding-top: 5px;
    font-size: 13px;
    line-height: 19px;
    color: var(--app-danger);
  }

  &__hint {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px 18px;
    margin-top: 8px;
    font-size: 18px;
    line-height: 1.6;
    color: var(--app-login-secondary);

    .el-button {
      min-height: 36px;
      margin: 0;
      font-size: inherit;
      &:focus-visible {
        outline: 2px solid var(--brand-primary);
        outline-offset: 3px;
      }
    }
  }

  &__demo-account {
    padding: 4px 16px;
    font-size: 18px;
    background: var(--app-surface);
    border: 1px solid var(--app-primary-soft-border);
    border-radius: 24px;
    box-shadow: var(--shadow-card);
    white-space: nowrap;
  }
}

@media (max-width: 1439px) and (min-width: 1001px) {
  .login {
    padding: 64px 48px;
    &__content {
      grid-template-columns: minmax(0, 1fr) minmax(0, 560px);
      gap: 48px;
    }
    &__identity {
      gap: 12px;
      margin-bottom: 60px;
    }
    &__name {
      font-size: 28px;
    }
    &__badge {
      font-size: 18px;
    }
    &__card {
      padding: 48px 44px;
    }
    &__card-head h2 {
      font-size: 36px;
    }
    &__card-head p {
      font-size: 18px;
    }
    &__features {
      gap: 20px;
      margin-top: 40px;
    }
    &__features li {
      gap: 10px;
      font-size: 16px;
    }
    &__feature-icon {
      width: 44px;
      height: 44px;
      font-size: 24px;
    }
    &__desktop-break {
      display: none;
    }
  }
}

@media (max-width: 1000px) {
  .login {
    padding: 72px 24px 40px;
    &__content {
      max-width: 560px;
      grid-template-columns: minmax(0, 1fr);
      gap: 32px;
    }
    &__identity {
      justify-content: center;
      margin: 0;
      gap: 12px;
    }
    &__identity img {
      width: 44px;
      height: 44px;
    }
    &__name {
      font-size: 26px;
    }
    &__badge {
      font-size: 17px;
      padding: 4px 10px;
    }
    &__title,
    &__description,
    &__features {
      display: none;
    }
    &__card {
      padding: 40px 32px;
      border-radius: 32px;
    }
    &__card-head {
      margin-bottom: 30px;
    }
    &__card-head h2 {
      font-size: 32px;
    }
    &__card-head p {
      font-size: 16px;
    }
    :deep(.el-input) {
      --el-input-height: 58px;
      --el-input-border-radius: 16px;
      font-size: 18px;
    }
    :deep(.el-input__wrapper) {
      padding: 0 18px;
    }
    :deep(.el-input__prefix) {
      font-size: 22px;
    }
    &__submit {
      height: 56px;
      font-size: 20px;
      border-radius: 16px;
    }
    &__hint {
      gap: 8px 12px;
      font-size: 14px;
    }
    &__demo-account {
      padding: 4px 10px;
      font-size: 14px;
    }
    &__arc--top {
      right: -850px;
    }
  }
}

@media (max-width: 400px) {
  .login {
    padding-right: 16px;
    padding-left: 16px;
    &__card {
      padding: 32px 24px;
    }
    &__name {
      font-size: 23px;
    }
    &__badge {
      font-size: 15px;
    }
  }
}

@media (prefers-reduced-motion: reduce) {
  .login :deep(*) {
    transition: none;
    animation: none;
  }
}
</style>
