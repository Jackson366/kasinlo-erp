<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import type { InputInstance } from 'element-plus'
import AppIcon from '@/components/AppIcon.vue'
import { useUserStore } from '@/store'
import { openPlatformApi, type DeviceAuthorization, type OpenPlatformGrant } from '@/api/openPlatform'

defineOptions({ name: 'OpenPlatformAuthorize' })

const userStore = useUserStore()
const code = ref('')
const codeInput = ref<InputInstance>()
const stageHeading = ref<HTMLElement>()
const pending = ref<{ code: string; details: DeviceAuthorization }>()
const result = ref<'approved' | 'denied'>()
const grants = ref<OpenPlatformGrant[]>([])
const loadingGrants = ref(true)
const action = ref<'' | 'inspect' | 'approve' | 'deny'>('')
const error = ref('')
const grantsError = ref('')
const grantsFeedback = ref('')
const revokeTarget = ref<OpenPlatformGrant>()
const revoking = ref(false)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined

const step = computed(() => result.value ? 3 : pending.value ? 2 : 1)
const expired = computed(() => !!pending.value && new Date(pending.value.details.expiresAt).getTime() <= now.value)
const activeGrants = computed(() => grants.value.filter((grant) => new Date(grant.expiresAt).getTime() > now.value))
const accountName = computed(() => userStore.profile?.username || userStore.displayName)

function clientName(id: string) {
  return id === 'kas' ? 'kas CLI' : id
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

function lifetime(seconds: number) {
  return seconds >= 3600 && seconds % 3600 === 0 ? `${seconds / 3600} 小时` : `${Math.ceil(seconds / 60)} 分钟`
}

function message(value: unknown) {
  return value instanceof Error ? value.message : '操作失败，请稍后重试'
}

async function focusHeading() {
  await nextTick()
  stageHeading.value?.focus()
}

async function loadGrants() {
  loadingGrants.value = true
  grantsError.value = ''
  try {
    grants.value = (await openPlatformApi.grants()) ?? []
    now.value = Date.now()
  } catch (e) {
    grantsError.value = message(e)
  } finally {
    loadingGrants.value = false
  }
}

async function inspect() {
  if (action.value) return
  const userCode = code.value.replace(/[\s-]/g, '').toUpperCase()
  code.value = userCode
  error.value = ''
  if (!/^[0-9A-F]{12}$/.test(userCode)) {
    error.value = '请输入终端显示的 12 位验证码，可直接粘贴。'
    codeInput.value?.focus()
    return
  }
  action.value = 'inspect'
  try {
    const details = await openPlatformApi.inspect(userCode)
    pending.value = { code: userCode, details }
    now.value = Date.now()
    await focusHeading()
  } catch (e) {
    error.value = message(e)
  } finally {
    action.value = ''
  }
}

async function decide(approve: boolean) {
  if (!pending.value || action.value) return
  now.value = Date.now()
  if (expired.value) return
  action.value = approve ? 'approve' : 'deny'
  error.value = ''
  try {
    await openPlatformApi.decide(pending.value.code, approve)
    pending.value = undefined
    code.value = ''
    result.value = approve ? 'approved' : 'denied'
    await focusHeading()
  } catch (e) {
    error.value = message(e)
  } finally {
    action.value = ''
  }
}

async function restart() {
  pending.value = undefined
  result.value = undefined
  error.value = ''
  code.value = ''
  await nextTick()
  codeInput.value?.focus()
}

async function revoke() {
  if (!revokeTarget.value || revoking.value) return
  const id = revokeTarget.value.id
  revoking.value = true
  grantsError.value = ''
  grantsFeedback.value = ''
  try {
    await openPlatformApi.revoke(id)
    grants.value = grants.value.filter((grant) => grant.id !== id)
    revokeTarget.value = undefined
    grantsFeedback.value = '授权已撤销。所有共用这份凭据的后续调用都会被拒绝。'
  } catch (e) {
    grantsError.value = message(e)
    revokeTarget.value = undefined
  } finally {
    revoking.value = false
  }
}

onMounted(() => {
  void loadGrants()
  clock = setInterval(() => { now.value = Date.now() }, 1000)
})
onUnmounted(() => { if (clock) clearInterval(clock) })
</script>

<template>
  <div class="authorize-page">
    <header class="authorize-header">
      <RouterLink to="/" class="authorize-brand" aria-label="返回 ERP 工作台">
        <img src="/brand/kasinlo-mark.png" alt="" width="32" height="32" />
        <span>Kasinlo ERP <span class="authorize-brand__divider">/</span> 工具授权</span>
      </RouterLink>
      <RouterLink to="/profile" class="authorize-account" aria-label="查看当前账号资料">
        <AppIcon name="User" aria-hidden="true" />
        <span>{{ userStore.displayName }}</span>
      </RouterLink>
    </header>

    <main class="authorize-main">
      <RouterLink to="/" class="authorize-back"><AppIcon name="ArrowLeft" aria-hidden="true" />返回工作台</RouterLink>
      <div class="authorize-intro">
        <h1>连接你的 CLI 与智能体</h1>
        <p>确认工具需要的权限，随时管理已授予的访问。</p>
      </div>

      <div class="authorize-layout">
        <section class="authorize-card authorize-request" aria-label="工具授权请求">
          <ol class="authorize-steps" aria-label="授权进度">
            <li v-for="(label, index) in ['输入验证码', '确认权限', '完成']" :key="label" :class="{ 'is-current': step === index + 1, 'is-complete': step > index + 1 }" :aria-current="step === index + 1 ? 'step' : undefined">
              <span class="authorize-steps__number" aria-hidden="true"><AppIcon v-if="step > index + 1" name="Check" /><template v-else>{{ index + 1 }}</template></span>
              <span>{{ label }}</span>
            </li>
          </ol>

          <div class="authorize-request__body" :aria-busy="!!action">
            <template v-if="!pending && !result">
              <div class="authorize-symbol" aria-hidden="true"><AppIcon name="Monitor" /></div>
              <h2>输入终端验证码</h2>
              <p class="authorize-description">在终端发起登录后，将显示的验证码粘贴到这里。</p>
              <form class="authorize-form" @submit.prevent="inspect">
                <label for="device-code">设备验证码</label>
                <p id="code-hint" class="authorize-hint">共 12 位，由数字和字母 A–F 组成。</p>
                <p v-if="error" id="code-error" class="authorize-error" role="alert"><AppIcon name="WarningFilled" aria-hidden="true" />{{ error }}</p>
                <el-input id="device-code" ref="codeInput" v-model="code" :maxlength="32" autocomplete="off" spellcheck="false" placeholder="例如 A1B2C3D4E5F6" :readonly="!!action" :aria-invalid="!!error" :aria-describedby="error ? 'code-hint code-error' : 'code-hint'" class="authorize-code" @input="error = ''" />
                <el-button type="primary" native-type="submit" :loading="action === 'inspect'" class="authorize-primary">查看授权请求<AppIcon name="ArrowRight" aria-hidden="true" /></el-button>
              </form>
            </template>

            <template v-else-if="pending">
              <div class="authorize-symbol" aria-hidden="true"><AppIcon name="Key" /></div>
              <h2 ref="stageHeading" tabindex="-1">允许 {{ clientName(pending.details.clientId) }} 访问库存？</h2>
              <p class="authorize-description">工具将代表账号 <strong>{{ accountName }}</strong> 执行以下操作。</p>
              <div class="authorize-permission">
                <AppIcon name="View" aria-hidden="true" />
                <div>
                  <h3>{{ pending.details.scope === 'inventory.read' ? '查询库存' : pending.details.scope }}</h3>
                  <p v-if="pending.details.scope === 'inventory.read'">查看商品、规格、基本单位和库存量。</p>
                  <p>访问范围受当前 ERP 账号的权限限制。</p>
                </div>
                <span class="authorize-badge">{{ pending.details.scope === 'inventory.read' ? '只读' : '申请权限' }}</span>
              </div>
              <dl class="authorize-facts">
                <div><dt>凭据有效期</dt><dd>领取后 {{ lifetime(pending.details.accessLifetimeSeconds) }}</dd></div>
                <div><dt>请求截止时间</dt><dd>{{ formatDate(pending.details.expiresAt) }}</dd></div>
                <div><dt>设备验证码</dt><dd class="authorize-mono">{{ pending.code }}</dd></div>
              </dl>
              <p class="authorize-hint">仅确认由你发起的登录请求。授权后可在下方随时撤销。</p>
              <div v-if="expired" class="authorize-notice authorize-notice--warning" role="alert"><AppIcon name="WarningFilled" aria-hidden="true" /><div><strong>这次授权请求已过期</strong><p>请在终端重新发起登录，获取新的验证码。</p></div></div>
              <p v-else-if="error" class="authorize-error" role="alert"><AppIcon name="WarningFilled" aria-hidden="true" />{{ error }}</p>
              <div v-if="!expired" class="authorize-decisions">
                <el-button type="primary" :loading="action === 'approve'" :disabled="action === 'deny'" @click="decide(true)">{{ pending.details.scope === 'inventory.read' ? '允许查询库存' : '允许授权' }}</el-button>
                <el-button :loading="action === 'deny'" :disabled="action === 'approve'" @click="decide(false)">拒绝授权</el-button>
              </div>
              <el-button v-else @click="restart">输入新的验证码</el-button>
              <button v-if="!action && !expired" type="button" class="authorize-text-button" @click="restart">使用其他验证码</button>
            </template>

            <div v-else class="authorize-result" role="status">
              <div class="authorize-symbol" :class="{ 'authorize-symbol--success': result === 'approved' }" aria-hidden="true"><AppIcon :name="result === 'approved' ? 'CircleCheck' : 'CircleClose'" /></div>
              <h2 ref="stageHeading" tabindex="-1">{{ result === 'approved' ? '已同意授权，请返回终端' : '已拒绝这次授权' }}</h2>
              <p class="authorize-description">{{ result === 'approved' ? '终端继续登录后，即可使用刚才确认的权限。你可以关闭此页。' : '这次请求不会获得访问权限。需要连接时，请在终端重新发起登录。' }}</p>
              <div v-if="result === 'approved'" class="authorize-notice"><AppIcon name="InfoFilled" aria-hidden="true" /><p>领取凭据后，点击下方“刷新列表”即可查看并管理授权。</p></div>
              <el-button @click="restart">连接另一个工具</el-button>
            </div>
          </div>
        </section>

        <aside class="authorize-guide" aria-labelledby="guide-title">
          <h2 id="guide-title">授权前，了解这些</h2>
          <div><AppIcon name="User" aria-hidden="true" /><h3>使用当前账号权限</h3><p>工具只能在你的 ERP 权限范围内访问已确认的数据。</p></div>
          <div><AppIcon name="Connection" aria-hidden="true" /><h3>凭据可以共用</h3><p>同一受信任环境中的多个智能体可共用登录，也会共享相同权限。</p></div>
          <div><AppIcon name="Lock" aria-hidden="true" /><h3>随时停止访问</h3><p>撤销一份授权，所有共用该凭据的后续调用都会失效。</p></div>
        </aside>
      </div>

      <section id="active-grants" class="authorize-card authorize-grants" aria-labelledby="grants-title" :aria-busy="loadingGrants">
        <header class="authorize-section-head">
          <div><h2 id="grants-title">有效授权<span v-if="!loadingGrants && !grantsError" class="authorize-count">{{ activeGrants.length }}</span></h2><p>仅显示当前账号已领取且未过期的授权。</p></div>
          <el-button :loading="loadingGrants" :disabled="revoking" @click="loadGrants"><AppIcon name="Refresh" aria-hidden="true" />刷新列表</el-button>
        </header>
        <div v-if="grantsError" class="authorize-notice authorize-notice--warning" role="alert"><AppIcon name="WarningFilled" aria-hidden="true" /><div><strong>授权操作未完成</strong><p>{{ grantsError }}</p><p>请刷新列表确认当前状态后重试。</p></div></div>
        <div v-if="grantsFeedback" class="authorize-notice authorize-notice--success" role="status"><AppIcon name="CircleCheck" aria-hidden="true" /><p>{{ grantsFeedback }}</p></div>
        <div v-if="loadingGrants" class="authorize-empty" role="status"><p>正在加载有效授权…</p></div>
        <template v-else-if="!grantsError">
          <ul v-if="activeGrants.length" class="authorize-grant-list">
            <li v-for="grant in activeGrants" :key="grant.id">
              <div class="authorize-grant-icon" aria-hidden="true"><AppIcon name="Monitor" /></div>
              <div class="authorize-grant-info"><h3>{{ clientName(grant.clientId) }}<span class="authorize-badge">已授权</span></h3><p>到期时间：{{ formatDate(grant.expiresAt) }}</p><details><summary>授权标识</summary><code>{{ grant.id }}</code></details></div>
              <el-button class="authorize-revoke" :aria-label="`撤销 ${clientName(grant.clientId)} 的授权 ${grant.id.slice(0, 8)}`" @click="revokeTarget = grant">撤销授权</el-button>
            </li>
          </ul>
          <div v-else class="authorize-empty"><AppIcon name="Key" aria-hidden="true" /><h3>暂无有效授权</h3><p>完成终端登录并领取凭据后，授权会显示在这里。</p></div>
        </template>
      </section>
      <footer class="authorize-footer">Kasinlo ERP · 工具访问由你掌控</footer>
    </main>

    <el-dialog :model-value="!!revokeTarget" title="撤销这份授权？" width="440px" class="authorize-revoke-dialog" :close-on-click-modal="!revoking" :close-on-press-escape="!revoking" :show-close="!revoking" @update:model-value="(visible: boolean) => { if (!visible && !revoking) revokeTarget = undefined }">
      <p>撤销后，所有共用这份 {{ revokeTarget ? clientName(revokeTarget.clientId) : '' }} 凭据的 CLI 和智能体都将无法继续访问 ERP。</p>
      <p>如需恢复访问，可以在终端重新登录授权。</p>
      <template #footer><el-button :disabled="revoking" @click="revokeTarget = undefined">保留授权</el-button><el-button type="danger" :loading="revoking" @click="revoke">确认撤销</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.authorize-page { min-height: 100%; background: var(--app-bg); color: var(--app-text); }
.authorize-header { min-height: 80px; padding: 16px max(24px, calc((100% - 1080px) / 2)); display: flex; align-items: center; justify-content: space-between; gap: 24px; border-bottom: 1px solid var(--app-border); background: var(--app-surface); }
.authorize-brand, .authorize-account, .authorize-back { display: inline-flex; align-items: center; gap: 12px; min-height: 48px; }
.authorize-brand { color: var(--app-text); font-weight: 700; font-size: 16px; }
.authorize-brand img { object-fit: contain; }
.authorize-brand__divider { margin: 0 12px; color: var(--app-text-secondary); font-weight: 400; }
.authorize-account { color: var(--app-text-secondary); max-width: 240px; overflow-wrap: anywhere; }
.authorize-main { max-width: 1008px; margin: auto; padding: 24px 24px 32px; }
.authorize-back { font-size: 14px; gap: 8px; }
.authorize-intro { margin: 16px 0 32px; }
.authorize-intro h1 { margin: 0 0 12px; font-size: 28px; line-height: 1.35; }
.authorize-intro p, .authorize-description { color: var(--app-text-secondary); font-size: 15px; line-height: 1.7; margin: 0; }
.authorize-layout { display: grid; grid-template-columns: minmax(0, 640px) minmax(0, 1fr); align-items: start; gap: 32px; }
.authorize-card { background: var(--app-surface); border: 1px solid var(--app-border); border-radius: var(--radius-md); box-shadow: var(--shadow-card); min-width: 0; }
.authorize-steps { display: flex; justify-content: space-between; gap: 12px; list-style: none; padding: 24px 32px; margin: 0; border-bottom: 1px solid var(--app-border); }
.authorize-steps li { display: flex; align-items: center; gap: 8px; color: var(--app-text-secondary); font-size: 13px; }
.authorize-steps__number { display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; flex-shrink: 0; border: 1px solid var(--app-control-border); border-radius: 50%; font-weight: 700; }
.authorize-steps .is-current { color: var(--brand-primary); font-weight: 700; }
.is-current .authorize-steps__number, .is-complete .authorize-steps__number { color: var(--brand-primary); background: var(--app-primary-soft); border-color: var(--brand-primary); }
.authorize-request__body { padding: 32px; }
.authorize-symbol { display: flex; align-items: center; justify-content: center; width: 48px; height: 48px; font-size: 24px; background: var(--app-primary-soft); color: var(--brand-primary); border-radius: 12px; margin-bottom: 24px; }
.authorize-symbol--success { background: var(--tone-green-soft); color: var(--tone-green); }
.authorize-request h2 { font-size: 22px; line-height: 1.45; margin: 0 0 12px; overflow-wrap: anywhere; }
.authorize-form { display: flex; flex-direction: column; align-items: stretch; margin-top: 32px; }
.authorize-form label { font-weight: 700; margin-bottom: 8px; }
.authorize-hint { font-size: 13px; line-height: 1.65; color: var(--app-text-secondary); margin: 0 0 12px; }
.authorize-code { --el-input-border-color: var(--app-control-border); --el-input-height: 52px; }
.authorize-code :deep(input) { font-family: ui-monospace, monospace; font-size: 18px; letter-spacing: 2px; }
.authorize-code[aria-invalid='true'] { --el-input-border-color: var(--app-danger); }
.authorize-primary { margin-top: 24px; justify-content: center; gap: 12px; }
.authorize-page :deep(.el-button) { min-height: 48px; font-size: 14px; }
.authorize-page :deep(.el-button:not(.el-button--primary):not(.el-button--danger)) { --el-button-border-color: var(--app-control-border); }
.authorize-page :deep(.el-button--primary) { --el-button-text-color: var(--app-on-accent); --el-button-hover-text-color: var(--app-on-accent); --el-button-active-text-color: var(--app-on-accent); }
.authorize-page :deep(.el-button + .el-button) { margin-left: 0; }
.authorize-guide { padding-top: 24px; }
.authorize-guide h2 { font-size: 15px; margin: 0 0 24px; }
.authorize-guide > div { position: relative; padding-left: 28px; margin-bottom: 24px; }
.authorize-guide > div > .app-icon { position: absolute; left: 0; top: 2px; color: var(--app-text-secondary); font-size: 18px; }
.authorize-guide h3 { margin: 0 0 8px; font-size: 14px; }
.authorize-guide p { font-size: 13px; color: var(--app-text-secondary); line-height: 1.75; margin: 0; }
.authorize-permission { display: flex; align-items: flex-start; gap: 12px; padding: 20px; background: var(--app-surface-subtle); border-radius: var(--radius-sm); margin: 24px 0; }
.authorize-permission > .app-icon { font-size: 22px; color: var(--brand-primary); flex-shrink: 0; }
.authorize-permission h3 { margin: 0 0 8px; font-size: 15px; }
.authorize-permission p { font-size: 13px; color: var(--app-text-secondary); line-height: 1.65; margin: 0; }
.authorize-badge { display: inline-flex; white-space: nowrap; font-size: 12px; font-weight: 400; color: var(--tone-green); background: var(--tone-green-soft); border-radius: 6px; padding: 3px 8px; }
.authorize-permission > .authorize-badge { margin-left: auto; }
.authorize-facts { display: grid; gap: 16px; margin: 0 0 24px; font-size: 14px; }
.authorize-facts > div { display: grid; grid-template-columns: 112px 1fr; gap: 12px; }
.authorize-facts dt { color: var(--app-text-secondary); }
.authorize-facts dd { margin: 0; overflow-wrap: anywhere; }
.authorize-mono { font-family: ui-monospace, monospace; }
.authorize-decisions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 24px; }
.authorize-decisions .el-button--primary { flex: 1; }
.authorize-text-button { display: block; min-height: 48px; padding: 8px 0; border: 0; background: none; color: var(--brand-primary); cursor: pointer; text-decoration: underline; text-underline-offset: 4px; margin-top: 8px; font: inherit; font-size: 13px; }
.authorize-error { display: flex; align-items: baseline; gap: 8px; color: var(--tone-red); line-height: 1.65; font-size: 14px; margin: 0 0 12px; }
.authorize-notice { display: flex; align-items: baseline; gap: 10px; padding: 16px; background: var(--tone-blue-soft); color: var(--tone-blue); border-radius: var(--radius-sm); margin: 20px 0; font-size: 14px; line-height: 1.65; }
.authorize-notice > .app-icon { flex-shrink: 0; }
.authorize-notice p { margin: 0; }
.authorize-notice strong { display: block; margin-bottom: 4px; }
.authorize-notice--warning { background: var(--tone-amber-soft); color: var(--tone-amber); }
.authorize-notice--success { background: var(--tone-green-soft); color: var(--tone-green); }
.authorize-result > .el-button { margin-top: 24px; }
.authorize-grants { margin-top: 32px; padding: 24px 32px; }
.authorize-section-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.authorize-section-head h2 { display: flex; align-items: center; gap: 10px; font-size: 18px; margin: 0 0 8px; }
.authorize-section-head p { font-size: 13px; color: var(--app-text-secondary); margin: 0; line-height: 1.6; }
.authorize-count { padding: 2px 8px; border-radius: 6px; background: var(--app-fill); color: var(--app-text-secondary); font-size: 13px; }
.authorize-section-head > .el-button { flex-shrink: 0; gap: 8px; }
.authorize-grant-list { list-style: none; padding: 0; margin: 24px 0 0; }
.authorize-grant-list > li { display: flex; gap: 16px; align-items: flex-start; padding: 24px 0; border-top: 1px solid var(--app-border); }
.authorize-grant-icon { display: flex; align-items: center; justify-content: center; width: 40px; height: 40px; border-radius: 10px; background: var(--app-fill); font-size: 20px; color: var(--app-text-secondary); flex-shrink: 0; }
.authorize-grant-info { flex: 1; min-width: 0; }
.authorize-grant-info h3 { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin: 0 0 8px; font-size: 15px; overflow-wrap: anywhere; }
.authorize-grant-info p, .authorize-grant-info details { color: var(--app-text-secondary); font-size: 13px; margin: 0; line-height: 1.65; }
.authorize-grant-info summary { cursor: pointer; min-height: 48px; padding-top: 8px; }
.authorize-grant-info code { display: block; padding-top: 4px; overflow-wrap: anywhere; }
.authorize-empty { display: flex; flex-direction: column; align-items: center; text-align: center; padding: 40px 8px 24px; color: var(--app-text-secondary); }
.authorize-empty > .app-icon { font-size: 28px; padding: 16px; box-sizing: content-box; border-radius: 50%; background: var(--app-fill); margin-bottom: 16px; }
.authorize-empty h3 { font-size: 15px; color: var(--app-text); margin: 0 0 8px; }
.authorize-empty p { font-size: 13px; margin: 0; line-height: 1.65; }
.authorize-footer { padding-top: 32px; font-size: 12px; color: var(--app-text-secondary); text-align: center; }
.authorize-page a:focus-visible, .authorize-text-button:focus-visible, .authorize-page summary:focus-visible { outline: 2px solid var(--brand-primary); outline-offset: 4px; border-radius: 4px; }
.authorize-request h2:focus { outline: none; }
@media (max-width: 760px) {
  .authorize-layout { grid-template-columns: 1fr; gap: 24px; }
  .authorize-guide { padding: 0 8px; }
  .authorize-guide h2 { margin-bottom: 16px; }
  .authorize-guide > div { margin-bottom: 16px; }
  .authorize-header { padding: 12px 20px; gap: 12px; }
  .authorize-account { max-width: 120px; font-size: 13px; }
}
@media (max-width: 480px) {
  .authorize-header { padding: 12px 16px; }
  .authorize-brand { font-size: 13px; gap: 8px; }
  .authorize-brand__divider { margin: 0 4px; }
  .authorize-account { max-width: 80px; gap: 6px; }
  .authorize-main { padding: 16px 16px 24px; }
  .authorize-intro { margin: 12px 0 24px; }
  .authorize-intro h1 { font-size: 24px; }
  .authorize-steps { padding: 20px 16px; gap: 8px; }
  .authorize-steps li { gap: 6px; font-size: 12px; }
  .authorize-steps__number { width: 24px; height: 24px; }
  .authorize-request__body { padding: 24px 20px; }
  .authorize-request h2 { font-size: 20px; }
  .authorize-permission { padding: 16px 12px; gap: 8px; }
  .authorize-facts > div { grid-template-columns: 88px 1fr; gap: 8px; font-size: 13px; }
  .authorize-grants { padding: 24px 20px; margin-top: 24px; }
  .authorize-section-head { flex-wrap: wrap; }
  .authorize-grant-list > li { flex-wrap: wrap; gap: 12px; }
  .authorize-revoke { margin-left: 52px; }
  .authorize-decisions { flex-direction: column; }
}
</style>

<style lang="scss">
.authorize-revoke-dialog.el-dialog { max-width: calc(100vw - 32px); border-radius: var(--radius-md); }
.authorize-revoke-dialog .el-dialog__headerbtn { width: 48px; height: 48px; }
.authorize-revoke-dialog p { line-height: 1.75; margin: 0 0 12px; color: var(--app-text-secondary); }
.authorize-revoke-dialog .el-dialog__footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 12px; }
.authorize-revoke-dialog .el-button--danger { --el-button-text-color: var(--app-on-accent); --el-button-hover-text-color: var(--app-on-accent); --el-button-active-text-color: var(--app-on-accent); }
.authorize-revoke-dialog .el-button { min-height: 48px; }
.authorize-revoke-dialog .el-button + .el-button { margin-left: 0; }
</style>
