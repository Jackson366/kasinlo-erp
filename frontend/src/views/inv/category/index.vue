<script setup lang="ts">
/**
 * 商品分类页：分类字典的维护入口。
 *
 * 分类从「商品表上的一列自由文本」收成字典表，解决的是两个具体问题：
 * 同一批商品被写成「钢材 / 钢材 」两个变体时报表会把它拆成两条；
 * 而那一列没有任何地方能改名、排序、停用。
 *
 * 删除有引用校验：分类下还有商品时不允许删。删掉之后那些商品的 category_id
 * 会悬空 —— 列表里显示为空、报表的品类占比凭空少一块，而且无从追溯。
 */
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { invApi } from '@/api'
import type { InvCategoryPayload, InvCategoryRow, Status } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import Pagination from '@/components/Pagination.vue'
import StatusTag from '@/components/StatusTag.vue'
import { formatDate } from '@/utils/format'
import { useCrudDialog } from '@/composables/useCrudDialog'
import { useTableQuery } from '@/composables/useTableQuery'

defineOptions({ name: 'InvCategory' })

interface CategoryQuery {
  page?: number
  pageSize?: number
  keyword?: string
  status?: Status
}

const { query, loading, rows, total, load, search, reset } = useTableQuery<
  CategoryQuery,
  InvCategoryRow
>({ keyword: '', status: undefined }, (params) =>
  invApi.categories({
    page: params.page,
    pageSize: params.pageSize,
    keyword: params.keyword || undefined,
    status: params.status ?? undefined,
  }),
)

const formRef = ref<FormInstance>()
const dialog = useCrudDialog<InvCategoryPayload>({
  formRef,
  create: (payload) => invApi.createCategory(payload),
  update: (id, payload) => invApi.updateCategory(id, payload),
  messages: { create: '新增成功', update: '修改成功' },
})
const { visible: dialogVisible, mode: dialogMode, submitting } = dialog

const form = reactive<InvCategoryPayload>({ name: '', sort: 0, status: 1, remark: '' })

const rules: FormRules<InvCategoryPayload> = {
  name: [{ required: true, message: '请输入分类名称', trigger: 'blur' }],
}

function openCreate(): void {
  Object.assign(form, { name: '', sort: 0, status: 1, remark: '' })
  dialog.openCreate()
}

function openEdit(row: InvCategoryRow): void {
  Object.assign(form, { name: row.name, sort: row.sort, status: row.status, remark: row.remark })
  dialog.openEdit(row.id)
}

async function submitForm(): Promise<void> {
  await dialog.submit(
    () => load(),
    () => ({ ...form }),
  )
}

/**
 * 删除分类。
 *
 * 引用数 > 0 时**直接拦下**，不再弹确认框让用户去撞 409：
 * 明知道后端会拒还让人点一次，等于把一次无谓的往返和一个看不懂的错误码
 * 丢给用户。正确做法是把「为什么不能删」和「该怎么做」当场说清楚。
 */
async function removeRow(row: InvCategoryRow): Promise<void> {
  if (row.productCount > 0) {
    ElMessageBox.alert(
      `「${row.name}」下还有 ${row.productCount} 个商品，删除会让这些商品的分类悬空 —— 报表品类占比也会凭空少一块。\n\n请先把商品改到其他分类，或在这里改为「停用」。`,
      '不能删除',
      { type: 'warning', confirmButtonText: '知道了' },
    ).catch(() => undefined)
    return
  }
  try {
    await ElMessageBox.confirm(`确定删除分类「${row.name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
    })
  } catch {
    return
  }
  await invApi.deleteCategory(row.id)
  ElMessage.success('删除成功')
  load()
}

onMounted(load)
</script>

<template>
  <div class="category-page">
    <div class="page-card search-bar">
      <el-form :inline="true" @submit.prevent>
        <el-form-item label="关键字">
          <el-input
            v-model="query.keyword"
            placeholder="分类名称"
            clearable
            style="width: 220px"
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
          <h3>商品分类</h3>
          <span class="text-muted">共 {{ total }} 个分类 · 商品录入时从这里选择</span>
        </div>
        <div class="toolbar__actions">
          <el-button v-perm="'inv:category:create'" type="primary" @click="openCreate">
            <AppIcon name="Plus" />
            <span>新增分类</span>
          </el-button>
          <el-button @click="load">
            <AppIcon name="Refresh" />
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="name" label="分类名称" min-width="180" show-overflow-tooltip />
        <el-table-column prop="sort" label="排序" width="90" align="center">
          <template #default="{ row }">{{ row.sort }}</template>
        </el-table-column>
        <el-table-column label="商品数" width="110" align="center">
          <template #default="{ row }">{{ row.productCount }}</template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.remark || '-' }}</template>
        </el-table-column>
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
        <el-table-column label="操作" width="140" fixed="right" align="center">
          <template #default="{ row }">
            <el-button
              v-perm="'inv:category:update'"
              link
              type="primary"
              @click="openEdit(row as InvCategoryRow)"
            >
              编辑
            </el-button>
            <el-button
              v-perm="'inv:category:delete'"
              link
              type="danger"
              @click="removeRow(row as InvCategoryRow)"
            >
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
      :title="dialogMode === 'create' ? '新增分类' : '编辑分类'"
      width="480px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="88px">
        <el-form-item label="分类名称" prop="name">
          <el-input v-model="form.name" placeholder="如 钢材 / 五金 / 包装" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch
            :model-value="form.status === 1"
            @update:model-value="(v) => (form.status = v ? 1 : 0)"
          />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="选填" />
        </el-form-item>
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
</style>
