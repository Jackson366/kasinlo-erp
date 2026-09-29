<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { menuApi } from '@/api'
import type { MenuNode, MenuPayload, MenuType } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { iconMap } from '@/components/icons'
import { flattenTree } from '@/utils/format'

defineOptions({ name: 'SystemMenu' })

// ---------------- 列表 ----------------
const loading = ref(false)
const tree = ref<MenuNode[]>([])
const keyword = ref('')
const typeFilter = ref<MenuType | ''>('')

const typeMeta: Record<MenuType, { label: string; tag: 'primary' | 'success' | 'info' }> = {
  dir: { label: '目录', tag: 'info' },
  menu: { label: '菜单', tag: 'primary' },
  button: { label: '按钮', tag: 'success' },
}

async function loadTree(): Promise<void> {
  loading.value = true
  try {
    const data = await menuApi.list({ keyword: keyword.value || undefined })
    tree.value = data
  } finally {
    loading.value = false
  }
}

/** 顶层可选的父级：不能把自己或自己的子孙选为父级 */
const parentOptions = computed(() => {
  const excluded = new Set<number>()
  if (editingId.value) {
    const collect = (nodes: MenuNode[]) => {
      nodes.forEach((node) => {
        excluded.add(node.id)
        if (node.children?.length) collect(node.children)
      })
    }
    const findSelf = (nodes: MenuNode[]): MenuNode | null => {
      for (const node of nodes) {
        if (node.id === editingId.value) return node
        if (node.children?.length) {
          const found = findSelf(node.children)
          if (found) return found
        }
      }
      return null
    }
    const self = findSelf(tree.value)
    if (self) collect([self])
  }

  const prune = (nodes: MenuNode[]): MenuNode[] =>
    nodes
      .filter((node) => !excluded.has(node.id) && node.type !== 'button')
      .map((node) => ({ ...node, children: node.children ? prune(node.children) : undefined }))

  return prune(tree.value)
})

/** 按类型过滤展示，同时保留命中节点的祖先链 */
const displayTree = computed(() => {
  if (!typeFilter.value) return tree.value
  const filter = (nodes: MenuNode[]): MenuNode[] =>
    nodes.reduce<MenuNode[]>((acc, node) => {
      const children = node.children ? filter(node.children) : []
      if (node.type === typeFilter.value || children.length) {
        acc.push({ ...node, children: children.length ? children : undefined })
      }
      return acc
    }, [])
  return filter(tree.value)
})

// ---------------- 新增 / 编辑 ----------------
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const submitting = ref(false)
const formRef = ref<FormInstance>()
const editingId = ref<number | null>(null)

const form = reactive<MenuPayload>({
  parentId: 0,
  name: '',
  path: '',
  component: '',
  redirect: '',
  title: '',
  icon: '',
  type: 'menu',
  permission: '',
  sort: 0,
  hidden: false,
  keepAlive: false,
  isExternal: false,
  status: 1,
})

const rules = computed<FormRules<MenuPayload>>(() => ({
  title: [{ required: true, message: '请输入菜单标题', trigger: 'blur' }],
  name: [
    { required: true, message: '请输入路由名称', trigger: 'blur' },
    {
      pattern: /^[A-Za-z][A-Za-z0-9_]*$/,
      message: '路由名称需以字母开头，且只包含字母、数字、下划线',
      trigger: 'blur',
    },
  ],
  path: form.type === 'button' ? [] : [{ required: true, message: '请输入路由路径', trigger: 'blur' }],
  component:
    form.type === 'menu' ? [{ required: true, message: '请输入组件路径', trigger: 'blur' }] : [],
}))

const iconNames = Object.keys(iconMap)

function openCreate(parentId = 0): void {
  dialogMode.value = 'create'
  editingId.value = null
  Object.assign(form, {
    parentId,
    name: '',
    path: '',
    component: '',
    redirect: '',
    title: '',
    icon: '',
    type: 'menu' as MenuType,
    permission: '',
    sort: 0,
    hidden: false,
    keepAlive: false,
    isExternal: false,
    status: 1,
  })
  dialogVisible.value = true
}

function openEdit(row: MenuNode): void {
  dialogMode.value = 'edit'
  editingId.value = row.id
  Object.assign(form, {
    parentId: row.parentId,
    name: row.name,
    path: row.path,
    component: row.component,
    redirect: row.redirect,
    title: row.title,
    icon: row.icon,
    type: row.type,
    permission: row.permission,
    sort: row.sort,
    hidden: row.hidden,
    keepAlive: row.keepAlive,
    isExternal: row.isExternal,
    status: row.status,
  })
  dialogVisible.value = true
}

async function submitForm(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    if (dialogMode.value === 'create') {
      await menuApi.create({ ...form })
      ElMessage.success('菜单创建成功')
    } else if (editingId.value) {
      await menuApi.update(editingId.value, { ...form })
      ElMessage.success('菜单已更新')
    }
    dialogVisible.value = false
    loadTree()
  } finally {
    submitting.value = false
  }
}

async function removeMenu(row: MenuNode): Promise<void> {
  try {
    await ElMessageBox.confirm(`确定删除「${row.title}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await menuApi.remove(row.id)
  ElMessage.success('菜单已删除')
  loadTree()
}

/** 统计各类型数量，用于顶部概览 */
const stats = computed(() => {
  const flat = flattenTree(tree.value)
  return {
    total: flat.length,
    dir: flat.filter((item) => item.type === 'dir').length,
    menu: flat.filter((item) => item.type === 'menu').length,
    button: flat.filter((item) => item.type === 'button').length,
  }
})

onMounted(loadTree)
</script>

<template>
  <div class="menu-page">
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="keyword"
            placeholder="标题 / 路由名称 / 权限标识"
            clearable
            style="width: 240px"
            @keyup.enter="loadTree"
          />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="typeFilter" placeholder="全部" clearable style="width: 120px">
            <el-option label="目录" value="dir" />
            <el-option label="菜单" value="menu" />
            <el-option label="按钮" value="button" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadTree">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button
            @click="
              () => {
                keyword = ''
                typeFilter = ''
                loadTree()
              }
            "
          >
            重置
          </el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="page-card">
      <div class="toolbar">
        <div class="toolbar__title">
          <h3>菜单结构</h3>
          <span class="text-muted">
            共 {{ stats.total }} 项（目录 {{ stats.dir }} · 菜单 {{ stats.menu }} · 按钮 {{ stats.button }}）
          </span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="'system:menu:create'" type="primary" @click="openCreate(0)">
            <AppIcon name="Plus" />
            <span>新增菜单</span>
          </el-button>
          <el-button @click="loadTree">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table
        v-loading="loading"
        :data="displayTree"
        row-key="id"
        :tree-props="{ children: 'children' }"
        default-expand-all
      >
        <el-table-column label="菜单标题" min-width="220">
          <template #default="{ row }">
            <span class="cell-title">
              <AppIcon :name="row.icon" />
              <span>{{ row.title }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="typeMeta[row.type as MenuType].tag" size="small" effect="light">
              {{ typeMeta[row.type as MenuType].label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="路由名称" min-width="150">
          <template #default="{ row }">
            <code v-if="row.name" class="cell-code">{{ row.name }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路由路径" min-width="140" />
        <el-table-column prop="component" label="组件路径" min-width="170">
          <template #default="{ row }">
            <span class="text-secondary">{{ row.component || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="permission" label="权限标识" min-width="180">
          <template #default="{ row }">
            <code v-if="row.permission" class="cell-code cell-code--perm">{{ row.permission }}</code>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="70" align="center" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="light">
              {{ row.status === 1 ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170" fixed="right" align="center">
          <template #default="{ row }">
            <el-button
              v-if="row.type !== 'button'"
              v-perm="'system:menu:create'"
              link
              type="primary"
              @click="openCreate(row.id)"
            >
              新增子项
            </el-button>
            <el-button v-perm="'system:menu:update'" link type="primary" @click="openEdit(row as MenuNode)">
              编辑
            </el-button>
            <el-button v-perm="'system:menu:delete'" link type="danger" @click="removeMenu(row as MenuNode)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 新增 / 编辑 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogMode === 'create' ? '新增菜单' : '编辑菜单'"
      width="640px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
        <el-form-item label="菜单类型">
          <el-radio-group v-model="form.type">
            <el-radio-button value="dir">目录</el-radio-button>
            <el-radio-button value="menu">菜单</el-radio-button>
            <el-radio-button value="button">按钮</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="上级菜单">
              <el-tree-select
                v-model="form.parentId"
                :data="[{ id: 0, title: '根目录', children: parentOptions }]"
                :props="{ label: 'title', children: 'children' }"
                node-key="id"
                check-strictly
                default-expand-all
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="菜单标题" prop="title">
              <el-input v-model="form.title" placeholder="显示在菜单上" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="路由名称" prop="name">
              <el-input v-model="form.name" placeholder="全局唯一，如 SystemUser" />
            </el-form-item>
          </el-col>
          <el-col v-if="form.type !== 'button'" :span="12">
            <el-form-item label="路由路径" prop="path">
              <el-input v-model="form.path" placeholder="以 / 开头，如 /system/user" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row v-if="form.type === 'menu'" :gutter="16">
          <el-col :span="12">
            <el-form-item label="组件路径" prop="component">
              <el-input v-model="form.component" placeholder="如 system/user/index" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="菜单图标">
              <el-select v-model="form.icon" placeholder="可选" clearable style="width: 100%">
                <el-option v-for="name in iconNames" :key="name" :label="name" :value="name">
                  <span class="icon-option">
                    <AppIcon :name="name" :fallback="false" />
                    <span>{{ name }}</span>
                  </span>
                </el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row v-if="form.type === 'dir'" :gutter="16">
          <el-col :span="12">
            <el-form-item label="重定向">
              <el-input v-model="form.redirect" placeholder="可选，如 /system/user" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="菜单图标">
              <el-select v-model="form.icon" placeholder="可选" clearable style="width: 100%">
                <el-option v-for="name in iconNames" :key="name" :label="name" :value="name">
                  <span class="icon-option">
                    <AppIcon :name="name" :fallback="false" />
                    <span>{{ name }}</span>
                  </span>
                </el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="权限标识">
              <el-input v-model="form.permission" placeholder="如 system:user:create" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="排序">
              <el-input-number v-model="form.sort" :min="0" :max="9999" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="隐藏菜单">
              <el-switch v-model="form.hidden" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="页面缓存">
              <el-switch v-model="form.keepAlive" :disabled="form.type !== 'menu'" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
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

.cell-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;

  :deep(.app-icon) {
    color: var(--app-text-secondary);
  }
}

.cell-code {
  padding: 1px 6px;
  font-size: 12px;
  color: var(--tone-slate);
  background: var(--tone-slate-soft);
  border-radius: 4px;

  &--perm {
    color: var(--tone-blue);
    background: var(--tone-blue-soft);
  }
}

.icon-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
</style>
