<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, type Ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import type { ElTree } from 'element-plus'
import { menuApi, roleApi } from '@/api'
import { useTableQuery } from '@/composables/useTableQuery'
import { useCrudDialog } from '@/composables/useCrudDialog'
import { runBatchAction } from '@/composables/useListTools'
import type { MenuNode, PageQuery, RoleItem, RolePayload } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import StatusTag from '@/components/StatusTag.vue'
import Pagination from '@/components/Pagination.vue'
import { formatDate, exportStamp, flattenTree } from '@/utils/format'
import { useDebouncedSearch } from '@/composables/useDebouncedSearch'

defineOptions({ name: 'SystemRole' })

// ---------------- 列表 ----------------
const dataScopeLabels: Record<number, string> = {
  1: '全部数据',
  2: '本部门',
  3: '本部门及以下',
  4: '仅本人',
}

// 解构时起别名：模板里沿用的名字（loadList / search / resetQuery / query / rows / total / loading）不动
const {
  query,
  loading,
  rows,
  total,
  load: loadList,
  search,
  reset: resetQuery,
} = useTableQuery<PageQuery, RoleItem>(
  { keyword: '' },
  (params) => roleApi.list({ ...params, keyword: params.keyword || undefined }),
)

// ---------------- 导出 ----------------
const exporting = ref(false)

/** 按当前筛选条件导出角色 */
async function handleExport(): Promise<void> {
  exporting.value = true
  try {
    await roleApi.export(
      { keyword: query.keyword || undefined },
      exportStamp('角色列表'),
    )
    ElMessage.success('导出成功')
  } catch {
    // 失败提示由响应拦截器统一处理
  } finally {
    exporting.value = false
  }
}

// ---------------- 菜单权限树 ----------------
const menuTree = ref<MenuNode[]>([])
const treeRef = ref<InstanceType<typeof ElTree>>()

/** 叶子节点 ID 集合：设置勾选状态时只回填叶子，避免父节点被整体勾选 */
const leafIds = computed(() => {
  const leaves = flattenTree(menuTree.value).filter((node) => !node.children?.length)
  return new Set(leaves.map((node) => node.id))
})

async function loadMenuTree(): Promise<void> {
  menuTree.value = await menuApi.list()
}

function setCheckedMenus(menuIds: number[]): void {
  const checked = menuIds.filter((id) => leafIds.value.has(id))
  treeRef.value?.setCheckedKeys(checked, false)
}

/** 收集勾选的菜单：叶子全选 + 半选的父节点 */
function collectMenuIds(): number[] {
  if (!treeRef.value) return []
  const checked = treeRef.value.getCheckedKeys(false) as number[]
  const half = treeRef.value.getHalfCheckedKeys() as number[]
  return Array.from(new Set([...checked, ...half]))
}

// ---------------- 新增 / 编辑 ----------------
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<RolePayload>({
  formRef,
  create: (payload) => roleApi.create(payload),
  update: (id, payload) => roleApi.update(id, payload),
  messages: { create: '角色创建成功', update: '角色已更新' },
})
// 解构保持模板里的旧名字：模板只展开 setup 顶层绑定，不会展开 dialog.visible 这类嵌套 ref
const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog

const form = reactive<RolePayload>({
  name: '',
  code: '',
  sort: 0,
  status: 1,
  dataScope: 1,
  remark: '',
})

const rules: FormRules<RolePayload> = {
  name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
  code: [
    { required: true, message: '请输入角色标识', trigger: 'blur' },
    {
      pattern: /^[a-zA-Z][a-zA-Z0-9_]{1,63}$/,
      message: '以字母开头，仅可包含字母、数字与下划线',
      trigger: 'blur',
    },
  ],
}

const isBuiltin = computed(() => rows.value.find((item) => item.id === editingId.value)?.isBuiltin === true)

async function openCreate(): Promise<void> {
  Object.assign(form, { name: '', code: '', sort: 0, status: 1, dataScope: 1, remark: '' })
  dialog.openCreate()
  if (await whenTreeReady(treeRef)) setCheckedMenus([])
  else ElMessage.warning('权限树尚未就绪，请重新打开弹窗')
}

async function openEdit(row: RoleItem): Promise<void> {
  Object.assign(form, {
    name: row.name,
    code: row.code,
    sort: row.sort,
    status: row.status,
    dataScope: row.dataScope,
    remark: row.remark,
  })
  dialog.openEdit(row.id)
  if (await whenTreeReady(treeRef)) setCheckedMenus(row.menuIds)
  else ElMessage.warning('权限树尚未就绪，请重新打开弹窗')
}

/**
 * 等待弹窗内树组件挂载完成后执行回调。
 *
 * 不能用固定 setTimeout 猜渲染时机：机器慢或树大时 60ms 内 ref 还没挂载，
 * setCheckedKeys 会静默 no-op，表现为「编辑角色时权限勾选回显随机丢失」且无报错。
 * 改为在微任务帧里轮询 ref（上限 10 帧），ref 就绪即执行。
 */
async function whenTreeReady(tree: Ref<InstanceType<typeof ElTree> | undefined>): Promise<boolean> {
  for (let i = 0; i < 10 && !tree.value; i++) {
    await nextTick()
  }
  return !!tree.value
}

async function submitForm(): Promise<void> {
  await dialog.submit(loadList, (mode) => {
    const payload: RolePayload = { ...form, menuIds: collectMenuIds() }
    if (mode === 'edit') delete payload.code // 角色标识创建后不允许修改
    return payload
  })
}

// ---------------- 分配权限 ----------------
const assignVisible = ref(false)
const assignTarget = ref<RoleItem | null>(null)
const assignTreeRef = ref<InstanceType<typeof ElTree>>()
const assignSubmitting = ref(false)

async function openAssign(row: RoleItem): Promise<void> {
  if (row.code === 'super_admin') {
    ElMessage.info('超级管理员默认拥有全部权限，无需分配')
    return
  }
  assignTarget.value = row
  assignVisible.value = true
  if (await whenTreeReady(assignTreeRef)) {
    assignTreeRef.value?.setCheckedKeys(row.menuIds.filter((id) => leafIds.value.has(id)), false)
  } else {
    ElMessage.warning('权限树尚未就绪，请重新打开弹窗')
  }
}

async function submitAssign(): Promise<void> {
  if (!assignTarget.value || !assignTreeRef.value) return
  const checked = assignTreeRef.value.getCheckedKeys(false) as number[]
  const half = assignTreeRef.value.getHalfCheckedKeys() as number[]
  assignSubmitting.value = true
  try {
    await roleApi.assignMenus(assignTarget.value.id, Array.from(new Set([...checked, ...half])))
    ElMessage.success('权限已保存')
    assignVisible.value = false
    loadList()
  } finally {
    assignSubmitting.value = false
  }
}

// ---------------- 删除 ----------------
async function removeRole(row: RoleItem): Promise<void> {
  try {
    await ElMessageBox.confirm(`确定删除角色「${row.name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await roleApi.remove(row.id)
  ElMessage.success('角色已删除')
  // 删的是末页唯一一条时先回退一页，否则会停在一个空白页上
  if (rows.value.length === 1 && query.page > 1) {
    query.page -= 1
  }
  loadList()
}

// ---------------- 勾选 / 打印 / 批量 ----------------
const tableRef = ref()
const selectedRows = ref<RoleItem[]>([])
const batchRunning = ref(false)

function handleSelectionChange(list: RoleItem[]): void {
  selectedRows.value = list
}

function clearSelection(): void {
  tableRef.value?.clearSelection()
  selectedRows.value = []
}

const printVisible = ref(false)
const printRows = computed(() => (selectedRows.value.length ? selectedRows.value : rows.value))
const printColumns: ListPrintColumn<RoleItem>[] = [
  { label: '角色名称', prop: 'name' },
  { label: '角色标识', prop: 'code' },
  { label: '数据范围', value: (r) => dataScopeLabels[r.dataScope] || '-' },
  { label: '关联用户', value: (r) => String(r.userCount), align: 'center' },
  { label: '状态', value: (r) => (r.status === 1 ? '启用' : '禁用') },
  { label: '备注', value: (r) => r.remark || '-' },
  { label: '创建时间', value: (r) => formatDate(r.createdAt) },
]
const printSubtitle = computed(() => {
  const parts: string[] = []
  if (query.keyword) parts.push(`关键字「${query.keyword}」`)
  if (selectedRows.value.length) parts.push('仅打印勾选')
  return parts.join(' · ') || '全部角色'
})

async function batchDelete(): Promise<void> {
  if (!selectedRows.value.length) return
  batchRunning.value = true
  try {
    const done = await runBatchAction(
      '批量删除',
      selectedRows.value,
      (r) => roleApi.remove(r.id),
      (r) => r.name,
    )
    if (done) {
      clearSelection()
      loadList()
    }
  } finally {
    batchRunning.value = false
  }
}

onMounted(() => {
  // 失败提示由请求层统一弹出；吞掉 rejection，避免未捕获告警
  void loadMenuTree()
    .then(() => loadList())
    .catch(() => {})
})
// 关键字录入即查询（防抖），不必再按回车
const { searchSoon } = useDebouncedSearch(search)
</script>

<template>
  <div class="role-page">
    <div class="page-card">
      <!-- 搜索与操作合并一行 -->
      <div class="list-head">
        <span class="list-head__title">角色<em>共 {{ total }} 条</em></span>
        <el-form class="filter-form" label-width="76px" @submit.prevent>
          <el-form-item label="关键字">
            <el-input
              v-model="query.keyword" @input="searchSoon"
              placeholder="角色名称 / 标识"
              clearable
              @keyup.enter="search"
            />
          </el-form-item>
        </el-form>
        <div class="list-head__actions">
          <el-button type="primary" @click="search">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="resetQuery">重置</el-button>
          <span class="list-head__divider" />
          <el-button v-if="selectedRows.length" :loading="batchRunning" @click="batchDelete">
            批量删除（{{ selectedRows.length }}）
          </el-button>
          <el-button v-perm="'system:role:export'" @click="printVisible = true">打印</el-button>
          <el-button v-perm="'system:role:create'" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>新增角色</span>
          </el-button>
          <el-button v-perm="'system:role:export'" :loading="exporting" @click="handleExport">
            <AppIcon name="Download" />
            <span>导出</span>
          </el-button>
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
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="44" />
        <el-table-column prop="name" label="角色名称" min-width="150">
          <template #default="{ row }">
            <div class="cell-role">
              <span class="cell-role__name">{{ row.name }}</span>
              <el-tag v-if="row.isBuiltin" size="small" type="warning" effect="plain">内置</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="code" label="角色标识" min-width="140">
          <template #default="{ row }">
            <code class="cell-code">{{ row.code }}</code>
          </template>
        </el-table-column>
        <el-table-column label="数据范围" width="130">
          <template #default="{ row }">{{ dataScopeLabels[row.dataScope] || '-' }}</template>
        </el-table-column>
        <el-table-column label="关联用户" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.userCount }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <StatusTag :status="row.status" disabled-text="禁用" />
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
        <el-table-column label="创建时间" width="120">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatDate(row.createdAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right" align="center">
          <template #default="{ row }">
            <el-button v-perm="'system:role:update'" link type="primary" @click="openEdit(row as RoleItem)">
              编辑
            </el-button>
            <el-button
              v-perm="'system:role:assign'"
              link
              type="primary"
              :disabled="row.code === 'super_admin'"
              @click="openAssign(row as RoleItem)"
            >
              分配权限
            </el-button>
            <el-button
              v-perm="'system:role:delete'"
              link
              type="danger"
              :disabled="row.isBuiltin || row.userCount > 0"
              @click="removeRole(row as RoleItem)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无角色数据" :image-size="90" />
        </template>
      </el-table>

      <Pagination
        v-model:page="query.page"
        v-model:page-size="query.pageSize"
        :total="total"
        @change="loadList"
      />
    </div>

    <!-- 新增 / 编辑 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增角色' : '编辑角色'"
      width="800px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="角色名称" prop="name">
              <el-input v-model="form.name" placeholder="如：系统管理员" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="角色标识" prop="code">
              <el-input
                v-model="form.code"
                :disabled="dialogMode === 'edit'"
                placeholder="如：system_admin"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="数据范围">
              <el-select v-model="form.dataScope" style="width: 100%">
                <el-option label="全部数据" :value="1" />
                <el-option label="本部门" :value="2" />
                <el-option label="本部门及以下" :value="3" />
                <el-option label="仅本人" :value="4" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="排序">
              <el-input-number v-model="form.sort" :min="0" :max="9999" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="状态">
              <el-switch
                :model-value="form.status === 1"
                :disabled="isBuiltin"
                @update:model-value="(v) => (form.status = v ? 1 : 0)"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="菜单权限">
          <div class="perm-tree">
            <el-tree
              ref="treeRef"
              :data="menuTree"
              :props="{ label: 'title', children: 'children' }"
              node-key="id"
              show-checkbox
              default-expand-all
              :check-on-click-node="false"
            />
          </div>
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

    <!-- 分配权限 -->
    <el-dialog v-model="assignVisible" title="分配菜单权限" width="640px" top="6vh" destroy-on-close>
      <el-alert
        v-if="assignTarget"
        :title="`角色：${assignTarget.name}`"
        type="info"
        :closable="false"
        class="mb-16"
      />
      <div class="perm-tree">
        <el-tree
          ref="assignTreeRef"
          :data="menuTree"
          :props="{ label: 'title', children: 'children' }"
          node-key="id"
          show-checkbox
          default-expand-all
          :check-on-click-node="false"
        />
      </div>
      <template #footer>
        <el-button @click="assignVisible = false">取消</el-button>
        <el-button type="primary" :loading="assignSubmitting" @click="submitAssign">保存</el-button>
      </template>
    </el-dialog>

    <ListPrintDialog
      v-model="printVisible"
      title="角色列表"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />
  </div>
</template>

<style scoped lang="scss">
.cell-role {
  display: flex;
  align-items: center;
  gap: 8px;

  &__name {
    font-weight: 500;
  }
}

.cell-code {
  padding: 2px 6px;
  font-size: 12px;
  color: var(--tone-slate);
  background: var(--tone-slate-soft);
  border-radius: 4px;
}

.perm-tree {
  width: 100%;
  max-height: 300px;
  padding: 10px;
  overflow-y: auto;
  border: 1px solid var(--app-border);
  border-radius: var(--radius-sm);
}
</style>
