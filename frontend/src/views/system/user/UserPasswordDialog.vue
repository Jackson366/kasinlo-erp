<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { userApi } from '@/api'
import type { UserItem } from '@/types/api'

/**
 * 重置密码弹窗。
 *
 * 从 user/index.vue 拆出：只有「一个输入框 + 一个提交」，自成一体。
 * target 由外层传入（null 表示未打开），弹窗打开时自动清空上一次的输入。
 */
const props = defineProps<{
  modelValue: boolean
  target: UserItem | null
}>()
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
})

const pwdForm = reactive({ password: '' })

// 切换目标用户时清空输入：不然给 A 打完密码取消，给 B 打开时还留着
watch(
  () => props.target,
  () => {
    pwdForm.password = ''
  },
)

const submitting = ref(false)

async function submitResetPassword(): Promise<void> {
  if (!props.target || submitting.value) return
  if (pwdForm.password.length < 8) {
    ElMessage.warning('密码长度不能少于 8 位')
    return
  }
  submitting.value = true
  try {
    await userApi.resetPassword(props.target.id, pwdForm.password)
    ElMessage.success('密码已重置')
    visible.value = false
  } catch {
    // 具体错误提示已由请求层统一处理；不加 catch 的话失败会产生未捕获 rejection
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="重置密码" width="640px" top="6vh" destroy-on-close>
    <el-alert
      v-if="target"
      :title="`正在为用户「${target.username}」重置密码`"
      type="info"
      :closable="false"
      class="mb-16"
    />
    <el-input
      v-model="pwdForm.password"
      type="password"
      show-password
      placeholder="请输入新密码（至少 6 位）"
    />
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submitResetPassword">确定</el-button>
    </template>
  </el-dialog>
</template>
