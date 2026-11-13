<script setup lang="ts">
import { ref, useId } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { InputInstance } from 'element-plus'
import { invApi } from '@/api'
import type { InvOrderDetail, InvRedReversalPreview } from '@/types/api'
import { formatMoney, formatQty } from '@/utils/format'

const emit = defineEmits<{ completed: [id: number] }>()
const visible = ref(false)
const order = ref<InvOrderDetail>()
const preview = ref<InvRedReversalPreview>()
const reason = ref('')
const reasonID = useId()
const reasonInput = ref<InputInstance>()
const reasonError = ref('')
const error = ref('')
const loading = ref(false)
const submitting = ref(false)
let request = 0

async function loadPreview() {
  const target = order.value
  if (!target) return
  const current = ++request
  error.value = ''
  preview.value = undefined
  loading.value = true
  try {
    const data = await invApi.redReversalPreview(target.id)
    if (request === current) preview.value = data
  } catch (err) {
    if (request === current) {
      error.value = err instanceof Error ? err.message : '无法加载红冲影响，请重试'
    }
  } finally {
    if (request === current) loading.value = false
  }
}

function open(target: InvOrderDetail) {
  order.value = target
  reason.value = ''
  reasonError.value = ''
  visible.value = true
  void loadPreview()
}
defineExpose({ open })

function validateReason() {
  reasonError.value = reason.value.trim() ? '' : '请填写红冲原因'
  return !reasonError.value
}

async function confirm() {
  const target = order.value
  const impact = preview.value
  if (!target || !impact || impact.blockReason || submitting.value) return
  if (!validateReason()) {
    reasonInput.value?.focus()
    return
  }
  error.value = ''
  submitting.value = true
  try {
    const red = await invApi.redReverseOrder(target.id, impact.stockOrderId, reason.value.trim())
    visible.value = false
    ElMessage.success(`整单红冲已完成，红冲单 ${red.orderNo}`)
    emit('completed', target.id)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '红冲结果未确认，请刷新影响后重试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="整单红冲"
    width="640px"
    top="6vh"
    append-to-body
    :close-on-click-modal="false"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
  >
    <div class="red-reversal">
      <p class="red-reversal__order">
        {{ order?.bizType === 'purchase' ? '采购单' : '销售单' }}
        <strong>{{ order?.orderNo }}</strong>
        <span>{{ order?.partyName }}</span>
      </p>
      <p v-if="loading" role="status">正在核对库存和未结金额…</p>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
      <template v-if="preview">
        <el-alert
          v-if="preview.blockReason"
          title="暂时无法红冲"
          :description="preview.blockReason"
          type="warning"
          show-icon
          :closable="false"
        />
        <template v-else>
          <section class="red-reversal__impact" aria-label="过账影响">
            <p>库存{{ preview.direction === 'out' ? '减少' : '增加' }} · {{ preview.items.length }} 行商品</p>
            <ul>
              <li v-for="(item, index) in preview.items" :key="index">
                <span>{{ item.productName }} {{ item.spec }}</span>
                <strong>{{ formatQty(item.qty) }} {{ item.unit }}</strong>
              </li>
            </ul>
            <p>
              {{ order?.bizType === 'purchase' ? '本单未付金额' : '本单未收金额' }}
              <strong>{{ formatMoney(preview.outstanding) }} → {{ formatMoney(0) }}</strong>
            </p>
          </section>
          <el-form label-position="top" @submit.prevent="confirm">
            <el-form-item label="红冲原因（必填）" :for="reasonID" :error="reasonError">
              <el-input
                :id="reasonID"
                ref="reasonInput"
                v-model="reason"
                type="textarea"
                :rows="3"
                :maxlength="255"
                :readonly="submitting"
                aria-required="true"
                @blur="validateReason"
                @input="reasonError = ''"
              />
            </el-form-item>
          </el-form>
          <p class="red-reversal__note">确认后立即过账，系统会保留原单和红冲记录。</p>
        </template>
      </template>
    </div>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">
        {{ preview && !preview.blockReason ? '取消' : '关闭' }}
      </el-button>
      <el-button v-if="error || preview?.blockReason" :disabled="submitting" @click="loadPreview">
        重新核对
      </el-button>
      <el-button v-if="preview && !preview.blockReason" type="danger" :loading="submitting" @click="confirm">
        确认红冲
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.red-reversal {
  display: grid;
  gap: 16px;

  p { margin: 0; }

  &__order {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    overflow-wrap: anywhere;
  }

  &__impact {
    padding: 12px 16px;
    border-radius: var(--radius-md);
    background: var(--app-surface-subtle);

    ul {
      list-style: none;
      padding: 0;
      margin: 8px 0 12px;
      max-height: 160px;
      overflow: auto;
    }

    li {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding: 4px 0;

      strong { white-space: nowrap; }
    }
  }

  &__note { color: var(--app-text-secondary); }
  :deep(.el-form-item) { margin-bottom: 8px; }
}
</style>
