<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import Pagination from './Pagination.vue'
const props = defineProps<{
  modelValue: boolean
  title: string
  succeeded: number
  skipped: number
  failures: { name: string; reason: string }[]
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const page = ref(1)
const pageSize = ref(10)
const copyError = ref(false)
watch(
  () => props.modelValue,
  () => {
    page.value = 1
    copyError.value = false
  },
)
const visible = computed(() =>
  props.failures.slice(
    (page.value - 1) * pageSize.value,
    page.value * pageSize.value,
  ),
)
const text = computed(() =>
  props.failures.map((f) => `${f.name}\t${f.reason}`).join('\n'),
)
async function copy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(text.value)
    ElMessage.success('已复制全部失败明细')
  } catch {
    copyError.value = true
  }
}
</script>
<template>
  <el-dialog
    :model-value="modelValue"
    :title="`${title}结果`"
    width="800px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p role="status">
      成功 {{ succeeded }} 条 · 失败 {{ failures.length }} 条 · 跳过
      {{ skipped }} 条。各单独立提交。
    </p>
    <el-table v-if="failures.length" :data="visible" max-height="360">
      <el-table-column prop="name" label="完整单号" min-width="200" />
      <el-table-column prop="reason" label="失败原因" min-width="320" />
    </el-table>
    <Pagination
      v-if="failures.length > 10"
      v-model:page="page"
      v-model:page-size="pageSize"
      :total="failures.length"
    />
    <template v-if="copyError"
      ><p role="alert">复制未获浏览器允许，请选择下方全部明细手动复制。</p>
      <el-input
        :model-value="text"
        type="textarea"
        readonly
        :rows="6"
        resize="none"
        aria-label="全部失败明细"
    /></template>
    <template #footer
      ><el-button v-if="failures.length" @click="copy"
        >复制全部失败明细</el-button
      ><el-button type="primary" @click="emit('update:modelValue', false)"
        >关闭</el-button
      ></template
    >
  </el-dialog>
</template>
