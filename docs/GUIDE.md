# Nova Admin 项目说明与开发指南

> 本文是**新接手开发者**的第一份读物：先讲清这个项目是什么、怎么跑起来、怎么用，
> 再给出「新增一个业务模块」的完整标准流程。
>
> 文档分工：
> - **本文** —— 项目说明、使用步骤、新增业务的全流程
> - [`README.md`](../README.md) —— 快速开始与目录速览
> - [`AGENTS.md`](../AGENTS.md) —— 改代码前必读的约定与 26 个已知坑
> - [`docs/API.md`](API.md) —— 接口清单与响应约定
> - [`docs/DATABASE.md`](DATABASE.md) —— 表结构与索引设计

---

# 第一部分　项目说明

## 1.1 这是什么

Nova Admin 是一套**简洁够用的后台管理系统骨架**：Go + Vue 3 + PostgreSQL。

它的定位不是「大而全的低代码平台」，而是**一个能长期自己维护的中后台底座**：
把用户、角色、菜单、部门、日志、导入导出这些每个后台都要的东西做扎实，
并且把目录结构与命名约定设计成「AI 也能照着加功能」的形态。

三条设计取向，理解了它们才能理解后面的所有取舍：

| 取向 | 具体表现 |
| --- | --- |
| **模块化** | 业务代码只放 `internal/module/<模块>/`，固定四文件制。加功能 = 加目录，不是改老文件 |
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
| Vue | ^3.5.13 | 框架（组合式 API + `<script setup>`） |
| Vite | ^6.1.0 | 构建与开发服务器 |
| TypeScript | ^5.7.3 | 类型（`vue-tsc` 严格检查） |
| Element Plus | ^2.9.5 | UI 组件库 |
| Pinia | ^3.0.1 | 状态管理（组合式 setup store） |
| vue-router | ^4.5.0 | 路由（**hash 模式**） |
| axios | ^1.7.9 | HTTP |
| ECharts | ^5.6.0 | 仪表盘图表 |
| sass | ^1.85.0 | 样式预处理 |

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
│  service 层：业务逻辑 + 数据范围（scopeQuery / ensureInScope）│
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
| **数据范围** | `sys_roles.data_scope` | 「能看到 / 改到**谁**的数据」 | 后端服务层 `scopeQuery` / `ensureInScope` |
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
   会泄露数据存在性。

### 内置角色对照

| 角色 | 权限范围 | 数据范围 | 说明 |
| --- | --- | --- | --- |
| `super_admin`（ID=1） | 全部权限 | 全部 | 不可删除、不可裁剪权限 |
| `system_admin`（ID=2） | 除导出类外的管理权限 | 本部门及以下 | 不含任何删除权限 |
| `viewer`（ID=3） | 只读（7 个菜单可见，**不含任何导出**） | 全部 | 能看列表但不能批量取数 |

> 「能查看列表」与「能批量导出」被刻意划成两个风险等级 ——
> 导出等于把整批数据带走，所以只读角色不授予 `*:export`。

## 1.5 目录结构详解

目录划分直接落地了「模块化」这条铁律：后端业务代码全部收在 `internal/module/<模块>/` 下，一个模块固定四个文件；
前端页面按 `views/<模块>/index.vue` 组织。先看后端，再看前端。

### 后端 `backend/`

```
backend/
├── cmd/server/main.go              进程入口：参数解析 + 优雅关停
├── config.example.yaml             配置模板（复制为 config.yaml）
├── internal/
│   ├── bootstrap/                  装配顺序：配置→日志→JWT→数据库→迁移→种子
│   ├── config/                     读 YAML + NOVA_* 环境变量覆盖 + 校验
│   ├── logger/                     zap 初始化与全局 L()
│   ├── database/                   连接、AutoMigrate、EnsureIndexes（部分唯一索引）
│   ├── authz/                      身份与权限集合，30 秒缓存
│   ├── middleware/                 TraceID / Logger / OperationLog / Recovery / CORS / Auth / RequirePerm
│   ├── model/                      GORM 模型（对应 8 张表）
│   ├── module/<模块>/              ★ 业务代码都在这里，固定四文件制
│   ├── oplog/                      操作日志异步落库
│   ├── pkg/                        无业务依赖的通用工具
│   ├── router/router.go            路由总装配
│   └── seed/seed.go                幂等种子数据 + 老库数据修正
└── test/                           进程内 E2E 测试（独立测试库 nova_admin_test）
```

**依赖方向**：`module → pkg / model / authz`，`pkg → 标准库与第三方`。
`pkg` 反向依赖 `module` **必须禁止**，否则通用层就被业务污染了。

各目录的「不要做的事」：

| 目录 | 不要做的事 |
| --- | --- |
| `cmd/server` | 不写业务逻辑 |
| `internal/config` | 不在别处直接读 `os.Getenv` |
| `internal/database` | 不在别处写 DDL |
| `internal/authz` | 不在业务代码里查授权表 |
| `internal/model` | 不放 DTO |
| `internal/module/*` | 不跨模块直接引用别人的 `service` |
| `internal/pkg` | 不 import `internal/module/*` |
| `internal/router` | 不写 handler |

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
├── components/       AppIcon / EChart / Pagination / icons.ts
├── directives/       permission.ts（v-perm 指令）
├── layout/           index.vue + Sidebar / Navbar / TagsView / Breadcrumb
├── router/
│   ├── index.ts      constantRoutes（静态路由，无 redirect）
│   ├── helper.ts     后端菜单 → vue-router 路由（import.meta.glob 动态导入）
│   └── guard.ts      ★ 守卫：登录态、动态路由注入、chunk 加载失败自愈
├── store/            user（身份权限）/ permission（菜单路由）/ app（UI 偏好、标签、主题）
├── styles/           variables.scss（亮暗两套设计令牌）+ element.scss + index.scss
├── types/api.ts      与后端 DTO 一一对应
├── utils/            auth（令牌）/ request（axios）/ download / format / theme
└── views/            页面
```

**前端路由是「菜单驱动」的**，这是本项目最省事也最容易误解的一点：

- 静态路由（`constantRoutes`）只有三条：`/login`、`/`（Layout 容器）、兜底 404。
- 其余所有业务路由都是登录后从 `GET /api/v1/profile/routes` 拉回来的菜单树，
  由 `router/helper.ts` 的 `buildRoutes()` 转换，再用 `router.addRoute()` 动态注入。
- **目录节点不注册路由，叶子拍平挂到 Layout 下。**
- 所以**新增页面不需要改任何前端路由文件**，只要在 `sys_menus` 里加一条菜单记录。

## 1.6 功能清单

8 个业务模块共 **47 个接口**，另有服务根的 `/health` 不计入其中。
先看模块总览，再看菜单树、内置角色与权限码是怎么分配的。

### 模块总览

| 模块 | 路径前缀 | 接口数 | 说明 |
| --- | --- | --- | --- |
| `auth` | `/auth` | 3 | 登录、刷新令牌、退出 |
| `profile` | `/profile` | 4 | 个人信息、改资料、改密码、动态路由树 |
| `dashboard` | `/dashboard` | 4 | 概览、趋势、模块排行、最近登录 |
| `user` | `/system/users` | 11 | 用户 CRUD + 启停 + 重置密码 + 导入导出 |
| `role` | `/system/roles` | 8 | 角色 CRUD + 分配菜单 + 导出 |
| `menu` | `/system/menus` | 5 | 菜单树 CRUD |
| `dept` | `/system/depts` | 5 | 部门树 CRUD |
| `log` | `/system/logs` | 7 | 操作日志、登录日志、清空、导出 |
| — | `/health` | 1 | 健康检查（不鉴权、不记日志） |
| **合计** | | **47 + 1** | |

> 服务根路径还有一个 `GET /health`（返回 200；数据库异常时返回 503 + `degraded`），
> 以及全局 `NoRoute` 统一 404。其余全部挂在 `/api/v1` 下。

### 功能要点

- **认证**：JWT 双令牌（访问 2 小时 / 刷新 7 天），访问令牌自动静默续期；
  连续登录失败 5 次锁定 300 秒；改密码 / 重置密码 / 禁用账号即刻吊销旧令牌。
- **动态路由**：登录后按当前用户的菜单权限生成路由与侧边栏，切换账号自动卸载重建。
- **按钮级权限**：`v-perm="'system:user:create'"`，无权限时节点直接从 DOM 移除。
- **数据范围**：四种范围，读写同时生效，越界按 404 处理。
- **导入导出**：用户 / 角色 / 操作日志 / 登录日志四类 Excel 导出；
  用户支持**两阶段导入**（先校验预览并给出逐行原因，确认后才写库）。
- **审计**：操作日志（含请求体，敏感字段递归脱敏）+ 登录日志；
  请求体超过限制或为二进制时不记录，避免被客户端内容搞挂。
- **主题**：亮色 / 暗色双主题，跟随系统偏好 + 手动切换 + 本地记忆，首屏无白闪。

## 1.7 数据表

8 张表，全部由 GORM `AutoMigrate` 自动创建。

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

**四条设计约定**（细节见 [`DATABASE.md`](DATABASE.md)）：

1. **唯一性用「部分唯一索引」**（`WHERE deleted_at IS NULL`），不用 `uniqueIndex` 标签 ——
   否则软删除的历史行仍占着唯一约束，会出现「删除后无法再建同名记录」。
   索引由 `database.EnsureIndexes()` 在启动时创建，并会清理历史遗留的普通唯一索引。
2. **`status` 字段不加 `gorm:"default:1"`** —— GORM 遇到「带 default 且值为零值」会把该列
   从 INSERT 里省略，导致「禁用（0）」被数据库默认值 1 悄悄改写成「启用」。
3. **`many2many` 必须写完整表名**（`many2many:sys_user_roles`），否则 GORM 会推导出不存在的表名。
4. **时间字段统一 `timestamptz`**，时区取配置项 `database.timezone`（默认 `Asia/Shanghai`）。

**内置数据**（`internal/seed/`，幂等，仅在表为空时写入）：

| 项 | 数量 | 内容 |
| --- | --- | --- |
| 部门 | 3 | 总公司 / 技术部 / 运营部 |
| 菜单 | 26 | 1 个目录 + 6 个页面菜单 + 19 个按钮权限点 |
| 权限点 | 25 | 6 个页面级 + 19 个按钮级 |
| 角色 | 3 | `super_admin` / `system_admin` / `viewer` |
| 用户 | 2 | `admin`、`demo` |

**菜单 ID 的分配规则**（重要，撞号会导致启动报错）：

| 区间 | 归属 | 现有占用 |
| --- | --- | --- |
| `1–999` | **内置菜单**（写在 `seed.go` 里） | `1`=仪表盘，`10`=系统管理目录，`11–15`=页面菜单，`101+`=按钮权限 |
| `1000+` | **管理员在界面上自建的菜单**（自增序列起点） | `seed.MenuIDFloor = 1000` |

所以给一个新的**内置**业务模块分配 ID 时，要在 `1–999` 里找空档，
沿用现有分组习惯即可（目录用 `10` 的倍数、按钮用 `1xx` 段）。

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
**数据库 `nova_admin` 与全部数据表都会由程序自动创建**，不需要手工执行任何 DDL。

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
  dbname: nova_admin
app:
  port: 8080
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
数据库连接成功    {"database": "nova_admin"}
HTTP 服务已启动   {"addr": "0.0.0.0:8080", "basePath": "/api/v1"}
```

验证：

```bash
curl http://127.0.0.1:8080/health
# {"app":"nova-admin","status":"ok","version":"v1"}
```

### 第 3 步：启动前端

```bash
cd frontend
npm install
npm run dev
```

打开 `http://localhost:5173`。Vite 已把 `/api` 代理到 `http://127.0.0.1:8080`，无需处理跨域。

### 第 4 步：准备测试配置（跑测试前必做）

测试使用**独立数据库** `nova_admin_test`，配置同样不进版本库：

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

### 仪表盘

登录后的默认落地页。四块内容：概览统计、7 天登录趋势、模块访问排行、最近登录记录。

- 时间范围参数 `days` 默认 7，传非法值会回退到 7 而不是报错。
- 图表随主题自动重绘（ECharts 消费的是 `chartPalette()` 读出的具体色值，不是 CSS 变量）。

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
  name: nova-admin          # 应用名
  mode: debug               # debug | release
  host: 0.0.0.0
  port: 8080
  prefix: /api              # 路由统一前缀
  version: v1               # API 版本段，最终拼接为 /api/v1

database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: CHANGE_ME       # 改成本机口令
  dbname: nova_admin        # 不存在会自动创建
  sslmode: disable
  timezone: Asia/Shanghai
  max_idle_conns: 10
  max_open_conns: 100
  conn_max_lifetime: 3600   # 秒
  log_level: warn           # silent | error | warn | info

jwt:
  secret: CHANGE_ME_USE_A_RANDOM_LONG_STRING   # 上线务必换成随机长串
  issuer: nova-admin
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
  max_login_fail: 5         # 连续失败锁定阈值
  login_lock_seconds: 300   # 锁定时长（秒）
  trusted_proxies: []       # 空 = 不信任任何代理
```

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
npm run dev          # 开发服务器 http://localhost:5173
npm run typecheck    # vue-tsc --noEmit
npm run build        # 类型检查 + 产出 dist/
npm run preview      # 预览生产产物（已配置 /api 代理）

# ── 端到端（需要服务已在运行）──────────────
node scripts/api-smoke.mjs                    # 默认 http://127.0.0.1:8080
node scripts/api-smoke.mjs http://host:port   # 指定实例
node scripts/browser-verify.mjs               # 需后端 + 前端都启动
APP_URL=http://localhost:4173 node scripts/browser-verify.mjs  # 验证生产产物
```

## 2.7 三层验证

本项目要求「改完必跑」，三层验证各有分工，**不能互相替代**：

| 层 | 命令 | 覆盖 | 规模 |
| --- | --- | --- | --- |
| 编译期 | `gofmt -l .` / `go vet ./...` / `npm run typecheck` | 语法、类型、常见误用 | — |
| 进程内 E2E | `go test ./test/... -count=1` | 行为契约（进程内 `httptest` 驱动完整路由） | **33 个测试 / 65 个子用例** |
| 接口冒烟 | `node scripts/api-smoke.mjs` | 协议层（真实 HTTP、二进制响应、文件上传下载） | **90 项断言** |
| 浏览器运行时 | `node scripts/browser-verify.mjs` | 真实渲染（白屏、主题、控制台报错） | **48 项断言** |

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
| 端口被占用 | 有旧进程还在跑 | `netstat -ano \| grep :8080` 找 PID 后 `taskkill //PID <pid> //F` |
| 前端 404 / 接口报跨域 | 后端没起来或代理目标不对 | 确认 8080 在监听；检查 `.env.development` 的 `VITE_PROXY_TARGET` |
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
| 7 | 可选 | `backend/internal/module/article/excel.go` | 需要导入导出时 |
| 8 | 修改 | `backend/internal/router/router.go`（加一行 `article.Register(base)`） | ✅ |
| 9 | 新增 | `backend/test/api_test.go` 里的 `TestArticleCRUD` | ✅ |
| 10 | 修改 | `backend/internal/seed/seed.go`（加菜单 + 权限点） | ✅ |
| 11 | 新增 | `frontend/src/api/article.ts` | ✅ |
| 12 | 修改 | `frontend/src/types/api.ts`（加类型） | ✅ |
| 13 | 新增 | `frontend/src/views/article/index.vue` | ✅ |

**一共只修改 4 个已有文件**，其余全是新增。

## 3.2 后端九步

后端加一个模块固定是这九步，顺序不能乱：先建目录与文件，再写 model 与 DTO，接着注册迁移，最后挂路由与权限。
下面逐步给出每一步的命令与代码。

### Step 1 — 建目录与四个文件

```
internal/module/article/
├── dto.go      # 请求 / 响应结构体
├── service.go  # 业务逻辑（不感知 HTTP）
├── handler.go  # HTTP 入参出参
├── route.go    # 路由 + 权限码
└── excel.go    # 可选：导入导出
```

四文件制的意义：**职责边界靠文件名就能看清**。
`service` 不知道 `gin.Context` 的存在，所以它能被单测直接调用；
`handler` 只做「绑参 → 调 service → 出参」，不含业务判断。

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

	"nova-admin/internal/middleware"
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

### Step 7 — 挂到路由树

```go
// internal/router/router.go
user.Register(base)
role.Register(base)
menu.Register(base)
dept.Register(base)
log.Register(base)
article.Register(base)   // ← 加这一行
```

### Step 8 — 分页与排序

```go
q := page.FromContext(c)
q.Normalize()   // 补齐默认值，pageSize 封顶 200

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

> 测试库与开发库隔离（`nova_admin_test`），可以放心造数据。
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

### 加菜单记录

在 `internal/seed/seed.go` 的 `menuDefs()` 里追加。注意 ID 要落在 **`1–999`** 的空档里
（`1000+` 是管理员自建菜单的自增区间，见 §1.7）：

```go
// 目录（不产生路由，只做侧边栏分组）
{
	BaseModel: model.BaseModel{ID: 20},
	ParentID:  0, Name: "ArticleRoot", Path: "/article", Title: "内容管理",
	Icon: "Document", Type: model.MenuTypeDir, Redirect: "/article/list",
	Sort: 4, Status: on,
},

// 页面菜单（产生一条动态路由）
menu(21, 20, "Article", "/article/list", "article/index", "文章管理",
	"Notebook", "article:list", 1, true),

// 按钮权限点（只提供权限码，不显示）
button(201, 21, "ArticleCreate", "新增文章", "article:create", 1),
button(202, 21, "ArticleDelete", "删除文章", "article:delete", 2),
```

`menu(...)` 与 `button(...)` 是 `menuDefs()` 里现成的小构造器，参数顺序：

```
menu(id, parent, name, path, component, title, icon, permission, sort, keepAlive)
button(id, parent, name, title, permission, sort)
```

字段对照：

| 字段 | 要求 |
| --- | --- |
| `ID` | 内置菜单用 `1–999` 的空档，**不要与已有 ID 重复** |
| `ParentID` | 上级菜单 ID；`0` 表示顶级 |
| `Name` | 大驼峰、全局唯一，**必须与页面 `defineOptions({ name })` 一致** |
| `Component` | `src/views` 下的相对路径，**无扩展名** |
| `Permission` | `模块:资源:动作`，与 `route.go` 里的完全一致 |
| `Type` | `dir` / `menu` / `button` |
| `Redirect` | 目录节点建议指向第一个子页面 |

> `seedMenus()` 采用「**按 ID 补齐**」而非「表为空才写入」，所以新增的权限点
> 能自动补进已经初始化过的数据库，不必手工执行 SQL 或重建库。
> 但它**不覆盖**已存在的记录 —— 管理员调整过的菜单文案不会被冲掉。
> 如果 ID 被历史数据占用，启动会明确报错并点名是哪个 ID，按提示清理即可。

### 授权给角色

`builtinRoleGrants()` 里把新菜单 ID 加进对应角色的授权列表：

```go
func builtinRoleGrants() map[int64][]int64 {
	readOnly := []int64{1, 10, 11, 12, 13, 14, 15}
	adminMenus := append([]int64{}, readOnly...)
	adminMenus = append(adminMenus,
		101, 102, 104, 105, 106,
		111, 112, 114, 115,
		121, 122,
		131, 132,
		149,
		20, 21, 201, 202,   // ← 加：文章模块的目录、页面、两个按钮
	)
	return map[int64][]int64{
		1: allMenuIDs(),          // 超管：全部
		2: adminMenus,            // 系统管理员：管理权限 + 文章
		3: readOnly,              // 只读访客：不给（不含任何 export）
	}
}
```

`syncBuiltinRoleMenus()` 会在每次启动时**补齐缺失的授权**（只增不减，
不会覆盖管理员手工做过的裁剪），所以加完授权重启即可生效。

> **只读角色不要授予 `*:export`。** 「能查看列表」与「能批量导出」是两个风险等级。

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
# 新建 internal/module/article/{dto,service,handler,route}.go
# 在 router.go 加 article.Register(base)
cd backend
gofmt -l . && go vet ./... && go build ./...
```

**第三步：加菜单与权限**

在 `seed.go` 的 `menuDefs()` 加菜单、`builtinRoleGrants()` 加授权，然后：

```bash
go run ./cmd/server -config config.yaml -seed
```

重启后登录 `admin`，侧边栏应该出现「内容管理 → 文章管理」。

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
[ ] 测试数据已清理

前端
[ ] npm run typecheck       无错误
[ ] npm run build           构建通过
[ ] 页面 defineOptions({ name }) 与菜单 name 一致
[ ] 根节点是稳定的 <div class="xxx-page">
[ ] 没有写死颜色，全部用 CSS 变量
[ ] 新颜色令牌亮/暗两套都补了
[ ] 按钮级权限用了 v-perm

文档与验证
[ ] AGENTS.md 的测试计数已更新
[ ] 接口有变动则同步 docs/API.md
[ ] 表结构有变动则同步 docs/DATABASE.md
[ ] 跑了浏览器验证（改了路由/守卫/主题时必跑）
```

## 3.8 必读的八个坑

完整 26 条见 [`AGENTS.md` §5](../AGENTS.md)。以下八条是新增业务时**最常撞上**的：

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

---

# 附录

## A. 接口总表（47 个）

前缀 `/api/v1`。「权限」列中 `|` 表示任一命中即可。

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

### 仪表盘

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/dashboard/overview` | 仅登录 | 概览统计 |
| GET | `/dashboard/trend` | 仅登录 | 登录趋势（`?days`） |
| GET | `/dashboard/modules` | 仅登录 | 模块排行（`?days&limit`） |
| GET | `/dashboard/recent-logins` | 仅登录 | 最近登录（`?limit`） |

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
| `api_test.go` | 10 个 | 健康检查、认证、个人中心、权限隔离、用户/角色/菜单/部门 CRUD、仪表盘、日志 |
| `excel_test.go` | 5 个 | 用户导出、导入、拒绝坏文件、导入导出权限隔离、内置权限补齐 |
| `hardening_test.go` | 6 个 | 超长 UA 登录日志、审计 IP 防伪造、改密吊销令牌、数据范围读写约束、未匹配路由的审计 |
| `regression_test.go` | 6 个 | 大文件导入、角色去重、导出参数校验、导出/导入审计、敌意输入、长字段截断 |
| `status_test.go` | 2 个 | 「禁用」状态不被默认值吃掉（四条创建路径 + 导入路径） |
| `upgrade_test.go` | 4 个 | 老库补列、老库数据修正、不覆盖显式配置、升级后禁用账号仍被拦 |

**合计 33 个测试 / 65 个子用例**，全部可在进程内运行（`httptest`），无需启动服务。

## D. 文档索引

| 想了解 | 去看 |
| --- | --- |
| 项目怎么跑起来、有哪些功能 | 本文第一、二部分 |
| 怎么加一个新模块 | 本文第三部分 |
| 改代码前必读的约定与 26 个坑 | [`AGENTS.md`](../AGENTS.md) |
| 每个接口的入参出参与示例 | [`docs/API.md`](API.md) |
| 表结构、索引设计、字段约定 | [`docs/DATABASE.md`](DATABASE.md) |
| 快速开始与目录速览 | [`README.md`](../README.md) |
