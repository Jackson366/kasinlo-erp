<script setup lang="ts">
/**
 * 客户 / 供应商页面主体。两者字段、列、交互完全一致，差异只有
 * 「调哪个接口」「余额口径」「文案」——收敛成一个组件，由两个薄壳页面传入 bizType。
 */
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type { InvPartyPayload, InvPartyRow, Status } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import StatusTag from '@/components/StatusTag.vue'
import { formatDate, formatMoney } from '@/utils/format'
import { useCrudDialog } from '@/composables/useCrudDialog'
import { useTableQuery } from '@/composables/useTableQuery'

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

interface PartyQuery {
  page?: number
  pageSize?: number
  keyword?: string
  status?: Status
}

const {
  query,
  loading,
  rows,
  total,
  load,
  search,
  reset,
} = useTableQuery<PartyQuery, InvPartyRow>(
  { keyword: '', status: undefined },
  (params) =>
    props.bizType === 'purchase'
      ? invApi.suppliers({
          page: params.page,
          pageSize: params.pageSize,
          keyword: params.keyword || undefined,
          status: params.status ?? undefined,
        })
      : invApi.customers({
          page: params.page,
          pageSize: params.pageSize,
          keyword: params.keyword || undefined,
          status: params.status ?? undefined,
        }),
)

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
  code: [{ required: true, message: '请输入编号', trigger: 'blur' }],
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

onMounted(load)
</script>

<template>
  <div class="party-page">
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="编号 / 名称 / 联系人"
            clearable
            style="width: 240px"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="query.status" placeholder="全部" clearable style="width: 140px">
            <el-option label="启用" :value="1" />
            <el-option label="停用" :value="0" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <h3>{{ title }}</h3>
          <span class="text-muted">{{ subtitle }} · 共 {{ total }} 条</span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="`inv:${bizType}:create`" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>{{ createText }}</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
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
            <el-button v-perm="`inv:${bizType}:update`" link type="primary" @click="openEdit(row as InvPartyRow)">
              编辑
            </el-button>
            <el-button v-perm="`inv:${bizType}:delete`" link type="danger" @click="removeRow(row as InvPartyRow)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="table-pager">
        <Pagination
          v-model:page="query.page"
          v-model:page-size="query.pageSize"
          :total="total"
          @change="load"
        />
      </div>
    </div>

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
              <el-input v-model="form.code" placeholder="如 KH001" />
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
.table-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.amount-warn {
  color: var(--tone-amber);
  font-weight: 600;
}
</style>
