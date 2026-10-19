<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import type { FormInstance, FormRules } from 'element-plus'
import { menuApi } from '@/api'
import type { MenuNode, MenuPayload, MenuType } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import ListPrintDialog, { type ListPrintColumn } from '@/components/ListPrintDialog.vue'
import StatusTag from '@/components/StatusTag.vue'
import { iconMap } from '@/components/icons'
import { flattenTree } from '@/utils/format'
import { downloadFile } from '@/utils/download'
import { availableViewPaths } from '@/router/helper'
import { useLatestGuard } from '@/composables/useLatestGuard'
import { useCrudDialog } from '@/composables/useCrudDialog'

defineOptions({ name: 'SystemMenu' })

// ---------------- 列表 ----------------
const loading = ref(false)
const tree = ref<MenuNode[]>([])
const keyword = ref('')
const typeFilter = ref<MenuType | ''>('')

const treeGuard = useLatestGuard()

const typeMeta: Record<MenuType, { label: string; tag: 'primary' | 'success' | 'info' }> = {
  dir: { label: '目录', tag: 'info' },
  menu: { label: '菜单', tag: 'primary' },
  button: { label: '按钮', tag: 'success' },
}

async function loadTree(): Promise<void> {
  const ticket = treeGuard.start()
  loading.value = true
  try {
    const data = await menuApi.list({ keyword: keyword.value || undefined })
    if (!treeGuard.isCurrent(ticket)) return // 连续查询时丢弃过期响应
    tree.value = data
  } finally {
    if (treeGuard.isCurrent(ticket)) loading.value = false
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

/** 按 id 在菜单树里找节点（用于取父级的权限码与同级排序） */
function findNode(nodes: MenuNode[], id: number): MenuNode | null {
  for (const node of nodes) {
    if (node.id === id) return node
    const found = node.children?.length ? findNode(node.children, id) : null
    if (found) return found
  }
  return null
}

// ---------------- 新增 / 编辑 ----------------
const formRef = ref<FormInstance>()
const dialog = useCrudDialog<MenuPayload>({
  formRef,
  create: (payload) => menuApi.create(payload),
  update: (id, payload) => menuApi.update(id, payload),
  messages: { create: '菜单创建成功', update: '菜单已更新' },
})
// 解构保持模板里的旧名字：模板只展开 setup 顶层绑定，不会展开 dialog.visible 这类嵌套 ref
const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog

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
    form.type === 'menu' ? [{ required: true, message: '请选择组件路径', trigger: 'change' }] : [],
}))

const iconNames = Object.keys(iconMap)

// ---------------- 表单辅助：把易错的手输项换成可选项 ----------------

/** 可挂载的页面组件，与动态路由共用同一份 glob，选得到就一定加载得到 */
const componentOptions = availableViewPaths()

/** 常用动作词。用下拉而非自由输入，避免同义不同码（list / query / search 混用） */
const ACTION_OPTIONS = [
  'list',
  'create',
  'update',
  'delete',
  'export',
  'import',
  'reset',
  'assign',
  'view',
]

/** 已有权限码里出现过的模块名，供下拉复用；下拉支持直接输入新的模块名 */
const namespaceOptions = computed(() => {
  const set = new Set<string>()
  flattenTree(tree.value).forEach((node) => {
    const ns = node.permission.split(':')[0]
    if (ns) set.add(ns)
  })
  return [...set].sort()
})

/**
 * 权限码拆成「模块 : 资源 : 动作」三段分别填写。
 *
 * 权限码的惯例就是三段式，而同一页面下的按钮共享前两段。拆开之后，
 * 用户改父级菜单时模块段与资源段自动跟随，只有动作需要选一次 ——
 * 既少打字，也从根上杜绝 system:user:create 被敲成 system:urser:create
 * 那种「菜单点得进去、接口 403」的排查噩梦。
 */
const permNs = ref('')
const permRes = ref('')
const permAction = ref('')

/** 三段拼回权限码。空段直接丢弃，于是 inv + '' + customer:create 还原为 inv:customer:create */
function syncPermission(): void {
  form.permission = [permNs.value, permRes.value, permAction.value]
    .map((part) => part.trim())
    .filter(Boolean)
    .join(':')
}
watch([permNs, permRes, permAction], syncPermission)

/** 反向解析已有的权限码。两段式（如 inv:overview:view）按「模块 + 动作」还原 */
function parsePermission(permission: string): void {
  const parts = permission.split(':').filter(Boolean)
  if (parts.length >= 3) {
    permNs.value = parts[0]
    permRes.value = parts[1]
    permAction.value = parts.slice(2).join(':')
  } else if (parts.length === 2) {
    permNs.value = parts[0]
    permRes.value = ''
    permAction.value = parts[1]
  } else {
    permNs.value = parts[0] ?? ''
    permRes.value = ''
    permAction.value = ''
  }
}

/**
 * 从父级菜单继承权限码的前两段。
 *
 * 按钮一定挂在某个页面上，模块段与资源段必定与父级一致；菜单则只继承模块段，
 * 资源段留给用户按页面自身命名。父级是目录时没有权限码可继承，保持原样。
 */
function inheritFromParent(parentId: number): void {
  const parts = (findNode(tree.value, parentId)?.permission ?? '').split(':').filter(Boolean)
  if (!parts.length) return
  permNs.value = parts[0]
  if (form.type === 'button' && parts.length >= 2) permRes.value = parts[1]
}

/** 同级排序取「当前最大值 + 1」，新增的菜单默认排到末尾，不用手填 */
function nextSort(parentId: number): number {
  const siblings = parentId ? (findNode(tree.value, parentId)?.children ?? []) : tree.value
  return siblings.reduce((max, node) => Math.max(max, node.sort), 0) + 1
}

/** 由路径推导路由名：/system/user → SystemUser */
function deriveName(path: string): string {
  return path
    .split('/')
    .filter(Boolean)
    .flatMap((seg) => seg.split(/[-_]/))
    .filter(Boolean)
    .map((seg) => seg.charAt(0).toUpperCase() + seg.slice(1))
    .join('')
}

/** 用户一旦手改过路由名就停止联动，避免「改完又被自动改回去」 */
const nameTouched = ref(false)

watch(
  () => form.path,
  (path) => {
    // 编辑已有菜单时不覆盖：它的路由名可能已被外部引用
    if (dialogMode.value !== 'create' || nameTouched.value) return
    // 按钮类型不带路径，推导结果会是空串，把用户手填的名字抹掉
    if (!path) return
    form.name = deriveName(path)
  },
)

// 父级变化时重新继承前两段。编辑态不参与 —— 否则会把已保存的权限码冲掉。
watch(
  () => form.parentId,
  (parentId) => {
    if (dialogMode.value !== 'create') return
    inheritFromParent(parentId)
  },
)

function openCreate(parentId = 0): void {
  // 先置模式再灌表单（useCrudDialog 的约定）。上面的 path / parentId 两个联动
  // watcher 都按 mode 分流，它们在本轮 flush 时才执行 —— mode 必须先就位。
  dialog.openCreate()
  nameTouched.value = false
  permNs.value = ''
  permRes.value = ''
  permAction.value = ''
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
    sort: nextSort(parentId),
    hidden: false,
    keepAlive: false,
    isExternal: false,
    status: 1,
  })
  // 显式继承一次：父级与上次相同时 watch 不会触发
  inheritFromParent(parentId)
}

function openEdit(row: MenuNode): void {
  // 同上：mode 若不是 'edit'，路径联动会把已保存的路由名改写成推导值
  dialog.openEdit(row.id)
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
  parsePermission(row.permission)
}

async function submitForm(): Promise<void> {
  await dialog.submit(loadTree, () => ({ ...form }))
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

function resetQuery(): void {
  keyword.value = ''
  typeFilter.value = ''
  loadTree()
}

// 类型切换会换掉 path/component 的校验规则（button 两者都不要）：不清掉旧报错，
// 用户会看到针对已隐藏字段的红字，submit 也会被陈旧的 validate 状态挡住。
// 顺带重新继承一次权限码：切到按钮时要连资源段一起补上，否则从菜单改按钮会缺一段。
watch(
  () => form.type,
  () => {
    formRef.value?.clearValidate()
    if (dialogMode.value === 'create') inheritFromParent(form.parentId)
  },
)

// ---------------- 导出 / 打印 ----------------
const printVisible = ref(false)
const exporting = ref(false)

/** 摊平后的打印行：与导出文件同一口径（按层级缩进） */
const flatRows = computed(() => {
  const out: Array<{ row: MenuNode; depth: number }> = []
  const walk = (list: MenuNode[], depth: number) => {
    for (const n of list) {
      out.push({ row: n, depth })
      if (n.children?.length) walk(n.children, depth + 1)
    }
  }
  walk(displayTree.value, 0)
  return out
})

const printRows = computed(() => flatRows.value.map((x) => x.row))
const printColumns: ListPrintColumn<MenuNode>[] = [
  { label: '名称', value: (r) => '　'.repeat(flatRows.value.find((x) => x.row.id === r.id)?.depth ?? 0) + r.title },
  { label: '类型', value: (r) => typeMeta[r.type]?.label || r.type },
  { label: '路由路径', value: (r) => r.path || '-' },
  { label: '权限标识', value: (r) => r.permission || '-' },
  { label: '排序', value: (r) => String(r.sort), align: 'center' },
]
const printSubtitle = computed(() => {
  const parts: string[] = []
  if (keyword.value) parts.push(`关键字「${keyword.value}」`)
  if (typeFilter.value) parts.push(typeMeta[typeFilter.value]?.label || '')
  return parts.filter(Boolean).join(' · ') || '全部菜单'
})

async function handleExport(): Promise<void> {
  if (exporting.value) return
  exporting.value = true
  try {
    await downloadFile('/system/menus/export', {
      keyword: keyword.value || undefined,
      type: typeFilter.value || undefined,
    }, '菜单配置.xlsx')
  } catch {
    // 失败提示由下载封装 / 响应拦截器统一处理；这里只保证不产生未捕获 rejection
  } finally {
    exporting.value = false
  }
}

onMounted(loadTree)
</script>

<template>
  <div class="menu-page">
    <div class="page-card search-bar">
      <el-form class="filter-form" label-width="76px" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="keyword"
            placeholder="标题 / 路由名称 / 权限标识"
            clearable
            @keyup.enter="loadTree"
          />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="typeFilter" placeholder="全部" clearable>
            <el-option label="目录" value="dir" />
            <el-option label="菜单" value="menu" />
            <el-option label="按钮" value="button" />
          </el-select>
        </el-form-item>
        <div class="filter-form__actions">
          <el-button type="primary" @click="loadTree">
            <AppIcon name="Search" />
            <span>查询</span>
          </el-button>
          <el-button @click="resetQuery">重置</el-button>
        </div>
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
          <el-button :loading="exporting" @click="handleExport">导出</el-button>
          <el-button @click="printVisible = true">打印</el-button>
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
            <StatusTag :status="row.status" />
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
      width="800px"
      top="6vh"
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
          <el-col v-if="form.type !== 'button'" :span="12">
            <el-form-item label="路由路径" prop="path">
              <el-input v-model="form.path" placeholder="以 / 开头，如 /system/user" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="路由名称" prop="name">
              <el-input
                v-model="form.name"
                placeholder="默认由路径推导，可改"
                @input="nameTouched = true"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row v-if="form.type === 'menu'" :gutter="16">
          <el-col :span="12">
            <el-form-item label="组件路径" prop="component">
              <el-select
                v-model="form.component"
                filterable
                clearable
                placeholder="选择页面文件"
                style="width: 100%"
              >
                <el-option
                  v-for="path in componentOptions"
                  :key="path"
                  :label="path"
                  :value="path"
                />
              </el-select>
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

        <el-form-item label="权限标识">
          <div class="perm">
            <el-select
              v-model="permNs"
              class="perm__ns"
              filterable
              allow-create
              default-first-option
              placeholder="模块"
            >
              <el-option v-for="ns in namespaceOptions" :key="ns" :label="ns" :value="ns" />
            </el-select>
            <span class="perm__sep">:</span>
            <el-input v-model="permRes" class="perm__res" placeholder="资源，如 user" />
            <span class="perm__sep">:</span>
            <el-select
              v-model="permAction"
              class="perm__action"
              filterable
              allow-create
              default-first-option
              placeholder="动作"
            >
              <el-option
                v-for="action in ACTION_OPTIONS"
                :key="action"
                :label="action"
                :value="action"
              />
            </el-select>
          </div>
          <div class="perm__preview">
            生成：<code>{{ form.permission || '（留空表示不校验权限）' }}</code>
          </div>
        </el-form-item>

        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" :max="9999" controls-position="right" />
          <span class="form-hint">默认排到同级末尾</span>
        </el-form-item>

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

    <ListPrintDialog
      v-model="printVisible"
      title="菜单结构"
      :subtitle="printSubtitle"
      :columns="printColumns"
      :rows="printRows"
    />
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

// 权限码三段式：模块 : 资源 : 动作。三格共行，分隔符由自己画，
// 免得用户以为要在输入框里手打冒号。
.perm {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;

  &__ns {
    flex: 0 0 128px;
  }

  &__res {
    flex: 1 1 auto;
  }

  &__action {
    flex: 0 0 132px;
  }

  &__sep {
    flex: 0 0 auto;
    color: var(--app-text-muted);
  }

  &__preview {
    width: 100%;
    margin-top: 2px;
    font-size: 12px;
    line-height: 1.6;
    color: var(--app-text-muted);

    code {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      color: var(--tone-blue);
    }
  }
}

.form-hint {
  margin-left: 10px;
  font-size: 12px;
  color: var(--app-text-muted);
}
</style>
