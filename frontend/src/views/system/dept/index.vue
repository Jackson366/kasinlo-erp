<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { deptApi } from '@/api'
import type { DeptNode, DeptPayload } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import StatusTag from '@/components/StatusTag.vue'
import { formatDate, flattenTree } from '@/utils/format'
import { useLatestGuard } from '@/composables/useLatestGuard'
import { useCrudDialog } from '@/composables/useCrudDialog'

defineOptions({ name: 'SystemDept' })

// ---------------- 列表 ----------------
const loading = ref(false)
const tree = ref<DeptNode[]>([])
const keyword = ref('')

const treeGuard = useLatestGuard()

async function loadTree(): Promise<void> {
  const ticket = treeGuard.start()
  loading.value = true
  try {
    const data = await deptApi.list({ keyword: keyword.value || undefined })
    if (!treeGuard.isCurrent(ticket)) return // 连续查询时丢弃过期响应
    tree.value = data
  } finally {
    if (treeGuard.isCurrent(ticket)) loading.value = false
  }
}

// ---------------- 新增 / 编辑 ----------------
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<DeptPayload>({
  formRef,
  create: (payload) => deptApi.create(payload),
  update: (id, payload) => deptApi.update(id, payload),
  messages: { create: '部门创建成功', update: '部门已更新' },
})
// 解构保持模板里的旧名字：模板只展开 setup 顶层绑定，不会展开 dialog.visible 这类嵌套 ref
const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog

const form = reactive<DeptPayload>({
  parentId: 0,
  name: '',
  leader: '',
  phone: '',
  email: '',
  sort: 0,
  status: 1,
})

const rules: FormRules<DeptPayload> = {
  name: [{ required: true, message: '请输入部门名称', trigger: 'blur' }],
  email: [{ type: 'email', message: '邮箱格式不正确', trigger: 'blur' }],
}

/** 可选父级：排除自身及其子孙，避免形成环 */
const parentOptions = computed(() => {
  const excluded = new Set<number>()
  if (!editingId.value) return tree.value

  // inSubtree 表示「已经进入被编辑节点的子树」。
  // 它必须逐节点重算，不能把 found 作为循环累加器往下带：
  // 一旦命中，后续所有兄弟节点都会被误判成子孙而被排除
  //（部门树通常按 sort 排序，于是表现为「编辑某节点时，它后面的兄弟部门在下拉里消失了」）。
  const walk = (nodes: DeptNode[], inSubtree: boolean): void => {
    for (const node of nodes) {
      const isSelf = node.id === editingId.value
      const inside = inSubtree || isSelf
      if (inside) excluded.add(node.id)
      if (node.children?.length) walk(node.children, inside)
    }
  }
  walk(tree.value, false)

  const prune = (nodes: DeptNode[]): DeptNode[] =>
    nodes
      .filter((node) => !excluded.has(node.id))
      .map((node) => ({ ...node, children: node.children ? prune(node.children) : undefined }))
  return prune(tree.value)
})

function openCreate(parentId = 0): void {
  // 先灌表单再开窗：反序的话弹窗会先用上一轮的旧值渲染一帧
  Object.assign(form, {
    parentId,
    name: '',
    leader: '',
    phone: '',
    email: '',
    sort: 0,
    status: 1,
  })
  dialog.openCreate()
}

function openEdit(row: DeptNode): void {
  Object.assign(form, {
    parentId: row.parentId,
    name: row.name,
    leader: row.leader,
    phone: row.phone,
    email: row.email,
    sort: row.sort,
    status: row.status,
  })
  dialog.openEdit(row.id)
}

async function submitForm(): Promise<void> {
  await dialog.submit(loadTree, () => ({ ...form }))
}

async function removeDept(row: DeptNode): Promise<void> {
  try {
    await ElMessageBox.confirm(`确定删除部门「${row.name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await deptApi.remove(row.id)
  ElMessage.success('部门已删除')
  loadTree()
}

const totalCount = computed(() => flattenTree(tree.value).length)

function resetQuery(): void {
  keyword.value = ''
  loadTree()
}

onMounted(loadTree)
</script>

<template>
  <div class="dept-page">
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="keyword"
            placeholder="部门名称 / 负责人"
            clearable
            style="width: 240px"
            @keyup.enter="loadTree"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadTree">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="resetQuery">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <h3>组织架构</h3>
          <span class="text-muted">共 {{ totalCount }} 个部门</span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="'system:dept:create'" type="primary" @click="openCreate(0)">
            <AppIcon name="Plus" />
            <span>新增部门</span>
          </el-button>
          <el-button @click="loadTree">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table
        v-loading="loading"
        :data="tree"
        row-key="id"
        :tree-props="{ children: 'children' }"
        default-expand-all
      >
        <el-table-column label="部门名称" min-width="220">
          <template #default="{ row }">
            <span class="cell-title">
              <AppIcon name="OfficeBuilding" />
              <span>{{ row.name }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="leader" label="负责人" min-width="110">
          <template #default="{ row }">{{ row.leader || '-' }}</template>
        </el-table-column>
        <el-table-column prop="phone" label="联系电话" min-width="130">
          <template #default="{ row }">{{ row.phone || '-' }}</template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="180">
          <template #default="{ row }">{{ row.email || '-' }}</template>
        </el-table-column>
        <el-table-column label="在职人数" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="row.userCount > 0 ? 'primary' : 'info'" effect="plain">
              {{ row.userCount }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="70" align="center" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <StatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="120">
          <template #default="{ row }">
            <span class="text-secondary">{{ formatDate(row.createdAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right" align="center">
          <template #default="{ row }">
            <el-button v-perm="'system:dept:create'" link type="primary" @click="openCreate(row.id)">
              新增下级
            </el-button>
            <el-button v-perm="'system:dept:update'" link type="primary" @click="openEdit(row as DeptNode)">
              编辑
            </el-button>
            <el-button v-perm="'system:dept:delete'" link type="danger" @click="removeDept(row as DeptNode)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增部门' : '编辑部门'"
      width="640px"
      top="6vh"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-form-item label="上级部门">
          <el-tree-select
            v-model="form.parentId"
            :data="[{ id: 0, name: '顶级部门', children: parentOptions }]"
            :props="{ label: 'name', children: 'children' }"
            node-key="id"
            check-strictly
            default-expand-all
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="部门名称" prop="name">
          <el-input v-model="form.name" placeholder="如：技术部" />
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="负责人">
              <el-input v-model="form.leader" placeholder="选填" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="联系电话">
              <el-input v-model="form.phone" placeholder="选填" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="邮箱" prop="email">
              <el-input v-model="form.email" placeholder="选填" />
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
                @update:model-value="(v) => (form.status = v ? 1 : 0)"
              />
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
.cell-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;

  :deep(.app-icon) {
    color: var(--app-text-secondary);
  }
}
</style>
