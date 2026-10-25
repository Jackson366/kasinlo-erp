# Kasinlo ERP UI behavior

本契约记录本次采购工作台的交互决定；视觉意图见 [DESIGN.md](DESIGN.md)。既有其他页面按现有业务契约运行，本文不宣称它们全部通过新的交互验收。

## Business sources

| Concern            | Authoritative source                      | UI consequence                                          |
| ------------------ | ----------------------------------------- | ------------------------------------------------------- |
| 审核与反审核       | docs/API.md §11.3、docs/GUIDE.md 采购管理 | 审核即入库；反审核回滚库存，已有付款时不可反审核        |
| 付款期限           | docs/API.md §11.3 dueDate                 | 写“付款到期日”；未填写显示未设付款期限                  |
| 权限               | AGENTS.md §3、§5.74；现有 inv 路由        | v-perm 控制显隐，服务端保持最终鉴权                     |
| 列表导出/打印/批量 | AGENTS.md §5.98                           | 导出筛选全集；打印选中或当前页；批量逐项执行            |
| 用户模型和结构     | 当前采购界面评审与修改请求                | 暂定高频经办人员为主要用户；待处理/全部单据两种活动视图 |

## Canonical UI Map

| Capability      | Canonical owner                    | Source of truth              | Allowed variants                                   | Verification                          |
| --------------- | ---------------------------------- | ---------------------------- | -------------------------------------------------- | ------------------------------------- |
| Table Selection | useListTools + ElTable             | AGENTS.md §5.45 / §5.98      | 当前页勾选；筛选或数据集合变化清空                 | scripts/purchase-workbench-verify.mjs |
| Select/Listbox  | Element Plus ElSelect              | Existing shared stack        | authored + 中文 locale                             | 同上，真实下拉打开与选择              |
| Date            | Element Plus ElDatePicker          | docs/API.md 日期闭区间       | authored，date / daterange                         | 同上，打开中文日历                    |
| Form            | OrderPage 现有录入状态 + ElForm    | docs/API.md §11.3            | 新建/编辑；失败保留输入；关闭前提示未保存          | 同上                                  |
| Scrollbar       | styles/index.scss                  | DESIGN.md / variables.scss   | 全局标准属性 + WebKit 回退，forced-colors 跟随系统 | 同上，样式与窄屏检查                  |
| Toast           | Element Plus ElMessage             | Existing request interceptor | 服务端成功后反馈；关键错误另有持续文案             | 同上                                  |
| CRUD            | invApi + useTableQuery + OrderPage | docs/API.md §11.3            | 采购连续核对 / 历史列表 / 既有销售列表             | 同上                                  |

## Procurement flow

默认待处理视图，状态过滤为 1。待办与历史共用服务端分页、查询和详情状态。列表页不以业务员自由文本匹配登录用户。已提交搜索、视图、状态、日期、页码和每页条数写入 URL，刷新可恢复。重置清空筛选；切换视图清空不适用状态和选择。

详情请求使用序号守卫，迟到响应不能覆盖后选单据。选择新的单据时不保留上一张的可执行内容。失败提供显式重载入口。列表失败不伪装成空队列；待办为空和筛选无结果使用不同文案。

审核前确认库存影响；等待服务端响应后才反馈成功，处理中阻止关闭确认窗与重复提交。成功后保留单据结果，再由用户选择处理下一张。服务端状态重新读取失败时提供重试，不继续显示旧状态的审核按钮。不存在收货确认的新业务状态。

新建/编辑继续使用共享 xl 弹窗；常规保存不加二次确认。失败保留输入；取消/关闭/离开页面前，对未保存修改提供放弃或继续编辑。浏览器刷新采用 beforeunload 机制。无持久化本地业务草稿。

采购详情与待办核对共用 OrderDetailContent。未审核不显示已形成的应付余额；单据状态与付款状态分开。附件和打印在详情可访问；列表更多菜单提供辅助动作。反审核保留现有确认及服务端校验。

窄屏由队列进入详情，保留返回按钮、选中单据和键盘焦点；表格横向滚动保留列关系，不裁掉金额或动作。所有新增操作使用语义按钮或已有组件。

## Verification boundary

采购浏览器脚本使用确定性的模拟 API，验证实际 Vue 页面、路由、权限显隐、表单与请求状态。它不证明真实后端的事务、入库或付款正确；这些继续由后端业务测试负责。静态审计对 TagsView 的 ElDropdown 触发按钮有既有误报：动作由父级组件处理，不能为让扫描通过而添加空点击处理。

## Sidebar navigation

主导航直接使用服务端已授权菜单树，沿用路由、权限过滤及外链行为。一级栏与二级栏分别由 app store 中的 `primarySidebarCollapsed` 与 `sidebarCollapsed` 控制，分别持久化到 `nova_sidebar_primary_collapsed` 与原有 `nova_sidebar_collapsed` 键。

一级栏展开时左图标右文字，宽 132px；收起时仅图标，宽 64px，图标及产品标识保持尺寸与横向位置。一级栏底部按钮与顶栏折叠按钮均只切换一级栏文字。图标模式的每一项保留 aria-label，并在悬停及键盘聚焦时提供 ElTooltip 名称提示。聚焦及当前状态可辨认。

右边缘箭头继续只切换二级栏，二级栏固定宽 148px；切换二级栏不改变一级栏尺寸。两栏都展开为 280px，一级仅图标且二级展开为 212px，仅一级文字栏为 132px，两栏都收起为 64px。没有二级菜单时只占当前一级栏宽度。内容区与侧栏从同一组尺寸令牌计算，不独立写死宽度。

点击一级分组自动打开二级栏，保留一级图标模式；选择分组用于浏览菜单，不自动跳到第一个子页。路由变化恢复对应分组高亮。二级栏隐藏时保留内部各分组的展开状态；两级折叠切换不改变字号、图标尺寸或 scale，减少动态效果模式取消宽度过渡。
