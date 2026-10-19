<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormInstance, FormRules } from 'element-plus'
import { profileApi } from '@/api'
import type { ProfileInfo } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { useUserStore } from '@/store'
import { useLogout } from '@/composables/useLogout'

defineOptions({ name: 'Profile' })

const userStore = useUserStore()
const { logout } = useLogout()

const loading = ref(true)
const profile = ref<ProfileInfo | null>(null)

async function loadProfile(): Promise<void> {
  loading.value = true
  try {
    profile.value = await userStore.fetchProfile()
    Object.assign(infoForm, {
      nickname: profile.value.nickname,
      email: profile.value.email,
      phone: profile.value.phone,
      avatar: profile.value.avatar,
    })
  } finally {
    loading.value = false
  }
}

// ---------------- 基本资料 ----------------
const infoFormRef = ref<FormInstance>()
const infoSubmitting = ref(false)
const infoForm = reactive({ nickname: '', email: '', phone: '', avatar: '' })

const infoRules: FormRules<typeof infoForm> = {
  nickname: [
    { required: true, message: '请输入昵称', trigger: 'blur' },
    { max: 64, message: '昵称不能超过 64 个字符', trigger: 'blur' },
  ],
  email: [{ type: 'email', message: '邮箱格式不正确', trigger: 'blur' }],
}

async function submitInfo(): Promise<void> {
  // 不能写成 infoFormRef.value?.validate().catch(...)：表单未挂载时 ?. 短路出
  // undefined，再 .catch 就是 TypeError，整个提交静默崩溃。
  const valid = infoFormRef.value ? await infoFormRef.value.validate().catch(() => false) : false
  if (!valid) return
  infoSubmitting.value = true
  try {
    await profileApi.update({ ...infoForm })
    ElMessage.success('资料已更新')
    await loadProfile()
  } finally {
    infoSubmitting.value = false
  }
}

// ---------------- 修改密码 ----------------
const pwdFormRef = ref<FormInstance>()
const pwdSubmitting = ref(false)
const pwdForm = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' })

const pwdRules: FormRules<typeof pwdForm> = {
  oldPassword: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, max: 64, message: '密码长度为 8 ~ 64 个字符', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (value !== pwdForm.newPassword) {
          callback(new Error('两次输入的密码不一致'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
}

async function submitPassword(): Promise<void> {
  // 不能写成 pwdFormRef.value?.validate().catch(...)：表单未挂载时 ?. 短路出
  // undefined，再 .catch 就是 TypeError，整个提交静默崩溃。
  const valid = pwdFormRef.value ? await pwdFormRef.value.validate().catch(() => false) : false
  if (!valid) return
  pwdSubmitting.value = true
  try {
    await profileApi.changePassword({
      oldPassword: pwdForm.oldPassword,
      newPassword: pwdForm.newPassword,
    })
    ElMessage.success('密码修改成功，请重新登录')
    // 后端已通过 token_epoch 让旧令牌失效，前端必须真的退出并跳登录页，
    // 否则提示与行为不符：用户停留在页面上，下一个请求就会 401。
    // 退出清理（清令牌、卸载动态路由、清标签页）走 useLogout，
    // 与 Navbar 的退出登录同一套——少了后两步，重新登录后会挂着
    // 上一个账号的标签页，而对应路由已卸载，点哪个都 404。
    await logout()
  } finally {
    pwdSubmitting.value = false
  }
}

const initials = computed(() => (profile.value?.nickname || profile.value?.username || 'N').slice(0, 1))

/** 权限按模块前缀分组，便于阅读 */
const permissionGroups = computed(() => {
  const list = profile.value?.permissions ?? []
  const groups = new Map<string, string[]>()
  list.forEach((code) => {
    const module = code.split(':').slice(0, 2).join(':') || '其他'
    const bucket = groups.get(module) ?? []
    bucket.push(code)
    groups.set(module, bucket)
  })
  return Array.from(groups.entries()).map(([module, codes]) => ({ module, codes: codes.sort() }))
})

onMounted(() => {
  // 失败提示由请求层统一弹出；吞掉 rejection，避免产生未捕获告警
  void loadProfile().catch(() => {})
})
</script>

<template>
  <div v-loading="loading" class="profile-page">
    <!-- 概览卡片 -->
    <div class="page-card profile-overview">
      <el-avatar :size="64" :src="profile?.avatar || undefined" class="profile-overview__avatar">
        {{ initials }}
      </el-avatar>
      <div class="profile-overview__info">
        <div class="profile-overview__name">
          <h2>{{ profile?.nickname || profile?.username }}</h2>
          <el-tag v-if="profile?.isSuper" type="warning" size="small" effect="light">超级管理员</el-tag>
        </div>
        <p class="text-secondary">
          {{ profile?.username }}
          <span v-if="profile?.deptName"> · {{ profile.deptName }}</span>
        </p>
        <div class="profile-overview__roles">
          <el-tag v-for="role in profile?.roles ?? []" :key="role" size="small" effect="plain">
            {{ role }}
          </el-tag>
          <span v-if="!profile?.roles?.length" class="text-muted">未分配角色</span>
        </div>
      </div>
    </div>

    <div class="profile-body">
      <!-- 基本信息与密码 -->
      <div class="page-card">
        <el-tabs>
          <el-tab-pane label="基本资料">
            <el-form
              ref="infoFormRef"
              :model="infoForm"
              :rules="infoRules"
              label-width="88px"
              class="profile-form"
            >
              <el-form-item label="用户名">
                <el-input :model-value="profile?.username" disabled />
              </el-form-item>
              <el-form-item label="昵称" prop="nickname">
                <el-input v-model="infoForm.nickname" placeholder="显示名称" />
              </el-form-item>
              <el-form-item label="手机号">
                <el-input v-model="infoForm.phone" placeholder="选填" />
              </el-form-item>
              <el-form-item label="邮箱" prop="email">
                <el-input v-model="infoForm.email" placeholder="选填" />
              </el-form-item>
              <el-form-item label="头像地址">
                <el-input v-model="infoForm.avatar" placeholder="选填，图片 URL" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="infoSubmitting" @click="submitInfo">保存修改</el-button>
              </el-form-item>
            </el-form>
          </el-tab-pane>

          <el-tab-pane label="修改密码">
            <el-form
              ref="pwdFormRef"
              :model="pwdForm"
              :rules="pwdRules"
              label-width="88px"
              class="profile-form"
            >
              <el-form-item label="原密码" prop="oldPassword">
                <el-input v-model="pwdForm.oldPassword" type="password" show-password />
              </el-form-item>
              <el-form-item label="新密码" prop="newPassword">
                <el-input v-model="pwdForm.newPassword" type="password" show-password />
              </el-form-item>
              <el-form-item label="确认密码" prop="confirmPassword">
                <el-input v-model="pwdForm.confirmPassword" type="password" show-password />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="pwdSubmitting" @click="submitPassword">
                  确认修改
                </el-button>
              </el-form-item>
            </el-form>
          </el-tab-pane>
        </el-tabs>
      </div>

      <!-- 权限清单 -->
      <div class="page-card">
        <header class="card-head">
          <h3>我的权限</h3>
          <span class="text-muted">
            <template v-if="profile?.isSuper">超级管理员拥有全部权限</template>
            <template v-else>共 {{ profile?.permissions?.length ?? 0 }} 项授权</template>
          </span>
        </header>

        <el-alert
          v-if="profile?.isSuper"
          title="当前账号为超级管理员，系统不做权限裁剪。"
          type="warning"
          :closable="false"
          class="mb-16"
        />

        <el-empty v-else-if="!permissionGroups.length" description="暂无权限点" :image-size="80" />

        <div v-else class="perm-groups">
          <div v-for="group in permissionGroups" :key="group.module" class="perm-group">
            <div class="perm-group__title">
              <AppIcon name="Key" />
              <span>{{ group.module }}</span>
            </div>
            <div class="perm-group__codes">
              <code v-for="code in group.codes" :key="code">{{ code }}</code>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.profile-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.profile-overview {
  display: flex;
  align-items: center;
  gap: 18px;

  &__avatar {
    flex-shrink: 0;
    font-size: 24px;
    font-weight: 600;
    color: #fff;
    background: var(--brand-gradient);
  }

  &__info {
    min-width: 0;
  }

  &__name {
    display: flex;
    align-items: center;
    gap: 10px;

    h2 {
      margin: 0;
      font-size: 19px;
      font-weight: 700;
      color: var(--app-text);
    }
  }

  &__info p {
    margin: 6px 0 8px;
    font-size: 13px;
  }

  &__roles {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    font-size: 12px;
  }
}

.profile-body {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;

  @media (max-width: 1000px) {
    grid-template-columns: 1fr;
  }
}

.profile-form {
  max-width: 420px;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;

  h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--app-text);
  }

  span {
    font-size: 12px;
  }
}

.perm-groups {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 460px;
  overflow-y: auto;
}

.perm-group {
  &__title {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--app-text-secondary);

    :deep(.app-icon) {
      color: var(--brand-primary);
    }
  }

  &__codes {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;

    code {
      padding: 3px 8px;
      font-size: 12px;
      color: var(--tone-blue);
      background: var(--tone-blue-soft);
      border-radius: 5px;
    }
  }
}
</style>
