<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage, type UploadFile, type UploadInstance } from 'element-plus'
import { userApi } from '@/api'
import type { ImportResult } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'

/**
 * 用户导入弹窗。
 *
 * 从 user/index.vue 拆出：整条导入链路（选文件 → dry-run 校验预览 →
 * 确认写库 → 结果反馈）自成一体，与外页唯一的交互是「导入成功后刷新列表」。
 * 留在主页里时它一个人占了约 200 行，把主页撑到近千行。
 */
const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  /** 导入成功：通知外层刷新列表 */
  imported: []
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
})

const uploadRef = ref<UploadInstance>()
const importFile = ref<File | null>(null)
const importPreview = ref<ImportResult | null>(null)
const importResult = ref<ImportResult | null>(null)
const importLoading = ref(false)
const importSubmitting = ref(false)
const onlyErrors = ref(false)

/** 校验明细：默认展示全部行，可切换为仅看失败行 */
const previewRows = computed(() => {
  const list = importPreview.value?.rows ?? []
  return onlyErrors.value ? list.filter((row) => row.errors.length > 0) : list
})

function resetImport(): void {
  importFile.value = null
  importPreview.value = null
  importResult.value = null
  onlyErrors.value = false
  uploadRef.value?.clearFiles()
}

/**
 * 选中文件后先做一次 dry-run 校验。
 * 关键点：这一步不写库，用户确认后才会真正导入。
 */
async function handleFileChange(file: UploadFile): Promise<void> {
  if (importLoading.value) return

  const raw = file.raw
  if (!raw) return
  if (!raw.name.toLowerCase().endsWith('.xlsx')) {
    ElMessage.warning('仅支持 .xlsx 格式，请用 Excel 另存为「Excel 工作簿(*.xlsx)」后重试')
    resetImport()
    return
  }

  importFile.value = raw
  importPreview.value = null
  importResult.value = null
  importLoading.value = true
  try {
    importPreview.value = await userApi.import(raw, true)
  } catch {
    // 表头不匹配等错误已由响应拦截器提示，这里回到初始状态让用户重新选文件
    importFile.value = null
  } finally {
    importLoading.value = false
    // 清空 el-upload 内部列表，否则重复选择同一个文件不会再触发校验
    uploadRef.value?.clearFiles()
  }
}

async function submitImport(): Promise<void> {
  const file = importFile.value
  const preview = importPreview.value
  if (!file || !preview) return
  if (preview.valid === 0) {
    ElMessage.warning('没有校验通过的数据行，请修正文件后重新上传')
    return
  }

  importSubmitting.value = true
  try {
    importResult.value = await userApi.import(file, false)
    ElMessage.success(`成功导入 ${importResult.value.created} 条用户`)
    emit('imported')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    importSubmitting.value = false
  }
}

async function downloadTemplate(): Promise<void> {
  try {
    await userApi.downloadTemplate()
  } catch {
    // 失败提示由响应拦截器统一处理
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="导入用户"
    width="1280px"
    top="6vh"
    :close-on-click-modal="false"
    destroy-on-close
    @closed="resetImport"
  >
    <div class="import">
      <div class="import__steps">
        <div class="import__row">
          <span class="import__label">1</span>
          <span class="import__text">下载模板，按「填写说明」填写数据</span>
          <el-button link type="primary" class="import__action" @click="downloadTemplate">
            <AppIcon name="Download" />
            <span>下载导入模板</span>
          </el-button>
        </div>
        <div class="import__row">
          <span class="import__label">2</span>
          <span class="import__text">上传填好的文件，系统会先校验并逐行展示结果</span>
          <el-upload
            ref="uploadRef"
            class="import__action"
            :auto-upload="false"
            :show-file-list="false"
            :limit="1"
            accept=".xlsx"
            :on-change="handleFileChange"
          >
            <el-button type="primary" :loading="importLoading">
              <AppIcon name="Upload" />
              <span>选择文件并校验</span>
            </el-button>
          </el-upload>
        </div>
        <div v-if="importFile" class="import__file">已选文件：{{ importFile.name }}</div>
      </div>

      <template v-if="importPreview">
        <el-alert
          class="import__summary"
          :type="importPreview.invalid === 0 ? 'success' : importPreview.valid > 0 ? 'warning' : 'error'"
          :closable="false"
          show-icon
        >
          <template #title>
            共 {{ importPreview.total }} 行数据：校验通过
            <b>{{ importPreview.valid }}</b> 行，未通过 <b>{{ importPreview.invalid }}</b> 行
          </template>
        </el-alert>

        <el-alert
          v-if="importResult"
          class="import__summary"
          type="success"
          :closable="false"
          show-icon
          :title="`导入完成，成功创建 ${importResult.created} 个用户`"
        >
          <template #default>
            密码列留空的账号已使用默认密码 <b>123456</b>，请提醒相关用户尽快修改。
          </template>
        </el-alert>

        <div v-if="importPreview.invalid > 0" class="import__detail-head">
          <span class="text-muted">校验明细</span>
          <el-switch v-model="onlyErrors" size="small" active-text="仅看失败行" />
        </div>

        <el-table v-if="previewRows.length" :data="previewRows" size="small" max-height="300" border>
          <el-table-column prop="row" label="行号" width="70" align="center" />
          <el-table-column prop="username" label="用户名" min-width="130">
            <template #default="{ row }">{{ row.username || '—' }}</template>
          </el-table-column>
          <el-table-column prop="nickname" label="昵称" min-width="120">
            <template #default="{ row }">{{ row.nickname || '—' }}</template>
          </el-table-column>
          <el-table-column label="校验结果" min-width="300">
            <template #default="{ row }">
              <el-tag v-if="row.errors.length === 0" type="success" size="small" effect="plain">
                通过
              </el-tag>
              <span v-else class="import__error">{{ row.errors.join('；') }}</span>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else description="没有失败的行" :image-size="70" />
      </template>
    </div>

    <template #footer>
      <el-button @click="visible = false">{{ importResult ? '完成' : '取消' }}</el-button>
      <el-button
        v-if="!importResult"
        type="primary"
        :loading="importSubmitting"
        :disabled="!importPreview || importPreview.valid === 0"
        @click="submitImport"
      >
        确认导入{{ importPreview?.valid ? ` ${importPreview.valid} 条` : '' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.import {
  &__steps {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 14px 16px;
    margin-bottom: 16px;
    background: var(--app-fill);
    border-radius: var(--radius-md);
  }

  &__row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  &__label {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    font-size: 12px;
    font-weight: 600;
    color: #fff;
    background: var(--brand-primary);
    border-radius: 50%;
  }

  &__text {
    flex: 1;
    font-size: 13px;
    color: var(--app-text-secondary);
  }

  &__action {
    flex-shrink: 0;
  }

  &__file {
    font-size: 12.5px;
    color: var(--app-text-muted);
    word-break: break-all;
  }

  &__summary {
    margin-bottom: 12px;
  }

  &__detail-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    font-size: 12.5px;
  }

  &__error {
    font-size: 12.5px;
    color: var(--tone-red);
  }
}
</style>
