<script setup lang="ts">
import { KasTable, useKasTable, type KasTableHandle } from '@/components/KasTable'
/**
 * 客户 / 供应商页面主体。两者字段、列、交互完全一致，差异只有
 * 「调哪个接口」「余额口径」「文案」——收敛成一个组件，由两个薄壳页面传入 bizType。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type { InvPartyPayload, InvPartyRow, Status } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import ListFilters from '@/components/ListFilters.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import StatusTag from '@/components/StatusTag.vue'
import { formatDate, formatMoney } from '@/utils/format'
import { useCrudDialog } from '@/composables/useCrudDialog'
import { useDebouncedSearch } from '@/composables/useDebouncedSearch'

const props = defineProps<{
  /** purchase = 供应商，sales = 客户 */
  bizType: 'purchase' | 'sales'
  title: string
  subtitle: string
  /** 新增按钮文案 */
  createText: string
  /** 余额列标题（应收余额 / 应付余额） */
  balanceLabel: string
}>()

// 权限码前缀与接口路径同源：客户是 inv:customer:*，供应商是 inv:supplier:*。
// 之前误用了业务线前缀（inv:sales / inv:purchase），后果是双输：
// 按后端权限码授权的「客户管理员」看不到新增/编辑/删除按钮，
// 而只有开单权限的人反而看到按钮，点了必然 403。
const partyPerm = computed(() => (props.bizType === 'purchase' ? 'supplier' : 'customer'))

interface PartyQuery {
  page?: number
  pageSize?: number
  keyword?: string
  status?: Status
}

// 列表和导出共享同一份筛选转换，避免后续加条件时漏改一处。
function filters(params: PartyQuery) {
  return { keyword: params.keyword || undefined, status: params.status ?? undefined }
}

const tableRef = ref<KasTableHandle>()
const {
  query,
  loading,
  error: listError,
  rows,
  total,
  load,
  search,
  reset,
  selection,
  exporting,
  batchRunning,
  onSelectionChange,
  clearSelection,
  exportRows,
  runBatch,
} = useKasTable<PartyQuery, InvPartyRow>({
  initial: { keyword: '', status: undefined },
  table: () => tableRef.value,
  fetch: (params) => {
    const payload = { page: params.page, pageSize: params.pageSize, ...filters(params) }
    return props.bizType === 'purchase' ? invApi.suppliers(payload) : invApi.customers(payload)
  },
  export: {
    exportUrl: '/inv/export',
    exportParams: (params) => ({
      entity: props.bizType === 'purchase' ? 'supplier' : 'customer',
      ...filters(params),
    }),
    fallbackName: `${props.title}.xlsx`,
  },
})

// ---------------- 新增 / 编辑 ----------------
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<InvPartyPayload>({
  formRef,
  create: (payload) =>
    props.bizType === 'purchase' ? invApi.createSupplier(payload) : invApi.createCustomer(payload),
  update: (id, payload) =>
    props.bizType === 'purchase'
      ? invApi.updateSupplier(id, payload)
      : invApi.updateCustomer(id, payload),
  messages: { create: '新增成功', update: '修改成功' },
})
const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog

const form = reactive<InvPartyPayload>({
  code: '',
  name: '',
  contact: '',
  phone: '',
  address: '',
  taxNo: '',
  status: 1,
  remark: '',
})

const rules: FormRules<InvPartyPayload> = {
  // 编号可留空由后端自动生成（客户 KH#### / 供应商 GYS####）
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
}

function openCreate(): void {
  Object.assign(form, {
    code: '',
    name: '',
    contact: '',
    phone: '',
    address: '',
    taxNo: '',
    status: 1,
    remark: '',
  })
  dialog.openCreate()
}

function openEdit(row: InvPartyRow): void {
  Object.assign(form, {
    code: row.code,
    name: row.name,
    contact: row.contact,
    phone: row.phone,
    address: row.address,
    taxNo: row.taxNo,
    status: row.status,
    remark: row.remark,
  })
  dialog.openEdit(row.id)
}

async function submitForm(): Promise<void> {
  await dialog.submit(load, () => ({ ...form }))
}

async function removeRow(row: InvPartyRow): Promise<void> {
  try {
    await ElMessageBox.confirm(`确定删除「${row.name}」吗？已被单据引用时无法删除。`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  if (props.bizType === 'purchase') {
    await invApi.deleteSupplier(row.id)
  } else {
    await invApi.deleteCustomer(row.id)
  }
  ElMessage.success('删除成功')
  load()
}

/** 打开弹窗时清掉上一条的校验残留 */
function onDialogOpen(): void {
  formRef.value?.clearValidate()
}

void editingId

// ---------------- 导出 / 打印 / 批量 ----------------
const printVisible = ref(false)
const printRows = computed(() => (selection.value.length ? selection.value : rows.value))

const printColumns = computed<ListPrintColumn<InvPartyRow>[]>(() => [
  { label: '编号', prop: 'code' },
  { label: '名称', prop: 'name' },
  { label: '联系人', value: (r) => r.contact || '-' },
  { label: '电话', value: (r) => r.phone || '-' },
  { label: '地址', value: (r) => r.address || '-' },
  { label: '开票信息', value: (r) => r.taxNo || '-' },
  { label: props.balanceLabel, value: (r) => formatMoney(r.balance), align: 'right' },
  { label: '状态', value: (r) => (r.status === 1 ? '启用' : '停用') },
])

const printSubtitle = computed(() => {
  const parts: string[] = []
  if (query.keyword) parts.push(`关键字「${query.keyword}」`)
  if (query.status !== undefined) parts.push(query.status === 1 ? '启用' : '停用')
  if (selection.value.length) parts.push('仅打印勾选')
  return parts.join(' · ') || '全部记录'
})

function deleteOne(row: InvPartyRow): Promise<unknown> {
  return props.bizType === 'purchase'
    ? invApi.deleteSupplier(row.id)
    : invApi.deleteCustomer(row.id)
}

async function batchDelete(): Promise<void> {
  if (!selection.value.length) return
  const done = await runBatch('批量删除', selection.value, deleteOne, (r) => r.name)
  if (done) load()
}

onMounted(load)
// 关键字录入即查询（防抖），不必再按回车
const { searchSoon, searchNow, cancel: cancelSearch } = useDebouncedSearch(search)
</script>

<template>
  <div class="party-page">
    <KasTable
      ref="tableRef"
      v-model:page="query.page"
      v-model:page-size="query.pageSize"
      :title="title"
      :subtitle="subtitle"
      :rows="rows"
      :loading="loading"
      :error="listError"
      :total="total"
      :selected-count="selection.length"
      @change="load"
      @retry="load"
      @selection-change="onSelectionChange"
      @clear-selection="clearSelection()"
    >
      <template #filters>
        <ListFilters>
          <el-form-item label="关键字">
            <el-input
              v-model="query.keyword"
              @input="searchSoon"
              placeholder="编号 / 名称 / 联系人"
              clearable
              @keyup.enter="searchNow"
            />
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="query.status" placeholder="全部" clearable>
              <el-option label="启用" :value="1" />
              <el-option label="停用" :value="0" />
            </el-select>
          </el-form-item>
          <template #actions>
            <el-button type="primary" @click="searchNow">
              <AppIcon name="Search" />
              <span>查询</span>
            </el-button>
            <el-button @click="cancelSearch(); reset()">重置</el-button>
          </template>
          <template #tools>
            <el-button v-if="selection.length" v-perm="`inv:${partyPerm}:delete`" :loading="batchRunning" @click="batchDelete">
              批量删除
            </el-button>
            <el-button :loading="exporting" :disabled="loading || listError" @click="exportRows(selection.length ? selection : undefined)">
              导出{{ selection.length ? '选中' : '' }}
            </el-button>
            <el-button :disabled="loading || listError" @click="printVisible = true">打印</el-button>
            <el-button v-perm="`inv:${partyPerm}:create`" type="primary" @click="openCreate">
              <AppIcon name="Plus" />
              <span>{{ createText }}</span>
            </el-button>
            <el-button @click="load">
              <AppIcon name="Refresh" />
            </el-button>
          </template>
        </ListFilters>
      </template>
      <el-table-column type="selection" width="44" />
      <el-table-column prop="code" label="编号" width="90" />
      <el-table-column prop="name" label="名称" min-width="130" show-overflow-tooltip />
      <el-table-column prop="contact" label="联系人" width="90" />
      <el-table-column prop="phone" label="电话" width="120" />
      <el-table-column prop="address" label="地址" min-width="160" show-overflow-tooltip />
      <el-table-column prop="taxNo" label="开票信息" min-width="150" show-overflow-tooltip />
      <el-table-column :label="balanceLabel" width="120" align="right">
        <template #default="{ row }">
          <span :class="row.balance > 0 ? 'amount-warn' : 'text-muted'">
            {{ formatMoney(row.balance) }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="80" align="center">
        <template #default="{ row }">
          <StatusTag :status="row.status" />
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="110">
        <template #default="{ row }">
          <span class="text-secondary">{{ formatDate(row.createdAt) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right" align="center">
        <template #default="{ row }">
          <el-button v-perm="`inv:${partyPerm}:update`" link type="primary" @click="openEdit(row as InvPartyRow)">
            编辑
          </el-button>
          <el-button v-perm="`inv:${partyPerm}:delete`" link type="danger" @click="removeRow(row as InvPartyRow)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </KasTable>

    <ListPrintDialog
      v-model="printVisible"
      :title="title"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />

    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? createText : '编辑资料'"
      width="800px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
      @open="onDialogOpen"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="编号" prop="code">
              <el-input v-model="form.code" placeholder="留空自动生成，也可自定义" clearable />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="名称" prop="name">
              <el-input v-model="form.name" placeholder="往来单位名称" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="联系人">
              <el-input v-model="form.contact" placeholder="选填" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="联系电话">
              <el-input v-model="form.phone" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="开票信息">
          <el-input v-model="form.taxNo" placeholder="统一社会信用代码" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.address" placeholder="选填" />
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="状态">
              <el-switch
                :model-value="form.status === 1"
                @update:model-value="(v) => (form.status = v ? 1 : 0)"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="备注">
              <el-input v-model="form.remark" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.amount-warn {
  color: var(--tone-amber);
  font-weight: 600;
}
</style>
