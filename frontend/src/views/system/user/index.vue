<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  ElMessage,
  ElMessageBox,
  type FormInstance,
  type FormRules,
  type UploadFile,
  type UploadInstance,
} from 'element-plus'
import { deptApi, roleApi, userApi } from '@/api'
import type {
  DeptNode,
  ImportResult,
  RoleOption,
  Status,
  UserExportQuery,
  UserItem,
  UserPayload,
  UserQuery,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import { fileStamp, formatDateTime } from '@/utils/format'
import { useUserStore } from '@/store'

defineOptions({ name: 'SystemUser' })

const userStore = useUserStore()

// ---------------- 列表 ----------------
const loading = ref(false)
const rows = ref<UserItem[]>([])
const total = ref(0)
const query = reactive<UserQuery>({ page: 1, pageSize: 10, keyword: '' })

const statusOptions = [
  { label: '启用', value: 1 },
  { label: '禁用', value: 0 },
]

async function loadList(): Promise<void> {
  loading.value = true
  try {
    const data = await userApi.list({
      ...query,
      keyword: query.keyword || undefined,
    })
    rows.value = data.list
    total.value = data.total
  } finally {
    loading.value = false
  }
}

function resetQuery(): void {
  query.keyword = ''
  query.status = undefined
  query.deptId = undefined
  query.page = 1
  loadList()
}

function search(): void {
  query.page = 1
  loadList()
}

// ---------------- 部门与角色选项 ----------------
const deptTree = ref<DeptNode[]>([])
const roleOptions = ref<RoleOption[]>([])

async function loadOptions(): Promise<void> {
  const [depts, roles] = await Promise.all([deptApi.list(), roleApi.options()])
  deptTree.value = depts
  roleOptions.value = roles
}

// ---------------- 新增 / 编辑 ----------------
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const submitting = ref(false)
const formRef = ref<FormInstance>()
const editingId = ref<number | null>(null)

const form = reactive<UserPayload>({
  username: '',
  password: '',
  nickname: '',
  email: '',
  phone: '',
  deptId: null,
  status: 1,
  roleIds: [],
  remark: '',
})

const rules: FormRules<UserPayload> = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度为 3 ~ 64 个字符', trigger: 'blur' },
  ],
  nickname: [{ required: true, message: '请输入昵称', trigger: 'blur' }],
  email: [{ type: 'email', message: '邮箱格式不正确', trigger: 'blur' }],
}

function openCreate(): void {
  dialogMode.value = 'create'
  editingId.value = null
  Object.assign(form, {
    username: '',
    password: '',
    nickname: '',
    email: '',
    phone: '',
    deptId: null,
    status: 1 as Status,
    roleIds: [],
    remark: '',
  })
  dialogVisible.value = true
}

async function openEdit(row: UserItem): Promise<void> {
  dialogMode.value = 'edit'
  editingId.value = row.id
  Object.assign(form, {
    username: row.username,
    password: '',
    nickname: row.nickname,
    email: row.email,
    phone: row.phone,
    deptId: row.deptId,
    status: row.status,
    roleIds: [...row.roleIds],
    remark: row.remark,
  })
  dialogVisible.value = true
}

async function submitForm(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    if (dialogMode.value === 'create') {
      await userApi.create({ ...form })
      ElMessage.success('用户创建成功')
    } else if (editingId.value) {
      const payload: UserPayload = { ...form }
      delete payload.username
      delete payload.password
      await userApi.update(editingId.value, payload)
      ElMessage.success('用户信息已更新')
    }
    dialogVisible.value = false
    loadList()
  } finally {
    submitting.value = false
  }
}

// ---------------- 状态切换 ----------------
async function toggleStatus(row: UserItem): Promise<void> {
  const next: Status = row.status === 1 ? 0 : 1
  try {
    await ElMessageBox.confirm(
      `确定要${next === 1 ? '启用' : '禁用'}用户「${row.username}」吗？`,
      '状态变更',
      { type: 'warning' },
    )
  } catch {
    return
  }
  await userApi.updateStatus(row.id, next)
  ElMessage.success('状态已更新')
  loadList()
}

// ---------------- 重置密码 ----------------
const pwdVisible = ref(false)
const pwdForm = reactive({ password: '' })
const pwdTarget = ref<UserItem | null>(null)

function openResetPassword(row: UserItem): void {
  pwdTarget.value = row
  pwdForm.password = ''
  pwdVisible.value = true
}

async function submitResetPassword(): Promise<void> {
  if (!pwdTarget.value) return
  if (pwdForm.password.length < 6) {
    ElMessage.warning('密码长度不能少于 6 位')
    return
  }
  await userApi.resetPassword(pwdTarget.value.id, pwdForm.password)
  ElMessage.success('密码已重置')
  pwdVisible.value = false
}

// ---------------- 删除 ----------------
async function removeUser(row: UserItem): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `删除后该账号将无法登录，确定删除用户「${row.username}」吗？`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger' },
    )
  } catch {
    return
  }
  await userApi.remove(row.id)
  ElMessage.success('用户已删除')
  if (rows.value.length === 1 && query.page && query.page > 1) {
    query.page -= 1
  }
  loadList()
}

// ---------------- 导出 ----------------
const selectedRows = ref<UserItem[]>([])
const exporting = ref(false)

function handleSelectionChange(rows: unknown[]): void {
  selectedRows.value = rows as UserItem[]
}

/**
 * 导出用户。
 * scope='selected' 时只导出勾选行（依赖 el-table 的 reserve-selection，可跨页保留）；
 * 否则复用当前的筛选条件，保证导出内容与页面所见一致。
 */
async function handleExport(scope: string): Promise<void> {
  if (scope === 'selected' && selectedRows.value.length === 0) {
    ElMessage.warning('请先勾选要导出的用户')
    return
  }
  exporting.value = true
  try {
    const params: UserExportQuery =
      scope === 'selected'
        ? { ids: selectedRows.value.map((item) => item.id).join(',') }
        : { keyword: query.keyword || undefined, status: query.status, deptId: query.deptId }
    await userApi.export(params, `用户列表_${fileStamp()}.xlsx`)
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exporting.value = false
  }
}

// ---------------- 导入 ----------------
const uploadRef = ref<UploadInstance>()
const importVisible = ref(false)
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

function openImport(): void {
  resetImport()
  importVisible.value = true
}

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
    await loadList()
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

onMounted(async () => {
  await loadOptions()
  await loadList()
})
</script>

<template>
  <div class="user-page">
    <!-- 搜索区 -->
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="用户名 / 昵称 / 手机号"
            clearable
            style="width: 220px"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="所属部门">
          <el-tree-select
            v-model="query.deptId"
            :data="deptTree"
            :props="{ label: 'name', children: 'children' }"
            node-key="id"
            check-strictly
            clearable
            placeholder="全部部门"
            style="width: 180px"
          />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="query.status" placeholder="全部" clearable style="width: 120px">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="resetQuery">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- 列表区 -->
    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <h3>用户列表</h3>
          <span class="text-muted">共 {{ total }} 条</span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="'system:user:create'" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>新增用户</span>
          </el-button>
          <el-button v-perm="'system:user:import'" @click="openImport">
            <AppIcon name="Upload" />
            <span>导入</span>
          </el-button>
          <el-dropdown v-perm="'system:user:export'" trigger="click" @command="handleExport">
            <el-button :loading="exporting">
              <AppIcon name="Download" />
              <span>导出</span>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="filtered">按当前筛选条件导出</el-dropdown-item>
                <el-dropdown-item command="selected" :disabled="selectedRows.length === 0">
                  导出选中项{{ selectedRows.length ? `（${selectedRows.length}）` : '' }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button @click="loadList">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table
        v-loading="loading"
        :data="rows"
        stripe
        row-key="id"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="46" reserve-selection />
        <el-table-column type="index" label="#" width="56" align="center" />
        <el-table-column label="账号" min-width="180">
          <template #default="{ row }">
            <div class="cell-user">
              <el-avatar :size="30" :src="row.avatar || undefined">
                {{ (row.nickname || row.username).slice(0, 1) }}
              </el-avatar>
              <div class="cell-user__info">
                <span class="cell-user__name">{{ row.nickname || row.username }}</span>
                <span class="cell-user__account">{{ row.username }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="deptName" label="所属部门" min-width="120">
          <template #default="{ row }">
            {{ row.deptName || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="角色" min-width="160">
          <template #default="{ row }">
            <el-tag v-for="name in row.roleNames" :key="name" size="small" effect="plain" class="cell-tag">
              {{ name }}
            </el-tag>
            <span v-if="!row.roleNames.length" class="text-muted">未分配</span>
          </template>
        </el-table-column>
        <el-table-column label="联系方式" min-width="180">
          <template #default="{ row }">
            <div class="cell-contact">
              <span>{{ row.phone || '-' }}</span>
              <span class="text-muted">{{ row.email || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.status === 1"
              :disabled="row.id === 1 || row.id === userStore.profile?.id"
              @change="toggleStatus(row as UserItem)"
            />
          </template>
        </el-table-column>
        <el-table-column label="最近登录" min-width="160">
          <template #default="{ row }">
            <div class="cell-contact">
              <span>{{ formatDateTime(row.lastLoginAt) }}</span>
              <span class="text-muted">{{ row.lastLoginIp || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right" align="center">
          <template #default="{ row }">
            <el-button v-perm="'system:user:update'" link type="primary" @click="openEdit(row as UserItem)">
              编辑
            </el-button>
            <el-button v-perm="'system:user:reset'" link type="primary" @click="openResetPassword(row as UserItem)">
              重置密码
            </el-button>
            <el-button
              v-perm="'system:user:delete'"
              link
              type="danger"
              :disabled="row.id === 1 || row.id === userStore.profile?.id"
              @click="removeUser(row as UserItem)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无用户数据" :image-size="90" />
        </template>
      </el-table>

      <Pagination
        v-model:page="query.page!"
        v-model:page-size="query.pageSize!"
        :total="total"
        @change="loadList"
      />
    </div>

    <!-- 新增 / 编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增用户' : '编辑用户'"
      width="620px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="用户名" prop="username" required>
              <el-input v-model="form.username" :disabled="dialogMode === 'edit'" placeholder="登录账号" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item v-if="dialogMode === 'create'" label="初始密码" required>
              <el-input v-model="form.password" type="password" show-password placeholder="至少 6 位" />
            </el-form-item>
            <el-form-item v-else label="昵称" prop="nickname">
              <el-input v-model="form.nickname" placeholder="显示名称" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col v-if="dialogMode === 'create'" :span="12">
            <el-form-item label="昵称" prop="nickname">
              <el-input v-model="form.nickname" placeholder="显示名称" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="所属部门">
              <el-tree-select
                v-model="form.deptId"
                :data="deptTree"
                :props="{ label: 'name', children: 'children' }"
                node-key="id"
                check-strictly
                clearable
                placeholder="请选择部门"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="状态">
              <el-radio-group v-model="form.status">
                <el-radio :value="1">启用</el-radio>
                <el-radio :value="0">禁用</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="手机号">
              <el-input v-model="form.phone" placeholder="选填" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="邮箱" prop="email">
              <el-input v-model="form.email" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="角色">
          <el-select v-model="form.roleIds" multiple placeholder="请选择角色" style="width: 100%">
            <el-option v-for="role in roleOptions" :key="role.id" :label="role.name" :value="role.id" />
          </el-select>
        </el-form-item>

        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" maxlength="255" show-word-limit />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码弹窗 -->
    <el-dialog v-model="pwdVisible" title="重置密码" width="420px" destroy-on-close>
      <el-alert
        v-if="pwdTarget"
        :title="`正在为用户「${pwdTarget.username}」重置密码`"
        type="info"
        :closable="false"
        class="mb-16"
      />
      <el-input
        v-model="pwdForm.password"
        type="password"
        show-password
        placeholder="请输入新密码（至少 6 位）"
      />
      <template #footer>
        <el-button @click="pwdVisible = false">取消</el-button>
        <el-button type="primary" @click="submitResetPassword">确定</el-button>
      </template>
    </el-dialog>

    <!-- 导入弹窗：先校验预览，确认后再写库 -->
    <el-dialog
      v-model="importVisible"
      title="导入用户"
      width="860px"
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
        <el-button @click="importVisible = false">{{ importResult ? '完成' : '取消' }}</el-button>
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
  </div>
</template>

<style scoped lang="scss">
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;

  &__title {
    display: flex;
    align-items: baseline;
    gap: 10px;

    h3 {
      margin: 0;
      font-size: 15px;
      font-weight: 600;
      color: var(--app-text);
    }

    span {
      font-size: 12px;
    }
  }

  &__actions {
    display: flex;
    gap: 8px;
  }
}

.cell-user {
  display: flex;
  align-items: center;
  gap: 10px;

  &__info {
    display: flex;
    flex-direction: column;
    line-height: 1.35;
  }

  &__name {
    font-weight: 500;
    color: var(--app-text);
  }

  &__account {
    font-size: 12px;
    color: var(--app-text-muted);
  }
}

.cell-contact {
  display: flex;
  flex-direction: column;
  font-size: 12.5px;
  line-height: 1.5;
}

.cell-tag {
  margin: 0 4px 4px 0;
}

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
