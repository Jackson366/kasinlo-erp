<script setup lang="ts">
/**
 * 附件抽屉：采购单 / 销售单 / 合同共用。
 *
 * 一个组件覆盖三类业务，靠 bizType 区分 —— 附件的交互（上传、预览、下载、
 * 删除）在三种业务里完全一样，分成三个组件只会三份同样的代码一起长 bug。
 *
 * 图片走 Blob 预览而不是直接把 <img src> 指向接口：附件接口要带 Authorization，
 * 而 img 标签带不了 header；顺带把「预览失败」与「下载失败」区分开。
 */
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { invApi } from '@/api'
import type { InvAttachBizType, InvAttachmentRow } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'

const props = defineProps<{
  modelValue: boolean
  bizType: InvAttachBizType
  /** 业务对象 ID；为空时不加载（弹窗先开、单据后选的情况） */
  bizId: number | null
  /** 抽屉标题，如「销售单 XS202610230001 的附件」 */
  title?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  /** 附件增删后通知父组件刷新列表角标 */
  changed: []
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

const loading = ref(false)
const uploading = ref(false)
const rows = ref<InvAttachmentRow[]>([])

const maxMB = 20
/** 与后端白名单保持一致；accept 只是给文件选择框的过滤，真正的校验在后端 */
const accept = '.jpg,.jpeg,.png,.gif,.webp,.bmp,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.zip,.rar,.7z'

async function load(): Promise<void> {
  if (!props.bizId) {
    rows.value = []
    return
  }
  loading.value = true
  try {
    rows.value = await invApi.attachments(props.bizType, props.bizId)
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.modelValue, props.bizId] as const,
  ([open]) => {
    if (open) load()
  },
  { immediate: true },
)

function beforeUpload(file: File): boolean {
  if (file.size > maxMB * 1024 * 1024) {
    ElMessage.warning(`单个附件不能超过 ${maxMB} MB`)
    return false
  }
  if (file.size === 0) {
    ElMessage.warning('这个文件是空的')
    return false
  }
  return true
}

async function doUpload(opt: { file: File }): Promise<void> {
  if (!props.bizId) {
    ElMessage.warning('请先保存单据，再上传附件')
    return
  }
  uploading.value = true
  try {
    await invApi.uploadAttachment(props.bizType, props.bizId, opt.file)
    ElMessage.success('上传成功')
    await load()
    emit('changed')
  } catch {
    // 失败提示由响应拦截器统一处理（含「不支持的文件类型」「超过 20 MB」）
  } finally {
    uploading.value = false
  }
}

async function remove(row: InvAttachmentRow): Promise<void> {
  try {
    await ElMessageBox.confirm(`确认删除附件「${row.fileName}」？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  try {
    await invApi.deleteAttachment(row.id)
    ElMessage.success('已删除')
    await load()
    emit('changed')
  } catch {
    // 同上
  }
}

async function download(row: InvAttachmentRow): Promise<void> {
  try {
    await invApi.downloadAttachment(row.id, row.fileName)
  } catch {
    // 同上
  }
}

// ---------------- 图片预览 ----------------
const previewVisible = ref(false)
const previewUrl = ref('')

async function preview(row: InvAttachmentRow): Promise<void> {
  try {
    const blob = await invApi.attachmentBlob(row.id)
    previewUrl.value = URL.createObjectURL(blob)
    previewVisible.value = true
  } catch {
    // 同上
  }
}

// 关掉预览要立刻释放 objectURL：不释放的话，翻十张图就多占十份内存，
// 一直开着抽屉的用户会在任务管理器里看到内存缓慢上涨。
watch(previewVisible, (open) => {
  if (!open && previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
    previewUrl.value = ''
  }
})

function formatSize(size: number): string {
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${Math.round(size / 1024)} KB`
  return `${size} B`
}
</script>

<template>
  <el-drawer v-model="visible" :title="title || '附件'" size="480px" :destroy-on-close="false">
    <el-upload
      class="uploader"
      drag
      multiple
      :show-file-list="false"
      :accept="accept"
      :before-upload="beforeUpload"
      :http-request="doUpload"
      :disabled="uploading || !bizId"
    >
      <div class="uploader__body">
        <AppIcon name="UploadFilled" />
        <p class="uploader__title">
          {{ uploading ? '上传中…' : '把文件拖到这里，或点击选择' }}
        </p>
        <p class="uploader__hint">
          支持图片、PDF、Office 文档、压缩包，单个不超过 {{ maxMB }} MB
        </p>
      </div>
    </el-upload>

    <div v-loading="loading" class="attach-list">
      <div v-for="row in rows" :key="row.id" class="attach">
        <div class="attach__thumb" @click="row.isImage ? preview(row) : download(row)">
          <AppIcon v-if="row.isImage" name="Picture" />
          <span v-else class="attach__ext">{{ (row.ext || '').replace('.', '').toUpperCase() || 'FILE' }}</span>
        </div>
        <div class="attach__main">
          <button class="attach__name" type="button" @click="row.isImage ? preview(row) : download(row)">
            {{ row.fileName }}
          </button>
          <span class="attach__meta">
            {{ formatSize(row.size) }}
            <template v-if="row.uploader"> · {{ row.uploader }}</template>
          </span>
        </div>
        <div class="attach__ops">
          <el-button v-if="row.isImage" link @click="preview(row)">预览</el-button>
          <el-button link @click="download(row)">下载</el-button>
          <el-button link type="danger" @click="remove(row)">删除</el-button>
        </div>
      </div>

      <el-empty v-if="!loading && rows.length === 0" description="还没有附件" :image-size="70" />
    </div>

    <el-dialog v-model="previewVisible" width="720px" align-center>
      <img v-if="previewUrl" :src="previewUrl" class="preview" alt="附件预览" />
    </el-dialog>
  </el-drawer>
</template>

<style scoped lang="scss">
.uploader {
  :deep(.el-upload),
  :deep(.el-upload-dragger) {
    width: 100%;
  }

  :deep(.el-upload-dragger) {
    padding: 18px 12px;
  }
}

.uploader__body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  color: var(--app-text-secondary);
  font-size: 20px;
}

.uploader__title {
  margin: 2px 0 0;
  font-size: 13px;
  color: var(--app-text);
}

.uploader__hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-muted);
}

.attach-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 14px;
  min-height: 80px;
}

.attach {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  background: var(--app-surface);
}

.attach__thumb {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  overflow: hidden;
  border-radius: var(--radius-sm, 4px);
  background: var(--app-surface-subtle);
  color: var(--app-text-secondary);
  cursor: pointer;
}

.attach__ext {
  font-size: 10px;
  font-weight: 600;
  color: var(--app-text-muted);
}

.attach__main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.attach__name {
  padding: 0;
  border: none;
  background: none;
  font-size: 13px;
  color: var(--app-text);
  text-align: left;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    color: var(--app-primary);
    text-decoration: underline;
  }
}

.attach__meta {
  font-size: 12px;
  color: var(--app-text-muted);
}

.attach__ops {
  flex: none;
  display: flex;
  align-items: center;
  gap: 2px;
}

.preview {
  display: block;
  max-width: 100%;
  max-height: 70vh;
  margin: 0 auto;
}
</style>
