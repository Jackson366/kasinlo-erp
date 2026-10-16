# Nova ERP 项目说明与开发指南

> 本文是**新接手开发者**的第一份读物：先讲清这个项目是什么、怎么跑起来、怎么用，
> 再给出「新增一个业务模块」的完整标准流程。
>
> 文档分工：
> - **本文** —— 项目说明、使用步骤、新增业务的全流程
> - [`README.md`](../README.md) —— 快速开始与目录速览
> - [`AGENTS.md`](../AGENTS.md) —— 改代码前必读的约定与 65 个已知坑
> - [`docs/API.md`](API.md) —— 接口清单与响应约定
> - [`docs/DATABASE.md`](DATABASE.md) —— 表结构与索引设计
> - [`docs/UPGRADE.md`](UPGRADE.md) —— 后台系统独立于业务的升级方案（目录契约、迁移分段、升级 SOP、回滚）
> - [`docs/CHANGELOG.md`](CHANGELOG.md) —— 版本变更明细（发布前必写）

---

# 第一部分　项目说明

## 1.1 这是什么

Nova ERP 是一套**简洁够用的后台管理系统骨架**：Go + Vue 3 + PostgreSQL。

它的定位不是「大而全的低代码平台」，而是**一个能长期自己维护的中后台底座**：
把用户、角色、菜单、部门、日志、导入导出这些每个后台都要的东西做扎实，
并且把目录结构与命名约定设计成「AI 也能照着加功能」的形态。

三条设计取向，理解了它们才能理解后面的所有取舍：

| 取向 | 具体表现 |
| --- | --- |
| **模块化** | 业务代码只放 `internal/module/<模块>/`，固定「四实现文件 + 一份 `module.go` 清单」。加功能 = 加目录 + 组装层加一行，不改框架文件 |
| **约定优于配置** | 菜单即权限即路由。在 `sys_menus` 里加一条记录，前端路由与侧边栏自动出现 |
| **安全默认** | 鉴权在后端、越权按 404、审计不可被客户端搞挂、改密即吊销令牌 —— 都做成了默认行为而不是可选项 |

## 1.2 技术栈

技术选型只有两条硬约束：**只用 MIT / Apache-2.0 / BSD-3-Clause 这类宽松许可**，以及**尽量少的运行时组件**。
基础设施只需要 PostgreSQL 与 Node.js 两项，没有 Redis、消息队列这类额外中间件。下表按后端、前端、基础设施三块列出具体版本。

### 后端

| 组件 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.25+ | 语言（`go.mod` 声明 `go 1.25.0`） |
| Gin | v1.10.1 | HTTP 框架 |
| GORM | v1.31.0 | ORM |
| pgx | v5.6.0 | PostgreSQL 驱动（`gorm.io/driver/postgres` v1.6.0） |
| golang-jwt/v5 | v5.3.0 | 双令牌签发与解析 |
| validator/v10 | v10.28.0 | 参数校验 |
| excelize/v2 | v2.11.0 | Excel 导入导出 |
| zap | v1.27.0 | 结构化日志 |
| golang.org/x/crypto | v0.53.0 | bcrypt 口令哈希 |

### 前端

| 组件 | 版本 | 用途 |
| --- | --- | --- |
| Vue | ^3.5.44 | 框架（组合式 API + `<script setup>`） |
| Vite | ^6.4.4 | 构建与开发服务器 |
| TypeScript | ^5.7.3 | 类型（`vue-tsc` 严格检查） |
| Element Plus | ^2.14.7 | UI 组件库（**按需导入**，见 §3.8.4） |
| unplugin-vue-components | ^32.1.0 | 按需导入 `el-*` 组件与样式 |
| Pinia | ^3.0.1 | 状态管理（组合式 setup store） |
| vue-router | ^4.5.0 | 路由（**hash 模式**） |
| axios | ^1.7.9 | HTTP |
| ECharts | ^6.1.0 | 概览与报表图表（6.x 修了 XSS） |
| sass | ^1.105.2 | 样式预处理 |

### 基础设施

| 组件 | 版本 |
| --- | --- |
| PostgreSQL | 13+（实测 17.6） |
| Node.js | 18+（实测 v22） |

## 1.3 一次请求的完整链路

理解这条链路，就理解了后端的分层为什么要这么切。

```
浏览器
  │  Authorization: Bearer <accessToken>
  ▼
┌──────────────────────────────────────────────────────────┐
│ Gin 引擎（中间件按注册顺序执行，洋葱模型）                 │
│                                                          │
│  1. TraceID          注入 X-Trace-Id（缺失则生成 16 位）   │
│  2. Logger           访问日志，/health 跳过                │
│  3. OperationLog     ★ 写操作审计（引擎级，在 Recovery 外） │
│  4. Recovery         panic → 500 统一响应体                │
│  5. CORS             跨域；配 * 时不带 Allow-Credentials   │
│  ──────────────── 路由匹配 ────────────────                │
│  6. Auth             解析令牌 → authz.Load → 校验 token_epoch│
│  7. RequirePerm      校验权限码 HasAnyPerm                  │
│  8. Handler          bind 入参 → 调 service → response 出参 │
│                                                          │
│  service 层：业务逻辑 + 数据范围（authz.ResolveScope，见 §1.4）│
│  model 层：GORM，AutoMigrate 建表                          │
└──────────────────────────────────────────────────────────┘
  │
  ▼  { code, message, data, traceId }
```

几个关键点：

- **`OperationLog` 在 `Recovery` 外层，且注册在引擎级。** 它靠 `c.Next()` 返回后读状态码落库；
  放在里层的话，panic 产生的 500 与未匹配路由的请求都不会留痕。
- **`Auth` 不只验签名，还比对 `token_epoch`。** 这是「改密码后旧令牌立即失效」的实现基础（§3.4）。
- **审计是异步的、可丢弃的。** 写日志走独立通道，写不进去只留 WARN，绝不拖慢业务请求。

## 1.4 三层权限模型（最容易搞混，务必读）

新人最常犯的错，是把「功能权限」当成全部。本项目实际有**三套互不替代**的机制：

| 机制 | 存在哪 | 回答的问题 | 谁负责拦截 |
| --- | --- | --- | --- |
| **功能权限** | `sys_menus.permission` + `sys_role_menus` | 「能不能用这个功能」 | 后端 `RequirePerm` + 前端 `v-perm` |
| **数据范围** | `sys_roles.data_scope` | 「能看到 / 改到**谁**的数据」 | 后端 `authz.ResolveScope`（唯一实现，见 §1.4）|
| **令牌有效性** | `sys_users.token_epoch` | 「这个令牌还算数吗」 | 后端 `Auth` 中间件 |

### 功能权限

- 权限码格式固定为 **`模块:资源:动作`**，例如 `system:user:create`、`system:log:export`。
- 写在 `route.go` 的 `middleware.RequirePerm(...)` 上，**支持传多个码，语义是「任一命中即可」**。
- `super_admin` 角色一律放行，不走权限集合判断。
- 前端 `v-perm` 指令只负责「别让人误点」，**真正的拦截在后端**，隐藏按钮永远不算鉴权。

### 数据范围

四个取值，多角色时**取最宽松者**：

| 值 | 含义 | 生效方式 |
| --- | --- | --- |
| `1` | 全部数据 | 不加额外条件 |
| `2` | 本部门 | `dept_id = 当前用户部门` |
| `3` | 本部门及以下 | `dept_id IN (部门子树)` |
| `4` | 仅本人 | `id = 当前用户`（用户表场景） |

两条必须记住的规则：

1. **读写都要收窄。** 列表、详情、下拉、导出、以及**增删改**都必须过范围检查。
   只挡读不挡写，等于给了越权改数据的口子。
2. **越界按 404 处理，不要返回 403。** 403 等于告诉对方「这条记录存在，只是你没权限」，
   会泄露数据存在性。（写操作例外：改 / 删超出范围的部门返回 403，因为那里 403 才是准确的语义——
   部门树已经按范围裁剪过了，调用方本来就知道自己有哪些部门。）

**范围判定的唯一实现在 `backend/internal/authz/scope.go`**，不要在模块里另写一份：

```go
scope := authz.ResolveScope(actor)

scope.ApplyUser(tx)          // 叠在 sys_users 查询上：WHERE dept_id IN (...)
scope.ApplyLog(tx)           // 叠在带 user_id 的日志表上
scope.CreatorCond("")        // 叠在带 created_by 的进销存单据上（含别名形式）
scope.CreatorVisible(uid)    // 写操作前判断单据创建人是否可见（越界按 404）
scope.AllowsDept(deptID)     // 纯判定
scope.EnsureDeptWritable(id) // 写操作前的闸门
```

**进销存的数据范围只作用于「单据」**（采购 / 销售 / 出入库 / 结算）及其派生的
概览与报表，按 `created_by`（创建人）关联到部门收窄：

- 「仅本人」→ 只看自己开的单；「本部门（及以下）」→ 只看本部门的人开的单。
- **商品 / 分类 / 客户 / 供应商是共享主数据**，不套范围 —— 它们没有「创建人」
  语义，隔离了会导致开单时选不到别人维护的商品与客户，业务直接断掉。
  这些主数据仍由权限码控制谁能改，只是不看数据范围。

`user` / `dept` 两个模块共用它。新增涉及「人」的读接口时，
**唯一需要做的就是拿到 `middleware.CurrentIdentity(c)` 并调 `ApplyUser` / `ApplyLog`**——
历史上 dept 完全漏掉了这件事，于是部门管理员能读到范围外的数据。

三个容易漏的细节：

- **日志表不能用 `ApplyUser`**。`login_logs.user_id` 可能为 0（未识别出账号的失败登录），
  这类记录不属于任何人的范围，非超管一律看不到——否则等于把匿名爆破的来源 IP 全部公开。
  所以另有一个 `ApplyLog`。
- **聚合字段也要收窄**。部门树裁剪了但 `userCount` 还是全库的，等于换个字段继续泄露。
- **改挂上级也要判**。只判「目标部门在不在范围内」会留下「把本部门挂到范围外的父部门下」这条路，
  挂上去之后该部门的实际管辖范围已经变了。

### 内置角色对照

| 角色 | 授权数 | 权限范围 | 数据范围 | 说明 |
| --- | --- | --- | --- | --- |
| `super_admin`（ID=1） | 62 | 全部权限 | 全部 | 不可删除、不可裁剪权限 |
| `system_admin`（ID=2） | 49 | 除导出类外的管理权限 | 本部门及以下 | 不含任何删除权限 |
| `viewer`（ID=3） | 18 | 只读（可见进销存与系统管理的列表页，**不含任何导出**） | 全部 | 能看列表但不能批量取数 |

> 「能查看列表」与「能批量导出」被刻意划成两个风险等级 ——
> 导出等于把整批数据带走，所以只读角色不授予 `*:export`。

**内置角色默认拿到哪些命名空间**由 `seed.frameworkNamespaces` 决定（现行值：
`system` / `profile` / `auth` / `inv`）。
它是一份**显式白名单**：业务模块默认**不会**被自动授予内置角色，
避免「加了个新模块，所有人忽然多了它的权限」这种静默越权。
进销存被加进白名单是有意的 —— 它就是这套后台的主线功能，
不加的话表现是「登录成功但侧边栏是空的」。

> 新增业务模块时：先跑一次 `api-smoke.mjs` 看内置账号能不能看到菜单，
> 再决定要不要把它加进 `frameworkNamespaces`。这一步很容易忘，
> 而表现是「菜单看得见 / 看不见」这种一眼可见的现象，成本很低。

## 1.5 目录结构详解

目录划分直接落地了「模块化」这条铁律：后端业务代码全部收在 `internal/module/<模块>/` 下，
每个模块固定四个实现文件（`dto/service/handler/route`）外加一份 `module.go` **自描述清单**；
`internal/assembly/` 是**全项目唯一** import 具体模块的地方。
前端页面按 `views/<模块>/index.vue` 组织。先看后端，再看前端。

### 后端 `backend/`

```
backend/
├── cmd/server/main.go              进程入口：参数解析 + 优雅关停
├── config.example.yaml             配置模板（复制为 config.yaml）
├── internal/
│   ├── bootstrap/                  装配顺序：配置→日志→JWT→数据库→迁移→模块登记→种子
│   ├── config/                     读 YAML + NOVA_* 环境变量覆盖 + 校验
│   ├── logger/                     zap 初始化与全局 L()
│   ├── database/                   连接、AutoMigrate、EnsureIndexes（部分唯一索引）
│   ├── authz/                      身份与权限集合，30 秒缓存
│   ├── middleware/                 TraceID / Logger / OperationLog / Recovery / CORS / Auth / RequirePerm
│   ├── model/                      GORM 模型（框架 8 张 + 进销存 12 张）
│   ├── module/contract.go          ★ 模块契约：MenuNode / Module / Build / Group（纯类型，不 import 任何具体模块）
│   ├── module/<模块>/              ★ 业务代码都在这里，固定四实现文件 + module.go 清单
│   ├── assembly/assembly.go        ★ 组装层：全项目唯一 import 具体模块、声明模块清单的地方
│   ├── oplog/                      操作日志异步落库
│   ├── pkg/                        无业务依赖的通用工具
│   ├── router/router.go            遍历模块登记表挂路由（不 import 具体模块）
│   └── seed/seed.go                幂等种子数据 + 老库数据修正（内置菜单来自登记表）
└── test/                           进程内 E2E 测试（独立测试库 nova_inv_v5_test）
```

**依赖方向**（单向，禁止反向）：
`assembly → <具体模块> → module（契约）/ pkg / model / authz → 标准库与第三方`。
`internal/module/contract.go` 只定义类型与校验，不 import 任何具体模块；
`internal/router`、`internal/seed` 只认契约类型 `module.Module`，靠 `module.All()` 拿到登记表 ——
所以**把某个业务模块整个删掉，框架照样编译通过**（边界靠编译器验证，不靠自觉）。
`pkg` 反向依赖 `module` **必须禁止**，否则通用层就被业务污染了。

各目录的「不要做的事」：

| 目录 | 不要做的事 |
| --- | --- |
| `cmd/server` | 不写业务逻辑 |
| `internal/config` | 不在别处直接读 `os.Getenv` |
| `internal/database` | 不在别处写 DDL |
| `internal/authz` | 不在业务代码里查授权表 |
| `internal/model` | 不放 DTO |
| `internal/module/*` | 不跨模块直接引用别人的 `service`；不 import `internal/module/<其他模块>` |
| `internal/module/contract.go` | 只放类型与校验，不 import 任何具体模块 |
| `internal/assembly` | 只做组装（列出模块），不写业务逻辑 |
| `internal/pkg` | 不 import `internal/module/*` |
| `internal/router` | 不写 handler，不 import 具体模块 |

> 以上边界由 `scripts/check-module-boundary.sh` 钉住：它用 grep 把「框架包 import 任意
> `internal/module/<子包>`」整体禁掉（排除组装层与 `_test.go`），比逐个枚举业务模块名更强 ——
> **新增模块不必维护白名单**。改动框架层后跑一次，有输出即违规。

### `internal/pkg/` 通用工具

| 包 | 关键导出 | 用途 |
| --- | --- | --- |
| `bind` | `JSON` / `Query` / `URI` | 绑参 + 中文校验提示（读 `label` tag） |
| `errcode` | `Code` 常量、`New` / `Wrap` / `Newf` / `Wrapf` | 业务错误码，前 3 位对应 HTTP 状态码 |
| `response` | `OK` / `OKPage` / `OKMsg` / `Fail` / `FailWith` | 统一响应体 |
| `page` | `Query`、`FromContext`、`Normalize`、`SafeOrderBy` | 分页与排序白名单 |
| `jwt` | `Init` / `M()`、`Generate` / `ParseAccess` / `ParseRefresh` | 双令牌（Claims 含 `epo` 版本号） |
| `hash` | `Password` / `Verify` | bcrypt |
| `sheet` | `Table[T]`、`Download` / `Parse` | Excel 通用表格能力（导出上限 10000 行） |
| `textutil` | `Fit(s, limit)` | 写库前文本收敛：`ToValidUTF8` + **按字符**截断 |

### 前端 `frontend/src/`

```
src/
├── api/              一个模块一个文件，只做接口封装，返回 Promise<T>
├── components/       AppIcon / EChart / Pagination / StatusTag / icons.ts
├── composables/      useLatestGuard（请求竞态）/ useTableQuery（列表查询）/
│                     useCrudDialog（新增编辑弹窗）/ useLogout（登出三件套）
├── directives/       permission.ts（v-perm 指令）
├── layout/           index.vue + Sidebar / Navbar / TagsView / Breadcrumb
├── router/
│   ├── index.ts      constantRoutes（静态路由，无 redirect）
│   ├── helper.ts     后端菜单 → vue-router 路由（import.meta.glob 动态导入）
│   └── guard.ts      ★ 守卫：登录态、动态路由注入、chunk 加载失败自愈
├── store/            user（身份权限）/ permission（菜单路由）/ app（UI 偏好、标签、主题）
├── styles/           variables.scss（亮暗两套设计令牌）+ element.scss + index.scss
├── types/api.ts      与后端 DTO 一一对应（含进销存全部类型）
├── utils/            auth（令牌）/ request（axios）/ download / format（含 exportStamp、
│                     formatMoney / formatQty）/ theme
└── views/            页面：system/ 系统管理、inv/ 进销存
    └── inv/
        ├── components/  PartyPage.vue（客户/供应商通用主体）
        │                OrderPage.vue（采购/销售单据通用主体）
        ├── overview/    数据概览（登录落地页）
        ├── purchase/    采购管理  sales/ 销售管理
        ├── stock/       库存管理（出入库单据 + 库存查询两个 tab）
        ├── finance/     应收应付   report/ 报表中心
        ├── print/       打印模板（卡片网格）
        └── customer/    客户管理  supplier/ 供应商管理
```

系统管理那几个列表页（user/role/menu/dept 与日志页）的查询状态与弹窗骨架
都复用上面的组合式函数，页面里只写「调哪个接口」和「本页独有的附加动作」，
约定见 `AGENTS.md` §3。进销存页同样照抄这套抽象：客户与供应商共用
`PartyPage.vue`（靠 `bizType` 区分），采购与销售共用 `OrderPage.vue`，
四个薄壳页面各自只传 `defineOptions({ name })` 与几个文案 prop。

**前端路由是「菜单驱动」的**，这是本项目最省事也最容易误解的一点：

- 静态路由（`constantRoutes`）只有三条：`/login`、`/`（Layout 容器）、兜底 404。
- 其余所有业务路由都是登录后从 `GET /api/v1/profile/routes` 拉回来的菜单树，
  由 `router/helper.ts` 的 `buildRoutes()` 转换，再用 `router.addRoute()` 动态注入。
- **目录节点不注册路由，叶子拍平挂到 Layout 下。**
- 所以**新增页面不需要改任何前端路由文件**，只要在 `sys_menus` 里加一条菜单记录。

## 1.6 功能清单

9 个模块共 **86 个接口**，另有服务根的 `/health` 不计入其中。
先看模块总览，再看菜单树、内置角色与权限码是怎么分配的。

> **进销存就是这套后台的主线业务**（`inv`），框架自带的 `dashboard`（仪表盘）
> 已整体下线。登录后的落地页是「数据概览」—— 一个**顶级菜单**，
> 由前端 `firstLeafPath(menus)` 按菜单树顺序推导而来；其余 10 个进销存页面
> 归「进销存」目录。老库升级时会自动完成这两步迁移（见 `docs/UPGRADE.md`）。

### 模块总览

| 模块 | 路径前缀 | 接口数 | 说明 |
| --- | --- | --- | --- |
| `auth` | `/auth` | 3 | 登录、刷新令牌、退出 |
| `profile` | `/profile` | 4 | 个人信息、改资料、改密码、动态路由树 |
| `inv` | `/inv` | 43 | 进销存：概览 / 采购 / 销售 / 库存 / 应收应付 / 报表 / 打印模板 / 商品档案（含左侧分类树）/ 客户 / 供应商 |
| `user` | `/system/users` | 11 | 用户 CRUD + 启停 + 重置密码 + 导入导出 |
| `role` | `/system/roles` | 8 | 角色 CRUD + 分配菜单 + 导出 |
| `menu` | `/system/menus` | 5 | 菜单树 CRUD |
| `dept` | `/system/depts` | 5 | 部门树 CRUD |
| `log` | `/system/logs` | 7 | 操作日志、登录日志、清空、导出 |
| — | `/health` | 1 | 健康检查（不鉴权、不记日志） |
| **合计** | | **86 + 1** | |

> 服务根路径还有一个 `GET /health`（返回 200；数据库异常时返回 503 + `degraded`），
> 以及全局 `NoRoute` 统一 404。其余全部挂在 `/api/v1` 下。

### 功能要点

- **认证**：JWT 双令牌（访问 2 小时 / 刷新 7 天），访问令牌自动静默续期；
  验签锁死 `HS256` 并校验 `issuer` 与 `uid > 0`；
  连续登录失败 5 次锁定 300 秒；改密码 / 重置密码 / 禁用账号即刻吊销旧令牌。
- **限流**：登录失败按「用户名 + IP」复合锁定，另有独立的来源 IP 失败限流；
  两者记账均为**单次加锁内完成**（否则并发下阈值被成倍突破），
  且内存 key 数量有 1 万硬上界——超界 fail-open，宁漏限也不 OOM。
- **动态路由**：登录后按当前用户的菜单权限生成路由与侧边栏，切换账号自动卸载重建。
- **按钮级权限**：`v-perm="'system:user:create'"`，无权限时打 `data-perm="denied"` 标记并用 CSS 隐藏
  （**不摘 DOM**——摘掉会破坏 `el-table` / `el-dropdown` 的 vnode 锚点，且指令钩子里读 Pinia
  不会建立响应式依赖，权限在运行期变化时无法重新判定）。详见 `AGENTS.md` §5.34。
- **数据范围**：四种范围，读写同时生效（用户 / 部门两个模块统一走 `authz.ResolveScope`），
  越界读按 404 处理、越界写按 403 处理。
- **导入导出**：用户 / 角色 / 操作日志 / 登录日志四类 Excel 导出；
  用户支持**两阶段导入**（先校验预览并给出逐行原因，确认后才写库）。
- **审计**：操作日志（含请求体，敏感字段递归脱敏）+ 登录日志；
  请求体超过限制或为二进制时不记录，避免被客户端内容搞挂。
  登录日志同时是账号锁定的计数来源，因此**清空登录日志时会保留锁定期内的记录**——
  删掉等于给攻击者一个免费的解锁接口。
- **主题**：亮色 / 暗色双主题，跟随系统偏好 + 手动切换 + 本地记忆，首屏无白闪。
- **进销存**：以 `inv` 命名空间提供 10 个页面。**采购与销售共用一张 `inv_orders` 表**
  （`biz_type` 区分），字段、明细、金额与结算语义完全一致，拆表只会让明细 / 结算 / 报表各写两遍。
  两个要点值得单独记住：
  1. **业务线要二次鉴权**：`/inv/orders` 一条路由服务两条业务线，路由上的 `RequirePerm`
     只做粗筛（采购或销售任一读权限），handler 内还按实体的 `bizType` 再判一次 ——
     否则只配了 `inv:sales:list` 的账号换个查询参数就能读采购单据。
  2. **出参结构不图省事复用**：出入库单只返回数量、不含金额（`StockOrderRow` 没有 `totalAmount`），
     有 `inv:stock:list` 不等于能看到成本。
  单据置「完成」时在**同一事务**内生成出入库单并用 `stock = stock + ?` 原子调整库存；
  出库前校验库存充足（禁负库存），作废按反方向回滚；收付款登记同样写记录 + 累加已结金额。
  **结算状态（`unpaid`/`partial`/`paid`）是推导值，不入库**，避免两份口径各写一遍。
- **商品是多规格多单位的**：`商品 → 规格 → 计量单位` 三层。
  **库存只记在规格上、且按基本单位计；单价记在单位上** —— 于是「`φ20` 剩 30 根 = 180 米」
  永远只有一个数，「1 箱 100 元 vs 1 个 10.5 元」的价差也能带对。
  单据明细必须选到「规格 + 单位」，落库按 `qtyBase = qty × rate` 折算；
  `resolveSpecUnits` 校验 `productId → specId → unitId` 三元组链真实存在，
  否则下拉串了会把库存记到不属于它的规格上（账面上看不出来，要等盘点才发现）。
  **品类是字典表不是自由文本**（`inv_categories`）+ 独立管理页，删除前校验商品引用。
  **商品录入查重只提示不拦截**：同名不同规格是合法场景，命中后列出来并可一键跳转编辑。


## 1.7 数据表

20 张表，全部由 GORM `AutoMigrate` 自动创建。框架表 8 张（`sys_` 前缀）：

| 表名 | 用途 | 软删除 |
| --- | --- | --- |
| `sys_users` | 用户 | 是 |
| `sys_roles` | 角色（含 `data_scope` 数据范围） | 是 |
| `sys_menus` | 菜单 / 权限点（同时驱动前端动态路由） | 是 |
| `sys_depts` | 部门（树形） | 是 |
| `sys_user_roles` | 用户-角色关联（复合主键） | 否 |
| `sys_role_menus` | 角色-菜单关联（复合主键） | 否 |
| `sys_operation_logs` | 操作日志 | 否 |
| `sys_login_logs` | 登录日志 | 否 |

进销存业务表 12 张（`inv_` 前缀）：

| 表名 | 用途 | 软删除 |
| --- | --- | --- |
| `inv_customers` / `inv_suppliers` | 客户 / 供应商档案 | 是 |
| `inv_categories` | 商品分类字典（`parent_id` 支持多级父子） | 是 |
| `inv_products` | 商品**身份**（编码 / 名称 / 分类 / 状态 / 备注） | 是 |
| `inv_product_specs` | 商品规格 = **库存桶**（`stock` / `warn_stock`） | 是 |
| `inv_product_units` | 计量单位（`rate` 换算率 / `is_base` / 采购价 / 销售价） | 是 |
| `inv_orders` | 采购 **与** 销售单据主表（`biz_type` 区分） | 是 |
| `inv_order_items` | 单据明细行（商品快照 + `spec_id` / `unit_id` / `rate` / `qty_base`） | 是 |
| `inv_settlements` | 收付款记录（应收应付共用） | 是 |
| `inv_stock_orders` / `inv_stock_items` | 出入库单据与明细（明细不存金额，带折算列） | 是 |
| `inv_print_templates` | 单据打印模板 | 是 |

> **商品拆成三张表**是本项目的核心数据模型决策：**库存只落在规格上、按基本单位计，
> 价格落在单位上**。若把库存与价格都挂在商品上，「一个商品多规格多单位」时
> 就没有地方分别存放每个规格的库存与每个单位的价格，只能退化成一档一规格一单位。
> 详见 [`DATABASE.md`](DATABASE.md) 与 [`API.md`](API.md) §11.2。

> **表名前缀即命名空间**：框架表用 `sys_`，业务模块用各自的短前缀（进销存是 `inv_`）。
> 一眼能看出这张表归谁管，删模块时也不会误伤框架表。

**四条设计约定**（细节见 [`DATABASE.md`](DATABASE.md)）：

1. **唯一性用「部分唯一索引」**（`WHERE deleted_at IS NULL`），不用 `uniqueIndex` 标签 ——
   否则软删除的历史行仍占着唯一约束，会出现「删除后无法再建同名记录」。
   索引由 `database.EnsureIndexes()` 在启动时创建，并会清理历史遗留的普通唯一索引。
2. **`status` 字段不加 `gorm:"default:1"`** —— GORM 遇到「带 default 且值为零值」会把该列
   从 INSERT 里省略，导致「禁用（0）」被数据库默认值 1 悄悄改写成「启用」。
3. **`many2many` 必须写完整表名**（`many2many:sys_user_roles`），否则 GORM 会推导出不存在的表名。
4. **时间字段统一 `timestamptz`**，时区取配置项 `database.timezone`（默认 `Asia/Shanghai`）。

**内置数据**（`internal/seed/`，幂等，按幂等键补齐缺失项）：

| 项 | 数量 | 内容 |
| --- | --- | --- |
| 部门 | 3 | 总公司 / 技术部 / 运营部 |
| 菜单 | 61 | 2 个目录 + 15 个页面菜单 + 44 个按钮权限点 |
| 权限点 | 60 | 15 个页面级 + 44 个按钮级（其中进销存 36 个） |
| 角色 | 3 | `super_admin`（61 条授权）/ `system_admin`（48）/ `viewer`（只读，17） |
| 用户 | 2 | `admin`、`demo` |
| 进销存演示数据 | — | 5 分类、5 客户、5 供应商、8 商品（含多规格多单位示例）、8 打印模板、14 张采购/销售单据、6 张手工出入库单 |

> 上面是**新库**的数字。老库升级时，清单里的节点按「权限码 / 路由名」幂等补齐，
> 因此只会补上缺的：管理员若手工删除过某个内置菜单，播种**不会**把它重建
> （只打一条 WARN），按钮权限点数量会相应少一个 —— 需要恢复请在菜单管理里手工处理。
>
> **商品档案还会做一次「只补不覆盖」的演示数据补齐**（`seedInvProductTopUp`）：
> 从旧的单规格结构迁移过来的库，每个商品只有一个规格 + 一个基本单位，
> 多单位示例（`1 根 = 6 米`、`1 盒 = 10 个`、`1 箱 = 4 桶`、`1 卷 = 100 米`）
> 一条都不会有。补齐按「演示编码」定位商品、按名比对规格与单位，
> **已存在的不动**，只把缺的补上 —— 客户改过的单位名与价格不会被种子覆盖回去。
> 补基本单位前先查该规格是否已有基本单位，避免出现两个 `is_base`。

**菜单主键怎么分配**（重构后不再需要手工选号）：

| 场景 | 主键来源 |
| --- | --- |
| 内置菜单（各模块 `module.go` 的 `Menus`） | 播种时由数据库**自增**分配；老库里命中同一幂等键的记录**原样沿用其已有 ID** |
| 管理员在菜单管理页自建 | 同样自增 |

也就是说，`1–999` / `1000+` 那套手工分段已经作废 ——
幂等键从「手工 ID」换成了**权限码（按钮）/ 路由名（目录、页面）**。
加一个内置菜单不需要挑号，也不会再撞号。
老库升级时内置菜单 ID 保持不变（`1`、`10`、`11`…`149` 仍是原值），
新库则从 `1` 开始按声明顺序自增。

## 1.8 依赖与许可

全部依赖为宽松许可，可商用、可闭源分发，无 GPL 传染风险。

| 依赖 | 许可 |
| --- | --- |
| gin / gorm / pgx / golang-jwt/v5 / validator/v10 / zap | MIT |
| golang.org/x/crypto / excelize/v2 | BSD-3-Clause |
| vue / vue-router / pinia / element-plus / axios / dayjs / nprogress / vite / sass | MIT |
| echarts / typescript | Apache-2.0 |

本项目自身以 [MIT](../LICENSE) 许可发布。

---

# 第二部分　使用步骤

## 2.1 环境要求

| 组件 | 版本 | 检查命令 |
| --- | --- | --- |
| Go | 1.25+ | `go version` |
| Node.js | 18+ | `node -v` |
| PostgreSQL | 13+ | `psql --version` |

PostgreSQL 只需**正常运行且 `postgres` 账号可连**。
**数据库 `nova_inv_v5` 与全部数据表都会由程序自动创建**，不需要手工执行任何 DDL。

## 2.2 首次启动

首次启动一共 4 步：准备配置 → 启动后端（自动建库建表）→ 启动前端 → 登录验证。
**全程不需要手工执行任何 DDL。**

### 第 1 步：准备后端配置

```bash
cd backend
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，至少改掉数据库口令：

```yaml
database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: CHANGE_ME      # ← 改成你本机的口令
  dbname: nova_inv_v5
app:
  port: 6802
```

> `config.yaml` 含口令，**已在 `.gitignore` 中，不会进版本库**。
> 仓库里只保留 `config.example.yaml` 模板。

### 第 2 步：启动后端

```bash
cd backend
go mod download
go run ./cmd/server -config config.yaml
```

首次启动会自动完成：建库 → 建表（`AutoMigrate`）→ 建索引 → 灌入种子数据。

看到下面这两行就说明成功了：

```
数据库连接成功    {"database": "nova_inv_v5"}
HTTP 服务已启动   {"addr": "0.0.0.0:6802", "basePath": "/api/v1"}
```

验证：

```bash
curl http://127.0.0.1:6802/health
# {"app":"nova-erp","status":"ok","version":"v1"}
```

### 第 3 步：启动前端

```bash
cd frontend
npm install
npm run dev
```

打开 `http://localhost:6801`。Vite 已把 `/api` 代理到 `http://127.0.0.1:6802`，无需处理跨域。

### 第 4 步：准备测试配置（跑测试前必做）

测试使用**独立数据库** `nova_inv_v5_test`，配置同样不进版本库：

```bash
cd backend
cp test/testdata/config.example.yaml test/testdata/config.yaml
```

## 2.3 登录

| 账号 | 密码 | 角色 | 用途 |
| --- | --- | --- | --- |
| `admin` | `admin123456` | 超级管理员 | 拥有全部权限，用于开发调试 |
| `demo` | `demo123456` | 只读访客 | 用来**验证权限隔离**：所有写操作都会被后端拒绝 |

> `demo` 账号很有用：拿它点一遍写按钮，就能确认「前端隐藏」和「后端拦截」是否都生效。
>
> ⚠️ 生产环境请第一时间修改默认口令与 `NOVA_JWT_SECRET`。

## 2.4 功能导览

按侧边栏顺序走一遍，即可覆盖全部功能：

### 进销存 → 数据概览

**登录后的默认落地页。** 五张指标卡（客户总数 / 本月销售额 / 应收总额 / 库存商品 / 待处理单据）、
近 30 天销售与采购趋势面积图、品类销售占比环形图、本月客户销售排行。

> 首页落点是**推导出来的**，不是配置项：前端取菜单树里第一个可访问的叶子。
> 所以改首页 = 调 `/profile/routes` 下发的顺序，代码上就是 `assembly.Modules()` 的声明顺序
> 加上播种时的顶层 `sort` 校准（见 `AGENTS.md` §5.62）。

### 进销存 → 采购管理 / 销售管理

两个页面共用 `OrderPage.vue`，靠 `bizType` 区分（`purchase` / `sales`），
列与操作完全一致：单号 / 往来单位 / 单据日期 / 结算状态 / 金额 / 明细 / 操作。

| 操作 | 说明 |
| --- | --- |
| 新增 / 修改 | 明细行内编辑：选商品 → 选**规格** → 选**单位**，再填数量与单价（非基本单位时显示折算后的基本数量）；**单价留 0 时自动取所选单位的采购价 / 销售价** |
| 审核 | 二次确认后**同事务**生成出入库单并按 `qtyBase` 调整库存；出库校验库存充足 |
| 取消 | 置为已取消，不产生任何库存变动 |
| 删除 | 仅未完成单据可删；已完成单据要先作废对应的出入库单 |

改动只在「待处理」状态开放（审核通过即锁定）；单据类型不可更改（`bizType` 与实体不一致会报 40000）。
明细的「商品 → 规格 → 单位」是**三级联动**：换商品会清空规格与单位并落到第一个规格的基本单位，
换规格会重挑单位。「单价」跟着单位走 —— 同规格下「1 箱」与「1 个」的价本就不同。
规格与单位没选完不许提交（前端拦一道，后端 `resolveSpecUnits` 再拦一道）。

**商品下拉支持输入字符模糊查找**：匹配**名称 / 编码 / 规格**三个字段
（现场沟通时用户报的往往是「φ20」而不是编码），与后端 `ProductList` 的关键字口径一致。

> 这里刻意**不用 el-select 的 `remote`**：remote 模式下 el-select **不做本地过滤**，
> 它原样显示你给的全部 options。之前把服务端搜索结果「合并」进选项后，下拉里
> 仍然是全部商品（含不匹配的），用户看到的就像「搜索没生效」。
> 现在用 `filterable` + `filter-method`，过滤结果自己维护（见 `applyPickFilter`）——
> 顺带解决了另一个问题：**已选中的商品必须始终留在选项里**，否则 el-select
> 反查不到 label，会把「热轧钢板」显示成数字。
>
> 商品量大时靠 `ProductPick` 的一次性上限兜住（`pickLimit = 1000`）；
> 真的超了会在明细区顶部提示「下拉只加载了一部分」，不会静默少给。
>
> **商品列要给足宽度**：下拉文案是「名称（编码）· N 个规格」，被压到一百多像素时
> 只能看到「热轧钢…」，几个名字相近的商品就分不清了。采购 / 销售弹窗
> 1040 → 1180px、出入库弹窗 760 → 1000px，其余列压到「刚好放下内容」，
> 省下的空间全给商品列（`min-width: 260px`，实测 ~294px）。
> 这条有断言兜底（宽度 < 260 直接失败）—— 靠肉眼是发现不了「窄了一点」的。

**打印**：每张采购 / 销售单都可按模板打印（A4 / A5），弹窗里选模板、预览、
一键调起系统打印。纸面带往来单位、明细规格 / 单位 / 单价 / 金额列与合计金额，
商品编码由商品档案补齐（商品已删留空）；非基本单位的数量会在下方补一行折算
（「×6 = 180 米」），模板设计器的单据类型指向采购单 / 销售单时，预览会自动切换到这张纸面。

> **到期日不进纸面。** 它是内部结算字段（只用于判定应收应付是否逾期），
> 版式配置里没有这一项、模板设计器也不提供 —— 纸面上只有单据编号、单据日期、
> 往来单位与制单 / 打印时间。历史上这里按「有值就打印」输出，未填写的单据
> 会因为零值时间被打出一行「到期日：0001-01-01」，模板里没有、纸面上却有。

### 进销存 → 库存管理

两个 tab：**出入库单据**（单号 / 方向 / 来源 / 日期 / 明细 / 总数量 / 打印 / 作废）与
**库存查询**（商品编码 / 商品 / **规格** / 单位[基本单位] / 品类 / 库存 / 预警下限 / 库存成本 / 状态）。

- 手工出入库**即时过账**；作废按反方向回滚库存。
- **库存查询的粒度是规格**：一行 = 一个商品的一个规格，数量以该规格的**基本单位**呈现 ——
  多规格商品会出现多行。库存本就归属到规格，压成一行反而要把不同规格数量加在一起，
  「3 根 + 5 个」得出的 8 没有任何意义。
- 库存低于预警下限的行会标红，顶部工具行给出预警项数与库存总值。
- 库存查询**只列启用商品与启用规格**：已停用的档案不再出现，否则视图里会混进一批
  早已下架、数量只增不减的历史条目。
- 新增出入库单的明细行同样走「商品 → 规格 → 单位」三级联动，非基本单位时显示折算后的基本数量
  （「12 米 = 2 根」）——「12」是 12 张还是 12 吨，在选择的那一刻就要看得见；
  出库时按折算后的 `qtyBase` 校验并扣减库存。
- 出入库单据的接口出参**不含金额** —— 有仓库权限不等于能看到成本。
- **打印**：每张出入库单可按模板打印（A4 / A5），弹窗里选模板、预览、
  一键调起系统打印。明细带商品编码 / 规格 / 单位与折算数量、制单人；**已作废的单据不可打印**——
  库存已回滚，再打出来就是一张与实物对不上的凭证。

### 进销存 → 应收应付

四张汇总卡（单据总额 / 已结算 / 未结余额 / 逾期金额），下分「应收明细」「应付明细」两个 tab。

- **只有「已完成」单据形成应收应付**，未完成与已取消的不计入。
- 筛选支持未结清 / 部分结算 / 已结清 / 已逾期；逾期按 `due_date` 与今天比较。
- **到期日是选填项，未填的永远不算逾期。** 一笔单据算逾期要同时满足
  「未结余额 > 0」「到期日有值且不是零值时间」「到期日早于今天」。
  汇总与明细走同一个口径（SQL 侧一个常量、Go 侧一个 `isOverdue()`），
  冒烟脚本断言「逾期汇总 == 逐行逾期之和」守住这条不变量。
- 登记收款 / 付款的金额上限是该单的未结余额，超额会被拒（40900）。

### 进销存 → 报表中心

时间范围切换（今日 / 本周 / 本月 / 季度 / 本年，默认本月），四张 KPI
（营业收入 / 销售成本 / 毛利 / 毛利率）+ 趋势图 + 品类占比 + 客户销售排行。

- **销售成本按商品当前采购价 join 计算**，`毛利 = 营业收入 − 销售成本`。
- 趋势按天补齐，没有数据的日子补 0（否则折线会被压缩成「只有几段」）。
- **打印**：右上角「打印」把当前时间范围的经营汇总、销售排行、品类占比
  渲染为 A4 黑白纸面（固定版式，不走打印模板）——打出来的就是页面正在看的数据快照。

### 进销存 → 打印模板

卡片网格展示各单据的打印版式（纸张规格 A4/A5 + 启用状态 + 单据类型），
支持**设计 / 新增 / 编辑 / 删除**。模板数量少，卡片比表格更好扫读。

**「设计」是模板的核心**：左侧调版式（单据标题、明细列显隐[编码 / 规格 / 单位，
采购 / 销售单另有单价 / 金额]、表头信息显隐[来源单号 / 制单人 / 打印时间]、
字号、页脚、纸张规格），右侧用示例数据**实时预览**纸面效果，保存即生效——
下次打印单据就是新版式。模板单据类型含「采购 / 销售」时预览自动切换到对应版式。

- 版式配置以 JSON 存在模板的 `content` 字段，后端只存取不解释；新增配置项
  必须在前端 `printConfig.ts` 的 `DEFAULT_CONTENT` 里给默认值——老模板的
  content 里没有它，解析时按默认补齐。
- 纸面颜色刻意用**固定黑白灰**而非主题令牌：打印纸永远是白底黑字，
  不跟随亮 / 暗主题；设计器预览同理，「所见即所得」指的是纸面。

### 进销存 → 商品档案（左分类树 + 右商品）

**布局是左右分栏**：左侧分类树、右侧商品表格。原「商品分类」独立页已合并进来 ——
维护分类时本来就要盯着它下面的商品，分成两页等于同一个动作开两个入口、来回切换。

商品档案的完整维护入口，**一个商品可以有多个规格、每个规格可以有多个计量单位**。
右栏列表列：编码 / 商品名称 / 分类 / 规格摘要 / 规格数 / 单位数 / 总库存 / 状态 / 创建时间。
展开一行可看到该商品下的**规格卡片**，每张卡片内嵌单位小表（单位 / 换算率 / 基本单位 / 采购价 / 销售价）。

筛选：关键字（匹配**编码 / 名称 / 规格**）、分类（`categoryId`）、状态。
筛选用分类下拉来自**全量分类字典**（含停用），因为停用分类下的商品仍在库里 ——
只给启用项的话，筛选器会变成「看起来正常但永远筛不到」的死选项。

**左栏分类树**：

- 树节点带商品数（`螺纹钢（4）`），选之前就知道这一分类下有多少货；
  停用分类压灰但仍显示，理由同上。
- 每个节点**常显**三个操作按钮：新增子分类 / 编辑分类 / 删除分类。
  （不要做成 hover 才出：el-tree 的节点内容区前面还有展开箭头与层级缩进，
  再写 `width: 100%` 会把按钮挤出容器裁掉 —— 按钮存在却永远点不到，
  用户只会认为「没有删除功能」。）
- 删除按钮会就地拦截：有子分类或下有商品时直接弹窗说明原因与下一步，
  不让用户点了才撞 409。
- 点节点即按该分类过滤右栏；底部「全部商品」清空筛选。
- **支持多级父子分类**（`inv_categories.parent_id`，0 = 根）。树由前端按
  `parent_id` 组装，后端返回平表 —— 不猜前端默认展开到第几层。

分类的写入约束：

| 约束 | 原因 |
| --- | --- |
| **同级**名称唯一（40900） | 不同父级下的「标准件」是两个合法分类，全局唯一会把多级树逼回一级 |
| 上级不能是自己或自己的子孙（40000） | `parent_id` 一成环，前端按它组装树会无限递归、页面直接卡死 |
| 有商品引用不可删（40900） | 否则 `inv_products.category_id` 悬空，报表品类占比凭空少一块 |
| 有子分类不可删（40900） | 否则子分类会集体变成根节点，树形结构凭空塌一层 |

删除在页面上**直接弹窗说明原因**，不再让用户点了才撞 409。

**编辑弹窗（960px）分两段**：

1. **基本信息**：编码 / 名称 / 分类 / 状态 / 备注。名称输入停止 400ms 后自动查一次同名商品（`GET /inv/products/duplicates`，带上当前编辑的 `excludeId`），命中则列出「编码 / 名称 / 分类 / 规格摘要」并支持一键跳转编辑。
   - **商品分类下拉可输入搜索**，与销售单选商品同一套交互。选项是**扁平化 + 层级路径**
     （`钢材 / 螺纹钢`），因为 el-option 的 v-for 只遍历数组顶层 ——
     直接把树结构喂进去会让**子分类全部消失**（下拉看着正常，只是少了选项）。
     扁平化之后 label 是完整路径，`filterable` 默认按 label 匹配，
     输入「钢材」或「螺纹」都能命中；路径分隔符用 ` / ` 而不是树控件的缩进，
     避免在窄下拉里被挤没。
2. **规格与单位（可留空）**：可增删规格、每个规格内可增删单位、用 radio 指定**基本单位**、填换算率与采购 / 销售价。被单据引用过的规格只允许**停用**，卡片上有开关。
   - **规格名 / 单位名 / 换算率 / 基本单位都不是必填**：留空由后端 `normalizeSpecs`
     兜底成「默认」「个」「1」，并把第一个单位提为基本单位。录一个「办公用品-签字笔」
     时不必先想清楚计量单位。
   - 但**不能真的允许为空**：规格是库存的归属层、单位是单据折算的基准 ——
     真留空的话，商品会因为「没有规格」而进不了开单的商品下拉
     （`ProductPick` 只给有规格的商品），等于建了个查不到、开不了单的僵尸档案。
     所以做法是**用户不用想，系统自己填**。
   - 唯一的硬约束是**不能重名**：两行「φ20」会把库存劈成两半、各自都不预警；
     同一规格下两个「箱」会让单据选单位时不知道看哪一行的价格。

| 操作 | 说明 |
| --- | --- |
| 新增 | 编码 / 名称必填；至少一个规格，每个规格至少一个单位，且恰有一个基本单位 |
| 修改 | 同上；保存时联动更新规格与单位 |
| 删除 | 被单据明细引用时禁止删除（40900），提示可改为**停用** |

> 权限点：`inv:product:list` / `create` / `update` / `delete`，以及
> `inv:category:list` / `create` / `update` / `delete`（分类的按钮权限点挂在商品菜单下）。
写入约束（违反返回 40000）：规格名不重复、单位名不重复、每个规格**有且只有一个基本单位**、基本单位 `rate` 必须为 1、`rate > 0`。

三个刻意的设计取舍：

1. **库存不在表单里。** 它由出入库单据驱动、记在**规格**上，新增 / 修改接口都不接受它。
   允许在档案上直接改，等于凭空多出一套与单据无关的库存账，盘点对不上时无从追溯。
2. **库存记规格、价格记单位，不能合并。** 「`φ20` 剩 30 根 = 180 米」必须只有一个数，
   所以库存按**基本单位**折算后落在规格上；而「1 箱 100 元」与「1 个 10.5 元」本就是不同的数，
   所以价格落在**单位**上。任何一边放错层级，多规格多单位场景下都会算错。
3. **改名要沿 ID，不能按名全删重建。** 规格或单位被历史单据引用后是删不掉的（只能停用），
   若保存时把它们全删再建，ID 会漂移，历史明细的 `spec_id` / `unit_id` 随即失效，
   那张单再保存就会被三元组校验打回。所以前端编辑时回传 `id`，后端**优先按 ID 匹配**，
   再「比对后增删改」，被移除但已被引用的规格只做停用。

> 为什么单独一页而不是并进库存页：库存回答「现在有多少」，档案回答
> 「这是什么、有哪些规格、按什么单位、卖多少钱」。前者随业务天天在动，后者是主数据，
> 变更频率低但影响面大（规格、单位、价格会直接进单据与打印纸面）。混在一起，
> 库存页会变成「一个列表两种职责」，权限也没法分开给。
>
> 权限点：`inv:product:list` / `create` / `update` / `delete`。

### 进销存 → 商品分类

分类字典的管理页。列：名称 / 排序 / 商品数（`productCount`，引用该分类的商品个数）/ 备注 / 状态 / 创建时间。

| 操作 | 说明 |
| --- | --- |
| 新增 / 修改 | 名称唯一（重复 40900），排序控制下拉顺序 |
| 删除 | **仍被商品引用时禁止删除（40900）**，页面直接提示「建议改为停用」 |

分类从「商品表上的一列自由文本」升级为独立字典表，是为了让品类统计稳定：
自由文本下「钢材」与「钢材 」（多一个空格）会各算一类，报表的品类占比会凭空多出一块。
删除前必须校验引用，否则 `inv_products.category_id` 悬空 —— 商品还在，分类没了。
定义上「分类可停用不可乱删」：停用后不影响历史商品，只是新增商品时选不到。
页面上的分类下拉会把当前值拼回去 —— 否则一个已停用的分类会让老商品在编辑时显示成空白。

> 权限点：`inv:category:list` / `create` / `update` / `delete`。

### 进销存 → 客户管理 / 供应商管理

两个页面共用 `PartyPage.vue`。列：编号 / 名称 / 联系人 / 电话 / 地址 / 开票信息（统一社会信用代码）
/ 余额 / 状态 / 创建时间。**余额不落库**，由该往来单位所有已完成单据的未结金额聚合而来；
列表行一次带出「余额 + 单据数」，避免逐行再发一次请求。
被单据引用过的往来单位不可删除（返回 40900），否则历史单据会指向一个查不到的单位。

### 系统管理 → 用户管理

列表支持按用户名/昵称/手机号搜索、按部门与状态筛选、按创建时间排序。

| 操作 | 说明 |
| --- | --- |
| 新增 / 修改 | 部门用树选择器；状态用开关 |
| 启用 / 禁用 | **禁用会立即吊销该用户的令牌**，已登录的会话下次请求就失效 |
| 重置密码 | 重置后同样立即吊销旧令牌 |
| 删除 | 内置管理员（ID=1）受保护，不可删除 |
| 导出 | 导出内容与当前筛选条件一致，也支持「导出选中项」 |
| 导入 | 两阶段：先校验预览（含逐行失败原因），确认后才写库 |

导入规则（详见 [§11.3](API.md)）：

- 表头列顺序必须与模板一致，否则整份拒绝
- **合法行照常写入、非法行跳过并给出原因**，不会因一行出错整批失败
- 密码列留空使用默认密码 `123456`
- 单文件上限 5 MB、单次最多 1000 行

### 系统管理 → 角色管理

- 每个角色有两个维度：**菜单权限**（功能）与**数据范围**（数据）。
- 「分配菜单」用树形勾选，父节点会随子节点自动带上。
- 内置角色（`super_admin` / `system_admin` / `viewer`）不可删除；超管的权限不可裁剪。

### 系统管理 → 菜单管理

这是**整个系统的骨架**，理解它就理解了权限与路由的关系。

| 类型 | 作用 | 是否需要 `component` |
| --- | --- | --- |
| 目录 | 侧边栏分组，不产生路由 | 不需要 |
| 菜单 | 一个页面，产生一条动态路由 | **需要**（如 `system/user/index`） |
| 按钮 | 只提供权限码，不显示 | 不需要 |

关键字段：

| 字段 | 说明 |
| --- | --- |
| `path` | 绝对路径、单数形式，如 `/system/user` |
| `component` | `src/views` 下的相对路径，**无扩展名**，如 `system/user/index` |
| `name` | 大驼峰且全局唯一，**必须与页面 `defineOptions({ name })` 一致**，否则 keep-alive 失效 |
| `permission` | 权限码 `模块:资源:动作` |
| `keepAlive` | 是否缓存页面 |
| `hidden` | 是否在侧边栏隐藏（如详情页） |

### 系统管理 → 部门管理

树形结构，支持拖拽排序。**部门下有用户时不允许删除**（会提示先转移用户）。
部门直接参与数据范围计算：「本部门及以下」就是沿这棵树向下展开。

### 系统管理 → 日志管理

- **操作日志**：谁、什么时候、在哪个模块做了什么、耗时多少、请求体是什么。
  敏感字段（`password` / `oldPassword` / `refreshToken`）落库前**递归脱敏**。
- **登录日志**：成功与失败都记，含来源 IP 与 User-Agent。
- 两者都支持按时间 / 用户 / 模块 / 状态筛选与导出。
- 「清空」需要指定 `before` 时间点，**避免误清全部**。

### 个人中心

改昵称 / 邮箱 / 手机号 / 头像，以及**修改本人密码**。
改密码后会立即吊销自己所有已签发的令牌，需要用新密码重新登录。

### 主题切换

顶栏右侧的太阳 / 月亮按钮；**登录页右上角**也有一个（避免暗色用户每次进登录页被闪一下）。

| 行为 | 说明 |
| --- | --- |
| 首次访问 | 未保存过选择时跟随系统偏好（`prefers-color-scheme`） |
| 手动切换后 | 写入 `localStorage.nova_theme`，此后不再跟随系统 |
| 首屏 | `index.html` 内联脚本在样式生效前写好 `<html class="dark">`，无白闪 |

## 2.5 配置项全表

`backend/config.example.yaml` 的完整配置项：

```yaml
app:
  name: nova-erp          # 应用名
  mode: debug               # debug | release
  host: 0.0.0.0
  port: 6802
  prefix: /api              # 路由统一前缀
  version: v1               # API 版本段，最终拼接为 /api/v1
  swagger: true             # 是否挂载 /swagger 在线文档；不写则跟随 mode（debug 开、release 关）

database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: CHANGE_ME       # 改成本机口令
  dbname: nova_inv_v5       # 不存在会自动创建
  sslmode: disable
  timezone: Asia/Shanghai
  max_idle_conns: 10
  max_open_conns: 100
  conn_max_lifetime: 3600   # 秒
  log_level: warn           # silent | error | warn | info

jwt:
  secret: CHANGE_ME_USE_A_RANDOM_LONG_STRING   # 上线务必换成随机长串
  issuer: nova-erp
  access_expire: 7200       # 秒，默认 2 小时
  refresh_expire: 604800    # 秒，默认 7 天

log:
  level: info               # debug | info | warn | error
  format: console           # console | json
  dir: ./logs
  file: app.log

cors:
  allow_origins:
    - "*"                   # 生产改成具体域名

security:
  max_login_fail: 5         # 「账号 + 来源 IP」连续失败锁定阈值
  login_lock_seconds: 300   # 锁定时长（秒）
  ip_max_login_fail: 30     # 同一来源 IP 的失败上限（拦「一个 IP 撞库多个账号」）；0 = 关闭
  refresh_rate_limit: 120   # /auth/refresh 每 IP 每分钟请求上限；0 = 关闭
  trusted_proxies: []       # 空 = 不信任任何代理
```

> `ip_max_login_fail` 与 `max_login_fail` 的分工：
> 前者按 **来源 IP** 计数，拦的是「用同一个出口 IP 试大量不同账号」；
> 后者按 **账号 + IP** 计数，拦的是「盯着一个账号猜密码」。
> 两者都**只统计失败**（登录成功即清零），因此不会连累 NAT / 公司网关后的正常用户。
> 0 表示关闭；不写则用默认值（30 / 120）。

### 环境变量覆盖

任意配置项都可用环境变量覆盖，规则为 **`NOVA_<SECTION>_<KEY>`（全大写）**：

```bash
NOVA_DATABASE_PASSWORD=xxxx      # 数据库口令
NOVA_APP_PORT=9090               # 服务端口
NOVA_JWT_SECRET=change-me        # JWT 密钥
NOVA_SECURITY_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8   # 逗号分隔
```

完整映射：`NOVA_` + `APP` / `DATABASE` / `JWT` / `LOG` / `SECURITY` + `_` + 大写字段名。
**例外**：`cors.allow_origins` 只能写在 YAML 里，不支持环境变量覆盖。

### 关于 `trusted_proxies`

默认**留空**，含义是「不信任任何代理」，`ClientIP` 取 TCP 对端地址。

只有真正部署在 Nginx 等反向代理后面时才填写代理网段：

```yaml
security:
  trusted_proxies:
    - 127.0.0.1
    - 10.0.0.0/8
```

> **不要为了「拿到真实 IP」就填 `0.0.0.0/0`。** gin 默认信任所有代理，
> 会把客户端自报的 `X-Forwarded-For` 直接当成来源 IP 写进审计日志 ——
> 任何人都能伪造自己的审计来源。留空是安全默认。

## 2.6 日常命令速查

```bash
# ── 后端 ──────────────────────────────────
cd backend
go run ./cmd/server -config config.yaml      # 启动（默认会跑种子）
go run ./cmd/server -config config.yaml -seed # 只跑迁移 + 种子后退出
gofmt -l .                                   # 格式检查（输出为空才合格）
go vet ./...                                 # 静态检查
go build ./...                               # 编译
go test ./test/... -count=1                  # 全部测试

# ── 前端 ──────────────────────────────────
cd frontend
npm run dev          # 开发服务器 http://localhost:6801
npm run typecheck    # vue-tsc --noEmit
npm run build        # 类型检查 + 产出 dist/
npm run preview      # 预览生产产物（已配置 /api 代理）

# ── 端到端（需要服务已在运行）──────────────
node scripts/api-smoke.mjs                    # 默认 http://127.0.0.1:6802
node scripts/api-smoke.mjs http://host:port   # 指定实例
node scripts/browser-verify.mjs               # 需后端 + 前端都启动
APP_URL=http://localhost:4173 node scripts/browser-verify.mjs  # 验证生产产物
```

## 2.7 三层验证

本项目要求「改完必跑」，三层验证各有分工，**不能互相替代**：

| 层 | 命令 | 覆盖 | 规模 |
| --- | --- | --- | --- |
| 编译期 | `gofmt -l .` / `go vet ./...` / `npm run typecheck` | 语法、类型、常见误用 | — |
| 进程内 E2E | `go test ./test/... -count=1` | 行为契约（进程内 `httptest` 驱动完整路由） | **68 个测试 / 139 条用例** |
| 接口冒烟 | `node scripts/api-smoke.mjs` | 协议层（真实 HTTP、二进制响应、文件上传下载） | **219 项断言** |
| 浏览器运行时 | `node scripts/browser-verify.mjs` | 真实渲染（白屏、主题、控制台报错） | **80 项断言 / 15 个页面** |

### 为什么必须有浏览器验证

**编译通过不等于运行时正确。** 本项目真实出现过这两类缺陷，类型检查全绿：

- 冷启动直接落 404（根路由写了 `redirect`，抢在守卫注入动态路由之前解析）
- 主按钮白底白字（只给亮色补了颜色令牌，暗色下留白底）

所以：**改了前端路由 / 守卫 / 状态管理 / 样式主题，必须跑浏览器验证。**

### 冒烟与浏览器脚本的细节

- 两个脚本都是**零依赖**的：`api-smoke.mjs` 用 Node 原生 `fetch`；
  `browser-verify.mjs` 用原生 `WebSocket` 走 CDP 驱动系统已装的 Chrome，不需要 Playwright。
- 退出码非 0 表示有失败项，**可直接接入 CI**。
- 结果分别落盘到 `.api-smoke.log` 与 `scripts/.browser-verify.json`。
- 无头 Chrome 的 `prefers-color-scheme` 默认是 `dark`，脚本会显式固定基线，否则主题断言会漂移。

## 2.8 故障排查

| 症状 | 原因 | 解决 |
| --- | --- | --- |
| `连接 PostgreSQL 失败` | 口令 / 端口 / 服务未启动 | 核对 `config.yaml` 的 `database` 段；`psql -U postgres -c 'select 1'` |
| 启动报 `自动迁移表结构失败` | 老库缺列 / 权限不足 | 见 §3.3；`23502` 类错误是「新增 NOT NULL 列没带默认值」 |
| 端口被占用 | 有旧进程还在跑 | `netstat -ano \| grep :6802` 找 PID 后 `taskkill //PID <pid> //F` |
| 前端 404 / 接口报跨域 | 后端没起来或代理目标不对 | 确认 6802 在监听；检查 `.env.development` 的 `VITE_PROXY_TARGET` |
| 登录后被立刻弹回登录页 | 令牌没读到 | 令牌必须走 `utils/auth.ts`，不要在 `computed` 里直接读 `localStorage` |
| 刷新页面落 404 | 动态路由注入被 redirect 抢先 | 检查静态路由里是否出现了 `redirect` |
| 页面切换后状态没保留 | `keep-alive` 未命中 | 页面 `defineOptions({ name })` 必须与菜单 `name` 完全一致 |
| 批量导入提示「请选择文件」 | 中间件读请求体后没完整回填 | 检查请求体回填逻辑（`io.MultiReader`） |
| 日志里有中文但库里是乱码 | 按字节截断了多字节字符 | 写库前统一走 `textutil.Fit` |
| 某个角色只能看到自己 | 该角色的数据范围是「仅本人」 | 在角色管理里改数据范围；老库可重启让启动逻辑自动纠正 |

---

# 第三部分　新增业务说明

## 3.1 全景：加一个模块要动哪些文件

先看全局。以新增「文章管理（`article`）」为例，需要碰的文件如下 ——
**注意「新增」远多于「修改」，这正是模块化设计的目的**：

| # | 动作 | 文件 | 必填 |
| --- | --- | --- | --- |
| 1 | 新增 | `backend/internal/model/article.go` | 需要持久化时 |
| 2 | 注册 | `backend/internal/database/database.go`（加进 `AutoMigrate`） | 同上 |
| 3 | 新增 | `backend/internal/module/article/dto.go` | ✅ |
| 4 | 新增 | `backend/internal/module/article/service.go` | ✅ |
| 5 | 新增 | `backend/internal/module/article/handler.go` | ✅ |
| 6 | 新增 | `backend/internal/module/article/route.go` | ✅ |
| 7 | 新增 | `backend/internal/module/article/module.go`（自描述清单：命名空间 + Register + 菜单树） | ✅ |
| 8 | 可选 | `backend/internal/module/article/excel.go` | 需要导入导出时 |
| 9 | 修改 | `backend/internal/assembly/assembly.go`（加一行 `article.Module()`） | ✅ |
| 10 | 新增 | `backend/test/api_test.go` 里的 `TestArticleCRUD` | ✅ |
| 11 | 新增 | `frontend/src/api/article.ts` | ✅ |
| 12 | 修改 | `frontend/src/types/api.ts`（加类型） | ✅ |
| 13 | 新增 | `frontend/src/views/article/index.vue` | ✅ |

**一共只修改 2 个已有文件**（`assembly.go` 与 `types/api.ts`），其余全是新增。
加菜单/权限**不用碰 `seed.go`** —— 写在 `module.go` 的 `Menus` 里，启动时按权限码自动补齐。

## 3.2 后端九步

后端加一个模块固定是这九步，顺序不能乱：先建目录与文件，再写 model 与 DTO，
接着注册迁移，最后写清单挂路由、把菜单交给框架播种。
下面逐步给出每一步的命令与代码。

### Step 1 — 建目录与文件

```
internal/module/article/
├── module.go   # ★ 自描述清单：命名空间 + Register + 菜单树（框架靠它装配与播种）
├── dto.go      # 请求 / 响应结构体
├── service.go  # 业务逻辑（不感知 HTTP）
├── handler.go  # HTTP 入参出参
├── route.go    # 路由 + 权限码
└── excel.go    # 可选：导入导出
```

四文件制的意义：**职责边界靠文件名就能看清**。
`service` 不知道 `gin.Context` 的存在，所以它能被单测直接调用；
`handler` 只做「绑参 → 调 service → 出参」，不含业务判断。

`module.go` 让模块**自描述**：框架不必知道「article 这个包长什么样」，
只读登记表就能把路由挂上、把菜单播下去（见 Step 7）。

### Step 2 — 定义模型（需要落库时）

```go
// internal/model/article.go
package model

// Article 文章。
type Article struct {
	BaseModel                          // 自带 ID / CreatedAt / UpdatedAt / DeletedAt
	Title     string `gorm:"size:128;not null" json:"title"`
	Content   string `gorm:"type:text" json:"content"`
	Status    int16  `gorm:"not null" json:"status"`   // ★ 不要写 default:1，见 §3.8
	Sort      int    `gorm:"default:0" json:"sort"`
	AuthorID  int64  `gorm:"index" json:"authorId"`
}

func (Article) TableName() string { return "sys_articles" }
```

然后注册到迁移列表：

```go
// internal/database/database.go
func AutoMigrate() error {
	return db.AutoMigrate(
		&model.Dept{},
		&model.User{},
		// ...
		&model.Article{},   // ← 加这一行
	)
}
```

### Step 3 — `dto.go`：每个字段都要有 `label`

```go
package article

import "time"

// CreateReq 新增文章请求。
type CreateReq struct {
	Title   string `json:"title" binding:"required,max=128" label:"标题"`
	Content string `json:"content" binding:"omitempty" label:"正文"`
	Status  *int16 `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	Sort    int    `json:"sort" label:"排序"`
}

// UpdateReq 修改文章请求。
type UpdateReq = CreateReq

// Item 列表项。
type Item struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Status    int16     `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
```

> `label` 会被 `pkg/bind` 用来拼中文校验提示（如「标题长度不能超过 128」）。
> **漏写标签会让用户看到英文报错。**

### Step 4 — `route.go`：权限码写在路由上

```go
package article

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
)

// Register 注册文章模块路由。
func Register(rg *gin.RouterGroup) {
	h := NewHandler(NewService())
	g := rg.Group("/articles", middleware.Auth())
	{
		g.GET("", middleware.RequirePerm("article:list"), h.List)
		g.GET("/export", middleware.RequirePerm("article:export"), h.Export) // ★ 静态段在前
		g.GET("/:id", middleware.RequirePerm("article:list"), h.Detail)
		g.POST("", middleware.RequirePerm("article:create"), h.Create)
		g.PUT("/:id", middleware.RequirePerm("article:update"), h.Update)
		g.DELETE("/:id", middleware.RequirePerm("article:delete"), h.Delete)
	}
}
```

**两个必须记住的规则**：

1. 权限码格式固定 **`模块:资源:动作`**；`RequirePerm` 支持传多个码，语义是「任一命中即可」。
2. **静态路径段必须注册在 `/:id` 之前。** 顺序反了的话，`/articles/export` 会先命中 `/:id`，
   `id` 拿到字符串 `"export"`，报「路径参数 id 非法」—— 这个错误信息完全指不到真正的原因。

### Step 5 — `handler.go`：固定骨架

```go
// Create 新增文章
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增文章") // ★ 操作日志里的动作名，写操作必须设置
	id, err := h.svc.Create(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}
```

出参只用这四个函数：

| 函数 | 用途 |
| --- | --- |
| `response.OK(c, data)` | 成功，返回 data |
| `response.OKPage(c, list, total, page, pageSize)` | 分页成功 |
| `response.OKMsg(c, "修改成功")` | 成功，仅返回提示语 |
| `response.Fail(c, err)` | 失败，从 `*errcode.Error` 自动取码 |

> `response.Fail` 会把底层错误（含 traceId）写进日志，所以**不要在 service 里重复打日志**。

**写操作必须设 `opAction`。** 操作日志中间件优先取它作为动作名；
不设的话只能退化成「POST /api/v1/articles」这种没有业务含义的记录。

### Step 6 — `service.go`：错误统一用 `errcode`

```go
func (s *Service) Create(req CreateReq) (int64, error) {
	if err := s.ensureTitleFree(req.Title); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	a := model.Article{Title: req.Title, Content: req.Content, Status: status, Sort: req.Sort}
	if err := s.db.Create(&a).Error; err != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return a.ID, nil
}
```

错误码的用法：

```go
if exists {
	return 0, errcode.New(errcode.ErrConflict, "文章标题已存在")   // 前端看到这句话
}
return 0, errcode.Wrap(errcode.ErrDatabase, err)                 // 前端看到"数据库操作失败"，
                                                                 // 真实原因写进日志
```

`Wrap` 与 `New` 的差别是**信息暴露程度**：`Wrap` 把底层错误藏进日志，
只在响应里给出安全的通用提示。涉及数据库、外部调用的错误一律用 `Wrap`。

### Step 7 — 写 `module.go` 清单，并在组装层登记

先写模块自己的清单文件 —— **菜单、按钮权限点都归声明它们的模块所有**：

```go
// internal/module/article/module.go
package article

import "nova-erp/internal/module"

// Module 返回文章模块的自描述清单。
func Module() module.Module {
	return module.Module{
		Namespace: "article",   // ★ 下面所有权限码必须以它开头，Build 会校验
		Register:  Register,    // 即 Step 4 的 route.go:Register
		Menus: []module.MenuNode{
			// 目录节点只做分组，不产生路由
			{Name: "ArticleRoot", Title: "内容管理", Icon: "Document",
				Path: "/article", Redirect: "/article/list", Sort: 4,
				Children: []module.MenuNode{
					{Name: "Article", Title: "文章管理", Icon: "Notebook",
						Path: "/article/list", Component: "article/index",
						Permission: "article:list", KeepAlive: true,
						Children: []module.MenuNode{
							{Name: "ArticleCreate", Title: "新增文章",
								Type: module.TypeButton, Permission: "article:create"},
							{Name: "ArticleDelete", Title: "删除文章",
								Type: module.TypeButton, Permission: "article:delete"},
						}},
				}},
		},
	}
}
```

字段与旧写法（`menu(id, parent, …)` 手工分配 ID）的差别：
**没有 ID 了** —— 主键交给数据库自增，靠 `Permission`（按钮）或 `Name`（目录/页面）做幂等键。
`Type` 省略即 `menu`（页面），目录节点写 `TypeDir`，按钮写 `TypeButton`。

然后到组装层加一行：

```go
// internal/assembly/assembly.go
func Modules(cfg *config.Config) []module.Module {
	return []module.Module{
		// ... 框架模块
		module.Group(
			module.MenuNode{Name: "System", Title: "系统管理", Icon: "Setting",
				Path: "/system", Redirect: "/system/user", Sort: 2},
			user.Module(), role.Module(), menu.Module(), dept.Module(), log.Module(cfg.Security),
		),
		article.Module(),   // ← 加这一行；业务模块归在顶层就写在这
	}
}
```

**`internal/router/router.go` 不用动** —— 它只遍历 `module.All()` 挨个调 `Register`；
`seed.go` 也不用动，它同样遍历登记表播种菜单。

`module.Build()` 会在启动时校验清单（越界即启动失败，不是运行到一半才 404）：

| 校验项 | 反例 |
| --- | --- |
| 权限码必须以 `Namespace` 开头 | `Namespace: "article"` 却写 `system:order:list` |
| 路由名 / 路径 / 组件路径全局唯一 | 两个模块都叫 `ArticleRoot` |
| 按钮节点必须有权限码 | `TypeButton` 但没有 `Permission` |
| 页面节点必须有组件路径 | 有 `Path` 却没有 `Component` |
| 组件路径不带 `.vue`、不带前导 `/` | `"article/index.vue"`、`"/article/index"` |
| 分组节点必须带一个路由函数 | 否则整组接口静默 404，编译期查不出来 |

> **`module.Group` 的坑**：分组要同时汇总子模块的 `Register` 与 `Menus`。
> 只收菜单不收路由的话，菜单点得进去、接口全 404，而且**没有任何编译错误** ——
> 所以契约单测里专门有一条 `TestGroupComposesRoutesAndMenus` 钉住它。

### Step 8 — 分页与排序

```go
q := page.FromContext(c)
q.Normalize()   // 补齐默认值：pageSize 封顶 200，page 按 OFFSET 上界（100 万）反推封顶

// 白名单：裸列名 → 排序表达式，杜绝 SQL 注入
var articleOrderWhitelist = map[string]string{
	"createdAt": "created_at",
	"sort":      "sort",
	"title":     "title",
}

order := q.SafeOrderBy(articleOrderWhitelist, "sort DESC, created_at DESC")
s.db.Offset(q.Offset()).Limit(q.Limit()).Order(order)
```

> **列表查询不要写 `LEFT JOIN` + `Select("主表.*")`。**
> 它会与 `Preload` 生成的派生表别名冲突，导致 500。
> 需要按关联表过滤时，用 `Where("dept_id IN (?)", subQuery)` 子查询代替。
>
> **`Normalize` 里一定要同时夹住 `page`**：只封 `pageSize` 的话，
> `?page=99999999` 会算出约 2×10⁹ 的 OFFSET，数据库要先扫描丢弃 2 亿行才能返回结果，
> 单请求即可占满连接池；更大的值还会让 `(Page-1)*PageSize` 整型溢出成负数。
> 见 `AGENTS.md` §5.41。

### Step 9 — 写测试

在 `test/api_test.go` 追加一个 `TestArticleCRUD`，用现成的 `call(t, method, path, token, body)` 基建：

```go
func TestArticleCRUD(t *testing.T) {
	setup(t)
	token := loginAs(t, adminAccount, adminSecret)

	created := call(t, http.MethodPost, "/api/v1/articles", token, map[string]any{
		"title": "测试文章", "status": 1,
	})
	assertCode(t, created, 0, "新增文章")
	id := num(created.obj(t), "id")

	detail := call(t, http.MethodGet, "/api/v1/articles/"+itoa(id), token, nil)
	assertCode(t, detail, 0, "文章详情")

	deleted := call(t, http.MethodDelete, "/api/v1/articles/"+itoa(id), token, nil)
	assertCode(t, deleted, 0, "删除文章")
}
```

> 测试库与开发库隔离（`nova_inv_v5_test`），可以放心造数据。
> 但**测试里造的数据要清理干净**，否则重复运行会撞唯一约束。

## 3.3 数据库变更

数据库变更分三类：**给已有表加字段**、**加唯一约束**、**修正老库历史数据**。
前两类有必须遵守的规则，第三类只在升级已上线的库时才用得上。依次来看。

### 新增字段

改 model 后重启服务即可，`AutoMigrate` 会自动补列。但有一条**必须遵守**的规则：

> **给已有数据的表新增 `NOT NULL` 列时，必须带 `default`。**

否则 PostgreSQL 会因为存量行取到 NULL 而直接拒绝迁移：

```
column "token_epoch" of relation "sys_users" contains null values (SQLSTATE 23502)
```

这类缺陷的表现极具迷惑性：**新库一切正常，老库起不来。**

至于「默认值是否安全」，判断标准是**零值语义与默认值是否一致**：

| 字段 | 零值 | 默认值 | 安全？ |
| --- | --- | --- | --- |
| `status` | `0` = 禁用 | `1` = 启用 | ❌ **相反，不能加默认值** |
| `sort` | `0` = 排最前 | `0` | ✅ |
| `parent_id` | `0` = 根节点 | `0` | ✅ |
| `token_epoch` | `0` = 从未改过密码 | `0` | ✅ |

### 新增唯一约束

**不要用 GORM 的 `uniqueIndex` 标签**，用「部分唯一索引 + 软删除」：

```go
// internal/database/database.go
var partialUniqueIndexes = []struct {
	name, table, legacy, columns, where string
}{
	// ...
	{name: "uk_sys_articles_title", table: "sys_articles",
		legacy: "", columns: "title", where: "deleted_at IS NULL"},
}
```

原因：这张表启用了软删除。普通唯一索引会把已删除的历史行也算进去，
导致「删除后无法再创建同名记录」这个反直觉的缺陷。

### 老库数据修正

如果这次改动需要**修正已有数据**（而不只是改结构），本项目没有独立 migration 目录 ——
把修正逻辑挂到 `seed.Run()` 的启动路径上。

`seed.syncBuiltinRoleDataScope()` 是现成的范例，它遵守三条约束：

1. **只认历史错误值** —— `Where` 里带上旧值，匹配不到就不写。
   写成无条件 `UPDATE ... SET x = 新值` 会在**每次重启**都回退管理员的显式配置。
2. **幂等** —— 重复启动零变更。
3. **可扩展** —— 加一张声明表即可：

```go
var historicalRoleDataFixes = []struct {
	code   string
	from   int16
	to     int16
	reason string
}{
	{code: "viewer", from: model.DataScopeSelf, to: model.DataScopeAll, reason: "..."},
}
```

**这类逻辑必须有测试。** 「新库能跑」证明不了「老库能升上来」——
老库才走的分支，全新初始化的库永远碰不到。
`backend/test/upgrade_test.go` 是现成模板，做法是主动把表或数据退回成老样子，再走真实启动路径。

## 3.4 菜单与权限配置

后端写完了，但**前端还看不到任何东西**。原因是前端路由完全由菜单驱动。

好消息是：**菜单不用再手工加了。** 从上一步的 `module.go` 起，菜单已经跟着模块走 ——
启动时 `seed` 遍历登记表，把每个模块的 `Menus` 自动播进库里。

### 菜单怎么入库（幂等三态）

播种时每个节点按幂等键（按钮用 `Permission`，目录/页面用 `Name`）在库里查一次，分三种情况：

| 情况 | 处理 | 结果 |
| --- | --- | --- |
| 命中**未删除**的记录 | 复用原 ID，**不覆盖任何字段** | 管理员改过的文案/图标不会被冲掉 |
| 命中**已软删**的记录 | 打印告警，**跳过整棵子树** | 管理员删掉的菜单不会「复活」 |
| 都没命中 | 新建，主键由数据库自增 | 新权限点自动补齐，不必重建库 |

> 所以**新增一个按钮权限，只需要在 `module.go` 的 `Children` 里加一行**；
> 内置菜单的新 ID 由自增分配，老库已有的 ID 原样保留 —— 不再有「1–999 空档不够用」的烦恼。

字段要求：

| 字段 | 要求 |
| --- | --- |
| `Name` | 大驼峰、全局唯一，**必须与页面 `defineOptions({ name })` 一致** |
| `Path` | 页面/目录的路由路径；按钮不填 |
| `Component` | `src/views` 下的相对路径，**无扩展名、无前导斜杠** |
| `Permission` | `模块:资源:动作`，**必须以本模块 `Namespace` 开头**，且与 `route.go` 里的完全一致 |
| `Type` | 省略 = 页面（`menu`）；目录写 `module.TypeDir`；按钮写 `module.TypeButton` |
| `Redirect` | 目录节点建议指向第一个子页面 |

> 在**菜单管理页**手工建菜单时，权限码不用自己拼：表单把「模块 : 资源 : 动作」拆成三段，
> 自动继承父级的模块段（按钮还会带上资源段），组件路径也是从 `import.meta.glob` 的同一份
> 清单里下拉选，选得到就一定加载得到。

### 授权给角色

授权规则也不再抄 ID 清单，而是**按节点语义求解**（`seed.go` 的 `grantRules`）：

```go
var frameworkNamespaces = []string{"system", "profile", "auth", "inv"}

var grantRules = []grantRule{
	{roleID: 1, name: "超级管理员", match: func(string, string) bool { return true }},
	{roleID: 2, name: "系统管理员",   // 框架命名空间内，除 *:delete 外全部
		namespaces: frameworkNamespaces,
		match:      func(_, permission string) bool { return !strings.HasSuffix(permission, ":delete") }},
	{roleID: 3, name: "只读访客",     // 框架命名空间内，非按钮节点（即各列表菜单）
		namespaces: frameworkNamespaces,
		match:      func(menuType, _ string) bool { return menuType != model.MenuTypeButton }},
}
```

`resolveGrants()` 在**运行库现有菜单**上求解出具体 ID，每次启动补齐缺失的授权
（只增不减，不覆盖管理员手工做过的裁剪）。

两条设计约束，别踩：

1. **`frameworkNamespaces` 决定内置角色「自动获得谁」。**
   业务模块（如 `article`）**默认不会**自动进入系统管理员和只读访客的授权范围 ——
   否则新增一个敏感业务模块就等于给所有老角色静默越权。要发，就把命名空间加进
   `frameworkNamespaces`（进销存 `inv` 就是这么加进去的），或到「角色管理」页手工分配。
2. **只读角色不要授予任何按钮权限。** 「能查看列表」与「能批量导出」是两个风险等级 ——
   导出等于把整批数据带走。

## 3.5 前端接线

前端三件事：**加接口封装 → 加类型 → 建页面**。
**不需要改任何路由文件**（路由由菜单生成）。

### 1. 接口封装

```ts
// src/api/article.ts
import { http } from '@/utils/request'
import type { ArticleItem, ArticleQuery, ArticleForm, PageResult } from '@/types/api'

export const articleApi = {
  list: (params: ArticleQuery) =>
    http.get<PageResult<ArticleItem>>('/v1/articles', params as Record<string, unknown>),
  detail: (id: number) => http.get<ArticleItem>(`/v1/articles/${id}`),
  create: (data: ArticleForm) => http.post<{ id: number }>('/v1/articles', data),
  update: (id: number, data: ArticleForm) => http.put<string>(`/v1/articles/${id}`, data),
  remove: (id: number) => http.delete<string>(`/v1/articles/${id}`),
}
```

三点注意：

- **路径不带 `/api` 前缀** —— `baseURL` 已由 `.env` 的 `VITE_API_BASE_URL` 提供。
- **`http.get` 的第二个参数直接就是查询参数对象**，不要再包一层 `{ params }`。
  `http.get<T>(url, params?, config?)` 的第三个参数才是 axios 配置。
- 分页统一用现成的 `PageResult<T>`（定义在 `types/api.ts`）。

### 2. 类型定义

```ts
// src/types/api.ts
/** 与后端 Item 结构一一对应 */
export interface ArticleItem {
  id: number
  title: string
  status: number
  createdAt: string
}

/** 列表查询参数 */
export interface ArticleQuery {
  page: number
  pageSize: number
  keyword?: string
}

/** 新增 / 修改表单 */
export interface ArticleForm {
  title: string
  content?: string
  status?: number
  sort?: number
}
```

类型要与后端 JSON 字段**完全一致（含大小写）**：`deptId`、`lastLoginAt`、`createdAt`。

> **新增字段时先改 `types/api.ts`。** 它是前后端的契约落点，
> 先定义类型再写页面，`vue-tsc` 才能在编译期就挡住字段拼写错误。

### 3. 页面组件

```vue
<!-- src/views/article/index.vue -->
<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { articleApi } from '@/api/article'

defineOptions({ name: 'Article' })   // ★ 必须与菜单 name 完全一致，否则 keep-alive 失效

const list = ref([])
const loading = ref(false)

const query = reactive({ page: 1, pageSize: 10, keyword: '' })

async function load() {
  loading.value = true
  try {
    const data = await articleApi.list(query)
    list.value = data.list
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="article-page">   <!-- ★ 根节点保持稳定的语义化 class -->
    <el-button v-perm="'article:create'" type="primary">新增文章</el-button>
    <el-table v-loading="loading" :data="list">
      <el-table-column prop="title" label="标题" />
    </el-table>
  </div>
</template>
```

前端的三个命名/结构约定：

| 约定 | 原因 |
| --- | --- |
| 根节点固定 `<div class="xxx-page">` | 避免 transition 进场动画残留导致白屏 |
| `defineOptions({ name })` 与菜单 `name` 一致 | 不一致则 `keep-alive` 的 `:include` 匹配不上，页面状态不保留 |
| `v-perm` 控制按钮显隐 | 只防误点，**真正的拦截在后端** |

### 4. 样式

**不要写死颜色**，一律引用 `styles/variables.scss` 的语义令牌：

```scss
.article-page {
  color: var(--app-text-primary);
  background: var(--app-bg-container);
}
```

> 新增颜色令牌时**亮/暗两套必须同时补**。只补一套的后果是暗色下留一块白底，
> 编译器不会报错，只能靠浏览器验证脚本的亮度扫描发现。

## 3.6 完整实战：article 模块走一遍

把上面九步串起来，实际执行顺序建议如下。

**第一步：后端数据层**

```bash
# 1. 建 model → 2. 注册 AutoMigrate → 3. 重启后端
cd backend
go run ./cmd/server -config config.yaml -seed
```

重启后 `sys_articles` 表会自动建好。

**第二步：后端接口层**

```bash
# 新建 internal/module/article/{module,dto,service,handler,route}.go
# 在 internal/assembly/assembly.go 的模块清单里加一行 article.Module()
cd backend
gofmt -l . && go vet ./... && go build ./...
```

**第三步：菜单与权限（已随模块自动生效）**

菜单写在 `module.go` 的 `Menus` 里，不用再改 `seed.go`；授权规则按命名空间语义求解。
只要重启即可：

```bash
go run ./cmd/server -config config.yaml -seed
```

重启后登录 `admin`，侧边栏应出现「内容管理 → 文章管理」。
若业务模块要发给系统管理员/只读访客，把 `article` 加进 `seed.go` 的 `frameworkNamespaces`
（或到「角色管理」页手工分配）。

**第四步：前端页面**

```bash
# 新增 src/api/article.ts、src/types/api.ts 加类型、src/views/article/index.vue
cd frontend
npm run typecheck
npm run dev
```

刷新浏览器（hash 模式下改路由不会重载文档，动态路由需要重新登录或刷新才会重新拉取）。

**第五步：验证**

```bash
# 后端测试
cd backend && go test ./test/... -count=1

# 接口冒烟（服务运行中）
node scripts/api-smoke.mjs

# 浏览器验证（前后端都运行中）
node scripts/browser-verify.mjs
```

**第六步：用受限账号确认权限真的生效**

用 `demo` 登录，确认：新增/删除按钮**不显示**；
即使手工构造请求，后端也返回 `40300 缺少权限`。

这一步不能省 —— 它是「前端隐藏」与「后端拦截」是否**都**生效的唯一验证。

## 3.7 提交前检查清单

```
后端
[ ] gofmt -l .              输出为空
[ ] go vet ./...            无告警
[ ] go build ./...          编译通过
[ ] go test ./test/... -count=1   全部通过
[ ] 新增写接口都挂了 RequirePerm
[ ] 新增写操作都在 handler 里设了 opAction
[ ] 写库前的文本都过了 textutil.Fit
[ ] 新增 NOT NULL 列都带了 default
[ ] 唯一约束用的是部分唯一索引
[ ] 新增模块有 module.go 清单，已在 internal/assembly/assembly.go 登记
[ ] 权限码以模块 Namespace 开头（module.Build 会在启动时校验）
[ ] 测试数据已清理

前端
[ ] npm run typecheck       无错误
[ ] npm run build           构建通过
[ ] 页面 defineOptions({ name }) 与菜单 name 一致
[ ] 根节点是稳定的 <div class="xxx-page">
[ ] 没有写死颜色，全部用 CSS 变量
[ ] 新颜色令牌亮/暗两套都补了
[ ] 按钮级权限用了 v-perm
[ ] 状态变更类操作有取消/失败回滚（写回绑定值 + 以服务端为准）
[ ] 会改筛选条件的页面，在 search/resetQuery 后清了表格选择
[ ] 表单提交前有 loading 前置守卫 + formRef 存在性兜底
[ ] 提交按钮所在的 el-form 有 validate，且长度类规则挂了 change 触发
[ ] 表单错误提示会聚焦到第一个出错字段

文档与验证
[ ] AGENTS.md 的测试计数已更新
[ ] 接口有变动：handler 注解已补，并执行 scripts/gen-swagger.sh 重新生成（backend/api/docs/ 一并提交）
[ ] docs/CHANGELOG.md 已写本次版本（改了什么、动了哪些表、要不要手工步骤）
[ ] 接口有变动则同步 docs/API.md
[ ] 表结构有变动则同步 docs/DATABASE.md
[ ] 跑了浏览器验证（改了路由/守卫/主题时必跑）→ node scripts/browser-verify.mjs
[ ] 依赖方向检查（改了框架层时必跑，见 docs/UPGRADE.md §2.2）：
    bash scripts/check-module-boundary.sh
    有输出 = 框架反向依赖了具体模块，必须整改
[ ] 改了 switch/checkbox/表单/分页/选择类交互时跑了交互验证 → node scripts/interaction-verify.mjs
```

## 3.8 必读的八个坑

完整 61 条见 [`AGENTS.md` §5](../AGENTS.md)。以下八条是新增业务时**最常撞上**的：

| # | 坑 | 症状 | 正确做法 |
| --- | --- | --- | --- |
| 1 | 静态路径注册在 `/:id` 之后 | `/articles/export` 报「路径参数 id 非法」 | 静态段写在 `/:id` 之前 |
| 2 | 前端常量路由写了 `redirect` | 冷启动 / F5 直接落 404 | 静态路由**一律不写** `redirect`；注入路由后 `next({ path: to.path, ... })` |
| 3 | 在 `computed` 里直接读 `localStorage` | 登录成功却被弹回登录页 | 令牌统一走 `utils/auth.ts` 的模块级 `ref` |
| 4 | 菜单 `name` 与页面 `defineOptions({ name })` 不一致 | 切走再回来页面状态丢失 | 两处保持完全一致 |
| 5 | 唯一性用 `uniqueIndex` 标签 | 删除后无法再建同名记录 | 用部分唯一索引 `WHERE deleted_at IS NULL` |
| 6 | `status` 字段加了 `default:1` | 「禁用」被静默写成「启用」 | 零值与默认值语义相反的字段**不加 default** |
| 7 | 用 `s[:n]` 截断写库文本 | 中文被切坏 → 数据库拒收整行 | 统一走 `pkg/textutil.Fit` |
| 8 | 给老库加 `NOT NULL` 列没带默认值 | 新库正常，**老库起不来** | 带 `default`，并写 `upgrade_test.go` |

## 3.8.1 前端交互必读的五个坑

这五条都是 Element Plus 受控组件带来的，**编译与类型检查都发现不了**，
只会在用户点下去的那一刻才暴露。对应 `AGENTS.md` §5.44–§5.53。

| # | 坑 | 症状 | 正确做法 |
| --- | --- | --- | --- |
| 9 | 取消确认 / 请求失败后没回写绑定值 | `el-switch` 看起来变了，实际没变 | 两条路径都 `row.status = origin`，`finally` 里 `loadList()` 兜底 |
| 10 | 状态按钮不拦连点 | 狂点发出串行翻转请求 | `submitting: Set<id>` + `:loading="isSubmitting(id)"` |
| 11 | `reserve-selection` 在改筛选条件后没清 | 「导出选中」导出**两个查询条件混合**的数据 | `search()` / `resetQuery()` / 删除成功后 `clearSelection()` |
| 12 | `el-pagination` 改 pageSize 双发 `change` | 一次操作发两遍列表请求 | 组件层同 tick 微任务归并 `emitChangeOnce()`，**别用防抖** |
| 13 | `formRef.value?.validate()` 返回 `undefined` | 首屏点登录**毫无反应** | 显式 `if (!formRef.value)` 兜底 + `focusFirstInvalidField()` |

第 12 条的共性启示：**这类缺陷优先在公共组件层修**。改 `Pagination.vue` 一处，
user/role/dept/menu 四个页面同时受益，不要每个页面各修一遍。

第 13 条的通用规律：**`?.` 和 `&&` 一样，都是静默的失败通道**。
链式调用里只要有一步可能缺席，就必须显式处理缺席分支，
否则错误会被当成「正常但不通过」而无声消失。

### 用浏览器验证这类交互

受控组件**无法用合成事件驱动**——`element.click()`、`dispatchEvent(new MouseEvent('click'))`
都不会触发 Vue 响应式更新，DOM 上 `.is-checked` 变了但业务计数不变，
验证会得到假阴性。必须走真实坐标点击：

```js
// scripts/browser-verify.mjs 里的做法
await cdp('Input.dispatchMouseEvent', { type: 'mousePressed',   x, y, button: 'left', clickCount: 1 })
await cdp('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button: 'left', clickCount: 1 })
```

另注意：`v-perm` 隐藏的元素走 `visibility: hidden`，此时 **`innerText` 取不到内容**。
验证计数请断言 DOM 状态（如 `querySelectorAll('.is-checked').length`）而不是文案。

**脚本报 FAIL 时，先证明脚本是对的再改产品代码**——
把修复临时退回去，确认用例真的会 FAIL，这一步能省掉大量无效排查。

## 3.8.2 模块化之后新增的三条坑

这三条是本次「模块自描述清单」改造带来的，对应 `AGENTS.md` §5.59–§5.61。

| # | 坑 | 症状 | 正确做法 |
| --- | --- | --- | --- |
| 14 | 手工分配的菜单 ID 是隐式契约 | 菜单清单与授权清单一处改歪 → **静默权限错配**（点得进去、接口 403） | 别抄第二份 ID；改为在 `module.go` 里声明 + 按语义求解授权 |
| 15 | 框架层 import 了具体模块 | 那个模块从此**删不掉**（删了框架编译不过） | 模块名只出现在 `internal/assembly`；其余框架包只认契约类型 `module.Module` |
| 16 | `module.Group` 只汇总菜单没收路由 | 菜单点得进去、**该组接口整块 404**，且无编译错误 | 分组必须带上子模块的 `Register`（已由 `contract_test.go` 钉住） |

一句话记住边界的心智模型：
**`internal/assembly` 是唯一「知道有哪些模块」的地方，其他所有包只认「一份契约」。**
这也是「框架能独立于业务升级」的地基 —— 删除任何业务模块，框架的 diff 都是零。

## 3.8.3 弹窗尺寸与表单规范

弹窗尺寸**按四档统一**，不要随手给一个数。此前 19 个弹窗用了 19 个不同宽度
（420 / 480 / 520 / 560 / 600 / 620 / 640 / 680 / 720 / 860 / 900 / 960 / 1000 / 1180 …），
同一个系统里点开不同弹窗宽度乱跳，观感是很直接的「不成体系」。

| 档 | 宽度 | 用在哪 | 现有 |
| --- | --- | --- | --- |
| `sm` | **640px** | 3–5 个字段的简单表单 | 重置密码、新增/编辑分类、登记收付款、打印模板编辑、部门、分配菜单 |
| `md` | **800px** | 常规表单 | 用户、角色、菜单、往来单位、单据详情 |
| `lg` | **1100px** | 字段较多、内含网格的表单 | 商品档案（规格卡片是 6 列网格） |
| `xl` | **1280px** | 带明细表格 / 双栏布局 | 采购销售单、出入库单、模板设计器、三个打印预览、导入用户 |

另外两条统一规则：

- **`top` 统一 `6vh`**（打印类 `4vh`，纸面需要更多高度）。不写 top 会贴着顶部，
  观感与其它弹窗不一致。
- **弹窗主体统一最大高度 + 内部滚动**，在 `styles/element.scss` 里定义
  （`max-height: calc(100vh - 240px); overflow-y: auto`）。
  此前只有打印预览写了 `max-height`，别的弹窗在矮屏上会把底部按钮顶出视口，
  用户只能去缩浏览器窗口。同时 `.el-dialog` 有 `max-width: calc(100vw - 32px)`，
  窄屏上不溢出。

> **必填项该不该存在，先问「不填会怎样」。**
> 商品档案的规格与单位曾经是硬必填（至少一个规格、每个规格至少一个单位、
> 恰有一个基本单位、基本单位换算率恒为 1），录入时得先把这些想清楚。
> 但真正的约束不在用户身上：规格是库存的归属层、单位是单据折算的基准 ——
> 缺了它们，商品会进不了开单的商品下拉，**系统自己没法工作**。
> 所以正确做法是服务端 `normalizeSpecs` 兜底（补「默认 / 个」、
> 把第一个单位提为基本单位、把基本单位换算率钉成 1），而不是把它转嫁成
> 用户面前的四个必填提示。**留下的硬约束只有「不能重名」** ——
> 那是真的会算错账（两行「φ20」把库存劈成两半）。

## 3.8.4 Element Plus 按需导入

`element-plus` 的**根入口**顶层会执行 `makeInstaller([...全部组件])`，
这个副作用调用让 rollup 无法摇树 —— 只要有一处 `import ... from 'element-plus'`，
整个组件库（约 950KB）就全进包。官方 `ElementPlusResolver` 生成的正是根入口导入，
所以这里自己写了 `ElementPlusDirectResolver`（`vite.config.ts`），
按**子路径**导入：

```ts
// 生成：import { ElSelect } from 'element-plus/es/components/select/index'
//       + sideEffects 注入 element-plus/es/components/select/style/css
```

效果：element chunk **961 → 609 kB**（gzip 308 → 195 kB）。
剩下的 609KB 是用到的组件本身（date-picker 含 dayjs、table、tree、upload 等），
不是没摇干净 —— 判据是「未使用的组件名是否出现在产物里」。

三条约束，新增页面时照着做：

1. **模板里的 `el-*` 会被自动解析**，不用手工 import，也不用 `app.use(ElementPlus)`。
2. **`ElMessage` / `ElMessageBox` / `v-loading` 不走模板**，要显式导入 ——
   且必须走子路径，类型可以留在根入口（`import type` 编译期擦除，不产生运行时导入）：
   ```ts
   import { ElMessage } from 'element-plus/es/components/message/index'
   import { ElMessageBox } from 'element-plus/es/components/message-box/index'
   import type { FormInstance, FormRules } from 'element-plus'
   ```
   它们的样式也要在 `main.ts` 手工引（见那里的三行注释）。
3. **用到新的子组件时，先确认它有没有独立目录**。
   像 `el-table-column` / `el-form-item` / `el-option` / `el-descriptions-item`
   在 `node_modules/element-plus/es/components/` 下**没有**自己的目录，
   要映射到父组件的 barrel（`EP_PARENT`），否则解析到一个不存在的模块、
   **构建直接失败**：
   ```bash
   ls node_modules/element-plus/es/components/ | grep descriptions   # 只有 descriptions
   ```
   `el-descriptions-item` 就是这样补进 `EP_PARENT` 的 —— 上游没踩到是因为
   它的仪表盘不用 descriptions，而进销存的单据详情用了。

> **dev server 的一个环境坑**：Vite 重新优化依赖时会把缓存目录
> （`node_modules/.vite/deps`，上百个文件）整体删除重建。若运行环境对
> 「单次批量删除超过 N 个文件」有安全拦截，dev server 会在启动阶段被拦下
> （`vite build` / `vite preview` 不受影响，它们不碰这个目录）。
> 症状是控制台一句 `error when starting dev server` 加一条删除守卫的报错，
> **与项目代码无关**。在普通终端里 `npm run dev` 不会有这个问题。

---

## 3.9 接口文档（Swagger / OpenAPI）

接口文档由 **handler 上的注解自动生成**，不再靠手工维护（手工版 `docs/API.md` 只保留业务语义与
边界说明，两者配合阅读）。

### 怎么用

服务启动后访问 `/swagger/index.html`。是否挂载由 `app.swagger` 决定，
不写则跟随 `mode`（`debug` 打开、`release` 关闭）：

```yaml
app:
  mode: release
  swagger: true    # 生产上确实需要时再显式打开，务必配上网关鉴权
```

`docs.SwaggerInfo.BasePath` 在启动时会同步成 `app.prefix + app.version`，
所以改了 `app.version` 之后文档里的路径不会跑偏。

### 新增接口时怎么注解

照抄同一个模块里已有 handler 的写法即可，要素是：

```go
// List 用户列表
// @Summary      分页查询用户
// @Description  结果会叠加当前登录用户的数据范围。
// @Tags         用户管理
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int     false  "页码"  default(1)
// @Param        body      body      CreateReq  true "新增参数"
// @Success      200       {object}  response.Body{data=response.PageData{list=[]user.VO}}
// @Failure      403       {object}  response.Body  "缺少权限"
// @Router       /system/users [get]
func (h *Handler) List(c *gin.Context) {
```

四个要点：

1. **统一响应体要包一层**：真实返回是 `{code, message, data, traceId}`，
   所以 `@Success` 必须写 `response.Body{data=...}`，否则生成的 schema 与真实返回不符。
   只返回 ID 用 `response.IDData`，清空类用 `response.DeletedData`，分页用
   `response.PageData{list=[]xxx.VO}`。
2. **路径不带前缀**：`@Router` 写 `/system/users`，前缀由 `main.go` 的 `@BasePath` 提供。
3. **需要登录的接口加 `@Security BearerAuth`**，公开接口（登录、刷新）不加。
4. **不要用 `gin.H` 作为返回**：它是 map，Swagger 无法推导字段，用 `response.IDData` 这类具体类型。

### 重新生成

```bash
bash scripts/gen-swagger.sh        # 内部就是 swag init --parseInternal
```

`--parseInternal` 不能省：业务代码都在 `internal/` 下，不加这个参数 swag 会整体跳过，
生成的文档只剩通用信息、一个接口都没有。

生成物在 `backend/api/docs/`（`docs.go` / `swagger.json` / `swagger.yaml`），**需要一并提交**，
这样没装 `swag` 的环境也能直接跑起文档。

---

# 附录

## A. 接口总表（47 个）

前缀 `/api/v1`。「权限」列中 `|` 表示任一命中即可。
另有 `/health`（健康检查）与 `/swagger/*`（在线文档）不在业务接口之列。
接口的入参/出参结构以在线文档（`/swagger/index.html`）为准，本表只做导航。

### 认证与个人中心

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/auth/login` | 公开 | 登录 |
| POST | `/auth/refresh` | 公开 | 刷新令牌 |
| POST | `/auth/logout` | 仅登录 | 退出登录 |
| GET | `/profile` | 仅登录 | 当前用户信息 |
| PUT | `/profile` | 仅登录 | 修改个人资料 |
| PUT | `/profile/password` | 仅登录 | 修改本人密码 |
| GET | `/profile/routes` | 仅登录 | 动态路由树 |

### 用户管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/system/users` | `system:user:list` | 列表 |
| GET | `/system/users/options` | `system:user:list` \| `system:role:list` \| `system:dept:list` | 下拉选项 |
| GET | `/system/users/export` | `system:user:export` | 导出 |
| GET | `/system/users/import-template` | `system:user:import` | 下载模板 |
| POST | `/system/users/import` | `system:user:import` | 批量导入（`?dryRun=1` 仅校验） |
| GET | `/system/users/:id` | `system:user:list` | 详情 |
| POST | `/system/users` | `system:user:create` | 新增 |
| PUT | `/system/users/:id` | `system:user:update` | 修改 |
| DELETE | `/system/users/:id` | `system:user:delete` | 删除 |
| PUT | `/system/users/:id/status` | `system:user:update` | 启用 / 禁用 |
| PUT | `/system/users/:id/password` | `system:user:reset` | 重置密码 |

### 角色管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/system/roles` | `system:role:list` | 列表 |
| GET | `/system/roles/options` | `system:role:list` \| `system:user:list` | 下拉选项 |
| GET | `/system/roles/export` | `system:role:export` | 导出 |
| GET | `/system/roles/:id` | `system:role:list` | 详情（含菜单 ID） |
| POST | `/system/roles` | `system:role:create` | 新增 |
| PUT | `/system/roles/:id` | `system:role:update` | 修改 |
| DELETE | `/system/roles/:id` | `system:role:delete` | 删除 |
| PUT | `/system/roles/:id/menus` | `system:role:assign` | 分配菜单 |

### 菜单管理

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/system/menus` | `system:menu:list` |
| GET | `/system/menus/:id` | `system:menu:list` |
| POST | `/system/menus` | `system:menu:create` |
| PUT | `/system/menus/:id` | `system:menu:update` |
| DELETE | `/system/menus/:id` | `system:menu:delete` |

### 部门管理

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/system/depts` | `system:dept:list` \| `system:user:list` |
| GET | `/system/depts/:id` | `system:dept:list` |
| POST | `/system/depts` | `system:dept:create` |
| PUT | `/system/depts/:id` | `system:dept:update` |
| DELETE | `/system/depts/:id` | `system:dept:delete` |

### 日志管理

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/system/logs/operations` | `system:log:list` |
| GET | `/system/logs/operations/export` | `system:log:export` |
| GET | `/system/logs/logins` | `system:log:list` |
| GET | `/system/logs/logins/export` | `system:log:export` |
| GET | `/system/logs/modules` | `system:log:list` |
| DELETE | `/system/logs/operations` | `system:log:delete` |
| DELETE | `/system/logs/logins` | `system:log:delete` |

### 全局

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/health` | 健康检查（不在 `/api/v1` 下，不鉴权、不记日志） |

## B. 业务错误码

业务码前 3 位与 HTTP 状态码对应，便于一眼看出性质。

| 码 | HTTP | 含义 |
| --- | --- | --- |
| `0` | 200 | 成功 |
| `40000` | 400 | 请求参数错误 |
| `40100` | 401 | 未登录或令牌失效 |
| `40101` | 401 | 用户名或密码错误 |
| `40102` | 401 | 账号已被禁用 |
| `40103` | 401 | 账号已被锁定（连续失败） |
| `40104` | 401 | 令牌已过期 |
| `40105` | 401 | 令牌无效（含改密后吊销） |
| `40106` | 401 | 原密码错误 |
| `40300` | 403 | 无权限 |
| `40400` | 404 | 资源不存在（**含越权访问**） |
| `40900` | 409 | 资源冲突（唯一性） |
| `42900` | 429 | 请求过于频繁 |
| `50000` | 500 | 服务器内部错误 |
| `50100` | 500 | 数据库操作失败 |
| `50200` | 500 | 配置错误 |
| `50300` | 500 | 外部服务调用失败 |

### 统一响应体

```json
{
  "code": 0,
  "message": "ok",
  "data": { },
  "traceId": "a1b2c3d4e5f60718"
}
```

分页响应的 `data`：

```json
{
  "list": [],
  "total": 0,
  "page": 1,
  "pageSize": 10
}
```

`traceId` 同时出现在响应头 `X-Trace-Id` 和后端日志里 —— **排查线上问题时就靠它串起来**。

## C. 测试清单

| 文件 | 测试 | 覆盖 |
| --- | --- | --- |
| `api_test.go` | 10 个 | 健康检查、认证、个人中心、权限隔离、用户/角色/菜单/部门 CRUD、进销存概览、日志 |
| `excel_test.go` | 5 个 | 用户导出、导入、拒绝坏文件、导入导出权限隔离、内置权限补齐 |
| `hardening_test.go` | 9 个 | 超长 UA 登录日志、审计 IP 防伪造、改密吊销令牌、数据范围读写约束、未匹配路由的审计、按账号+IP 的登录锁定、角色授予越权（含数据范围轴）、Trace ID 注入 |
| `authz_scope_test.go` | 4 个 | 导入不得绕过授予校验、宽数据范围角色不可授予/不可改宽、不传 roleIds 也不能禁用超管、半更新不清空部门 |
| `grant_bypass_test.go` | 2 个 | 建角色不能授予超出授予者自身的权限、改角色不能授予自己未持有的菜单（越权授予的两条通道） |
| `data_scope_test.go` | 5 个 | 部门树/详情/增改删受数据范围约束（含「挂到范围外父部门下」）、最近登录按范围收窄、个人资料半更新不清空字段、清空登录日志不得解除账号锁定 |
| `log_scope_test.go` | 2 个 | 日志模块按数据范围收窄（部门管理员只看本部门）、菜单 `permission` 必须是父级权限码的子集 |
| `regression_test.go` | 6 个 | 大文件导入、角色去重、导出参数校验、导出/导入审计、敌意输入、长字段截断 |
| `robustness_test.go` | 18 个 | 限流器并发原子性（100 并发只放行阈值数）、key 数量硬上界、限流器空接收者安全、JWT 拒绝 HS384/HS512/alg=none/错误 issuer/错误密钥/uid≤0、过期令牌仍报「已过期」、真实登录令牌仍有效、分页 OFFSET 上界与整型溢出、角色列表/详情查询错误必须上报而非静默返回空 |
| `status_test.go` | 2 个 | 「禁用」状态不被默认值吃掉（四条创建路径 + 导入路径） |
| `swagger_test.go` | 1 个 | 在线接口文档可访问、`basePath` 与配置一致、未开启时不暴露 |
| `upgrade_test.go` | 4 个 | 老库补列、老库数据修正、不覆盖显式配置、升级后禁用账号仍被拦 |

**合计 68 个测试 / 139 条用例**，全部可在进程内运行（`httptest`），无需启动服务。
包内单测另有 11 个（21 个子用例，`go test ./internal/...`），其中
`internal/module/contract_test.go` 的 4 个用例（含 10 个「该拒绝的清单」子用例）
专门钉住模块契约的校验规则。

> 后端测试覆盖不到的交互类缺陷，需要浏览器验证（零依赖 CDP 脚本，见 `scripts/browser-verify.mjs`）：
> 权限指令是否真的隐藏（而非摘除）节点、上级部门下拉是否误排除兄弟节点、
> 并发 401 是否只弹一次提示、连续刷新是否泄漏 keep-alive 实例、
> 状态开关取消后是否真的回滚、跨页勾选是否会残留到新查询、
> 首屏空表单点登录是否有提示、分页改每页条数是否只发一次请求。
>
> 注意：**受控组件无法用合成事件驱动**（`element.click()` / 合成 `MouseEvent` 不触发 Vue 响应式更新），
> 必须用 CDP `Input.dispatchMouseEvent` 按坐标点真实鼠标；`v-perm` 隐藏元素的 `innerText` 为空，
> 计数要断言 DOM 状态而非文案。详见 §3.8.1。

另有不依赖数据库的包级单测：`internal/middleware`（Trace ID 收敛、限流中间件）、
`internal/pkg/limiter`（失败限流器的阈值/重置/窗口）、`internal/module/user`（导入校验的 seen 语义）。

## D. 文档索引

| 想了解 | 去看 |
| --- | --- |
| 项目怎么跑起来、有哪些功能 | 本文第一、二部分 |
| 怎么加一个新模块 | 本文第三部分 |
| 改代码前必读的约定与 78 个坑 | [`AGENTS.md`](../AGENTS.md) |
| 每个接口的入参出参与示例 | 在线文档 `/swagger/index.html`；业务语义见 [`docs/API.md`](API.md) |
| 表结构、索引设计、字段约定 | [`docs/DATABASE.md`](DATABASE.md) |
| 独立升级方案（边界、SOP、回滚） | [`docs/UPGRADE.md`](UPGRADE.md) |
| 快速开始与目录速览 | [`README.md`](../README.md) |
