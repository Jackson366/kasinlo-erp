<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { TableInstance } from 'element-plus'
import {
  ElMessage,
  ElMessageBox,
  type FormInstance,
  type FormRules,
} from 'element-plus'
import { deptApi, roleApi, userApi } from '@/api'
import { useTableQuery } from '@/composables/useTableQuery'
import { useCrudDialog } from '@/composables/useCrudDialog'
import type {
  DeptNode,
  RoleOption,
  Status,
  UserExportQuery,
  UserItem,
  UserPayload,
  UserQuery,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import UserImportDialog from './UserImportDialog.vue'
import UserPasswordDialog from './UserPasswordDialog.vue'
import { exportStamp, formatDateTime } from '@/utils/format'
import { useUserStore } from '@/store'

defineOptions({ name: 'SystemUser' })

const userStore = useUserStore()

// ---------------- 列表 ----------------
/** 表格实例：clearSelection 需要拿到真实组件，单向数据够不着内部选中状态 */
const tableRef = ref<TableInstance>()

const statusOptions = [
  { label: '启用', value: 1 },
  { label: '禁用', value: 0 },
]

// 解构时起别名：模板里沿用的名字（loadList / query / rows / total / loading）不动
const {
  query,
  loading,
  rows,
  total,
  load: loadList,
  search: searchTable,
  reset: resetTable,
} = useTableQuery<UserQuery, UserItem>(
  { keyword: '', status: undefined, deptId: undefined },
  (params) => userApi.list({ ...params, keyword: params.keyword || undefined }),
)

function search(): void {
  // 改筛选条件时必须清选择：reserve-selection 会按 row-key 保留已选项，
  // 于是「搜张三时勾了张三 → 改成搜李四」，导出选中项里仍混着张三，
  // 两个查询条件的数据被拼在一起导出。翻页保留是对的，换条件保留就是脏数据。
  clearSelection()
  searchTable()
}

function resetQuery(): void {
  // 重置等于换查询条件，勾选同样要清（理由同 search）
  clearSelection()
  resetTable()
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
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<UserPayload>({
  formRef,
  create: (payload) => userApi.create(payload),
  update: (id, payload) => userApi.update(id, payload),
  messages: { create: '用户创建成功', update: '用户信息已更新' },
})
// 解构保持模板里的旧名字：模板只展开 setup 顶层绑定，不会展开 dialog.visible 这类嵌套 ref
// editingId 不解构：本页脚本与模板都不再用它（删除回退看的是 rows/page）
const { visible: dialogVisible, mode: dialogMode, submitting } = dialog

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
  password: [
    { min: 6, max: 64, message: '密码长度为 6 ~ 64 个字符', trigger: 'blur' },
  ],
  email: [{ type: 'email', message: '邮箱格式不正确', trigger: 'blur' }],
}

function openCreate(): void {
  // 先灌表单再开窗：反序的话弹窗会先用上一轮的旧值渲染一帧
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
  dialog.openCreate()
}

async function openEdit(row: UserItem): Promise<void> {
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
  dialog.openEdit(row.id)
}

async function submitForm(): Promise<void> {
  await dialog.submit(loadList, (mode) => {
    const payload: UserPayload = { ...form }
    // 编辑时不带账号口令：username 建后不可改，password 走独立的重置入口
    if (mode === 'edit') {
      delete payload.username
      delete payload.password
    }
    return payload
  })
}

// ---------------- 状态切换 ----------------

/**
 * 正在提交状态变更的用户 ID。
 *
 * 没有它的话，用户在确认框弹出期间再点一次开关，会发出第二个请求；
 * 两个请求乱序返回时，界面状态取决于响应顺序而非用户意图。
 */
const statusSubmitting = ref<Set<number>>(new Set())

function isStatusSubmitting(id: number): boolean {
  return statusSubmitting.value.has(id)
}

/**
 * 切换用户启用状态。
 *
 * `@change` 触发时 el-switch 已经把内部值翻过来了，而它是 `:model-value` 单向绑定
 * —— 绑定值没变，Vue 不会重渲染，开关就停在一个「看起来改了、其实没改」的状态。
 * 所以取消确认或请求失败时，必须**显式改写绑定值**把视图拉回去：
 * 用相反的值赋回去，触发响应式更新，视觉即刻回到原状。
 */
async function toggleStatus(row: UserItem): Promise<void> {
  if (isStatusSubmitting(row.id)) return

  const origin = row.status
  const next: Status = origin === 1 ? 0 : 1

  // 开关被禁用时（内置超管 / 自己）不应走到这里，兜底防止绕过 UI 直接调
  if (row.id === 1 || row.id === userStore.profile?.id) {
    row.status = origin
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定要${next === 1 ? '启用' : '禁用'}用户「${row.username}」吗？`,
      '状态变更',
      { type: 'warning' },
    )
  } catch {
    // 用户取消：把绑定值写回原值，视图随之回滚
    row.status = origin
    return
  }

  statusSubmitting.value = new Set(statusSubmitting.value).add(row.id)
  try {
    await userApi.updateStatus(row.id, next)
    row.status = next
    ElMessage.success('状态已更新')
  } catch {
    // 请求失败（无权限 / 网络错误）：同样回滚，否则界面显示启用、库里还是禁用
    row.status = origin
  } finally {
    const rest = new Set(statusSubmitting.value)
    rest.delete(row.id)
    statusSubmitting.value = rest
    // 无论成败都以服务端为准重新拉一次，避免并发改动下的漂移
    loadList()
  }
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
  // 删掉的行若还在勾选集合里，reserve-selection 不会因为「数据里没有它」而自动移除，
  // 导出选中项就会带出一个已不存在的 ID。
  clearSelection()
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
 * 清空勾选。
 *
 * `reserve-selection` 让翻页时勾选得以保留（这符合预期），但也意味着
 * 勾选集合与「当前查询条件」是解耦的。删除行 / 改筛选条件后必须显式清掉，
 * 否则导出选中项会把已经不在结果集里的行也带上。
 */
function clearSelection(): void {
  tableRef.value?.clearSelection()
  selectedRows.value = []
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
    await userApi.export(params, exportStamp('用户列表'))
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exporting.value = false
  }
}

// ---------------- 重置密码 ----------------
// 弹窗拆在 UserPasswordDialog.vue，这里只留开关与目标行
const pwdVisible = ref(false)
const pwdTarget = ref<UserItem | null>(null)

function openResetPassword(row: UserItem): void {
  pwdTarget.value = row
  pwdVisible.value = true
}

// ---------------- 导入 ----------------
// 弹窗与整条导入链路拆在 UserImportDialog.vue，这里只留开关
const importVisible = ref(false)

function openImport(): void {
  importVisible.value = true
}

onMounted(async () => {
  // 两个请求互不依赖，串行会白等一个 RTT
  await Promise.all([loadOptions(), loadList()])
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
        ref="tableRef"
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
              :loading="isStatusSubmitting(row.id)"
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
      width="800px"
      top="6vh"
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

    <UserPasswordDialog v-model="pwdVisible" :target="pwdTarget" />

    <UserImportDialog v-model="importVisible" @imported="loadList" />
  </div>
</template>

<style scoped lang="scss">
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
</style>
