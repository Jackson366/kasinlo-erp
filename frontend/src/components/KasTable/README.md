# KasTable

列表组件按功能聚合，页面统一从 `@/components/KasTable` 导入。已接入客户、供应商、
采购/销售单据、合同台账、出入库单据和应收应付。

## 文件职责

| 路径 | 职责 |
| --- | --- |
| `index.ts` | 对外组件、hooks 与类型出口 |
| `KasTable.vue` | 组合标题、筛选插槽、表格、失败/空态与分页 |
| `components/KasTableToolbar.vue` | 标题、条数、已选提示及清空入口 |
| `components/KasTableState.vue` | 失败重试与空结果展示 |
| `hooks/useKasTable.ts` | 当前页勾选列表的组合入口 |
| `hooks/useKasTableQuery.ts` | 分页查询与请求竞态守卫 |
| `hooks/useKasTableSelection.ts` | 勾选与表格实例同步清空 |
| `hooks/useKasTableExport.ts` | 筛选全集/选中项导出与执行状态 |
| `hooks/useKasTableBatch.ts` | 单条接口调用、确认及批量结果汇总 |
| `hooks/useKasTableLayout.ts` | 供 ListPanel 使用的唯一尺寸计算实现 |
| `types.ts` | 公共契约 |
| `__tests__/` | 查询竞态、失败恢复、勾选、导出及批量回归 |

## 页面接入

```ts
import { ref, onMounted } from 'vue'
import { KasTable, useKasTable, type KasTableHandle } from '@/components/KasTable'

const tableRef = ref<KasTableHandle>()
const {
  query, rows, total, loading, error, load, search, reset,
  selection, onSelectionChange, clearSelection, exportRows,
} = useKasTable({
  initial: { keyword: '' },
  table: () => tableRef.value,
  fetch: (params) => api.list(params), // 业务 API 返回 PageResult<Row>
  export: {
    exportUrl: '/export',
    exportParams: (params) => ({ keyword: params.keyword || undefined }),
    fallbackName: '列表.xlsx',
  },
})
onMounted(load)
```

```vue
<KasTable
  ref="tableRef"
  v-model:page="query.page"
  v-model:page-size="query.pageSize"
  title="列表"
  :rows="rows"
  :total="total"
  :loading="loading"
  :error="error"
  :selected-count="selection.length"
  @change="load"
  @retry="load"
  @selection-change="onSelectionChange"
  @clear-selection="clearSelection()"
>
  <template #filters>
    <ListFilters>
      <!-- 筛选控件及 actions/tools 插槽仍由页面编写 -->
    </ListFilters>
  </template>
  <el-table-column type="selection" width="44" />
  <el-table-column prop="name" label="名称" />
  <!-- 金额、状态、行操作、展开列继续使用原生列插槽 -->
</KasTable>
```

## 行为边界

- 标准分页列表共用表格骨架。采购通过 `body` 插槽保留 PurchaseWorkbench 的专用核对界面，
  并通过 `before` 插槽保留原位详情；库存查询、卡片列表和树形表格继续使用既有结构。
- `unit` / `resultLabel` 保留张、份、条及筛选结果等计数口径；`emptyText` / `errorText`
  由页面传入业务文案。`surface=false` 用于已有卡片内的出入库标签，`fit=false` 用于采购核对/详情。
- `before` 直接呈现详情或标签栏；`body` 可替换主体，默认仍为原生表格。两个插槽都不额外包 DOM，
  保持原位详情与 ListPanel 的尺寸契约。`showPagination` 沿用采购列表失败时隐藏分页的行为。
- `useKasTable` 不自动首屏请求；页面决定加载时机。分页只接 `@change="load"`，
  不额外 watch 页码发请求，保留 Pagination 的同 tick 事件归并。
- 组合入口采用当前页选择：load/search/reset 前同步清空勾选。跨页保留选择的页面
  继续单独组合 query/selection hooks，并由页面在筛选变化时清空；当前组件未开放跨页保留。
- hooks 保留失败前的数据用于恢复；KasTable 在失败时隐藏旧行并显示重试。
  页面应禁用依赖旧结果的导出、打印及批量动作，新增/编辑等业务流程由页面控制。
- 列表 API 与导出共用业务筛选转换。导出默认取筛选全集，传记录时附加 IDs；
  列表打印继续使用 ListPrintDialog，有选择时打印选择，否则打印当前页。
- 批量操作在等待确认前快照目标记录，调用各记录的单条接口，允许部分成功并汇总；
  业务后果、权限、可执行记录和接口静默选项由页面/API 层负责。
- `ListPanel` 调用 `useKasTableLayout`；KasTable 复用 ListPanel，不注册第二套尺寸测量。
  `.el-table` 和 `.table-pager` 保持面板直接子节点，避免破坏原滚动与页脚布局。
- 不 import 具体业务 API/store，不从表格推断采购审核、财务结算或权限规则。

## 单据页接入差异

| 页面 | 查询/状态组合 | 保留的业务行为 |
| --- | --- | --- |
| 客户/供应商 | useKasTable | CRUD 弹窗、业务权限、打印列 |
| 采购/销售 | Query + Selection + Export + Batch hooks | URL 恢复、分页回退、审核、原位详情、专用核对区 |
| 合同 | Query + Export hooks | 阶段/日期筛选转换、合同详情和付款计划 |
| 出入库单据 | useKasTable | 库存标签、来源跳转、作废校验、原单反审核 |
| 应收应付 | 原有汇总查询 + Selection / Export hooks | 同一请求票据同步更新列表与汇总、单据/合同期次限制、批量收付款接口 |

财务批量收付款仍由原事务接口执行，不套用逐条调用的通用批量 hook。
业务选择和异步状态保持单一来源，不为接入组件复制查询、表单或审核状态。

## 兼容迁移

`composables/useTableQuery.ts` 是新查询 hook 的别名出口；`composables/useListTools.ts`
组合新的 selection/export/batch hooks。旧页面和新页面都使用同一份实现，没有第二套状态机。
ListFilters、Pagination、StatusTag 和 ListPrintDialog 保留原有职责。

## 验证

```sh
cd frontend
npm run typecheck
npm test
npm run build
npm run test:list-workspace
```

列表工作区脚本使用真实 Vue 与 Mock API，验证新旧列表的几何、分页、选择清理及失败恢复，
不证明真实后端删除、审核或收付款事务正确。
