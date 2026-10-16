# AGENTS.md — AI 开发约定

本文件是**改代码前必读**的约定集合。它记录了本项目的分层职责、命名规则、标准做法，
以及一批「踩过才知道」的坑（都是真实发生过的缺陷，不是假设）。

约定优先级：**本文件 > 个人习惯**。若某条约定确实需要改，请连同本文件一起更新。

---

## 0. 四条硬规则

1. **业务代码只放 `backend/internal/module/<模块>/`**，不要往 `router.go` 里塞业务逻辑。
2. **前端路由不要写 `redirect`**，动态路由注入统一交给 `router/guard.ts`（详见 §5.1）。
3. **不要把 `localStorage` 当作响应式数据源**，令牌读写一律走 `utils/auth.ts`（详见 §5.2）。
4. **组件样式里不要写死颜色**，一律引用 `styles/variables.scss` 的语义令牌（详见 §3）。

---

## 1. 后端目录职责

| 目录 | 职责 | 不要做的事 |
| --- | --- | --- |
| `cmd/server` | 进程入口、参数解析、优雅关停 | 不写业务逻辑 |
| `internal/bootstrap` | 装配顺序（配置→日志→数据库→JWT→路由） | 测试与 main 共用，不要在 main 里重复装配 |
| `internal/config` | 读 YAML + `NOVA_*` 环境变量 | 不在别处直接读 `os.Getenv` |
| `internal/logger` | zap 初始化与全局 `L()` | 不用 `fmt.Println` 打日志 |
| `internal/database` | 连接、`AutoMigrate`、`EnsureIndexes` | 不在别处写 DDL |
| `internal/authz` | 身份 + 权限集合（30 秒缓存） | 不在业务里查授权表 |
| `internal/middleware` | 横切关注点 | 不写业务判断 |
| `internal/model` | GORM 模型 | 不放 DTO |
| `internal/module`（根目录的 `contract.go`） | **模块契约**：`Module` / `MenuNode` 类型与登记表 | 不 import 任何具体模块 |
| `internal/module/<模块>` | **业务代码 + 自描述清单 `module.go`** | 不跨模块直接引用别人的 `service` |
| `internal/assembly` | **组装层**：唯一列出全部模块的地方 | 不写业务逻辑；只做装配 |
| `internal/pkg` | 无业务依赖的通用工具 | 不 import `internal/module/*` |
| `internal/router` | 遍历登记表注册路由 | 不写 handler，不认识任何具体模块 |

**依赖方向**：`具体模块 → module 契约 / pkg / model / authz`，`pkg → 标准库与第三方`。

**框架层一切包（`pkg` / `authz` / `middleware` / `oplog` / `database` / `seed` /
`router` / `bootstrap` / `model` / `config` / `logger`，以及 `module` 契约本身）
都不许 import 具体模块**，唯一例外是 `internal/assembly`。
跑 `bash scripts/check-module-boundary.sh` 校验；越界时它会指名道姓地列出来。
为什么这条线值得一条门禁：框架一旦依赖了业务，那个业务模块就再也删不掉了 ——
而且是编译期报错的「删不掉」，比「不敢删」更硬。

---

## 2. 新增一个后端模块（以 `article` 为例）

### Step 1 — 建 4 个文件

```
internal/module/article/
├── dto.go      # 请求 / 响应结构体
├── service.go  # 业务逻辑（不感知 HTTP）
├── handler.go  # HTTP 入参出参
├── route.go    # 路由 + 权限码
├── module.go   # ★ 自描述清单：命名空间 + 路由 + 菜单树（见 Step 6）
└── excel.go    # 可选：导入导出（见 §7.1）
```

### Step 2 — `dto.go`：每个字段都要有 `label`

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

`label` 会被 `pkg/bind` 用来拼中文校验提示（如「标题长度不能超过 128」），**漏写标签会导致用户看到英文报错**。

### Step 3 — `route.go`：权限码写在路由上

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
		g.GET("/:id", middleware.RequirePerm("article:list"), h.Detail)
		g.POST("", middleware.RequirePerm("article:create"), h.Create)
		g.PUT("/:id", middleware.RequirePerm("article:update"), h.Update)
		g.DELETE("/:id", middleware.RequirePerm("article:delete"), h.Delete)
	}
}
```

权限码格式固定为 **`模块:资源:动作`**（`system:user:create`）。
`RequirePerm` 支持传多个码，语义是「任一命中即可」。

**静态路径段必须注册在 `/:id` 之前**：

```go
g.GET("/export", middleware.RequirePerm("article:export"), h.Export)  // ✅ 先注册
g.GET("/:id", middleware.RequirePerm("article:list"), h.Detail)       // 后注册
```

顺序反了的话，`/articles/export` 会先命中 `/:id`，`id` 拿到字符串 `"export"`，
报「路径参数 id 非法」——错误信息完全指不到真正的原因。

### Step 4 — `handler.go`：固定骨架

```go
// Create 新增文章
//
//	@Summary      新增文章
//	@Description  创建一篇文章，返回新记录 ID
//	@Tags         文章
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body body CreateReq true "文章内容"
//	@Success      200 {object} response.Body{data=response.IDData}
//	@Failure      400 {object} response.Body
//	@Router       /articles [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增文章") // ★ 操作日志里的动作名，必须设置
	id, err := h.svc.Create(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}
```

**每个 handler 必须带 Swagger 注解**（§9），否则 `swag init` 生成的文档会缺这个接口。
注解里的出参类型必须**具体**——`gin.H` 是 map，swag 推不出字段，
所以「只返回 id / deleted 数」也要用 `response.IDData` / `response.DeletedData`，不要写 `gin.H`。

出参只用这几个函数：

| 函数 | 用途 |
| --- | --- |
| `response.OK(c, data)` | 成功，返回 data |
| `response.OKPage(c, list, total, page, pageSize)` | 分页成功 |
| `response.OKMsg(c, "修改成功")` | 成功，仅返回提示语 |
| `response.Fail(c, err)` | 失败，从 `*errcode.Error` 自动取码 |

`response.Fail` 会把底层错误写进日志（带 traceId），所以**不要在 service 里重复打日志**。

### Step 5 — `service.go`：错误统一用 `errcode`

```go
if err != nil {
	return 0, errcode.Wrap(errcode.ErrDatabase, err)   // 日志会记录真实原因，前端只看到"数据库操作失败"
}
if exists {
	return 0, errcode.New(errcode.ErrConflict, "文章标题已存在")
}
```

### Step 6 — 写 `module.go` 清单，并在组装层登记

以前「加一个模块」要改 5 处：`route.go` 写权限码、`seed.go` 的 `menuDefs()` 手工
分配菜单 ID、`seed.go` 的 `builtinRoleGrants()` 把 ID 再抄一遍、前端加页面、
最后去菜单管理页手工录一条。权限码写两遍最危险 —— 漂移后的症状是
**「菜单点得进去、接口 403」**，排查成本极高。

现在模块自己描述自己，一段清单说清命名空间、路由、菜单树：

```go
// internal/module/article/module.go
package article

import "nova-erp/internal/module"

func Module() module.Module {
	return module.Module{
		Namespace: "article",   // 权限码必须以它开头，Build 会校验
		Register:  Register,    // 就是 route.go 里那个函数
		Menus: []module.MenuNode{
			{
				Name: "ArticleList", Title: "文章管理", Icon: "Document",
				Path: "/article", Component: "article/index",
				Permission: "article:list",
				Children: []module.MenuNode{
					{Name: "ArticleCreate", Title: "新增文章", Type: module.TypeButton, Permission: "article:create"},
				},
			},
		},
	}
}
```

然后在**组装层**加一行：

```go
// internal/assembly/assembly.go → Modules()
article.Module(),
```

要点：

- **`Sort` 可以完全不写**：留空即按声明顺序排，所以清单里读不到一个手工数字。
- **`Type` 留空即菜单**，只有按钮和目录需要显式写。
- **路由注册也由登记表驱动**，`internal/router/router.go` 一个字都不用改。
- 多个模块共用一个目录时用 `module.Group`（见 §2.1），别让模块自己声明挂在哪个目录下。

装配顺序（`Modules()` 的排列）等于路由注册顺序与菜单插入顺序。校验在启动时跑，
清单写错直接起不来，报错会指出是哪个节点的哪一处 —— 比运行期 403 好查得多。

### Step 7 — 分页查询用 `pkg/page`

```go
q := page.FromContext(c)
q.Normalize()                       // 补齐默认值，pageSize 封顶 200
// userOrderWhitelist 用「裸列名 → 排序表达式」的白名单，杜绝 SQL 注入
order := q.SafeOrderBy(articleOrderWhitelist, "created_at DESC")
db.Offset(q.Offset()).Limit(q.Limit()).Order(order)
```

> 列表查询不要写 `LEFT JOIN` + `Select("主表.*")`，会与 `Preload` 生成的派生表别名冲突导致 500。
> 需要过滤关联表时用 `Where("dept_id IN (?)", subQuery)` 子查询代替。

### Step 8 — 写测试

在 `test/api_test.go` 追加一个 `TestArticleCRUD`，用现成的 `call(t, method, path, token, body)` 基建。
测试库与开发库隔离（`test/testdata/config.yaml` → `nova_inv_v5_test`）。

### Step 9 — 前端接上（可选）

- **菜单不用手工加**：Step 6 的清单里写过了，启动时自动补进 `sys_menus`
- 在 `src/views/article/index.vue` 建页面
- 在 `src/api/article.ts` 加接口封装
- 在 `src/types/api.ts` 加类型

页面路径由后端菜单决定，前端**不需要**改路由文件。

---

## 3. 前端目录职责

| 目录 | 职责 |
| --- | --- |
| `api/` | 只做接口封装，一个模块一个文件，返回 `Promise<T>` |
| `store/` | `user`（身份权限）/ `permission`（菜单与动态路由）/ `app`（UI 偏好、标签页、主题） |
| `router/` | `index.ts` 常量路由 · `helper.ts` 后端菜单 → 路由 · `guard.ts` 守卫 |
| `layout/` | 布局骨架 |
| `views/` | 页面 |
| `components/` | 跨页面复用组件：`AppIcon` · `EChart` · `Pagination` · `StatusTag` |
| `composables/` | 跨页面状态机：`useLatestGuard`（请求竞态）· `useTableQuery`（列表查询）· `useCrudDialog`（弹窗）· `useLogout`（登出三件套） |
| `styles/` | `variables.scss`（设计令牌，亮/暗两套）· `element.scss`（Element Plus 覆写）· `index.scss`（含 `.page-card` `.toolbar` 等公共版式类） |
| `utils/` | `auth`（令牌）· `request`（axios）· `format`（格式化，含 `exportStamp`）· `theme`（主题状态与令牌读取） |
| `types/api.ts` | **与后端 DTO 一一对应**，新增字段先改这里 |

### 列表页的三个公共抽象（新页面照抄，不要重写）

系统管理四个页面（user/role/menu/dept）与日志页曾各自维护同一套状态与骨架，
差异只在「调哪个接口」。这些已经收敛成组合式函数，**新列表页直接用**：

```ts
// 列表查询：query/loading/rows/total + 竞态守卫 + search/reset
const {
  query, loading, rows, total,
  load: loadList, search: searchTable, reset: resetTable,
} = useTableQuery<UserQuery, UserItem>(
  { keyword: '', status: undefined },            // initial：reset() 会恢复成这些值
  (params) => userApi.list({ ...params, keyword: params.keyword || undefined }),
)

// 新增/编辑弹窗：visible/mode/submitting/editingId + 校验→建改→关窗→刷新
const dialog = useCrudDialog<UserPayload>({
  formRef,
  create: (payload) => userApi.create(payload),
  update: (id, payload) => userApi.update(id, payload),
  messages: { create: '用户创建成功', update: '用户信息已更新' },
})
```

两条硬约束：

1. **解构后再用**。模板只展开 setup 顶层绑定，`table.query` 这种嵌套 ref 不会被展开，
   写进模板就是拿到一个 ref 对象本身（比较、判断全错）。页面上沿用的旧名字用解构别名保住：
   `load: loadList`、`reset: resetQuery`。
2. **页面独有的附加动作包一层**，不要塞进组合式函数。例如 user 页 `search()` 还要
   `clearSelection()`（§5.45），就写成 `function search() { clearSelection(); searchTable() }`。

状态列的「启用/停用」标签统一用 `components/StatusTag.vue`，不要在表格里再写三元表达式
（配色口径必须全站一致）。导出文件名统一 `utils/format.ts` 的 `exportStamp('用户列表')`。


### 页面命名约定

- 页面根节点固定一个语义化 class：`<div class="xxx-page">`
- 用 `defineOptions({ name: 'SystemUser' })` 声明组件名，**必须与后端菜单的 `name` 一致**，否则 `keep-alive` 不会生效
- 路由 `path` 用单数：`/system/user`、`/system/role`

### 主题与配色（写样式前先读这节）

主题开关只有一个：**`<html>` 上有没有 `dark` 类**。

| 层 | 做法 |
| --- | --- |
| 自定义样式 | `styles/variables.scss` 用 `@mixin theme-tokens($mode)` 输出两套 `--app-*`，`:root` 出亮色、`html.dark` 出暗色 |
| Element Plus | `main.ts` 引入官方 `dark/css-vars.css`；`styles/element.scss` 再把它对齐到本项目调色板 |
| 切换动作 | `store/app.ts` 的 `setTheme()/toggleTheme()` → `utils/theme.ts` 的 `applyTheme()` |
| 首屏防闪 | `index.html` 内联脚本在样式生效前就写好类名，取值逻辑与 `resolveInitialTheme()` 保持一致 |

**写样式的规则**：

1. 只用 `var(--app-*)` / `var(--tone-*)` / `var(--brand-*)`，**不写 `#hex` 与 `rgba()`**
   （白字写在品牌渐变上、`color: #fff` 这类与主题无关的场景例外）。
2. 新增颜色令牌时，**亮/暗两套必须同时补**。只补一套的后果是暗色下留一块白底，
   编译器不会报错，只能靠 §6 的亮度扫描断言发现。
3. Element Plus 组件一律通过**覆写它的 CSS 变量**来改配色，不要用 `!important` 硬压它的样式。
4. 语义色调（`--tone-blue/green/violet/amber/red/slate` + `-soft`）用于标签、图标底、状态文本；
   `--app-success/warning/danger/info` 用于 Element Plus 的语义色。两者不要混用。

**ECharts 这类消费方**：canvas 不认 CSS 变量，必须用 `utils/theme.ts` 的 `chartPalette()` /
`readToken()` 读出**具体色值**，并把 option 放进依赖 `appStore.theme` 的 `computed`，
这样切主题时图表会自动重绘（参考 `views/dashboard/index.vue`）。

### 按钮级权限

```vue
<el-button v-perm="'system:user:create'" @click="openCreate">新增用户</el-button>
```

`v-perm` 无权限时直接移除 DOM 节点。它只负责「别让人误点」，
**真正的拦截在后端**，前端隐藏按钮永远不能替代接口鉴权。

---

## 4. 命名与风格

| 场景 | 规则 | 示例 |
| --- | --- | --- |
| Go 文件名 | 小写下划线 | `handler.go`、`route.go` |
| 权限码 | `模块:资源:动作` | `system:user:create` |
| 菜单 `name` | 大驼峰，全局唯一 | `SystemUser` |
| 菜单 `path` | 绝对路径、单数 | `/system/user` |
| 组件路径 | `src/views` 下相对路径，无扩展名 | `system/user/index` |
| 前端类型 | 与后端 JSON 字段完全一致（含大小写） | `deptId`、`lastLoginAt` |
| 数据库表 | 前缀 `sys_` | `sys_users` |

- Go 提交前必须 `gofmt` + `go vet ./...` 干净
- 前端提交前必须 `npm run typecheck` 干净
- 注释写「为什么」，不写「是什么」

---

## 5. 已知坑（都是真实踩过的）

### 5.1 前端路由：`redirect` 会吞掉动态路由注入

**症状**：冷启动或 F5 直接进业务页，一路落到 404；菜单点击正常，刷新就 404。

**原因**：vue-router 在 `beforeEach` 之前先解析 `redirect`。
若根路由或兜底路由带了 `redirect`，导航会被提前改写，
守卫里「拉取菜单 → `addRoute`」的逻辑根本没机会执行。

**正确做法**：

- 常量路由（`constantRoutes`）里**不写任何 `redirect`**
- 兜底路由直接渲染 404 组件，不 redirect
- 落地页决策交给守卫的 `to.path === '/'` 分支
- 注入路由后重新导航要**按 path 重建 location**：

```ts
// ✅ 正确
next({ path: to.path, query: to.query, hash: to.hash, replace: true })

// ❌ 错误：此刻 to 命中的是兜底路由，展开后 name='NotFound' 会覆盖 path，又被送回 404
next({ ...to, replace: true })
```

### 5.2 前端状态：`localStorage` 不是响应式的

**症状**：登录接口返回成功、提示"欢迎回来"，但立刻被弹回登录页。

**原因**：`computed(() => !!localStorage.getItem('token'))` 没有响应式依赖，
会被**永久缓存**成首次求值的结果。

**正确做法**：令牌在 `utils/auth.ts` 里维护一份模块级 `ref` 作为唯一数据源，
`getAccessToken()` 返回 `ref.value`，写入时同步落 `localStorage`。
这样 `store/user.ts` 的 `isLoggedIn` 以及 axios 拦截器都自动是响应式的。

### 5.3 动态路由要在切换账号时卸载

`permissionStore.reset()` 必须调用 `router.addRoute` 返回的卸载函数，
否则「admin 退出 → demo 登录」后，admin 的路由仍留在路由表里。
`store/permission.ts` 已用 `disposers` 统一管理，新增路由注册方式时不要绕过它。

### 5.4 GORM `many2many` 必须写完整表名

```go
Roles []Role `gorm:"many2many:sys_user_roles;"`  // ✅
Roles []Role `gorm:"many2many:user_roles;"`      // ❌ 推导出另一张表
```

写错的表现很隐蔽：登录正常，但权限集合为空、`/profile/routes` 返回 `[]`、只读账号一路 403。

### 5.5 软删除 + 唯一索引

唯一性一律用部分唯一索引（`WHERE deleted_at IS NULL`），
不要用 GORM 的 `uniqueIndex` 标签，否则删掉的记录会一直占着那个值。
新增需要唯一的字段时，去 `database.EnsureIndexes()` 的 `partialUniqueIndexes` 里加一行。

### 5.6 测试里断言操作日志要显式刷盘

操作日志是异步批量落库的，写完立刻查可能查不到。测试中先调用：

```go
_ = oplog.Flush(2 * time.Second)
```

### 5.7 元组件/图标必须显式注册

图标走 `components/icons.ts` 的显式注册表。
模板里写了一个没注册的图标名，**只会渲染成空白 + 一条 Vue 警告**，不会报错。
新增图标时同时更新注册表。

### 5.8 页面根节点要稳定

`layout/index.vue` 用 `<transition><keep-alive><component /></keep-alive></transition>` 包裹页面。
页面根节点如果是 `v-if/v-else` 且会换元素，进场动画可能被打断并残留 `-enter-from`，
表现为**页面永久透明（白屏）**。
新增页面请保证 template 下第一层是稳定的 `<div class="xxx-page">`。

### 5.9 项目用 hash 模式，改 hash 不会重新加载文档

这在写自动化验证时会咬人：

- **只改 hash 的导航不触发文档重载**，Pinia / 模块级状态不会重新初始化。
  想模拟「F5 / 冷启动深链」，必须先把页面跳到 `about:blank` 再跳到目标地址。
- 报告当前路由要用 `location.hash`，`location.pathname` 在 hash 模式下恒为 `/`，
  永远匹配不上任何业务路径（会得到「所有菜单都跳到 `/`」的假象）。

`scripts/browser-verify.mjs` 里的 `coldStart()` 就是为此封装的。

### 5.10 `el-tree-select` 的 `props` 不接受 `value`

Element Plus 2.14 的 `TreeOptionProps` 只有 `label` / `children` / `disabled` / `isLeaf` / `class`。

```vue
<!-- ✅ 取值字段靠 node-key 指定 -->
<el-tree-select :data="tree" :props="{ label: 'name', children: 'children' }" node-key="id" />

<!-- ❌ 类型报错 TS2353，且运行时无效 -->
<el-tree-select :data="tree" :props="{ label: 'name', value: 'id', children: 'children' }" />
```

来源实现是 `const key = props.nodeKey || props.valueKey || 'value'`，
所以写了 `node-key` 就已经决定了取值字段。

### 5.11 表格插槽的 `row` 不能直接传给强类型函数

Element Plus 的插槽 `row` 类型是 `DefaultRow = Record<PropertyKey, any>`，
它**不会**满足接口的必填属性。给 `#default` 整体加类型标注会报 TS2322。

```vue
<!-- ❌ TS2322 -->
<template #default="{ row }: { row: UserItem }">

<!-- ✅ 在调用点断言 -->
<template #default="{ row }">
  <el-button @click="openEdit(row as UserItem)">编辑</el-button>
</template>
```

### 5.12 `el-button--default` 是**尺寸**类，不是「无 type」

Element Plus 的按钮类名是 `el-button el-button--{type} el-button--{size}`，
而 `size` 的默认值就是 `default`。所以一个 `type="primary"` 的按钮实际类名是：

```
el-button el-button--primary el-button--default
```

**症状**：写了一条规定默认按钮外观的样式后，所有主按钮变成「白底白字」——
亮色下几乎不可见，暗色下碰巧还能看，所以只在亮色截图里能发现。

```scss
/* ❌ 会连主按钮一起改掉：--el-button-bg-color 被覆盖成卡片底色，
      而 --el-button-text-color 仍是 #fff → 白底白字 */
.el-button--default:not(.is-text):not(.is-link) {
  --el-button-bg-color: var(--app-surface);
}
```

**正确做法**：不要去定义「默认按钮」的样式。
EP 的默认按钮已经用 `--el-fill-color-blank` / `--el-border-color` 跟主题走，
而这些都被 `element.scss` 映射到了 `--app-*`，暗色下自然就是对的。

真要按尺寸区别对待，用 `.el-button--small` / `.el-button--large`；
只按「有没有 type」区分时，用属性选择器 `[class*='el-button--primary']` 这类显式写法，
不要依赖 `--default` 的语义。

### 5.13 只在亮色下加的新颜色，暗色会留白底

**症状**：暗色下某个标签、徽标、图标底是一块刺眼的浅色。

**原因**：新加的 `#eff4ff`、`#fdf3e3` 这类浅底只写了亮色一套。
暗色下没有对应令牌，就直接用了亮色值。

**正确做法**：颜色统一进 `variables.scss` 的 `@mixin theme-tokens`，
**两个分支都补**；暗色下把浅底换成「半透明主色」而不是深色实底：

```scss
/* 亮色 */
--tone-amber-soft: #fef6e7;
/* 暗色：半透明能自然融进任何深色表面 */
--tone-amber-soft: rgba(251, 191, 36, 0.14);
```

写完跑 `node scripts/browser-verify.mjs`，§6 的「暗色下无浅色底残留」会按亮度阈值扫出来。

### 5.14 无头 Chrome 的 `prefers-color-scheme` 默认是 `dark`

写主题相关的自动化验证时，**基线与浏览器实现有关**：
无头 Chrome（`--headless=new`）报告的系统偏好是 `dark`，
所以「未选择主题」的页面默认就是暗色，断言会莫名其妙地反掉。

正确做法是在 CDP 连接后显式固定：

```js
await send('Emulation.setEmulatedMedia', {
  media: '',
  features: [{ name: 'prefers-color-scheme', value: 'light' }],
})
```

`scripts/browser-verify.mjs` 的 `emulateColorScheme()` 就是干这个的，
它也顺便用来验证「跟随系统偏好」这条路径。

### 5.15 GORM 的 `default` 标签会把零值悄悄换成默认值

**症状**：新增时明确选了「禁用」，接口返回 200，列表里却是「启用」。前端反复刷新都一样。

**原因**：GORM 对「字段带 `default` 标签、且当前取值等于 Go 零值」的字段，
**会把该列整列从 INSERT 语句里省略**，落库时用数据库默认值。而：

```go
Status int16 `gorm:"default:1"`   // ← 问题在这里
const StatusDisabled int16 = 0    // ← 零值恰好是「禁用」
```

于是 `status = 0` 被当成「没设置」，数据库默认值 `1` 生效 → 记录变成「启用」。

这个缺陷的可怕之处在于**所有常规检查都发现不了**：
`gofmt` 干净、`go vet` 干净、编译通过、接口 200、类型检查通过，
连常规 CRUD 测试也照样通过（因为它通常只测「创建成功」而不回读 `status`）。
它同时存在于单条 `Create` 和批量 `CreateInBatches` **两条独立路径**，只修一处另一处照旧。

**正确做法**：零值是合法业务值的字段，**不要给 `default` 标签**，由应用层显式赋值。

```go
// ✅ status 由应用层显式写入，数据库不给默认值
Status int16 `gorm:"not null" json:"status"`
```

保留 `default` 是安全的场景：零值语义与默认值一致
（`sort = 0`、`parent_id = 0`、`data_scope = 0` 不是合法取值）。
新增字段前先问一句：**「0 是不是一个有意义的业务取值？」** 是就照 `model.User.Status` 写。

回归测试在 `backend/test/status_test.go`，覆盖用户/部门/角色/菜单四条创建路径 + 导入路径，
并且**同时断言数据库真实值与接口回读值**，避免 VO 层兜底把问题重新掩盖。

> 顺带一条：`Updates()` 传 **map** 不受此影响（map 的键一定会被写进 SQL），
> 所以本项目的 `Update` 全部用 `map[string]any`。改写成 struct 会重新引入同一个坑。

### 5.16 软删除的记录仍然占着主键

**症状**：给内置菜单加一个新的按钮权限点时，插入报主键冲突；
但 `SELECT * FROM sys_menus WHERE id = 142` 查不到任何行。

**原因**：软删除只是写入 `deleted_at`，**行还在表里，主键仍被占用**。
`WHERE id = 142` 默认会被 GORM 附加 `deleted_at IS NULL`，所以「查不到」不等于「可以复用」。

**历史成因**：内置菜单当时用**硬编码 ID** 写入，
而「菜单管理」新建的菜单走 `bigserial` 自增，两者共用同一段 ID 空间时就会互相踩。

**约定**：

- **内置菜单的 ID 现在也交给数据库自增**，幂等键是「权限码（按钮）/ 路由名（目录、页面）」，
  于是 `1–999 给内置 / 1000+ 给自增` 那套手工分段作废（原先靠 `seed.MenuIDFloor = 1000` 兜着）
- `seedMenus` / `ensureNode` 按幂等键做**三态处理**：
  命中存活记录 → 复用原 ID 且不覆盖字段；命中**已被软删**的记录（`db.Unscoped()` 查得到）
  → `logger.Warn("内置菜单已被删除，跳过该节点及其子节点")` 并整棵子树跳过；
  都不命中 → 新建。**绝不静默丢弃，也绝不让管理员删掉的菜单自己复活**
- 查真实占用情况要显式绕过软删除：
  `SELECT id, title, deleted_at FROM sys_menus WHERE deleted_at IS NOT NULL`
  （不要用 `WHERE id = N` 判断某号是否空闲 —— 那个判断本身就会被软删除骗过）

**另一条相关约定**：`syncBuiltinRoleMenus` 必须清理**孤儿授权**
（`menu_id` 指向已软删除菜单的 `sys_role_menus` 行）。
不清理会破坏「超管授权数 == 存活菜单数」这个不变量，且症状是莫名其妙的授权数量对不上。

### 5.17 等路由切换不能只靠 sleep，也不能只等「没动画」

**症状**：自动遍历侧边栏时偶发报「菜单 A 点进去还是上一页」；
换个写法后又变成「每次都读到上一页」。

**原因**：固定 `sleep` 撞在动画临界点上会抖；而只判断「没有 `-leave-active` 类」也不行——
从「点击」到「路由真正生效」之间有一段空档，这期间既没有动画类、`location.hash` 也还没变，
会被立刻误判成「已收敛」，于是读到的是**旧页面**。

**正确做法**：同时满足两个条件才算稳定，并要求连续命中两次：

```js
const hashOK = !expectedHash || hash === expectedHash
stable = !busy && hashOK ? stable + 1 : 0
if (stable >= 2) return true
```

见 `scripts/browser-verify.mjs` 的 `waitForRouteSettled()` / `inspectPage()`。

> 另外注意：**批量替换要小心自己**。把 `evaluate(INSPECT_PAGE)` 全局换成
> `inspectPage()` 时，会把 `inspectPage()` 自身实现里的那一处也换掉，
> 造成无限递归——表现是脚本静默卡死、没有任何输出。

### 5.18 中间件读请求体后必须**完整**回填

**症状**：小于 64KB 的导入文件正常，稍大一点的文件一律报「请选择要上传的 Excel 文件」。
用户明明选了文件，提示却指向完全无关的原因，只会反复重试。

**原因**：操作日志中间件为了记录请求体，读了一段又回填回去，但**只回填了读到的那一段**：

```go
raw, _ := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))
c.Request.Body = io.NopCloser(bytes.NewReader(raw))   // ← 超过 64KB 的部分永久丢失
```

multipart 上传的结尾是 boundary，被截掉后 `c.FormFile()` 解析必然失败。
**任何会读取请求体的中间件都有这个风险**，不只是日志。

**正确做法**：把「已读部分」和「未读部分」接回去。

```go
raw, err := io.ReadAll(io.LimitReader(body, maxCaptureBytes+1))
// io.MultiReader 要在 ReadAll 之后构造：此时 body 正好停在未读处
c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), body))
```

更稳的做法是**对不该记录的请求体直接不碰**：`middleware/oplog.go` 只对
`application/json` / `text/plain` / `x-www-form-urlencoded` 做记录，
multipart 等二进制请求体整体跳过，从根上避开回填问题。

回归测试：`TestImportAcceptsLargeFile`（1000 行、> 64KB 的上传文件）。

### 5.19 审计日志不能被**客户端内容**搞挂

**症状**：导入接口的操作日志一条都没有；日志里只有一行 WARN
`操作日志写入失败 {"error": "... (SQLSTATE 22021)"}`。导出接口的 `opAction` 设了却查不到记录。

**原因**：有三个各自独立的坑，后果都是**整条日志丢失**（不是内容被裁剪）：

| 坑 | 触发 | 报错 |
| --- | --- | --- |
| 二进制请求体当文本写 | 上传 xlsx（含 `0x00`） | 22021 非法 UTF-8 |
| 按**字节**截断 | `s[:2000]` 切在多字节中文中间 | 22021 非法 UTF-8 |
| 字段完全没截断 | `Path` 是 `varchar(255)`，超长 URL 直接写 | 22001 value too long |

三者叠加成同一个安全后果：**客户端可以构造请求让自己的操作不留痕迹**，
而失败只写 WARN，主流程毫无察觉。

**正确做法**：

1. 只在**写入边界**统一收敛，而不是散在各个字段上：

   ```go
   func fit(s string, limit int) string {
       s = strings.ToValidUTF8(s, "")     // 剔除非法字节
       runes := []rune(s)                 // 按「字符」而非字节截断
       if len(runes) <= limit { return s }
       return string(runes[:limit])
   }
   ```

   注意 PostgreSQL 的 `varchar(n)` 按**字符**计数，所以截断也必须按字符，
   阈值要与 model 的 `gorm:"size:n"` 严格对齐（见 `oplog` 包顶部的 `col*` 常量）。

2. 非文本请求体不记录，标记为 `(二进制请求体，未记录)`。

3. **落库条件加一条**：写操作，或者 handler 显式 `c.Set("opAction", ...)`。
   导出走的是 GET，只按 HTTP 方法判断会让「数据外带风险最高的操作」完全没有审计。

4. 脱敏必须**递归**：`{"data":{"password":"..."}}` 这种嵌套结构，
   只遍历顶层 key 会把口令原样写进日志。

回归测试：`TestAuditLogCoversExportAndImport`、`TestAuditLogSurvivesHostileInput`、
`TestAuditLogTruncatesLongFields`。

### 5.20 批量导入：预览通过 ≠ 能写进去

**症状**：导入预览显示「2 行全部通过」，点确认后整单失败，只回一句「数据库操作失败」。
用户完全无法定位是哪一行、哪个字段的问题。

**原因**：`sys_user_roles` 的主键是 `(user_id, role_id)`。角色列里把同一个角色写多遍
（`只读访客、只读访客`，或用 `、`/`,`/`；` 混写）会产生重复关联，
批量 INSERT 直接主键冲突 → **整个事务回滚**，已校验通过的行也一起丢掉。
而校验阶段只检查「角色是否存在」，不检查重复，所以预览是绿的。

**正确做法**：

- 校验阶段就对多值字段**去重**（`splitNames` 的结果先过一遍 `seen` map）
- 写库失败时的提示要能指导下一步动作。用 `pgconn.PgError` 判 `Code == "23505"`
  区分「唯一性冲突」与真正的数据库故障，前者回 409 并提示重新校验，
  而不是抛出无信息量的 `ErrDatabase`

**通用原则**：**校验阶段的结论必须与写入阶段一致**。
凡是「校验说没问题、写入才失败」的路径，都要问一句「预览到底校了什么」。

回归测试：`TestImportDedupesRepeatedRoles`。

### 5.21 「选中项」类参数解析失败不能降级成「全部」

**症状**：请求「导出选中项」，拿到的却是整张表。响应完全正常，没有任何异常迹象。

**原因**：

```go
if ids := parseIDList(f.IDs); len(ids) > 0 {
    tx = tx.Where("id IN (?)", ids)
} else {
    tx = filter(tx)          // ← ids=abc 解析为空，悄悄走到这里变成全表导出
}
```

「没传 ids」和「传了 ids 但一个都解析不出来」语义完全不同，前者是按筛选导出，
后者是参数有问题。合并成一个分支等于**把请求语义悄悄放大**，数据量越大越危险。

**正确做法**：先判「参数是否为空」而不是「解析结果是否为空」，解析不出来就报错。

```go
if strings.TrimSpace(f.IDs) != "" {
    ids := parseIDList(f.IDs)
    if len(ids) == 0 {
        return nil, errcode.New(errcode.ErrBadRequest, "选中项参数无效，请重新勾选后导出")
    }
    ...
}
```

**推广**：凡是「筛选条件解析失败 → 退化到无筛选」的写法都要审一遍，
它们在批量导出、批量删除、批量改状态这类接口上等价于数据事故。

回归测试：`TestExportRejectsInvalidIDs`。

---

### 5.22 按字节截断的字符串会让数据库**整行**拒收

**症状**：登录失败锁定完全失效 —— 攻击者可无限次试密码，且每次尝试都不留任何日志。

**成因链**（三环缺一不可，所以只看单点代码很难发现）：

1. `writeLoginLog` 用 `ua = ua[:512]` 截断 User-Agent。User-Agent 完全由客户端控制，
   构造 `511 个 ASCII + 一个汉字`（514 字节）时，按**字节**切到 512 会把汉字切成两半。
2. 截断结果是非法 UTF-8，PostgreSQL 以 `22021` 拒绝**整行** INSERT。
3. 错误被 `_ = s.db.Create(...).Error` 吞掉，日志里连一行告警都没有。
4. `isLocked` 正是按这张表统计失败次数 —— 写不进去就永远累计不到阈值。

**正确做法**：所有写库前的文本都过一次 `pkg/textutil.Fit`（`ToValidUTF8` + **按字符**截断），
并且**日志写入失败必须留告警**。收敛点只保留一处，不要在各模块各写一份：

```go
// internal/pkg/textutil
func Fit(s string, limit int) string {
    s = strings.ToValidUTF8(s, "")
    runes := []rune(s)          // ← 按字符，不是 s[:limit]
    if len(runes) <= limit { return s }
    return string(runes[:limit])
}
```

**自查方法**：搜 `[:` 后面跟着数字或常量名的字符串切片，凡是落在「要写进 varchar 列」的路径上，
就要改用 `textutil.Fit`。PostgreSQL 的 `varchar(n)` 按字符计数，Go 的切片按字节 —— 这两个单位不一致。

回归测试：`TestLoginLogSurvivesLongMultibyteUserAgent`。

---

### 5.23 数据范围必须**同时**约束读和写

**症状**：角色上能配「本部门 / 仅本人」，导出文件里也印着数据范围，但没有任何一条查询用它 ——
配了「仅本人」的角色照样列出并导出全表。属于「看起来有、实际不起作用」的安全控件。

**正确做法**：把范围做成一个 `scopeQuery(actor, tx)` 助手，读路径（列表 / 详情 / 下拉 / 导出）
与写路径（改 / 删 / 改状态 / 重置密码 / 导入）**共用同一份判定**。

- 只收窄列表会留下「看不到但能改」的洞：受限管理员虽列不出别的部门的人，
  却仍能拿已知 ID 重置对方密码。写操作必须走 `ensureInScope`。
- 越界一律按 **404 不存在** 处理，不要返回 403 —— 否则可以用返回信息探测别人有哪些用户。
- 新增 / 导入时还要校验**目标部门**是否在范围内（`checkDeptInScope`），
  否则可以用一份 Excel 把用户批量建到范围之外。
- 新增 `data_scope` 相关查询时注意：`authz` 对身份有 30 秒缓存，
  测试里**直接改库不会失效缓存**，必须走接口修改或显式调用 `authz.InvalidateAll()`。

回归测试：`TestDataScopeIsEnforced`、`TestDataScopeGatesWrites`。

---

### 5.24 无状态 JWT 的吊销：靠 `token_epoch` 而不是「等它过期」

**症状**：管理员重置了某个用户的密码，该用户**此前泄露的令牌仍然有效**，
最长 2 小时（访问令牌）甚至 7 天（刷新令牌）。改密码这个动作给人「已经安全了」的错觉。

**正确做法**：`sys_users.token_epoch` 作为令牌版本号，签发时写进 Claims（`epo`），
`Auth` 中间件比对不一致即 401。修改密码 / 重置密码 / 禁用账号时 `token_epoch + 1`。

**两个容易漏掉的点**：

1. `Refresh` 也必须校验版本号。只挡访问令牌的话，攻击者手里的旧**刷新令牌**
   还能换出一对全新的有效令牌，吊销等于没做。
2. 加 `NOT NULL` 列时别忘了 `default:0`，否则给已有数据的表迁移会直接失败
   （`SQLSTATE 23502: column contains null values`）。这里给默认值是安全的 ——
   令牌版本号的零值与默认值都是 0，与 §5.15 里 `status` 的情况不同。

回归测试：`TestPasswordChangeRevokesTokens`。

---

### 5.25 gin 默认信任所有代理 -> 审计日志里的 IP 可以随便写

**症状**：操作日志与登录日志记录的是客户端自报的 `X-Forwarded-For`，
用 `curl -H "X-Forwarded-For: 203.0.113.77"` 就能让审计记录显示任意来源 IP。

**正确做法**：`engine.SetTrustedProxies(cfg.Security.TrustedProxies)`，
配置项默认**留空**（不信任任何代理，`ClientIP` 取 TCP 对端地址）。
只有真正部署在 Nginx 等代理后面时才填写代理网段，并在 `config.validate()` 里校验格式。

**同类项**：CORS 配了 `*` 时不要再声明 `Access-Control-Allow-Credentials: true`。
把任意来源反射回去 + 允许携带凭证，等于允许任何站点发起「带身份」的跨站请求。

回归测试：`TestAuditIPIgnoresForwardedFor`。

---

### 5.26 老库升级：给已有数据的表加 `NOT NULL` 列必须带默认值

**症状**：给 model 加一个 `gorm:"not null"` 字段后，**新库一切正常，老库直接起不来** ——
`AutoMigrate` 生成的 `ALTER TABLE` 被 PostgreSQL 拒绝：

```
column "token_epoch" of relation "sys_users" contains null values (SQLSTATE 23502)
```

**原因**：加列时既有行会被填成 NULL，而列声明是 `NOT NULL`，
若没有 `DEFAULT`，数据库只能拒绝整条 DDL。

**正确做法**：`gorm:"not null;default:0"`。这**不违反**本项目「不用数据库默认值兜业务字段」
的约定（§7 / `model.User.Status`）—— 那条约定针对的是「零值本身是合法业务值」的字段。
判断标准是**零值语义是否与默认值一致**：`token_epoch = 0` 就是「从未改过密码」，
一致，安全；而 `status = 0` 是「禁用」、默认值 1 是「启用」，相反，有害，所以不加默认值。

**同类风险**：种子/初始化逻辑里的**数据修正**同样只在老库上触发 ——
新库的角色是按最新种子值建的，永远走不到纠正分支。
因此「新库能跑」证明不了「老库能升上来」，这两条路径都要有测试。
数据修正放在 `seed.Run()` 里（本项目没有独立 migration 目录），
且必须满足「只认历史错误值、不覆盖管理员显式配置、幂等」三条（§7.7）。

回归测试：`backend/test/upgrade_test.go` 四个用例。

### 5.27 角色授予必须校验「可授予范围」，否则存在垂直越权

**症状**：普通管理员调 `PUT /users/:id/roles` 传 `roleIds: [1]`（super_admin）就能把自己或别人提成超级管理员——
`RequirePerm` 只校验「有没有分配角色的权限」，不校验「能把哪些角色分给别人」。

**正确做法**：分配/替换角色前调 `authz.EnsureGrantable`，规则是：

- `super_admin` 只有超管本人能授予；
- 目标角色包含的**任何**权限点，操作者自己必须也有——否则等于变相提权（把「用户管理」权限的角色分给自己，就拿到了没被授予的权限）；
- **数据范围不得比操作者更宽**（`1 全部 2 本部门 3 本部门及以下 4 仅本人`，数值越小越宽）。这条最容易漏：一个「不挂任何菜单 + `dataScope=1`」的角色权限码集合为空，只比权限码时会被判为「可授予」，受赠者却拿到了全库可见性；
- `authz.GrantableRoleIDs` 同时用于角色下拉 `Options`，前端根本看不到授不了的角色。

**判定所需字段必须由 authz 自己从库里取**（`loadRolesForGrant`），不能信调用方传进来的结构体：
调用方多半只 `Select("id","name","code")`，`data_scope` 会是零值，
「范围不得更宽」这条判定就会被**静默跳过**——这是一个已经真实发生过的漏洞。

**对称约束**：非超管不能对持有 super_admin 角色的用户执行删除/禁用/改密/编辑
（`ensureTargetNotSuper`），否则可以「先禁用超管，再找超管重置密码」绕一圈拿到超管身份。
注意 `saveErr` 保留 `*errcode.Error`（403），不要 `errcode.Wrap` 成 500。

> **`ensureTargetNotSuper` 要放在 status 分支之前**。只在 `req.RoleIDs != nil` 时调用的话，
> 「不传 roleIds、只传 status」就能禁用另一个超管账号；系统允许多个账号持有 `super_admin`，
> 只挡内置 `id=1` 是不够的。

**授予一共有四条入口，都要过同一套判定**（历史上正是漏了后三条才出的漏洞）：

| 入口 | 落点 | 校验 |
| --- | --- | --- |
| 新建/修改用户的角色 | `user.replaceRoles` | `authz.EnsureGrantable` |
| Excel 批量导入 | `user.ensureImportGrantable` | 同上（**跨行汇总后判一次**，逐行判既慢又拿不到全局） |
| 修改角色属性 | `role.Update` | `ensureRoleMutable` + 数据范围闸门 |
| 给角色分配菜单 | `role.AssignMenus` | `authz.EnsureRoleMenusGrantable`（改角色菜单 = 批量授予） |

回归测试：`backend/test/hardening_test.go` 的 `TestRoleGrantCannotEscalate`、
`backend/test/authz_scope_test.go` 的 `TestImportCannotEscalate` 与
`TestRoleScopeAndMenusCannotWiden`。

### 5.28 登录锁定按「账号+IP」计数，且成功登录必须重置窗口

**症状**：锁定判定只按 username 数失败次数，任何人对着 `admin` 乱打密码就能把管理员锁在门外（锁定 DoS）；
反过来，若按 IP 全局计数，办公网 NAT 后所有同事共用一个出口 IP 会互相连坐。

**正确做法**：`isLocked(username, ip)` 复合计数；窗口起点取「最近一次同账号同 IP 成功登录」，
成功登录天然重置计数器，不用删审计行（审计行要留档）。此外 IP 维度还有一道独立的
`FailLimiter`（内存滑动窗口，`security.ip_max_login_fail`，0 关闭），先把纯爆破流量挡在查库之前。

**不要**为了「重置计数」去删 `login_logs` 的行——那是审计数据。

回归测试：`TestLoginLockoutIsScopedToAccountAndIP`、`internal/pkg/limiter` 单测。

### 5.29 客户端可控的标识符必须校验字符集

**症状**：`X-Trace-Id` 直接透传进响应头和日志。值为 `abc\r\nX-Foo: bar` 时发生响应头注入；
含 ANSI 转义序列时污染终端日志。**报文格式类注入的入口永远是「字符串进结构化字段之前」。**

**正确做法**：白名单正则 `^[A-Za-z0-9_-]{1,64}$`，不匹配就生成新的，而不是报错——
trace id 本来就是可丢弃的调试信息，拒绝服务没有必要。同样的思路适用于所有
「客户端传入、会被写进日志或响应头」的字段。

回归测试：`TestTraceIDRejectsInjectedHeader`、`TestTraceIDSanitized`。

### 5.30 批量导入的「已见过」状态只能记到整行校验通过之后

**症状**：导入校验里 `seen[username] = true` 在**用户名格式校验一通过**就写，
后面行号校验失败时用户名已经占了坑——同一用户名第二次出现被误报「表内重复」，
错误行号指向的却是一条本该合法的数据，用户对着模板对半天找不出问题。

**正确做法**：`seen` 的写入必须收敛到「该行全部校验通过」之后（`len(errs) == 0`）。
凡是「跨行状态」都要问一句：这个状态是在哪个校验阶段成立的？

回归测试：`backend/internal/module/user/excel_validate_test.go`。

### 5.31 数据范围的口径必须只有一份实现，且读与写一起收窄

**症状**：第一轮加固把「数据范围」的判定做进了 `user` 模块，其余模块各写各的或干脆不写，
于是「本部门及以下」的部门管理员：能列用户列表（收窄了），却能看全公司部门树（dept 模块无 actor）、
在首页读到全库人数（dashboard 无范围）、看到别人的登录 IP 与 User-Agent（recent-logins 无范围）。
典型的**「读漏了」型越权**——判定逻辑本身没错，错的是「在哪些地方必须调用它」。

**正确做法**：把范围解析收敛到 `authz.ResolveScope` / `authz.Scope` 唯一实现，
各模块只做薄封装。要点：

1. **唯一的 Scope 实现**。`Scope` 提供 `ApplyUser`（叠在 `sys_users` 查询上）、
   `ApplyLog`（叠在带 `user_id` 的日志表上）、`AllowsDept` / `AllowsUser`（纯判定）。
   三个模块共用，任何一处漏掉都是编译期或一行代码的事，不再靠自觉。
2. **日志表不能用 `UserCond`**。`login_logs.user_id` 可能为 0（未识别出账号的失败登录），
   这类记录不属于任何人的范围，非超管一律看不到——所以要单独一个 `LogUserCond`。
3. **写接口也要调**。「看不到但能改」依然是越权；`dept` 的 Update / Delete / Create
   都要判目标部门与**父部门**都在范围内。只判目标会留下「把本部门挂到范围外父节点下」这条路。
4. **聚合字段同样要收窄**。部门树裁剪了但 `userCount` 还是全库的，等于换个字段继续泄露。
5. **越界按「不存在」返回**（404），不要用 403，否则可以用返回信息探测其他部门有哪些数据。

回归测试：`backend/test/data_scope_test.go`（6 个）。

### 5.32 会参与安全判定的日志，不能被「清空日志」接口删掉

**症状**：`isLocked` 判定账号锁定靠的是「窗口内 `login_logs` 的失败条数」。
而 `DELETE /system/logs/logins` 可以把全表删光——于是**清空登录日志 = 解除所有账号的锁定**，
攻击者只要能触发锁定就能立刻恢复无限次尝试；管理员误点一次同样会解锁。

**正确做法**：给删除加**硬下界**——锁定期内的记录一律保留（`log.ClearLogins` 里的 `earliest`）。
注意这不是「要不要留审计证据」的问题，而是「这份数据是安全判定的输入」，
删它等于给攻击者一个免费的解锁接口。

同理适用于任何「既是审计数据、又被安全判定引用」的表：要么拆表，要么在删除路径上单独设护栏。

回归测试：`backend/test/data_scope_test.go` 的 `TestClearLoginsKeepsLockWindow`。

### 5.33 半更新接口的可选字段必须是指针，且 PATCH 语义要能显式清空

**症状**：与 5.17 同源。`profile.Update` 把 nickname/email/phone/avatar 四个字段
全部写进 `Updates` 的 map，只改昵称的请求会把邮箱、手机号、头像一起清空。

**正确做法**：改成 `*string` 三态——`nil` 不写、非 nil 写值、**显式传 `""` 即清空**。
这里有个连带坑：**指针字段不能用 `binding:"omitempty,email"`**。
validator 的 `omitempty` 判断的是「指针是否为 nil」，显式传空串清空时指针非 nil，
`email` 规则照常生效并报「邮箱格式不正确」——清空这条路被堵死。
解法是格式校验下移到服务层，只在非空时生效。

**不要**用一个 `ClearEmail bool` 之类的显式标志位来代替指针：多一套要维护的状态，
而且前端「字段值为空」和「字段没传」两种情况本来就只能靠「有没有这个 key」区分。

回归测试：`backend/test/data_scope_test.go` 的 `TestProfilePartialUpdateKeepsFields`。

### 5.34 权限指令不能「摘 DOM」，只能打标记 + CSS 隐藏

**症状**：旧的 `v-perm` 在无权限时用 `replaceChild` 把元素换成注释节点占位。
表面上功能正常，实际有三个结构性问题：

1. **`updated` 钩子永远不触发**。指令钩子里调 `useUserStore()` 会因为
   `pauseTracking` 的关系**不建立响应式依赖**，权限在运行期变化时元素不会重新判定。
2. **破坏 Vue 的 vnode 锚点**。`el-table` 的单元格、`el-dropdown` 的触发器都靠 DOM
   位置维持内部状态，手动摘掉元素后，后续 patch 会把内容写进已被丢弃的 vnode。
3. **注释节点会泄漏**。`hostRemove` 在 `parentNode === null` 时是 no-op，
   异常路径下占位注释留在 DOM 里，`querySelectorAll` 之外的手段能直接看到它们。

**正确做法**：只打属性、不动结构。

```ts
el.setAttribute('data-perm', allowed ? 'granted' : 'denied')
el.classList.toggle('perm-denied', !allowed)
```

样式用 `visibility: hidden` + `pointer-events: none` + `width: 0` + `overflow: hidden`，
**不用 `display: none`**——后者会影响 `el-dropdown` / `el-table` 的尺寸测量与浮层定位。

运行期权限变化靠**独立的 `effectScope` + `watch`**（在 `mounted` 里创建，
运行在正常响应式上下文中）解决，不要试图在 `updated` 里补救。

### 5.35 「排除子树」不能用循环累加器，要逐节点重算

**症状**：部门编辑弹窗的上级下拉里，**兄弟节点消失**。编辑「技术部」时看不到「运营部」。

**原因**：排除集合用的是一个「命中即置位、且置位后不再清除」的累加器。
一旦在某个兄弟节点上命中「自己」，`inside` 就会沿着递归传染给它的**所有后代**，
等于把整棵同级子树都排除了。累加器只能表达「或」，表达不了「以自己为根的子树」。

**正确做法**：把「是否在自己子树内」作为递归的**参数**逐节点重算，而不是靠外部状态累积。

```ts
const walk = (nodes: DeptNode[], inSubtree: boolean): void => {
  for (const node of nodes) {
    const inside = inSubtree || node.id === editingId.value
    if (inside) excluded.add(node.id)
    if (node.children?.length) walk(node.children, inside)   // 把结论传下去
  }
}
```

凡是「收集需要排除的节点」这类逻辑，都检查一遍是不是用了不可逆的累加器。

### 5.36 并发 401 会弹 N 个模态框，必须有去重闸门

**症状**：页面首屏并发 6 个请求，令牌恰好失效时弹出 6 个「登录已过期」对话框。

**原因**：每个响应拦截器各自独立地判断「要不要提示」，彼此之间没有任何共享状态。
刷新令牌又是并发的，多个请求会同时拿着同一个 refresh token 去换新令牌。

**正确做法**：模块级闸门 + 显式重置函数。

```ts
let sessionExpiredNotified = false
export function resetSessionExpiredGuard(): void { sessionExpiredNotified = false }
```

闸门在**刷新成功后**、以及 `store.user.login` / `store.user.reset` 时都要重置，
否则用户重新登录后仍会被旧闸门吞掉提示。

`resetSessionExpiredGuard` **必须用函数声明**（有提升），不能用 `const` 箭头函数：
它在 `refreshAccessToken` 之后才被调用，箭头函数无提升，首轮 401 会撞 TDZ。

顺带两个同源的口径要对齐：

- `AUTH_ERROR_CODES` 必须**包含全部**认证类错误码（含 `40103` 账号锁定），漏一个就绕过闸门
- `AUTH_ENDPOINTS` 必须**豁免 `/auth/logout`**，否则退出登录时拿 401 又弹一次「已过期」

### 5.37 keep-alive 的 `:key` 里绝不能拼「刷新计数」

**症状**：每点一次刷新按钮，内存里的组件实例就多一份，且永不回收。

**原因**：`keep-alive` 按 `key` 缓存 vnode。`:key="route.path + '-' + refreshKey"`
每次刷新都产生一个**新 key**，旧 key 的缓存条目不会被清除——连带定时器、监听器、
DOM 引用一起泄漏，点几十次就能看到内存明显上涨。

**正确做法**：`key` 只用 `route.path`，刷新靠**把当前页面从 `include` 名单里摘掉一帧**。

```ts
const refreshing = ref(false)
provide('pageRefresh', () => {
  refreshing.value = true
  nextTick(() => { refreshing.value = false })
})
```

模板：`<keep-alive :include="refreshing ? [] : cachedViews">`。

### 5.38 限流器的「检查」与「记账」必须在同一次加锁里完成

**症状**：`RateLimit` 中间件用 `Allow(key)` 判断、`Fail(key)` 记账，两步各自加锁。
100 个并发请求可以**全部**通过 `Allow`（都读到「还剩 1 格」），然后全部 `Fail`，
阈值被突破 20 倍。限流器恰好在它最需要生效的场景（爆破）下失效。

**正确做法**：提供原子方法 `AllowAndConsume`，一次持锁完成检查 + 占额度。

```go
func (l *FailLimiter) AllowAndConsume(key string) bool {
    // 一次加锁：读窗口内计数 → 判断 → 追加时间戳
}
```

`Allow` / `Fail` 仍然保留，但**只用于「只在失败时记账」的场景**
（登录的 `ipFails`：Allow 通过不代表要记账，只有密码错/用户不存在才 Fail）。
凡是「每请求占一格」的地方，一律用 `AllowAndConsume`。

### 5.39 内存里的 map 必须有硬上界

**症状**：限流器的 `items` map 靠 GC 周期清理，文档里写着「上界 ≈ 攻击速率 × 窗口时长」。
但攻击者用**随机源 IP** 刷时，这个上界是攻击者自己决定的——高速率下无界增长，
最终 OOM。map 分配失败是不可 recover 的，整个进程直接死。

**正确做法**：加 `maxKeys` 硬上界（默认 1 万），超出后**新 key 不再记录、直接放行**，
并计入 `evicted` 供监控观察。

这里的取舍要写清楚：**宁可放过攻击，也不能 OOM**——限流器误伤正常用户会导致
业务不可用，比漏放几个攻击请求更糟。所以超上界时是 fail-open，不是 fail-closed。

### 5.40 JWT 验签必须锁死算法并校验 issuer

**症状一**：keyfunc 里只断言「是 `SigningMethodHMAC` 的子类」。
`HS384` / `HS512` 同样属于 HMAC，一样能通过——等于把「用哪个算法验签」
的决定权交给了签发方。

**正确做法**：用 `jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})`，
并在 keyfunc 里显式比对 `t.Method.Alg()`。

**症状二**：`RegisteredClaims.Issuer` 签的时候写了，**从来没人比对**。
多环境共用一份 `jwt.secret` 是很常见的配置，不校验 issuer 时，
别的系统签发的令牌能直接拿到本服务会话用。

**正确做法**：`jwt.WithIssuer(m.issuer)`，并在解析后再兜一次 `claims.Issuer != m.issuer`。
两道断言都要有——上游库版本差异会影响 `WithIssuer` 的行为，兜底那行不能省。

顺带一条：**`UserID <= 0` 的令牌直接判无效**。各处都拿它做数据范围判定，
0 值会落到「查不到用户」的分支而不是明确报错，属于把非法输入变成了静默的错结果。

### 5.41 `pageSize` 封顶了不代表 `page` 也封顶

**症状**：`Offset() = (Page-1) * PageSize`。`PageSize` 收敛到 200 了，
但 `Page` 来自 `strconv.Atoi(c.Query("page"))`，可以是任意大。
`?page=99999999` 算出约 **2×10⁹ 的 OFFSET**，PostgreSQL 执行
`LIMIT 200 OFFSET 2000000000` 必须先扫描并丢弃这 2 亿行——
单个请求就能占满连接池几分钟，CPU/IO 曲线与真实业务量完全无关。
更糟的是 `?page=9223372036854775807` 让 `(Page-1)*PageSize` **整型溢出成负数**，
SQL 里出现负 OFFSET 直接报错。

**正确做法**：`Normalize` 里按 `maxOffset` 反推页码上界并夹住，`Offset()` 再兜一层。

```go
maxPage := maxOffset/q.PageSize + 1
if q.Page > maxPage { q.Page = maxPage }
```

**凡是「客户端可控的整数直接参与算术运算」的地方，都要检查溢出与量级。**

### 5.42 聚合查询不能吞 error

**症状**：`role.toVOs` 里两条聚合查询写着 `_ = s.db...Scan(...)`。
DB 一抖动，角色列表就显示「所有角色都没有菜单、没有用户」。

**正确做法**：`toVOs` / `toVO` 返回 `error`，把 `ErrDatabase` 往上传。

**判断标准**：吞掉 error 只有一种正当理由——这个值**确实无关紧要**。
「查不到」和「没配置」是两回事，把前者渲染成后者是把故障伪装成业务结论，
管理员会据此做出错误判断。`toVOs` 里已经注释写明「查询次数与分页大小无关」，
那更要报错——性能优化的前提是结果正确。

### 5.43 `ch == nil` 时静默 return 是「审计链断了却没人知道」

**症状**：`oplog.Write` 在 `ch == nil`（`Init` 没被调用）时直接 `return`。
后果是**所有**审计日志无声消失，页面上看不到任何异常，没有任何日志。

**正确做法**：打一条 `logger.Error`，并且**只打一次**
（否则每个请求刷一行，反过来把真正的日志淹掉）。

```go
var initWarnOnce sync.Once
func reportUninitialized() {
    initWarnOnce.Do(func() { logger.Error("操作日志未初始化，审计日志全部丢失", ...) })
}
```

与 §5.38、§5.42 同源：**失败路径上的静默，比失败本身更危险。**

### 5.44 受控组件：取消/失败必须显式改写绑定值

**症状**：`el-switch` 的 `v-model="row.status"`，用户点一下内部状态翻转，
`@change` 弹出确认框；点「取消」时代码直接 `return`，什么都没做。
结果：开关看起来已经变了，实际没变——**受控组件的 UI 状态不是自己说了算**。

**正确做法**：取消和请求失败两条路径都要**显式把绑定值写回原值**。

```ts
const origin = row.status
try {
  await ElMessageBox.confirm(/* ... */)
} catch {
  row.status = origin   // 取消：显式回滚
  return
}
try {
  await userApi.updateStatus(row.id, next)
  row.status = next
} catch {
  row.status = origin   // 失败：同样回滚
} finally {
  loadList()            // 最终以服务端为准
}
```

**为什么必须显式赋值**：`:model-value` 单向绑定下，组件内部值已翻转但绑定值没变，
Vue 认为无变化 → 不重渲染 → UI 停在「看起来改了、其实没改」。
主动写一次绑定值才能触发响应式更新。

**配套**：加 `submitting: Set<id>` 拦连点（`el-switch` 的 `:loading`），
否则用户狂点会发出一串状态翻转请求。

### 5.45 `reserve-selection` 是双刃剑：翻页保留，改条件必须清

**症状**：用户勾了 3 行，翻到第 2 页再勾 2 行，回到第 1 页发现 5 行都选着——
这是 `reserve-selection` 的**设计意图**（跨页批量操作），不是 bug。
真正的 bug 是：改了筛选条件（搜索关键字、清空查询）后旧选择还在，
用户点「导出选中项」会导出**两个查询条件混合**的数据。

**正确做法**：`reserve-selection` 保留，但 `search()` / `resetQuery()` /
删除成功后主动清：

```ts
function clearSelection(): void {
  tableRef.value?.clearSelection()   // 需要 <el-table ref="tableRef">
}
```

**判断标准**：**保留选择的前提是「数据集合没变」**。翻页没变，改筛选条件变了。

### 5.46 `el-pagination` 改 pageSize 会双发 `change`

**症状**：切换「每页条数」时父组件收到**两次** `change`，列表接口发两遍。
原因：`el-pagination` 内部在 pageSize 变化时会自行重算 current-page，
于是 `size` 和 `currentPage` 两个 setter 各触发一次 `emit('change')`。
`el-table` 自身的 size-change/change 同理。

**正确做法**：同 tick 内归并，**不要用防抖**（防抖会延迟翻页点击，手感变差）。

```ts
const changePending = ref(false)
function emitChangeOnce(): void {
  if (changePending.value) return
  changePending.value = true
  void Promise.resolve().then(() => {   // 微任务：同 tick 内的多次触发只发一次
    changePending.value = false
    emit('change')
  })
}
```

顺带在 setter 里加 `if (value !== props.pageSize)` 挡掉重复赋值。
改公共 `Pagination.vue` 一处，user/role/dept/menu 四个页面同时受益——
**这类共性缺陷优先在组件层修，不要每个页面各修一遍。**

### 5.47 `formRef.value?.validate()` 返回 undefined 被 `if (!valid) return` 静默吞掉

**症状**：首屏点「登录」毫无反应，连报错都没有。

**根因**：`formRef.value?.validate().catch(() => false)` —— 当 `formRef.value`
是 `undefined` 时，optional chaining 让整个表达式变成 `undefined`；
`if (!valid) return` 把 `undefined` 当成「校验未通过」直接吞掉。
用户看到的是「点了没反应」。

**正确做法**：三道防线。

```ts
if (loading.value) return                          // ① validate 是异步的，validate→loading 置位有空窗，防连点
if (!formRef.value) {                             // ② formRef 兜底，必须给用户明确提示
  ElMessage.warning('表单尚未就绪，请稍后重试')
  return
}
const valid = await formRef.value.validate().catch(() => false)
if (!valid) {
  focusFirstInvalidField()                        // ③ 焦点落到第一个出错字段，别让用户自己找
  return
}
```

**配套**：只挂 `trigger: 'blur'` 是不够的。用户「输入 → 直接回车」这条
最常见路径**全程不失焦**，错误提示要到点登录往返后才出现。
密码这类「长度/强度」规则必须 `trigger: ['blur', 'change']`。

**判断标准**：`?.` 和 `&&` 一样，都是**静默的失败通道**。
链式调用里只要有一步可能缺席，就必须显式处理缺席分支——
否则错误会被当成「正常但不通过」而无声消失。

### 5.48 用 CDP 合成事件验证 `el-checkbox` / `el-switch` 会假阴性

**症状**：用 `element.click()` 或 `dispatchEvent(new MouseEvent('click'))`
驱动 Element Plus 的 `el-checkbox` / `el-switch`，DOM 上 `.is-checked` 已经变了，
但页面的计数不变 —— 断言 FAIL，改代码改到怀疑人生，实际代码是对的。

**根因**：这些是**受控组件**，内部状态由 Vue 响应式数据决定，
合成事件驱动不了响应式系统。必须走浏览器的**真实命中测试**路径。

**正确做法**：用 CDP `Input.dispatchMouseEvent` 按元素中心坐标点真实鼠标。

```js
await cdp('Input.dispatchMouseEvent', { type: 'mousePressed', x, y, button: 'left', clickCount: 1 })
await cdp('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button: 'left', clickCount: 1 })
```

**另一个假阴性**：`v-perm` 隐藏的元素走 `visibility: hidden`，
此时 `innerText` **取不到内容**。验证计数要改用 DOM 状态
（如 `document.querySelectorAll('.el-table__body .el-checkbox.is-checked').length`），
不要断言文案。

**教训**：验证脚本报 FAIL 时，先证明「脚本是对的」再改产品代码。
双向验证（把修复临时退回去，确认用例真的会 FAIL）能省掉大量无效排查。

### 5.49 「改角色」和「建角色」本身也是授权行为

**症状**：`AssignMenus` 挂了 `EnsureRoleMenusGrantable`，但 `Update` 的
`replaceMenus` 和 `Create` 都没有——持有 `system:role:update` 的非超管可以把一个
**已挂用户的角色**改成任意菜单组合（比如加上自己都没有的 `system:user:delete`），
该角色现有持有者在 authz 缓存失效后立刻拿到新权限；`Create` 更是连 actor 都不收，
非超管可以直接造出 `data_scope=全部数据` 的角色。

**正确做法**：三条路径（授予用户 / 修改角色菜单 / 创建角色）都要过同一道防线：

```go
// Update：事务前校验（与 AssignMenus 一致）
if req.MenuIDs != nil && r.Code != authz.SuperRoleCode {
    if err := authz.EnsureRoleMenusGrantable(actor, req.MenuIDs); err != nil {
        return err
    }
}
// Create：范围不宽于自身 + 菜单不超出自身权限码
if actor != nil && !actor.IsSuper && scope < actor.DataScope { ... }
if err := authz.EnsureRoleMenusGrantable(actor, req.MenuIDs); err != nil { ... }
```

**判断标准**：凡是「能把权限写进系统持久层」的入口，都是授权行为。
只堵一条路径的防线等于没堵——审计时要列出同一资源的**全部**写入口。

### 5.50 兜底路由会吃掉冷启动深链，404 页永远不能无条件放行

**症状**：把「404 页免登录」实现成 `if (to.name === 'NotFound') next()` 放在守卫前部。
冷启动直接访问 `/system/user` 时，动态路由还没注册，这个路径**先命中兜底路由**
`/:pathMatch(.*)*` → `to.name === 'NotFound'` 成立 → 直接放行显示 404，
路由注册流程根本走不到——§5.1 修过的深链冷启动被整个打破。

**正确做法**：404 页不进白名单。未登录访问未匹配路径统一回登录页（带 redirect）；
登录后冷启动深链先走「拉资料 → 注册动态路由 → 重新导航」，注册后仍不匹配
才会看到真正的 404 页。

```ts
if (!userStore.isLoggedIn) {
  next({ path: '/login', query: { redirect: to.fullPath } })
  return
}
// ……之后才是动态路由注册流程
```

**教训**：`to.name === 'NotFound'` 在「路由注册前」是**无信息**的——
它不代表「用户敲错了地址」，只代表「路由表还没就绪」。
给守卫加放行分支前，先问：这个条件在冷启动的第一跳是否也成立？

### 5.51 指令语义改了，验证断言必须跟着改

**症状**：§5.34 把 v-perm 从「摘 DOM」改成「CSS 隐藏」后，
`browser-verify.mjs` 里「写操作按钮被 v-perm 移除」的断言还在用
`textContent` 判断——隐藏的按钮 `textContent` 依然存在，断言永远 FAIL，
但它跟产品是否正确毫无关系（真实状态：`perm-denied` 类已挂、
`visibility:hidden` 已生效、`body.innerText` 里根本没有这些文字）。

**正确做法**：断言跟着语义走——检查**可见性**（`getComputedStyle().visibility`
+ `getBoundingClientRect().width`），并额外锚定新语义本身
（被拒按钮应带 `perm-denied` 标记且**仍在 DOM 里**）。

**判断标准**：改任何「实现机制不变、对外行为不变」的内部实现时，
全局搜一遍验证脚本里引用旧机制字眼的断言。**过期断言比没有断言更糟**——
它会把「实现正确」误报为「回归」，训练人去忽略验证结果。

### 5.52 按需加载的标志位必须「成功才置位」

**症状**：`ensureXxx()` 里先 `flag = true` 再 `await load()`。
首次加载一旦失败，标志永久为真，之后所有调用直接 return——
tab 永远空白且没有任何重试机会。

**正确做法**：

```ts
async function ensureOperations(): Promise<void> {
  if (operationLoaded.value) return
  try {
    await loadOperations()
    operationLoaded.value = true // 只有成功才标记：失败后还能重试
  } catch { /* 提示由拦截器统一处理 */ }
}
```

**同源问题——列表加载的响应竞态**：快速连续查询时「先发的慢响应」会覆盖
「后发的快响应」。加载序列要有序号守卫（见 `src/composables/useLatestGuard.ts`）：
每次 `start()` 拿票据，响应回来 `isCurrent(ticket)` 为 false 就丢弃；
`finally` 里也要判，否则过期请求提前返回会把 loading 提前关掉。

### 5.53 setup 顶层的 `document.addEventListener` 不会随组件卸载

**症状**：`Navbar.vue` 的 `fullscreenchange` 监听器写在 `<script setup>` 顶层。
Layout 挂载 Navbar，每次「退出登录 → 再登录」都重新 setup 并**再挂一个**，
旧监听器持有已卸载组件的 ref，持续泄漏。

**正确做法**：`onMounted` 注册、`onBeforeUnmount` 移除，且回调用具名函数
（匿名函数无法 remove）。

**同源问题——`setTimeout` 猜渲染时机**：`setTimeout(60)` 后取树 ref 再
`setCheckedKeys`，机器慢或树大时 ref 还没挂载，调用静默 no-op，
表现为「权限勾选回显随机丢失」。改为在 `nextTick` 微任务帧里**轮询 ref**
（有上限），就绪即执行，超时给出用户可见的提示。

---

### 5.54 测试禁止修改共享种子数据——污染是跨运行的、静默的

**症状**：菜单权限子集测试直接 `PUT /system/menus/11` 把种子菜单的权限码
改成「自己没有的码」来验证 403。双向验证（临时禁用防线）时该请求**成功**，
脏数据永久写进测试库；此后每次全量测试，探针用户的权限集都含了这个码，
防线恢复后子集校验反而「放行」，测试间歇性 FAIL 且与代码改动无关。

**正确做法**：
1. 断言越权用**自建探针资源**（先建自己持有码的按钮，再尝试改成越权码），
   不碰任何 seed 出来的记录；
2. 若历史版本已可能留下脏数据，测试开头做**幂等自愈**（无条件把种子数据
   恢复为正确值），并在注释里说明为什么；
3. 自愈本身要走 Update 时注意 §5.55——部分字段恢复会清掉没传的字段。

**同源问题——模糊匹配污染断言**：用 `?username=admin`（ILIKE %admin%）统计
「admin 的日志数」做范围清空断言，但其他测试建的探针用户
（`dept_admin_probe`、`esc_probe_admin`）恰好都在同一个部门且用户名含
"admin"，A 清掉自己范围内记录属**正确行为**，却让计数下降造成误报。
精确场景的信号源要用全库唯一、不含通配歧义的标识。

### 5.55 Update 是整体覆盖——「补丁式恢复」会清掉未传字段

**症状**：为修复菜单 11 被污染的 permission，用 `PUT /system/menus/11`
只传 `permission` 等部分字段。menu 的 Update 是全字段覆盖
（`updates["component"] = req.Component`），未传的 `component`/`icon`/
`keepAlive` 被清成零值 → 路由树缺组件路径 → `TestProfile/动态路由树` FAIL。

**正确做法**：对全量覆盖型 Update 做数据修复时，要么传全量字段，
要么直接 SQL 精确 UPDATE 目标列。凡是「只想改一个字段」的调用，
先确认该接口是 PATCH 语义还是 PUT 全量语义。

### 5.56 异步落盘的基础设施，测试断言必须轮询等待

**症状**：oplog 批量落盘（64 条或 2 秒 flush）。测试做一次写操作后立即
查询操作日志，断言「应有 1 条」偶发失败——记录还在 channel 里没落库。
带 `sleep` 固定等待则要么浪费要么不够。

**正确做法**：轮询 + 上限（如 20 次 × 300ms），可见即通过，超时给明确
的失败信息。同类基础设施（异步审计、异步统计、消息队列）一律如此，
不要在「概率性失败」和「固定大 sleep」之间二选一。

### 5.57 组合式函数里 `reactive<泛型>` 传给回调，TS 会拒绝

**症状**：写 `useTableQuery` 时这样声明，`fetch(query)` 报错：

```ts
const query = reactive<TableQuery<F>>({ page: 1, pageSize: 10, ...initial })
const data = await fetch(query)
// Argument of type 'Reactive<TableQuery<F>>' is not assignable to parameter of type 'TableQuery<F>'.
//   'Reactive<...>' is assignable to the constraint of type 'F',
//   but 'F' could be instantiated with a different subtype of constraint 'PageQuery'.
```

**根因**：`reactive` 的返回类型是 `Reactive<T>` = `UnwrapNestedRefs<T>`。参数是**泛型**时，
TS 无法证明 `UnwrapNestedRefs<F & {...}>` 就等于 `F`——它能证明的只有「满足 F 的约束」，
而约束满足不等于类型相等，于是拒绝赋值。

**正确做法**：**先让 TS 推断、再一次性断言**，不要在 `reactive` 上写泛型实参：

```ts
// 推断出具体类型后断言收口；写成 reactive<TableQuery<F>>(...) 会在另一个位置
// 报「对象字面量不满足 F 的未知成员」，换汤不换药
const query = reactive({ page: 1, pageSize: 10, ...initial }) as TableQuery<F>
```

断言只出现在组合式函数内部一处，调用方拿到的仍是完整类型。同类问题在
「泛型参数 + Vue 包装类型（`reactive`/`ref`/`computed`）」组合下都会复现。

### 5.59 手工分配的 ID 是隐式契约，抄进第二份清单就开始漂移

菜单原来用硬编码 ID（`1`、`10`、`101~149`），`seed.go` 里既要定义它、又要
在 `builtinRoleGrants()` 里把这些数字再抄一遍做授权。两份清单必须手工同步，
改歪一处就是**静默的权限错配**：角色少一个按钮、或者多一个本不该有的。

**改成按语义定位**：菜单的幂等键用权限码（有权限码的节点）或路由名（纯目录），
授权规则按节点语义（类型 / 权限码后缀 / 命名空间）判定。于是：

- 主键交给数据库自增，清单里一个数字都不用写；
- 老库平滑迁移 —— 历史记录带着同样的权限码/路由名，照样被认出来复用，
  ID 与已有授权关系一个都不动；
- 新增模块无需回来改授权清单。

**判断标准**：如果两份清单里出现同一批字面量（ID、路径、权限码），它们迟早会分叉。
让其中一份只描述**意图**，让另一份在运行时**求解**出具体值。

### 5.60 框架层 import 具体模块，那个模块就变成「删不掉」

「业务模块能不能整块删掉」不是靠自觉，而是取决于框架层认不认识它。
只要有一个框架包写了 `import "…/internal/module/invflow"`，
删掉 invflow 就是编译失败 —— 这比「不敢删」更硬。

做法：框架层只认**契约类型**（`module.Module` / `module.MenuNode`），
把「有哪些模块」这件事整体交给组装层 `internal/assembly`。
需要配置的模块在自己的包内构造闭包（`log.Module(cfg.Security)`），
于是契约包连 `config` 都不必依赖。

`scripts/check-module-boundary.sh` 钉住这条线。它比「grep 业务模块名」更强的地方是
把「框架包引用 `internal/module/<任意子包>`」整体禁掉 —— 新增模块不必维护白名单。

### 5.61 新版 Element Plus 的 `el-select`，选中值不在 `input.value` 里

自动化验证里读下拉当前值时，**不要读 `input.value`**：新版 `el-select`
把选中值渲染进 `.el-select__selected-item`，`<input>` 只剩过滤框的作用，
永远为空。照旧读会得到「明明选了却读成空」的假失败，然后你会去改本来正确的业务代码。

顺带一个同源的坑：**`el-dialog` 里按 Escape 会关掉整个弹窗**（`close-on-press-escape`
默认为 true）。用 Escape 收 `el-select` 的浮层时，后续所有断言都会落在已关闭的
DOM 上，表现为「改完之后所有字段都读不到」。收起浮层改成点弹窗标题。

### 5.58 没有调用方的 prop 是负债，别「留着以后用」

**症状**：`components/EChart.vue` 有个 `loading` prop，转发到
`echarts.showLoading`，还为它从设计令牌读三个色值。全项目**没有任何调用方**
传过它——页面级 loading 一律由 `v-loading` 指令负责。

**为什么要删而不是留**：这类「预留」代价不是几行代码，而是它会**永久绑住**别的东西：
`readToken` 的引入、一套色值的亮暗一致性、一段永远走不到的分支（改主题时还得验它）。
删掉后 `readToken` 的 import 也随之消失。判断标准很简单：**搜一遍 prop 名，
没有传参处就是死代码**。

**同类信号**：`onMounted` 里注册但没人 await 的回调、`defineEmits` 里没人 `@` 的事件、
`defineExpose` 里没人 `ref` 的方法。审计时按「谁在用」而不是「看起来有用」来判断。

### 5.62 「首个菜单即落地页」：`sort` 平局时 `id` 决定胜负

**症状**：模块清单读起来「业务排第一」，登录后却落在另一个页面上。

**根因**：前端落地页由 `firstLeafPath(menus)` 推导——取菜单树里第一个可访问的叶子。
菜单树按 `sort ASC, id ASC` 取，而 `Sort` 只是模块清单里的**声明值**：
两个顶级节点都声明 `Sort: 1` 时，平局由 `id` 决胜，**老库里先建的赢**。
新库不受影响（`Modules()` 顺序决定插入顺序），所以这类缺陷**只在老库上出现**，
本地全新建库时完全复现不了。

**现状（v1.1 起）**：框架仪表盘（`dashboard`）已整体下线，
「数据概览」是 `inv` 注册的**顶级菜单**（`Sort: 1`），严格小于其它顶级节点，
不存在平局。老库升级时 `ensureLandingOrder` 会把它从「进销存」目录下
**提取为顶级菜单**（`parent_id = 0`）——`ensureNode` 对已存在的记录一律不覆盖字段，
所以「挪位置」这种结构变更必须显式写迁移代码，改声明值对存量行无效。

**教训**：任何「靠列表顺序决定行为」的机制，都要问一句平局时谁赢、存量数据会不会翻盘。
声明式配置只在**首次写入**时生效，收敛存量数据必须显式写一段校准代码。

### 5.63 事务里 `Wrap(ErrDatabase, err)` 会把业务错误码冲成 50100

**症状**：出库数量超过库存，接口返回 `50100 数据库操作失败`，
而不是「商品「XX」库存不足（现有 12.00）」的 40900。

**根因**：业务校验是**带错误码抛出来的**（`errcode.Newf(ErrConflict, ...)`），
事务返回后又被无脑套了一层 `errcode.Wrap(errcode.ErrDatabase, err)`。
`Wrap` 不 `Unwrap` 原错误码，只是把它藏进 `Err` 字段里，
`response.Fail` 的 `errors.As` 先命中外层 `*errcode.Error`，于是取了 50100。
用户看到的提示与真实原因完全无关。

**做法**：写入边界统一走 `saveErr(err)`（`user` 模块叫同一个名字，第 4 个模块又抄了一遍，
说明这该进模板）：

```go
func saveErr(err error) error {
	if err == nil {
		return nil // 允许直接包在 return saveErr(txErr) 这种写法里
	}
	var be *errcode.Error
	if errors.As(err, &be) {
		return be // 业务错误原样透出
	}
	if dberr.IsUniqueViolation(err) { // 23505：先查后插的 TOCTOU 窗口
		return errcode.New(errcode.ErrConflict, "编号已被占用，请重试")
	}
	return errcode.Wrap(errcode.ErrDatabase, err)
}
```

**两道防线**：即使写了 `saveErr`，也不能只用它兜住——所有会撞唯一索引的写入口
都要**先**做显式预检（`customerCodeFree` 这类），数据库兜底只负责并发那一瞬间。

**顺带**：`err == nil` 必须先判。`saveErr` 能直接包在 `return saveErr(...)` 里时，
忘了这一步会让成功路径返回一个非 nil 的「错误」。

---

## 6. 验证流程（改完必跑）

```bash
# ── 后端 ──────────────────────────────
cd backend
gofmt -l . && go vet ./... && go build ./...
go test ./test/... -count=1          # 68 个测试 / 139 条用例
bash scripts/gen-swagger.sh          # 接口有变动时重新生成 api/docs/（一并提交）

# ── 前端 ──────────────────────────────
cd ../frontend
npm run typecheck                    # vue-tsc --noEmit
npm run build                        # 类型检查 + 构建

# ── 端到端（服务已在跑时）──────────────
node ../scripts/api-smoke.mjs        # 187 项接口断言（含进销存分段）
```

**改了前端路由 / 守卫 / 状态管理 / 样式主题，必须做一次浏览器验证**，不能只看编译通过：

```bash
# 前提：后端 + 前端都已启动（npm run dev 或 npm run preview）
node scripts/browser-verify.mjs
APP_URL=http://localhost:4173 node scripts/browser-verify.mjs   # 验证生产产物
```

脚本零依赖（Node 原生 fetch + WebSocket 驱动系统 Chrome），共 73 项断言，覆盖：

1. 冷启动落登录页、表单登录进**数据概览**（`#/inv/overview`，见 §5.62）
2. **F5 深链直接进业务页不应落 404**（§5.1 的回归锚点）
3. 侧边栏逐个菜单点击（现为 15 个页面）：页面可见、无残留 `-enter-from`（白屏检测）
   —— 每个页面都要在 `expected` 表里登记 `hash + 页面根 class`，漏登记会报「匹配期望=false」
4. `el-tree-select` 弹层里 `.el-tree-node` 数量 > 0（确认渲染成树而非空下拉）
5. **商品多规格编辑弹窗**：商品下规格卡片可增删、单位小表列数正确、
   弹窗「整组校验」确实被触发（空规格提交被拦，见 §5.70）
6. 低权限账号的写操作按钮被 `v-perm` 移除，**且看不到导入/导出入口**
7. 管理员的导入入口可见、导入弹窗能打开（含模板下载与上传控件）
8. 退出登录清空令牌、换账号后可正常进入
9. 全程 console 无 error / warning、无未捕获异常
10. **双主题**：跟随系统偏好、登录页/顶栏切换、刷新后持久化、暗色下逐页渲染
11. **暗色下无浅色底残留**（面积 > 1200px²、亮度 > 0.72 的元素会被点名）
12. **关键文字与主按钮的对比度**（WCAG 相对亮度算法，正文 ≥ 4.5、按钮 ≥ 3）
13. **暗色下弹窗 / 下拉弹层 / 输入框的底色**（最容易漏改的三类组件）

退出码非 0 即存在失败项，可直接接进 CI。


### 5.64 下线一个模块 = 摘装配 + 记退役清单 + 删目录，三步缺一不可

**症状**：从 `assembly.Modules()` 里摘掉一个模块后，老库里它的菜单还在：
侧边栏多一块点不动的区域，角色授权列表里也还看得见，
「超管授权数 == 存活菜单数」这个不变量被破坏。

**根因**：菜单 seed 是**只增不删**的（`ensureNode` 命中即沿用），
「模块不注册了」和「菜单该消失了」之间没有任何联动。

**做法**（v1.1 下线 `dashboard` 时确立，完整流程见 `docs/UPGRADE.md`）：

1. `internal/assembly/assembly.go` 摘掉 `Module()` 调用与 import；
2. `internal/seed/seed.go` 的 `retiredModules` 登记命名空间
   （权限码前缀 + 无权限码节点的路由名），下次启动自动软删菜单并清授权；
3. 删掉 `internal/module/<名字>/` 目录与前端 `views/`、`api/` 对应文件。

**为什么退役清单用「权限码前缀」而不是给 `sys_menus` 加 `namespace` 列**：
每个模块的权限码都以 `namespace:` 开头（`module.Build` 会校验），
前缀即命名空间，零 schema 成本；目录节点没有权限码，用路由名兜底。

**顺带**：软删而不是物理删——`name` / `permission` 是部分唯一索引
（`WHERE deleted_at IS NULL`），软删后编号可复用，模块将来回归也不会撞索引。

### 5.65 改了表结构，pgx 的语句缓存会延迟爆炸（SQLSTATE 0A000）

**症状**：升级测试里 `ALTER TABLE ... DROP COLUMN` 之后，
毫不相关的接口开始报 `已缓冲的计划不能改变结果类型 (SQLSTATE 0A000)`，
报错点是登录、是查角色——离真正的变更点很远。

**根因**：pgx 默认把语句计划按**连接**缓存。AutoMigrate 补列/改列后，
同一连接复用同一 SQL，缓存计划的结果列类型对不上，就报 0A000。
哪个连接踩到取决于连接池调度，所以表现为**随机失败**。

**做法**：`config.DatabaseConfig.DSN()` 固定带
`default_query_exec_mode=describe_exec`——每次执行前重新 Describe 拿结果类型，
代价一次往返，换来「改表不必重启所有连接」。

**两个坑**：
- 参数名是 pgx **v5** 的写法。v4 的 `statement_cache_mode` 在 v5 会被当成
  未知启动参数透传给服务器，表现是连库时报「未认可的配置参数」(42704)；
- `describe` 也不是合法值（合法值：`cache_statement` / `cache_describe` /
  `describe_exec` / `exec` / `simple_protocol`），写错连库直接失败。

### 5.66 改业务文案，别动权限码

**诱因**：把「完成」改叫「审核」，顺手把权限码 `inv:purchase:finish` 改成
`inv:purchase:audit`——看起来只是「把命名改对」。

**后果**：权限码是菜单 / 按钮的**幂等键**。改码后播种会插一条新按钮权限，
旧权限码的菜单行还躺在库里（只增不删），而存量角色授权仍指向旧码——
结果所有角色**集体失去**这个操作权限，表现是「升级后审核按钮对管理员都不见了」。

**做法**：权限码是**对外协议**（角色授权、前端 v-perm、接口 RequirePerm 三处共用），
一旦发布就不再改；显示文案（按钮文字 / 状态标签 / 错误提示）随时可改。
新模块起名时一步到位想清楚动词，`finish` 这种命名不理想也留着，
历史包袱比静默收权便宜得多。

### 5.67 选填日期字段用非指针 `time.Time`，「没填」就长成了 `0001-01-01`

**诱因**：`InvOrder.DueDate` 是选填的结算到期日，用 `time.Time` 承载，
`parseDate("")` 按注释「返回零值时间（表示未填）」，于是零值被原样写进 `date` 列。

**后果**（三处同时出问题，且症状看起来互不相关）：

1. 打印纸面多出一行「到期日：0001-01-01」——`if (d.dueDate)` 对
   `"0001-01-01T00:00:00Z"` 判定为真，模板里没有这一项、纸面上却有；
2. 应收应付**逾期汇总**多算：汇总 SQL 只写 `due_date < today`，没有 `IsZero` 守卫，
   而零值时间恒小于今天，于是「没填到期日」的单据被算成「逾期两千年」；
   行级 `overdue` 却有 `!IsZero()` 守卫 —— 同一页上汇总与明细自相矛盾；
3. 无法表达「把它改回空」：`Updates` 写零值又变回 `0001-01-01`。

**做法**：
- 选填日期一律用 `*time.Time`（列可空），空值就是 `NULL`，任何比较天然为假；
  解析入口单独给 `parseDatePtr()`，必填字段仍走 `parseDate()`——
  让「空值」与「零值时间」在**类型**上就分开；
- 存量脏值不要指望人工改：登记到 `database.PatchData()`（幂等、启动即执行、
  改了哪些行有日志），与 `AutoMigrate`（管结构）分工明确；
- 查询侧再加一道 SQL 兜底（`due_date IS NOT NULL AND due_date > '1900-01-01'`）——
  修补脚本没跑成、或将来又有新写入路径漏掉，汇总也不会凭空多出一笔逾期；
- 汇总与明细**必须共用同一个口径**（SQL 一个常量 + Go 一个 `isOverdue()`），
  并在冒烟里断言「汇总 == 逐行之和」——这条不变量比任何一个具体数字都值钱。

### 5.68 打印纸面只由版式配置驱动，「有值就打印」等于绕开设计器

**诱因**：`OrderSheet.vue` 的 `metaItems` 里有一行 `if (d.dueDate) rows.push(到期日)`——
写的时候看着合理（有值才显示），但这一项**在 `printConfig` / 模板设计器里根本不存在**。

**后果**：模板设计器是这个纸面的唯一控制台（列显隐、表头信息、字号、页脚）。
任何「配置里没有、代码里硬写」的字段，都会让**预览与真机打印不一致**：
设计器喂示例数据时该字段恰好为空 → 预览看不到；真实单据有值 → 纸面多一行。
用户会认为「打印模板不起作用」，而这是最难排查的一类不一致。

**做法**：
- 纸面上出现的每个字段，都必须能追溯到 `PrintContentConfig` 的某一项（受设计器控制），
  或至少是**结构性固定**的（编号 / 日期 / 往来单位 / 签字栏）。
  两者之外的东西不放；
- 内部管理字段（结算到期日这类）默认**不进纸面**——对供应商 / 客户没有意义；
- 设计器预览的示例数据要覆盖边界（如刻意留一条「未填到期日」的示例），
  否则「预览看不到、真机打出来」的差异不会被发现。

### 5.69 ECharts 按需注册漏了系列类型，图是「静默空白」

**诱因**：`EChart.vue` 按需注册以减小打包体积，注册表里只有 `LineChart` / `BarChart`，
但数据概览与报表中心都用 `type: 'pie'`。

**后果**：ECharts 对未注册的系列**不抛错**，只 `console.warn` 一句然后丢掉整个 series。
页面不白屏、类型检查全绿、构建成功 —— 只有那块图是空的。
这类缺陷靠 `npm run build` 永远发现不了。

**做法**：注册表必须覆盖**全部实际用到的**系列 / 组件 / 渲染器，改图表类型时同步改注册表；
`browser-verify.mjs` 的控制台检查（无 error / warning）是这条的守门人 ——
它抓到的不只是一条 warning，而是「某个图不渲染」。

### 5.70 数组型字段的「整组校验」必须绑 `prop`，否则 `validate()` 调不到

**诱因**：商品编辑弹窗的规格是数组（`specs: [{ spec, units: [...] }]`），
「至少要有一个规格、每个规格至少一个单位、恰有一个基本单位」这类**跨行约束**
写在 `rules.specs` 的 validator 里。

**后果**：`el-form` 的 `validate()` **只跑绑在具体 `el-form-item prop` 上的规则**。
validator 挂在 `rules.specs` 上、模板里却没有 `prop="specs"` 的 form-item，
等于这条规则从来没被执行过 —— 空规格能直接提交，界面上一片绿。
这类「死配置」类型检查与构建都不会报。

**做法**：

- 数组型字段要在模板里放一个承载用的 `<el-form-item prop="specs" label-width="0" />`
  （无 label、只用来挂校验结论），validator 才会被 `validate()` 调到。
- **职责分层，别报两遍**：逐字段必填交给嵌套 `prop`（`specs.0.units.1.unit`），
  整组 validator **只校验跨行约束**、不再重复校验空值 ——
  否则同一个空单位会同时冒出「第 1 个规格：请填写规格名称」与「请填写规格名称」两条。

### 5.71 子表保存不能全删重建 —— ID 漂移会让历史单据失联

**诱因**：商品编辑时，规格与单位是子表（`inv_product_specs` / `inv_product_units`）。
最省事的写法是先 `DELETE WHERE spec_id = ?` 再按请求重建。

**后果**：ID 每次都变。历史单据明细里存的 `spec_id` / `unit_id` 指向的是**旧 ID**，
于是那张单再被打开保存时，三元组校验直接把它打回（「规格下不存在所选计量单位」）。
**改动的是商品档案，坏掉的是几个月前的单据** —— 排查时很难往这个方向想。

**做法**：

- 保存改为**按名比对后增删改**：命中同名则 `UPDATE` 保留原 ID，未命中才 `INSERT`，
  最后删掉「没被保留」的那些。
- 前端编辑时把已有的 `id` 一并回传，后端**优先按 ID 匹配**，名称变了也能接住
  （否则「`φ20` → `φ20A`」会被判成「新增一个 + 删除一个」，而后者删不掉只能停用）。
- 被单据引用过的规格 / 单位**只停用不删除**（`specInUse`），保住历史单据可编辑。
- 冒烟脚本专门断言「重存商品档案后单位 ID 不变」，守住这条不变量。

### 5.72 幂等种子「表非空就跳过」，会让结构迁移后的示例永远缺一份

**诱因**：进销存种子的幂等口径是「客户表有数据就整体跳过」，避免覆盖客户改动。
但商品档案从「一档一规格一单位」升级成三层结构后，迁移只会把老商品
升级成「一条规格 + 一条基本单位」—— 多单位示例（`1 根 = 6 米`）一条都没有。

**后果**：种子脚本「跑过了」，示例数据却永远停在旧形态。冒烟断言
「示例档案含多计量单位」失败，而看代码完全看不出问题 —— 因为代码没错，
是**幂等口径**在这个场景下不适用。

**做法**：给「演示数据补齐」单独一步（`seedInvProductTopUp`），按**演示编码**定位，
**只补缺的、不覆盖已有的**：

- 已存在的规格 / 单位字段一律不动（客户改过的名称与价格不被冲回去）。
- 补基本单位前先查该规格是否已有 `is_base`，避免补出两个基本单位。
- 注册进 `seed.Run()` 的 `steps`，老库重启一次即可补齐。

### 5.73 三元组外键要显式校验，下拉串了会把库存记到不属于它的规格上

**诱因**：单据明细要同时带 `productId` / `specId` / `unitId` 三个 ID。
它们之间是链式关系（单位属于规格、规格属于商品），数据库层面没有复合外键约束。

**后果**：三个下拉各自独立时，用户先选了「商品 A 的 `φ20`」，再把商品改成 B，
规格 / 单位若没被清空，提交的就是「B 商品 + A 的规格」。库存会被记到
**不属于这个商品的规格**上 —— 账面上完全看不出来，要等盘点才发现。

**做法**：

- 落库前统一走 `resolveSpecUnits(refs)` 校验三元组链，不成立返回 40000 并点名
  （「商品「X」下不存在所选规格」/「规格「Y」下不存在所选计量单位」）。
- 前端联动要**主动清空下游**：换商品清规格与单位并落到第一个规格的基本单位，
  换规格重挑单位；提交前再拦一道「有明细行还没选完规格与单位」。
- 两道防线都要有：前端拦是体验，后端拦是正确性。

### 5.74 「业务线二次鉴权」必须精确到动作，只校验业务线等于没校验

**诱因**：`/inv/orders` 一条路由服务采购与销售两条线，路由上的 `RequirePerm`
写成「`inv:purchase:finish` 或 `inv:sales:finish` 任一」。handler 里补了一次
业务线校验 `ensureBizPerm(bizType)`，检查的是「持有 `inv:<bizType>:` 前缀的
**任意**权限」。

**后果**：一个持有「`inv:purchase:list`（采购只读）+ `inv:sales:finish`（销售审核）」
的账号，路由能过（它有 finish），业务线校验也能过（它有 `inv:purchase:` 前缀）——
**于是它能审核采购单**。越权不需要绕过中间件，只要权限码拼得够巧就能穿过去。

**做法**：

- 读路径可以宽松（校验该业务线任意权限）；**写路径必须精确到动作**：
  `ensureBizPermAction(c, bizType, "create|update|delete|finish")`，比对完整的
  `inv:<bizType>:<action>`。
- 判断标准：**路由上凡是写了「两个业务线任一」的，handler 里就必须再精确到动作**。
- 冒烟脚本用「采购只读 + 销售审核」的临时账号断言审核采购单返回 403，守住这条。

### 5.75 状态推进必须带条件且放在事务最后，否则并发会重复过账

**诱因**：`OrderChangeStatus` 先 `First(&o, id)` 在事务外读状态、判「不是已审核就放行」，
再开事务做出入库与库存，最后 `Update("status", done)`。

**后果**：两个请求并发点「审核」，两次都读到「待处理」，于是**生成两张出入库单、
扣两次库存**。第二次的状态更新覆盖第一次，界面上只看到一张单，库存却少了两倍。
这类缺陷在单机点几下永远复现不了，只在现场并发时偶发。
`StockOrderVoid` 同理（并发作废 = 回滚两次库存）。

**做法**：

- 把状态推进写成**条件更新**：`UPDATE ... WHERE id = ? AND status <> ?`，
  并检查 `RowsAffected == 0` → 返回冲突错误。
- 而且**放在事务的最后一步**：影响 0 行时回滚整个事务，
  出入库单与库存变动一起撤销，不会留下「库存扣了但状态没变」的半成品。
- 判据：**凡是「读状态 → 做副作用 → 改状态」的三段式，都必须是条件更新**。

### 5.76 关键字搜索要转义 `%` 与 `_`，否则结果「静默地不是你要的那批」

**诱因**：模糊搜索统一写成 `ILIKE '%' || keyword || '%'`，没转义。

**后果**：PostgreSQL 的 LIKE 里 `%` 匹配任意串、`_` 匹配单个字符。
搜「`M12_40`」会连「`M12X40`」一起搜出来，搜「`50%`」等于搜 `50` 开头的一切。
**结果既不报错也不为空**，用户无从判断「是没找到，还是被通配符吃掉了」，
只会以为系统数据不对。

**做法**：统一走 `likeKW()` 转义 `\ % _`，并且每条 `ILIKE` 都要跟着写 `ESCAPE '\'`
（只转义不加 ESCAPE 子句，反斜杠会被当成普通字符，等于没转义）。
用 `likeExpr("p.name")` 生成片段，避免漏写。
本模块 12 处模糊搜索已全部改完。

### 5.77 会截断的列表要把「截断了」告诉调用方

**诱因**：单据明细的三级下拉一次最多给 500 个商品（`pickLimit`），
商品超过 500 后下拉里就没有剩下的了。

**后果**：用户翻遍下拉找不到某个商品，只会以为「系统里没录这个商品」——
比报错更难排查，因为**没有任何迹象表明结果不完整**。

**做法**：

- 返回体带上 `total` 与 `truncated`，前端拿到 `truncated=true` 明确提示
  「只加载了一部分，请用关键字搜索」。
- 多取 1 条来判截断（`LIMIT pickLimit + 1`），省掉一次 COUNT。
- 前端的「用关键字搜索」必须**真的能搜**：`el-select` 加 `remote` +
  `remote-method`，把服务端结果**合并**进本地列表而非替换 ——
  替换会让已选好商品的那几行找不到自己的 `specId`、显示成空白，
  「搜一次把别人的行弄坏」这种缺陷最难复现。
- 同理适用于任何带 `LIMIT` 的「下拉数据源」接口。

### 5.78 停用要挡在服务端，不能只靠下拉过滤

**诱因**：`ProductPick` 只返回启用项，但单据落库前的 `resolveSpecUnits`
只校验「三元组存在」，不校验状态。

**后果**：用户在开单页面停留期间，管理员把某个规格停用了 ——
下拉里还有旧数据、提交照收，**库存会继续记到已停用的规格上**。
绕过界面直接调接口更是畅通无阻。「停用」在数据上形同虚设。

**做法**：三元组校验里补上三层状态检查（商品 / 规格 / 单位都必须启用），
错误信息点名是哪个、已停用。代价是历史单据若引用了已停用规格会改不动 ——
这恰恰是**正确的**：档案都已停用，不该继续改那张单，用户应先恢复启用。

### 5.79 多级树必须防环，否则前端组装时会无限递归

**诱因**：分类加 `parent_id` 支持父子层级后，「上级分类」是个自由选择的下拉。
用户（或绕过界面直接调接口）把 A 的上级设成 A 的儿子 B，`parent_id` 就成环了。

**后果**：前端按 `parent_id` 递归组装树，环上的节点会被无限展开 ——
**页面直接卡死**，而且不是报错，是浏览器无响应，用户只能强杀标签页。
服务端不拦的话，这个环一旦写进去，每次打开商品页都会重现。

**做法**：

- 服务端 `categoryCycleFree(id, parentID)`：从 `parentID` 沿父链往上走，
  走到根之前若遇到 `id` 即返回 40000。链长加个上限（100）防脏数据死循环。
- 前端下拉里过滤掉自己与子孙只是**体验**，不能当防线 —— 直接调接口一样能造出环。
- 「同级名称唯一」而不是全局唯一：不同父级下的「标准件」是两个合法分类，
  全局唯一会把多级树逼回一级，用户只能建「标准件-五金」这种难看的名字。

### 5.80 合并菜单要「移挂子节点再软删」，只软删父节点会把权限点埋掉

**诱因**：「商品分类」页合并进「商品档案」页（左侧树），`module.go` 里把分类的
4 个按钮权限点改挂到商品菜单下，然后删掉 `InvCategory` 那个页面节点。

**后果**：老库里 `InvCategory` 的 4 个按钮子节点**还挂在它下面**。
而 `ensureNode` 的语义是「命中幂等键即沿用、不重建」—— 那些按钮于是永远挂在一条
**已软删的菜单**底下。症状极隐蔽：角色授权列表里看不见（前端只渲染存活菜单树），
`/role` 页也点不到，但 `sys_role_menus` 里的授权行还在 —— **权限等于白给**，
而且没人会发现。

**做法**（`seed.mergeMenuNodes`）：

1. 先按 `name` 找到旧节点与目标节点；
2. 把旧节点的 `type = button` 子节点 `UPDATE parent_id = 目标节点`（授权按
   `menu_id` 关联，换父节点不影响任何授权）；
3. 再清旧节点自身的授权行、软删它（顺序不能反，否则留下孤儿行，
   破坏「超管授权数 == 存活菜单数」不变量）。
4. 放在 `ensureNode` **之后**执行：新库里目标节点已经带上了这些按钮，
   老库里才需要这一步补移。

与 `retiredModules`（整个模块下线）区分：那个是删干净，这个是搬走再删。

### 5.81 树/列表里的「hover 才显示」操作按钮，会被容器宽度吃掉

**诱因**：分类树的节点写成「名称占满 + 右侧三个图标按钮，hover 才显形」，
`.cat-node` 用 `width: 100%` 撑开。

**后果**：el-tree 的 `.el-tree-node__content` 是 flex 容器，节点内容前面还有
**展开箭头 + 层级缩进**。内容区再写 `width: 100%` 就比容器还宽，三个按钮被推到
可视区之外裁掉 —— **按钮代码存在、DOM 里也有，但用户永远看不到、hover 也够不到**。
用户反馈是「分类没有删除按钮」，看起来像功能缺失，实际是布局问题。

**做法**：

- 用 `flex: 1` + `min-width: 0` 占满剩余宽度，**不要用 `width: 100%`**。
- 操作按钮**常显**，别做 hover 才出：一是会被上面这个坑吃掉，二是多一层
  「找到它」的发现成本 —— 用户看不到按钮就会认为没有这个功能。
- 顺带把 Element 相邻按钮的 12px 默认左间距压掉，否则三个图标在窄栏里占一大截。
- **验证方式**：浏览器脚本断言按钮的 `getBoundingClientRect().right` 落在
  容器 `right` 之内（只查「按钮存在」是查不出这个问题的）。

### 5.82 el-select 的 `remote` 不做本地过滤，用它做搜索等于列表不变

**诱因**：开单时商品下拉要支持关键字查找，按「远程搜索」思路配了
`filterable` + `remote` + `:remote-method`，把服务端结果**合并**进选项。

**后果**：**remote 模式下 el-select 不做本地过滤**，它原样显示你给的全部 options。
合并之后下拉里仍是全部商品（含不匹配的），用户输入关键字后列表没有任何变化 ——
看起来就是「搜索没生效」。这个坑的迷惑性在于：网络请求发了、数据也回来了、
代码逻辑完全正确，只有渲染层不符合预期。

**做法**：

- 用 `filterable` + `:filter-method`，**过滤结果自己维护**（`applyPickFilter`）。
- 匹配多字段（名称 / 编码 / 规格），与后端关键字口径保持一致。
- **已选中的项必须始终留在选项里**：el-select 靠 options 反查 label，
  找不到就把名称显示成数字。所以过滤结果 = `已选 ∩ 不匹配` + `匹配`。
- 选中后与打开下拉时都要把过滤状态重置回全量，否则会留下
  「输入框空了、列表只剩几条」的错位。

### 5.83 CDP 脚本里的选择器要精确到组件，否则报错完全指不到原因

**诱因**：验证脚本里想点开明细行的商品下拉，写了
`document.querySelector('.el-dialog .col-product')`。

**后果**：明细**表头**里也有一个 `<span class="col-product">商品</span>`，
`querySelector` 返回的是那个 span。span 里没有 `<input>`，于是
`setter.call(null, value)` 抛出 **`TypeError: Illegal invocation`** ——
报错信息指向「setter 调用」，而真正的原因是「选择器命中了另一个元素」，
中间隔了两层，排查成本极高。

**做法**：

- 选择器精确到组件根：`.el-dialog .el-select.col-product`。
- 点开下拉用 `.el-select__wrapper`（与 `probeTreeSelect` 一致），不要点根元素。
- 拿不到元素时**先判空再操作**，让失败信息是「没找到」而不是一个莫名的方法调用错误。
- 同理适用于任何「同一 class 名被用在表头 / 单元格 / 控件」三处的地方 ——
  本项目的 `col-product` / `col-spec` / `col-unit` 都是这种命名。

### 5.84 树结构不能直接喂给 `el-option` 的 v-for —— 子项会静默消失

**诱因**：分类支持多级后，商品表单的分类下拉沿用原来的写法，
把 `buildTree()` 出来的**树结构**（`{ id, label, children }`）直接 v-for 给 `el-option`。

**后果**：`v-for` 只遍历数组**顶层**，于是下拉里只有根分类，
子分类**一个都选不到**。最要命的是下拉看着完全正常（有几项、能选中、能提交），
只是少了一批选项 —— 用户只会觉得「这个分类我建过怎么找不到」，
而开发者在页面上扫一眼也发现不了。

**做法**：

- 下拉选项一律**扁平化**再 v-for，层级用**路径文本**表达（`钢材 / 螺纹钢`）：
  既能一眼看出所属，又让 `filterable` 的按-label 匹配天然支持
  「输入父级名也能搜到子级」。
- 路径分隔符用 ` / ` 而不是树控件的缩进空格 —— 缩进在窄下拉里会被挤没。
- **验证方式**：断言「树渲染的节点数 == 下拉里的选项数」。
  树是按 `parent_id` 递归组装的正确实现，两边数量对不上就是漏了层级。
  这比「下拉里有几个选项」这种断言有效得多。

### 5.85 统计下拉选项时必须判「每条选项的可见性」，否则断言会假失败

**诱因**：验证脚本里统计下拉选项，写成
`pop.querySelectorAll('.el-select-dropdown__item').length`。

**后果**：Element Plus 的**原生 `filterable` 过滤是用 `v-show` 隐藏不匹配项**，
DOM 里一条都不少；而「用 v-for 重建列表」的**自定义过滤**是 DOM 里真的只剩匹配项。
同一个统计口径下，前者会被数成「没过滤」—— **断言假失败**，
而且排查时第一反应是「功能坏了」，会往完全错误的方向查。

**做法**：

- 统计选项时逐条判可见性：`getComputedStyle(el).display !== 'none'`
  且 `visibility !== 'hidden'`，再排除 `.is-hidden` 之类的语义 class。
- 一句话区分这两种实现：**`v-show` 藏、`v-for` 删**。
  写涉及下拉内容的断言前，先想清楚当前是哪种。
- 反过来也成立：如果断言只想验证「有没有渲染出这一项」，
  那就不该判可见性 —— 先明确断言到底在测什么。

### 5.86 决定一个字段要不要必填，先问「不填会怎样」

**诱因**：商品档案的规格与单位原来有四个硬必填：至少一个规格、每个规格至少一个
单位、恰有一个基本单位、基本单位换算率恒为 1。前端四个 `required` 规则 + 后端
`validateSpecs` 双保险，录入一个「办公用品-签字笔」得先把这些想清楚。

**后果**：用户反馈「不需要必须」。但**直接把约束删掉会造出更坏的东西** ——
规格是库存的归属层、单位是单据折算的基准，真允许空的话：
  - `ProductPick` 会把「没有规格」的商品过滤掉 → 开单时选不到它；
  - 库存没有计数单位 → 出入库记不进任何桶。
  等于建了一个**查不到、开不了单的僵尸档案**，比多填两个字段严重得多。

**做法**：把「必填」换成「**缺了自动补**」，约束留在服务端而不是用户面前。

- `normalizeSpecs()`：空 specs → 补「默认」规格 + 「个」基本单位；
  规格名空 → 「默认」；规格下无单位 / 单位名空 → 补「个」；
  换算率 <= 0 → 1；基本单位不是恰一个 → 没指定就把第一个提上来，
  指定多个只留第一个；基本单位换算率钉成 1。
- `validateSpecs()` 只保留**真正的错误**：规格重名、同一规格下单位重名。
  （两行「φ20」会把库存劈成两半、各自都不预警；同规格下两个「箱」会让单据
  选单位时不知道看哪一行的价格。）
- DTO 里对应的 `required,min=1` 一律去掉，只留长度与范围（`max=64` / `gte=0`）。
- 前端 `validateSpecs` 同步放宽到只查重名；嵌套 `required` 规则删掉；
  placeholder 写成「选填，留空按『默认』处理」—— **让用户知道可以不管**，
  否则界面上没有 required 星号、人反而不敢提交。

判断标准：**这个约束是「用户必须知道的事」，还是「系统自己能决定的事」？**
前者才配当必填项。（「日期不能晚于今天」「金额必须为正」属于前者；
「库存按什么单位计数」属于后者 —— 系统取个合理的默认值就行。）

### 5.87 弹窗尺寸要分档统一，主体统一给最大高度

**诱因**：19 个弹窗各写各的 `width`，值有 420 / 480 / 520 / 560 / 600 / 620 /
640 / 680 / 720 / 860 / 900 / 960 / 1000 / 1180 —— 同一系统里点开不同弹窗
宽度乱跳，观感是很直接的「不成体系」。`top` 也是有的写有的不写。

**后果**：单个弹窗看不出问题，**放在一起才露馅**，而「统一」这件事没有任何
机制兜底 —— 下次加弹窗时又随手写一个数。

**做法**：

- 定四档并在 `GUIDE.md 3.8.3` 列成表：`sm 640` / `md 800` / `lg 1100` / `xl 1280`。
  新弹窗从表里挑一档，不要临时决定。
- `top` 统一 `6vh`（打印类 `4vh`，纸面要更多高度）。
- **主体统一给最大高度 + 内部滚动**，写在 `styles/element.scss` 的
  `.el-dialog__body` 上（`max-height: calc(100vh - 240px); overflow-y: auto`）。
  此前只有打印预览写了 `max-height`，别的弹窗在矮屏上会把**底部按钮顶出视口**，
  用户只能去缩浏览器窗口。
- 同时给 `.el-dialog` 加 `max-width: calc(100vw - 32px)`，窄屏上不溢出。
- 浏览器脚本断言**最大两档弹窗的实际宽度**（1280 / 1100）——
  尺寸跑偏靠肉眼发现不了。

### 6.1 交互缺陷另跑 `interaction-verify.mjs`


改了 `el-switch` / `el-checkbox` / `el-form` / 分页 / 选择类交互，必须再跑一次：

```bash
node scripts/interaction-verify.mjs
APP_URL=http://localhost:4173 node scripts/interaction-verify.mjs   # 验证生产产物
```

共 9 项断言，覆盖 §5.44–§5.47：

1. 空表单点登录有明确提示（而非「点了没反应」）
2. 焦点自动落到第一个出错字段
3. **未失焦**时长度校验已生效（输入 → 直接回车这条路径）
4. 改筛选条件后旧勾选归零
5. switch 取消确认后回到原状态
6. switch 请求失败时错误被捕获（不静默吞掉）
7. 日志页首屏不请求登录日志
8. 切 tab 才加载登录日志
9. 改每页条数只发一次列表请求

**用生产产物跑时第 6 项会显式跳过**（它靠 `import('/src/api/user.ts')` 注入坏令牌，
而 `vite preview` 只发 dist、没有 `/src/*`）。跳过与通过分开计数并打印原因——
把它算成通过等于谎报「已验证」，算成失败则是假失败。要覆盖它就用 dev server（默认 5173）。

两个脚本的分工：`browser-verify.mjs` 管**结构性**问题（权限指令、部门树、请求层、
keep-alive、主题对比度），`interaction-verify.mjs` 管**受控组件交互正确性**。

⚠️ 写这个脚本（或任何 Element Plus 交互验证）时注意 §5.48：
合成事件驱动不了受控组件，必须 `Input.dispatchMouseEvent` 点真实坐标；
`v-perm` 隐藏元素的 `innerText` 为空，计数要断言 DOM 状态。

---

## 7. 导入导出约定

### 7.1 表格逻辑单独放 `excel.go`

```
internal/module/user/
├── dto.go      # 请求 / 响应结构体（含 ExportQuery / ImportRow / ImportResult）
├── service.go  # CRUD 业务逻辑
├── handler.go  # HTTP 入参出参
├── route.go    # 路由 + 权限码
└── excel.go    # ★ 表格构建、列定义、逐行校验、批量写入
```

表格相关代码体量大、依赖 `excelize`，与 CRUD 混在 `service.go` 里会让两边都难读。
列顺序用 `iota` 常量集中定义，**模板、导出、导入校验三处共用同一组常量**，避免改一处漏两处。

### 7.2 通用表格能力在 `internal/pkg/sheet`

基于**泛型**的链式 API，业务侧只描述列：

```go
data, err := sheet.NewTable[VO]("用户数据").
    Col("ID", 8, func(v VO) string { return strconv.FormatInt(v.ID, 10) }).
    Col("用户名", 16, func(v VO) string { return v.Username }).
    Rows(items).
    Notes("说明文字放到独立的「填写说明」工作表").
    Build()
```

已内置：表头加粗浅底、**数据单元格强制文本格式**（防手机号被 Excel 转成科学计数法）、
冻结首行、自动筛选、按内容估算列宽（中日韩字符按 2 列宽）、说明页独立工作表。

### 7.3 导出：返回二进制流，不能走统一响应体

```go
// ✅ 直接写字节流 + 手写响应头
sheet.Download(c, "用户列表_20260929_205419.xlsx", data)

// ❌ response.OK(c, data) —— 会被 JSON 序列化，文件损坏
```

中文文件名必须按 **RFC 5987** 放在 `filename*=UTF-8''<百分号编码>`，
同时保留一个 ASCII 的 `filename` 作为回退。前端优先读 `filename*`。

**导出范围必须与列表筛选一致**：把列表的 `where` 构造抽成独立函数（如 `buildFilter`），
列表与导出共用。否则用户在页面看到 10 条、导出却是全表，属于数据泄漏级别的缺陷。

### 7.4 导入：两阶段 + 逐行明细

- 前端先带 `dryRun=1` 校验，把逐行结果展示给用户，确认后再真正写库
- 行号用 **Excel 里的真实行号**（含表头，从 1 开始），用户能直接定位
- 合法行照常写入，非法行跳过并返回原因，**不因为一行出错整批失败**
- 整批写入放在**一个事务**里，任何写库失败整体回滚
- 相同明文密码只算一次 bcrypt 哈希，否则 1000 行会从毫秒级退化到分钟级

**导入模板的数据区必须留空**。放了示例行，用户很可能不删就直接上传，
把示例数据导进正式库。格式示例统一放「填写说明」工作表。

**表头必须严格一致**（列顺序 + 列名），不一致直接拒绝——
否则用户调整了列顺序却以为没事，数据会错位写进库里。

### 7.5 前端下载/上传不能复用 `utils/request.ts`

`request.ts` 的 `unwrap()` 假定响应体是统一 JSON。下载必须走 `utils/download.ts`：

| 能力 | 说明 |
| --- | --- |
| `downloadFile(url, params, fallbackName)` | blob 直取 + `<a download>` 触发，解析 `filename*` |
| `uploadFile(url, file)` | `FormData` 上传，**不要手写 `Content-Type`**（boundary 由浏览器补） |

错误处理：导出接口失败时响应体仍是 JSON，`request.ts` 已对 blob 做反解
（`readBlobError`），所以下载失败也能拿到中文错误提示。

### 7.6 权限要按「风险等级」分，不要按「页面」分

批量导出 = 一次性取走全量数据，批量导入 = 一次性写入大量数据。
这两件事的风险等级都**高于**「能看列表」，因此：

- 权限码独立：`system:user:export`、`system:user:import`、`system:role:export`、`system:log:export`
- **只读角色不授予任何导出/导入权限点**，即使它已有 `system:user:list`

### 7.7 角色有**两套**互不替代的权限

| 机制 | 字段 | 回答的问题 | 落地位置 |
| --- | --- | --- | --- |
| 功能权限 | `sys_role_menus` -> `sys_menus.permission` | 「能不能用这个功能」 | `RequirePerm` 中间件 + 前端 `v-perm` |
| 数据范围 | `sys_roles.data_scope` | 「能看到/改到谁的数据」 | 服务层 `scopeQuery` / `ensureInScope` |

新增会**按条件返回一批业务数据**的接口时，除了挂权限码，还要想清楚数据范围要不要生效。
范围取值：`1` 全部 / `2` 本部门 / `3` 本部门及以下 / `4` 仅本人，多角色取最宽松者。

修改内置角色数据范围的**升级提醒**：`viewer`（只读访客）的种子值已从「仅本人」改为「全部」，
否则它连用户列表都只剩自己一行。老库**无需手工处理** ——
`seed.syncBuiltinRoleDataScope` 会在每次启动时把仍停留在历史错误值上的角色纠正过来。

这条纠正逻辑的设计约束（新增类似逻辑时必须遵守）：

- **只在值等于历史错误值时纠正**。若管理员已显式改成别的范围（如「本部门」），
  说明是有意为之，启动逻辑不得覆盖 —— 否则每次重启都会悄悄回退管理员的配置。
- **幂等**。匹配不到就不写，重复启动不产生额外 UPDATE。
- **走启动路径而不是迁移脚本**。本项目没有独立的 migration 目录，表结构靠
  `AutoMigrate` 增量同步，数据修正就放在 `seed.Run()` 里，与 `syncBuiltinRoleMenus` 一致，
  好处是运维没有「忘记执行 SQL」这一步。

对应 `historicalRoleDataFixes` 表，新增一行即可扩展。回归测试见 `backend/test/upgrade_test.go`。

---

## 8. 安全红线

（原有条目之外，本轮审计新增以下四条）

- 密码只存 bcrypt 哈希，任何接口都不得返回 `password` 字段
- 操作日志的敏感字段（`password` / `oldPassword` / `refreshToken`）必须脱敏后才落库
- 前端隐藏按钮 ≠ 鉴权，新增写接口必须挂 `RequirePerm`
- `admin`（ID=1）与内置角色受保护，不可禁用/删除
- 生产环境必须替换 `NOVA_JWT_SECRET` 与默认口令
- **写库前的文本一律过 `pkg/textutil.Fit`**，不要用 `s[:n]`（§5.22）
- **日志写入失败不能 `_ =` 吞掉**，至少要留一条 WARN —— 静默失败会让风控与审计一起失效
- **数据范围要同时覆盖读与写**，越界按 404 处理（§5.23）
- **改密码 / 重置密码 / 禁用后旧令牌必须立即失效**，`token_epoch + 1`（§5.24）
- **`trusted_proxies` 默认为空**，不要为了「拿到真实 IP」就填 `0.0.0.0/0`（§5.25）
- **角色授予必须过 `authz.EnsureGrantable`**，可授予范围 = 目标角色的每个权限点自己都有（§5.27）
- **非超管不得对超管执行删除/禁用/改密/编辑**，也不得给非超管授 `super_admin`（§5.27）
- **登录锁定按「账号+IP」复合计数**，成功登录重置窗口；IP 维度独立限流（§5.28）
- **客户端可控的标识符（如 `X-Trace-Id`）先过字符集白名单再使用**（§5.29）
- **批量导入的跨行状态（`seen`）只能记到整行校验通过之后**（§5.30）
- **数据范围只有一个实现 `authz.ResolveScope`**，新增涉及「人」的读接口必须调它（§5.31）
- **聚合字段（`userCount` 之类）也要跟着收窄**，裁剪了列表却返回全量统计等于换个字段泄露（§5.31）
- **参与安全判定的日志不允许被清空接口删掉**，删除路径要设硬下界（§5.32）
- **半更新的可选字段用指针**，且 PATCH 要支持显式传空串清空（§5.33）
- **权限指令只打标记 + CSS 隐藏，绝不摘 DOM**；运行期权限变化靠独立 `effectScope` + `watch`（§5.34）
- **「排除子树」逐节点重算，不用不可逆累加器**，否则兄弟节点被误排除（§5.35）
- **认证失败提示要有去重闸门**，`AUTH_ERROR_CODES` 与 `AUTH_ENDPOINTS` 要覆盖完整（§5.36）
- **`keep-alive` 的 `:key` 只用 `route.path`**，拼刷新计数会持续泄漏组件实例（§5.37）
- **限流器的检查与记账必须原子**，「每请求占一格」用 `AllowAndConsume`（§5.38）
- **内存 map 要有硬上界**，超上界 fail-open（宁漏限不可 OOM），并暴露 `evicted` 指标（§5.39）
- **JWT 只接受 HS256 且必须校验 issuer**，`UserID <= 0` 直接判无效（§5.40）
- **`pageSize` 封顶不等于 `page` 封顶**，客户端可控整数参与算术前查溢出与量级（§5.41）
- **聚合查询不吞 error**，把「查不到」渲染成「没配置」是把故障伪装成业务结论（§5.42）
- **失败路径不静默**，`ch == nil` 这类要打日志且只打一次（§5.43）
- **受控组件的取消/失败路径必须显式回写绑定值**，否则 UI「看起来改了、其实没改」（§5.44）
- **`reserve-selection` 只在数据集合未变时可保留**，改筛选条件必须清选择（§5.45）
- **`?.` 与 `&&` 都是静默失败通道**，链式调用有环节可能缺席时必须显式处理（§5.47）
- **「能把权限写进持久层」的入口全是授权行为**，Update/Create 与 Assign 必须过同一道防线（§5.49）
- **404 页不进白名单**，兜底路由在冷启动会吃掉深链，放行分支必须先过登录判定（§5.50）
- **框架层不得 import 具体模块**，只有 `internal/assembly` 可以（§5.60，`scripts/check-module-boundary.sh`）
- **内置菜单的幂等键是权限码/路由名，不是主键**，新模块的菜单不会被自动授予内置角色，除非把命名空间加进 `seed.frameworkNamespaces`（§5.59）
- **下线模块必须登记 `seed.retiredModules`**，只摘 `assembly` 不登记的话，老库会留下点不动的菜单和孤儿授权（§5.64）
- **断言不许硬编码数据库主键**（菜单 ID、用户 ID）。主键由库分配，种子顺序一变全表翻新——`api-smoke.mjs` 因此改用 `menuId(token, '权限码')` 现查，E2E 同理由此删掉了具体 ID 断言
- **路由上的权限码只做粗筛**：一条路由承载多个子资源/业务线时（如 `/inv/orders` 同时服务采购与销售），
  handler 内必须按请求参数与**实体的真实归属**再判一次（`ensureBizPerm`），
  否则「换个查询参数」就能跨业务线读数（§5.62 同源的「声明值不等于生效值」）
- **出参结构不为图省事复用**：`inv:stock:list` 只能看数量，就不给它返回含金额的行结构（§11.4）
- **事务结果一律过 `saveErr`**，不要 `Wrap(ErrDatabase, err)` 把业务错误码冲成 50100（§5.63）

## 8.1 交互正确性红线

以下几条不属于「安全」，但会直接导致**用户看到的状态与真实状态不符**，
在管理后台里这类缺陷的实际危害不低于安全漏洞。

- **状态变更必须能回滚**：取消确认、请求失败两条路径都要恢复原值并以服务端数据为准（§5.44）
- **分页/筛选参数变更只发一次请求**，用同 tick 归并而非防抖（§5.46）
- **表单校验要覆盖「不回车不丢焦」路径**，长度类规则必须挂 `change` 触发（§5.47）
- **公共组件的共性缺陷在组件层修**，不要每个页面各修一遍（§5.46）
- **自动化验证脚本报 FAIL 时先证明脚本是对的**，双向验证（退回修复确认用例会 FAIL）再改产品代码（§5.48）
- **列表页与弹窗的状态机只留一份**：新页用 `useTableQuery` / `useCrudDialog`，
  不要在页面里再抄一套 query/loading/rows 与弹窗四态（§3）
- **状态列一律用 `StatusTag`、导出名一律用 `exportStamp`**，同一个口径不能有两个实现（§3）
- **无调用方的 prop / 事件 / expose 直接删**，别「留着以后用」——它绑着依赖、色值与
  永不执行的分支（§5.58）

---

## 9. 接口文档（swaggo / OpenAPI）

接口文档由 **swag 注解**生成，单一事实来源是 handler 与 `cmd/server/main.go` 上的注释，**不要手改** `backend/api/docs/` 下的三个生成文件。

### 9.1 生成与访问

```bash
cd backend
bash scripts/gen-swagger.sh   # 即 swag init -g cmd/server/main.go -o api/docs --parseInternal -ot go,json,yaml
```

- 生成物 `api/docs/{docs.go,swagger.json,swagger.yaml}` **必须一并提交**，线上构建不装 swag CLI
- 访问 `GET /swagger/index.html`（Swagger UI），由 `router.go` 装配、`app.swagger` 配置门控（缺省=调试模式开启）
- `main.go` 启动时把 `docs.SwaggerInfo.BasePath` 同步为运行时配置的 `server.api_prefix`，避免改了前缀文档还指着旧路径
- 新增配置项：`app.swagger`（env `NOVA_APP_SWAGGER`）

### 9.2 注解约定

- **每个 handler 一段注解**：`@Summary/@Tags/@Accept/@Produce/@Param/@Success/@Failure/@Security BearerAuth/@Router`，缺一段文档就缺一块
- `@BasePath /api/v1` 写在 `main.go` 的包级注解上，handler 的 `@Router` 只写 `/users/{id}` 这种**不含前缀**的路径
- 出参类型必须具体：统一响应体写作 `response.Body{data=response.XxxVO}`，swag 靠泛型实例化展开 `data` 字段
- **`gin.H` 不许出现在 `@Success` 里**——map 推不出字段。只返回 id 用 `response.IDData`，只返回删除数用 `response.DeletedData`（`internal/pkg/response` 里已定义，缺新形态就在那里加具体 struct）
- `@Security BearerAuth` 对应 `main.go` 里的 `@securityDefinitions.apikey BearerAuth In:header Name:Authorization`；导出等二进制接口标 `@Produce application/octet-stream`
- 鉴权失败的公共形态不必逐个展开：文档顶部已注明统一响应体 `{code, message, data}` 与 401/403 语义

### 9.3 相关防护（文档里能看到的行为）

- 登录/刷新接口在配置的阈值内多次失败会返回 **429**（`security.ip_max_login_fail` / `security.refresh_rate_limit`，0 关闭）
- 越权授予角色返回 **403**，错误信息说明缺哪个权限（§5.27）
- `X-Trace-Id` 非法字符会被静默替换为新 trace id，响应头永远回合法值（§5.29）

回归测试：`backend/test/swagger_test.go`（文档可访问、basePath 与配置一致、未开启时不暴露）。
