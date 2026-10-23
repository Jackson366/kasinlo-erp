<script setup lang="ts">
/**
 * 合同管理台账。
 *
 * 与单据列表的差别：合同没有审核 / 出入库 / 结算这套状态机，只有
 * 「执行中 / 已完成 / 已终止」三个人工维护的状态；所以这一页是纯粹的
 * 增删改查 + 附件。设计上刻意不把它做成单据的上游约束（见 model.InvContract），
 * 免得每开一张单都要先建合同。
 */
defineOptions({ name: 'InvContract' })

import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { invApi } from '@/api'
import type {
  InvAttachBizType,
  InvContractBizType,
  InvContractPayload,
  InvContractRow,
  InvContractStatus,
  InvPartyRow,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import { useTableQuery } from '@/composables/useTableQuery'
import { useDebouncedSearch } from '@/composables/useDebouncedSearch'
import { formatDay, formatMoney } from '@/utils/format'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import AttachmentDrawer from '../components/AttachmentDrawer.vue'

interface ContractQuery {
  keyword?: string
  bizType?: InvContractBizType | ''
  status?: InvContractStatus
  /** 签订日期区间，提交时拆成 startDate / endDate */
  dateRange?: [string, string] | null
  page: number
  pageSize: number
}

const {
  query,
  loading,
  rows,
  total,
  load,
  search,
  reset,
} = useTableQuery<ContractQuery, InvContractRow>(
  { keyword: '', bizType: '', status: undefined, dateRange: null, page: 1, pageSize: 10 },
  (p) => {
    const [startDate, endDate] = p.dateRange ?? []
    return invApi.contracts({
      page: p.page,
      pageSize: p.pageSize,
      keyword: p.keyword || undefined,
      bizType: p.bizType || undefined,
      status: p.status,
      startDate,
      endDate,
    })
  },
)

// 关键字录入即查询（防抖），不必再按回车
const { searchSoon } = useDebouncedSearch(search)

type TagTone = 'primary' | 'success' | 'warning' | 'info' | 'danger'

const statusOptions: Array<{ label: string; value: InvContractStatus; tone: TagTone }> = [
  { label: '执行中', value: 1, tone: 'primary' },
  { label: '已完成', value: 2, tone: 'success' },
  { label: '已终止', value: 3, tone: 'info' },
]

function statusLabel(v: InvContractStatus): string {
  return statusOptions.find((s) => s.value === v)?.label ?? '-'
}

function statusTone(v: InvContractStatus): TagTone {
  return statusOptions.find((s) => s.value === v)?.tone ?? 'info'
}

function bizLabel(v: InvContractBizType): string {
  return v === 'purchase' ? '采购合同' : '销售合同'
}

/** 履约期展示：没填起止就显示「—」，不留两个空箭头 */
function periodText(row: InvContractRow): string {
  const start = row.startDate ? formatDay(row.startDate) : ''
  const end = row.endDate ? formatDay(row.endDate) : ''
  if (!start && !end) return '—'
  if (start && end) return `${start} ~ ${end}`
  return start ? `${start} 起` : `至 ${end}`
}

// ---------------- 往来单位（按合同类型切换客户 / 供应商） ----------------
const customers = ref<InvPartyRow[]>([])
const suppliers = ref<InvPartyRow[]>([])

async function loadParties(): Promise<void> {
  try {
    const [cus, sup] = await Promise.all([
      invApi.customers({ page: 1, pageSize: 500 }),
      invApi.suppliers({ page: 1, pageSize: 500 }),
    ])
    customers.value = cus.list
    suppliers.value = sup.list
  } catch {
    // 失败提示由响应拦截器统一处理
  }
}

// ---------------- 新增 / 编辑 ----------------
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const editingId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<InvContractPayload>({
  contractNo: '',
  name: '',
  bizType: 'sales',
  partyId: 0,
  signDate: '',
  startDate: '',
  endDate: '',
  amount: 0,
  status: 1,
  salesman: '',
  remark: '',
})

const rules: FormRules<InvContractPayload> = {
  name: [{ required: true, message: '请输入合同名称', trigger: 'blur' }],
  partyId: [{ required: true, message: '请选择往来单位', trigger: 'change' }],
  signDate: [{ required: true, message: '请选择签订日期', trigger: 'change' }],
}

const partyOptions = computed(() => (form.bizType === 'purchase' ? suppliers.value : customers.value))
const partyLabel = computed(() => (form.bizType === 'purchase' ? '供应商' : '客户'))

function today(): string {
  const d = new Date()
  const m = `${d.getMonth() + 1}`.padStart(2, '0')
  const day = `${d.getDate()}`.padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

function openCreate(): void {
  Object.assign(form, {
    contractNo: '',
    name: '',
    bizType: 'sales',
    partyId: 0,
    signDate: today(),
    startDate: '',
    endDate: '',
    amount: 0,
    status: 1,
    salesman: '',
    remark: '',
  })
  dialogMode.value = 'create'
  editingId.value = null
  dialogVisible.value = true
  // 档案可能刚被别人新增：开弹窗时重拉一次，避免下拉里没有新客户
  loadParties()
}

function openEdit(row: InvContractRow): void {
  Object.assign(form, {
    contractNo: row.contractNo,
    name: row.name,
    bizType: row.bizType,
    partyId: row.partyId,
    signDate: row.signDate ? formatDay(row.signDate) : today(),
    startDate: row.startDate ? formatDay(row.startDate) : '',
    endDate: row.endDate ? formatDay(row.endDate) : '',
    amount: row.amount,
    status: row.status,
    salesman: row.salesman,
    remark: row.remark,
  })
  dialogMode.value = 'edit'
  editingId.value = row.id
  dialogVisible.value = true
  loadParties()
}

/** 切换合同类型时清掉已选往来单位：采购合同不能挂客户 */
function onBizTypeChange(): void {
  form.partyId = 0
}

async function submitForm(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    const payload: InvContractPayload = {
      contractNo: form.contractNo?.trim() || undefined,
      name: form.name.trim(),
      bizType: form.bizType,
      partyId: form.partyId,
      signDate: form.signDate,
      startDate: form.startDate || undefined,
      endDate: form.endDate || undefined,
      amount: Number(form.amount) || 0,
      status: form.status,
      salesman: form.salesman?.trim(),
      remark: form.remark,
    }
    if (dialogMode.value === 'create') {
      await invApi.createContract(payload)
      ElMessage.success('新增成功')
    } else if (editingId.value) {
      await invApi.updateContract(editingId.value, payload)
      ElMessage.success('修改成功')
    }
    dialogVisible.value = false
    load()
  } catch {
    // 失败提示由响应拦截器统一处理（含编号重复、结束早于开始）
  } finally {
    submitting.value = false
  }
}

async function removeRow(row: InvContractRow): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确认删除合同「${row.contractNo} ${row.name}」？删除后附件也不再可见。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除' },
    )
  } catch {
    return
  }
  try {
    await invApi.deleteContract(row.id)
    ElMessage.success('已删除')
    load()
  } catch {
    // 同上
  }
}

// ---------------- 附件 ----------------
const attachVisible = ref(false)
const attachBizId = ref<number | null>(null)
const attachTitle = ref('')

function openAttach(row: InvContractRow): void {
  attachBizId.value = row.id
  attachTitle.value = `合同 ${row.contractNo} 的附件`
  attachVisible.value = true
}

// ---------------- 导出 ----------------
//
// 导出与列表共用同一批筛选（含签订日期区间）：页面上筛什么、导出就是什么。
const exporting = ref(false)

async function exportRows(): Promise<void> {
  exporting.value = true
  try {
    const [startDate, endDate] = query.dateRange ?? []
    await invApi.exportList(
      {
        entity: 'contract',
        keyword: query.keyword || undefined,
        bizType: query.bizType || undefined,
        status: query.status,
        startDate,
        endDate,
      },
      '合同台账.xlsx',
    )
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exporting.value = false
  }
}

// ---------------- 打印 ----------------
const printVisible = ref(false)
const printColumns: ListPrintColumn<InvContractRow>[] = [
  { label: '合同编号', prop: 'contractNo' },
  { label: '合同名称', prop: 'name' },
  { label: '类型', value: (r: InvContractRow) => bizLabel(r.bizType) },
  { label: '往来单位', prop: 'partyName' },
  { label: '签订日期', value: (r: InvContractRow) => formatDay(r.signDate) },
  { label: '履约期', value: (r: InvContractRow) => periodText(r) },
  { label: '金额', value: (r: InvContractRow) => formatMoney(r.amount), align: 'right' as const },
  { label: '状态', value: (r: InvContractRow) => statusLabel(r.status) },
  { label: '业务员', value: (r: InvContractRow) => r.salesman || '-' },
]
const printRows = computed(() => rows.value.map((r) => ({ ...r })))

onMounted(() => {
  load()
  loadParties()
})

const attachType: InvAttachBizType = 'contract'
</script>

<template>
  <div class="contract-page">
    <div class="page-card">
      <div class="list-head">
        <span class="list-head__title">
          合同管理<em>共 {{ total }} 份</em>
        </span>
        <el-form class="filter-form" label-width="76px" @submit.prevent>
          <el-form-item label="关键字">
            <el-input
              v-model="query.keyword"
              placeholder="编号 / 名称 / 往来单位 / 业务员"
              clearable
              @input="searchSoon"
              @keyup.enter="search"
            />
          </el-form-item>
          <el-form-item label="类型">
            <el-select v-model="query.bizType" placeholder="全部" clearable>
              <el-option label="采购合同" value="purchase" />
              <el-option label="销售合同" value="sales" />
            </el-select>
          </el-form-item>
          <el-form-item label="状态">
            <el-select v-model="query.status" placeholder="全部" clearable>
              <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="签订日期">
            <el-date-picker
              v-model="query.dateRange"
              type="daterange"
              value-format="YYYY-MM-DD"
              range-separator="至"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
              unlink-panels
            />
          </el-form-item>
        </el-form>
        <div class="list-head__actions">
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="reset">重置</el-button>
          <span class="list-head__divider" />
          <el-button :loading="exporting" @click="exportRows">导出</el-button>
          <el-button @click="printVisible = true">打印</el-button>
          <el-button v-perm="'inv:contract:create'" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>新增合同</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="contractNo" label="合同编号" width="140" show-overflow-tooltip />
        <el-table-column prop="name" label="合同名称" min-width="180" show-overflow-tooltip />
        <el-table-column label="类型" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.bizType === 'purchase' ? 'warning' : 'success'" effect="plain">
              {{ bizLabel((row as InvContractRow).bizType) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="partyName" label="往来单位" min-width="150" show-overflow-tooltip />
        <el-table-column label="签订日期" width="110">
          <template #default="{ row }">{{ formatDay(row.signDate) }}</template>
        </el-table-column>
        <el-table-column label="履约期" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">{{ periodText(row as InvContractRow) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="130" align="right">
          <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTone(row.status)" effect="light">
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="salesman" label="业务员" width="100" show-overflow-tooltip />
        <el-table-column label="操作" width="196" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openAttach(row as InvContractRow)">
              附件{{ row.attachmentCount ? `(${row.attachmentCount})` : '' }}
            </el-button>
            <el-button v-perm="'inv:contract:update'" link type="primary" @click="openEdit(row as InvContractRow)">
              编辑
            </el-button>
            <el-button v-perm="'inv:contract:delete'" link type="danger" @click="removeRow(row as InvContractRow)">
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

    <!-- 新增 / 编辑 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增合同' : '编辑合同'"
      width="760px"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="合同编号">
              <el-input v-model="form.contractNo" placeholder="留空自动生成" />
            </el-form-item>
          </el-col>
          <el-col :span="16">
            <el-form-item label="合同名称" prop="name">
              <el-input v-model="form.name" placeholder="如：2026 年度钢材采购框架合同" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="合同类型">
              <el-select v-model="form.bizType" style="width: 100%" @change="onBizTypeChange">
                <el-option label="采购合同" value="purchase" />
                <el-option label="销售合同" value="sales" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="16">
            <el-form-item :label="partyLabel" prop="partyId">
              <el-select v-model="form.partyId" filterable placeholder="请选择" style="width: 100%">
                <el-option v-for="p in partyOptions" :key="p.id" :label="p.name" :value="p.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="签订日期" prop="signDate">
              <el-date-picker
                v-model="form.signDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="必填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="开始日期">
              <el-date-picker
                v-model="form.startDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="结束日期">
              <el-date-picker
                v-model="form.endDate"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="选填"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="合同金额">
              <el-input-number
                v-model="form.amount"
                :min="0"
                :precision="2"
                :controls="false"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="状态">
              <el-select v-model="form.status" style="width: 100%">
                <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="业务员">
              <el-input v-model="form.salesman" placeholder="选填" />
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="备注">
              <el-input v-model="form.remark" type="textarea" :rows="2" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <AttachmentDrawer
      v-model="attachVisible"
      :biz-type="attachType"
      :biz-id="attachBizId"
      :title="attachTitle"
      @changed="load"
    />

    <ListPrintDialog
      v-model="printVisible"
      title="合同台账"
      :subtitle="`共 ${total} 份`"
      :columns="printColumns"
      :rows="printRows"
    />
  </div>
</template>

<style scoped lang="scss">
.table-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
