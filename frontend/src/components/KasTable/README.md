# KasTable

列表组件按功能聚合，页面统一从 `@/components/KasTable` 导入。客户与供应商的
`views/inv/components/PartyPage.vue` 是首个真实调用方。

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

- 当前组件服务标准分页表格。采购工作台、卡片列表、树形表格和无分页查询暂不迁移。
- `useKasTable` 不自动首屏请求；页面决定加载时机。分页只接 `@change="load"`，
  不额外 watch 页码发请求，保留 Pagination 的同 tick 事件归并。
- 组合入口采用当前页选择：load/search/reset 前同步清空勾选。跨页保留选择的页面
  继续单独组合 query/selection hooks，并由页面在筛选变化时清空；首批组件未开放跨页保留。
- hooks 保留失败前的数据用于恢复；KasTable 在失败时隐藏旧行并显示重试。
  页面应禁用依赖旧结果的导出、打印及批量动作，新增/编辑等业务流程由页面控制。
- 列表 API 与导出共用业务筛选转换。导出默认取筛选全集，传记录时附加 IDs；
  列表打印继续使用 ListPrintDialog，有选择时打印选择，否则打印当前页。
- 批量操作在等待确认前快照目标记录，调用各记录的单条接口，允许部分成功并汇总；
  业务后果、权限、可执行记录和接口静默选项由页面/API 层负责。
- `ListPanel` 调用 `useKasTableLayout`；KasTable 复用 ListPanel，不注册第二套尺寸测量。
  `.el-table` 和 `.table-pager` 保持面板直接子节点，避免破坏原滚动与页脚布局。
- 不 import 具体业务 API/store，不从表格推断采购审核、财务结算或权限规则。

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
