<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElButton, ElInput } from 'element-plus'
import { openPlatformApi, type DeviceAuthorization, type OpenPlatformGrant } from '@/api/openPlatform'

const code = ref('')
const pending = ref<{ code: string; details: DeviceAuthorization }>()
const grants = ref<OpenPlatformGrant[]>([])
const loadingGrants = ref(true)
const busy = ref(false)
const error = ref('')
const grantsError = ref('')
const feedback = ref('')

function formatDate(value: string) {
  return new Date(value).toLocaleString()
}

function message(value: unknown) {
  return value instanceof Error ? value.message : '操作失败，请重试'
}

async function loadGrants() {
  loadingGrants.value = true
  grantsError.value = ''
  try {
    grants.value = await openPlatformApi.grants()
  } catch (e) {
    grantsError.value = message(e)
  } finally {
    loadingGrants.value = false
  }
}

async function inspect() {
  if (busy.value) return
  const userCode = code.value.trim().toUpperCase()
  error.value = ''
  feedback.value = ''
  pending.value = undefined
  if (!/^[0-9A-F]{12}$/.test(userCode)) {
    error.value = '请输入终端显示的 12 位验证码'
    return
  }
  busy.value = true
  try {
    const details = await openPlatformApi.inspect(userCode)
    pending.value = { code: userCode, details }
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}

async function decide(approve: boolean) {
  if (!pending.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await openPlatformApi.decide(pending.value.code, approve)
    pending.value = undefined
    code.value = ''
    feedback.value = approve ? '已确认授权。请返回终端领取凭据。' : '已拒绝授权。'
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}

async function revoke(id: string) {
  if (busy.value) return
  busy.value = true
  grantsError.value = ''
  try {
    await openPlatformApi.revoke(id)
    grants.value = grants.value.filter((grant) => grant.id !== id)
    feedback.value = '已撤销该授权。共用这份凭据的调用将失效。'
  } catch (e) {
    grantsError.value = message(e)
  } finally {
    busy.value = false
  }
}

onMounted(loadGrants)
</script>

<template>
  <main class="authorize">
    <h1>开放平台授权</h1>
    <p>输入你刚刚发起登录的终端上显示的验证码。</p>
    <form class="authorize-actions" @submit.prevent="inspect">
      <ElInput v-model="code" aria-label="设备验证码" maxlength="12" :disabled="busy" @input="pending = undefined" />
      <ElButton native-type="submit" :loading="busy">查看请求</ElButton>
    </form>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="feedback" role="status">{{ feedback }}</p>
    <section v-if="pending" class="authorize-panel">
      <h2>确认授权范围</h2>
      <dl>
        <dt>应用</dt><dd>{{ pending.details.clientId }}</dd>
        <dt>允许的操作</dt><dd>{{ pending.details.scope === 'inventory.read' ? '查询库存' : pending.details.scope }}</dd>
        <dt>请求到期时间</dt><dd>{{ formatDate(pending.details.expiresAt) }}</dd>
        <dt>凭据有效期</dt><dd>领取后 {{ pending.details.accessLifetimeSeconds / 3600 }} 小时</dd>
      </dl>
      <p>应用将以当前 ERP 账号的权限查询库存。</p>
      <div class="authorize-actions">
        <ElButton type="primary" :disabled="busy" @click="decide(true)">允许</ElButton>
        <ElButton :disabled="busy" @click="decide(false)">拒绝</ElButton>
      </div>
    </section>

    <section class="authorize-panel">
      <div class="authorize-actions">
        <h2>有效授权</h2>
        <ElButton :loading="loadingGrants" :disabled="busy" @click="loadGrants">刷新</ElButton>
      </div>
      <p v-if="grantsError" role="alert">{{ grantsError }}</p>
      <p v-else-if="!loadingGrants && grants.length === 0">暂无有效授权。</p>
      <ul v-if="grants.length" class="authorize-grants">
        <li v-for="grant in grants" :key="grant.id" class="authorize-actions">
          <span>{{ grant.clientId }} · {{ grant.id.slice(0, 8) }} · {{ formatDate(grant.expiresAt) }} 到期</span>
          <ElButton :disabled="busy || loadingGrants" @click="revoke(grant.id)">撤销</ElButton>
        </li>
      </ul>
    </section>
  </main>
</template>

<style scoped>
.authorize { max-width: 42rem; margin: 0 auto; padding: 2rem 1rem; }
.authorize-panel { margin-block-start: 2rem; }
.authorize-actions { display: flex; align-items: center; gap: 0.75rem; }
.authorize-grants { display: grid; gap: 0.75rem; padding-inline-start: 0; }
</style>
