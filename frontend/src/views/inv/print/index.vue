<script setup lang="ts">
/**
 * 打印模板页：以卡片网格展示各单据的打印版式。
 *
 * 模板数量少（十位数以内），用卡片而不是表格——纸张规格与启用状态是
 * 这一页的主要识别信息，卡片网格比一行行文字更好扫读。
 */
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { invApi } from '@/api'
import type { InvPrintTemplatePayload, InvPrintTemplateRow } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { useCrudDialog } from '@/composables/useCrudDialog'
import { useTableQuery } from '@/composables/useTableQuery'
import TemplateDesigner from '../components/TemplateDesigner.vue'

defineOptions({ name: 'InvPrint' })

interface TemplateQuery {
  keyword?: string
  status?: number
}

const { query, loading, rows, total, load, search, reset } = useTableQuery<
  TemplateQuery,
  InvPrintTemplateRow
>({ keyword: '', status: undefined }, (params) =>
  invApi.printTemplates({
    page: params.page,
    pageSize: params.pageSize,
    keyword: params.keyword || undefined,
    status: params.status,
  }),
)

const formRef = ref<FormInstance>()
const designerRef = ref<InstanceType<typeof TemplateDesigner> | null>(null)
const form = reactive<InvPrintTemplatePayload>({
  code: '',
  name: '',
  docType: '',
  paper: 'A4',
  status: 1,
  remark: '',
})

const rules: FormRules = {
  code: [{ required: true, message: '请输入模板编码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入模板名称', trigger: 'blur' }],
}

const dialog = useCrudDialog<InvPrintTemplatePayload>({
  formRef,
  create: (payload) => invApi.createPrintTemplate(payload),
  update: (id, payload) => invApi.updatePrintTemplate(id, payload),
  messages: { create: '模板已创建', update: '模板已更新' },
})
// 解构而不是直接用 dialog.xxx：模板只展开 setup 顶层绑定
const {
  visible: dialogVisible,
  isEdit,
  submitting,
  openCreate,
  openEdit,
  submit,
} = dialog

function openCreateDialog(): void {
  openCreate()
  Object.assign(form, { code: '', name: '', docType: '', paper: 'A4', status: 1, remark: '' })
}

function openEditDialog(row: InvPrintTemplateRow): void {
  openEdit(row.id)
  Object.assign(form, {
    code: row.code,
    name: row.name,
    docType: row.docType,
    paper: row.paper === 'A5' ? 'A5' : 'A4',
    status: row.status,
    remark: row.remark,
  })
}

async function submitForm(): Promise<void> {
  await submit(load, () => ({ ...form }))
}

async function removeRow(row: InvPrintTemplateRow): Promise<void> {
  try {
    await ElMessageBox.confirm(`确定删除模板「${row.name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await invApi.deletePrintTemplate(row.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>

<template>
  <div class="print-page">
    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <el-input
            v-model="query.keyword"
            placeholder="编码 / 名称 / 单据类型"
            clearable
            style="width: 240px"
            @keyup.enter="search"
          />
          <el-select v-model="query.status" placeholder="全部状态" clearable style="width: 140px">
            <el-option label="启用" :value="1" />
            <el-option label="停用" :value="0" />
          </el-select>
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
          <span class="text-muted">共 {{ total }} 个模板</span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="'inv:print:create'" type="primary" @click="openCreateDialog">
            <AppIcon name="Plus" />
            <span>新增模板</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <div v-loading="loading" class="tpl-grid">
        <article
          v-for="row in rows"
          :key="row.id"
          class="tpl-card"
          :class="{ 'is-off': row.status === 0 }"
        >
          <header class="tpl-card__head">
            <div class="tpl-card__paper">{{ row.paper || 'A4' }}</div>
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="light">
              {{ row.status === 1 ? '启用' : '停用' }}
            </el-tag>
          </header>

          <h4 class="tpl-card__name">{{ row.name }}</h4>
          <p class="tpl-card__code">{{ row.code }}</p>

          <div class="tpl-card__meta">
            <span class="tpl-card__label">单据类型</span>
            <span class="tpl-card__value">{{ row.docType || '-' }}</span>
          </div>
          <div class="tpl-card__meta">
            <span class="tpl-card__label">备注</span>
            <span class="tpl-card__value">{{ row.remark || '-' }}</span>
          </div>

          <footer class="tpl-card__foot">
            <el-button v-perm="'inv:print:update'" link type="primary" @click="designerRef?.open(row as InvPrintTemplateRow)">
              设计
            </el-button>
            <el-button v-perm="'inv:print:update'" link type="primary" @click="openEditDialog(row)">
              编辑
            </el-button>
            <el-button
              v-perm="'inv:print:delete'"
              link
              type="danger"
              @click="removeRow(row as InvPrintTemplateRow)"
            >
              删除
            </el-button>
          </footer>
        </article>

        <el-empty v-if="!loading && rows.length === 0" description="暂无打印模板" />
      </div>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑打印模板' : '新增打印模板'"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-form-item label="模板编码" prop="code">
          <el-input v-model="form.code" placeholder="如 BG01" />
        </el-form-item>
        <el-form-item label="模板名称" prop="name">
          <el-input v-model="form.name" placeholder="如 销售订单" />
        </el-form-item>
        <el-form-item label="单据类型">
          <el-input v-model="form.docType" placeholder="选填，默认与名称一致" />
        </el-form-item>
        <el-form-item label="纸张规格">
          <el-radio-group v-model="form.paper">
            <el-radio-button value="A4">A4</el-radio-button>
            <el-radio-button value="A5">A5</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.status" :active-value="1" :inactive-value="0" />
          <span class="text-muted switch-hint">{{ form.status === 1 ? '启用' : '停用' }}</span>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" placeholder="选填" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <TemplateDesigner ref="designerRef" @saved="load" />
  </div>
</template>

<style scoped lang="scss">
.tpl-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(248px, 1fr));
  gap: 16px;
  min-height: 120px;
}

.tpl-card {
  border: 1px solid var(--app-border-light);
  border-radius: var(--radius-md);
  background: var(--app-bg-elevated);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  transition:
    border-color 0.16s ease,
    box-shadow 0.16s ease;

  &:hover {
    border-color: var(--brand-primary);
    box-shadow: 0 4px 14px rgb(0 0 0 / 6%);
  }

  &.is-off {
    opacity: 0.66;
  }

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  &__paper {
    width: 40px;
    height: 40px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    font-weight: 600;
    color: #fff;
    background: var(--brand-gradient);
    letter-spacing: 0.5px;
  }

  &__name {
    margin: 4px 0 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--app-text-primary);
  }

  &__code {
    margin: 0;
    font-size: 12px;
    color: var(--app-text-muted);
    font-family: var(--font-mono, monospace);
  }

  &__meta {
    display: flex;
    gap: 8px;
    font-size: 12px;
    line-height: 18px;
    margin-top: 2px;
  }

  &__label {
    color: var(--app-text-muted);
    flex: none;
    width: 52px;
  }

  &__value {
    color: var(--app-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__foot {
    margin-top: auto;
    padding-top: 10px;
    border-top: 1px dashed var(--app-border-light);
    display: flex;
    justify-content: flex-end;
    gap: 4px;
  }
}

.switch-hint {
  margin-left: 10px;
}
</style>
