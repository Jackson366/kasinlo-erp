---
version: alpha
name: 'Kasinlo ERP'
description: '面向连续业务处理的蓝灰色 ERP 工作界面'
colors:
  primary: '#2563eb'
  background: '#f5f7fb'
  surface: '#ffffff'
  text: '#1f2937'
typography:
  sans:
    fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif'
  mono:
    fontFamily: 'ui-monospace, monospace'
rounded:
  sm: '6px'
  md: '10px'
  lg: '14px'
omitted:
  - section: spacing
    reason: 'Existing application has no canonical spacing token scale; touched layouts document concrete grouping below.'
components:
  button:
    backgroundColor: '{colors.primary}'
    typography: '{typography.sans}'
  card:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.text}'
    rounded: '{rounded.md}'
  dialog:
    backgroundColor: '{colors.surface}'
    rounded: '{rounded.lg}'
---

# Kasinlo ERP Design

## Overview

以办公桌上的待办夹和核对单为结构参考：待办持续可见，明细就近核对，处理结果保留到用户继续工作。当前主要场景为中文桌面 ERP 高频业务处理；这是暂定用户模型，尚无真实用户研究。产品保留 Kasinlo 名称与蓝色身份，按本次请求将字母占位替换为蓝底白色几何 K 产品图标。未有日本市场业务要求的证据，不从使用者所在地推断产品市场。

采购的视觉特征是带选中边线的紧凑队列与宽核对区。避免营销式大数字、装饰渐变、无法追溯到真实数据的预警卡。

令牌权威源为 `frontend/src/styles/variables.scss`；本文件镜像已有意图与部分值，不生成另一套主题。映射：`variables.scss` → `element.scss` 的 `--el-*` 适配 → Element Plus 与业务组件的 `var(--app-*)`。字体以 `element.scss` 为准。变更主题时同时核对亮暗模式。

## Colors

品牌主色用于主要动作、链接、选中状态与键盘焦点。背景、表面、次级表面区分空间层次；次要文案用 `--app-text-secondary`。`--tone-amber*` 表达待核对及库存影响提示，`--app-success` 表达服务端确认成功。状态必须带文字，不依赖颜色。暗色全部继承同名语义令牌。

## Typography

沿用系统中文字体回退链，不下载装饰字体。采购页标题 18px/600，区标题与供应商 14–15px/600，正文 13–14px，辅助信息 12px。金额使用 tabular-nums，单号可使用系统等宽字体；中文说明保持正常字重与至少 1.6 行高。

## Layout

侧栏采用分层导航：一级栏展开宽 132px、收起宽 64px，二级栏固定宽 148px。一级菜单使用左图标、右文字，展开时图标 20px、文字 14px/500（选中 600）、行高至少 44px；收起时图标平滑缩到 18px，文字从 14px 缩到 12px，同时淡出并收窄至隐藏。图标在固定 20px 槽内以中心为锚点缩放，一级栏宽度、文字和图标共用 200ms 动效节奏。二级切换不缩放一级菜单。二级项目使用 16px 图标、13px 常规文字（选中 500）和 36px 行高，分组标题为 12px/500 次级文案，形成明确的业务分组/功能入口层次。一级栏使用 fill 表面，选中业务分组为 surface 底、细阴影与左侧标记；二级栏使用 surface 表面，当前页面采用浅蓝底平面高亮。品牌区与顶栏同高 56px，30px 产品标识与 Kasinlo 名称横排，标识不参与菜单缩放。一级收起时悬停或键盘聚焦提供名称提示，并保留可访问名称。两栏独立控制、独立记住偏好；总宽为 280/212/132/64px，两栏独立滚动。菜单与权限沿用现有来源。

现有外壳由 `.layout__content` 负责滚动。采购页不另设 100vh 或裁剪祖先。内容卡片统一 padding 16px；采购标题、视图与新增按钮桌面同排，不展示说明段落；组间 20–24px，组内 8–12px。待办卡片按内容自然高度排列，单条数据也不拉伸填满容器。日期、数量及金额在表格中不换行，列宽不足时使用表格横向滚动。待办宽 296px，中等窗口 248px，核对区填充剩余空间；800px 以下切换为队列/详情单列，保留返回路径。明细表横向滚动，录入表单保持自然高度并沿用 xl 弹窗。

## Elevation & Depth

静态区域以间距和表面区分，核对区用细分隔线。阴影沿用 `--shadow-card` / `--shadow-popup`，不为每个字段增加卡片。审核结果提示不改变操作含义。

## Shapes

沿用 6/10/14px 圆角系统，控件保留 Element Plus 的 8px 基础圆角。新增样式不写死颜色值。

## Components

按钮用 ElButton，页面级新增与当前单据审核分别属于独立决策区。打印、附件等辅助动作低强调；反审核在更多菜单中，确认时说明库存回滚及限制。队列使用原生 button，具有 hover、focus-visible、pressed、selected、disabled 状态。

列表/日期/下拉/表单使用现有 Element Plus 组件与中文 locale。异步使用现有 loading 指令；错误、无结果与业务空队列分别表达。弹窗沿用全局最大高度和内部滚动，采购编辑关闭前保护未保存输入。导航图标沿用 AppIcon 注册表。产品标识使用 `frontend/public/brand/kasinlo-mark.png`，由内置 imagegen 生成，侧栏与浏览器图标共用同一资源；固定宽高防止图片加载时跳动。生成提示：蓝底白色几何 K，由三个相连模块构成，适合 24–32px，简洁平面，无文字或场景。侧栏颜色继续直接消费 `variables.scss` 语义令牌，不增加独立配色；尊重 reduced-motion，禁用侧栏宽度、文字和图标的所有过渡。

新增工作台不使用装饰动画，状态变化直接反馈。金额、数量、日期格式沿用 `utils/format.ts`；付款到期日明确标注用途。

## Do's and Don'ts

- 从实际核对任务排列信息与操作，核心明细持续可见。
- 复用详情内容与查询状态，避免工作台和历史记录各维护一套业务逻辑。
- 不把付款日期描述成交货日期，不把未审核金额描述成已形成的应付。
- 不添加当前模型无法支持的交期、分批到货、生产影响或供应商评分。

侧栏右边缘提供箭头切换柄（距顶部 80px），切换二级栏。一级栏底部及顶栏的折叠按钮切换一级文字显隐；点击一级业务图标自动打开二级栏，保持一级栏的收起偏好。二级栏隐藏时保留内部各分组的展开状态。

## 采购入门路径

首页在经营数据之前呈现紧凑采购任务区：16px/600 标题，13px 次级说明，真实待处理数量与明确动作；指南采用 Element Plus ElSteps 表达录入、入库、付款三个环节；当前单据用 4px ElProgress 与“已完成 n / 3 个环节”表达服务端已确认的办理进度，不能把阅读指南视为业务完成。未选单据仅展示路线。沿用 page-card 与现有语义令牌；800px 以下纵向排列。新建单据说明使用次级表面，资料维护链接低于保存动作。经营数据加载不阻断独立任务入口。

指南初次呈现紧凑邀请，主色按钮“开始引导”、次级文字按钮“直接办理”；关闭时只保留带信息图标的文字入口。展开时以 14px/600 标题、12px 单号与进度、13px 环节说明建立层级，左侧品牌色边线标识辅助区。当前环节指导集中在一块次级表面，退出按钮低强调；窄屏步骤纵向排列，使用同一份步骤 DOM。指南沿用现有边框、表面、主色及圆角令牌；审核风险与保存结果使用 ElAlert，关闭教学不影响业务提示。
