# 数据库说明

- 数据库：PostgreSQL（实测 17.6）
- 库名：`nova_admin_v5`（测试库 `nova_admin_v5_test`）
- ORM：GORM，`AutoMigrate` 在启动时自动建表
- 建库：若目标库不存在，程序会自动创建（见 `internal/database/database.go`）

---

## 一、表清单

| 表名 | 说明 | 软删除 |
| --- | --- | --- |
| `sys_users` | 用户 | 是 |
| `sys_roles` | 角色 | 是 |
| `sys_menus` | 菜单 / 权限点 | 是 |
| `sys_depts` | 部门（树） | 是 |
| `sys_user_roles` | 用户-角色关联 | 否 |
| `sys_role_menus` | 角色-菜单关联 | 否 |
| `sys_operation_logs` | 操作日志 | 否 |
| `sys_login_logs` | 登录日志 | 否 |

### 公共字段

业务表统一带以下字段（`BaseModel`）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigserial | 主键，自增 |
| `created_at` | timestamptz | 创建时间，自动填充 |
| `updated_at` | timestamptz | 更新时间，自动填充 |
| `deleted_at` | timestamptz | 软删除时间戳，`NULL` 表示未删除 |

> 关联表与日志表不使用软删除，直接物理删除。

---

## 二、表结构

### sys_users

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `username` | varchar(64) | not null | 登录账号 |
| `password` | varchar(128) | not null | bcrypt 哈希 |
| `nickname` | varchar(64) | | 显示名 |
| `email` | varchar(128) | | |
| `phone` | varchar(32) | | |
| `avatar` | varchar(255) | | 头像地址 |
| `dept_id` | bigint | index | 所属部门 |
| `status` | smallint | not null | 1 启用 / 0 禁用，**不使用数据库默认值** |
| `token_epoch` | bigint | not null, default 0 | 令牌版本号，改密码/重置密码/禁用时 +1，旧令牌立即失效 |
| `remark` | varchar(255) | | |
| `last_login_at` | timestamptz | | 最近登录时间 |
| `last_login_ip` | varchar(64) | | 最近登录 IP |

### sys_roles

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `name` | varchar(64) | not null | 角色名称 |
| `code` | varchar(64) | not null | 角色标识，如 `super_admin` |
| `sort` | int | default 0 | 排序 |
| `status` | smallint | not null | 状态，**不使用数据库默认值** |
| `data_scope` | smallint | default 1 | 1 全部 / 2 本部门 / 3 本部门及以下 / 4 仅本人；多角色取最宽松者，由服务层 `scopeQuery` 落地 |
| `is_builtin` | boolean | default false | 内置角色不可删除 |
| `remark` | varchar(255) | | |

### sys_menus

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `parent_id` | bigint | index, default 0 | 父菜单，0 为根 |
| `name` | varchar(64) | not null | 路由 name，全局唯一 |
| `path` | varchar(255) | | 路由 path（绝对路径） |
| `component` | varchar(255) | | 组件路径，如 `system/user/index` |
| `redirect` | varchar(255) | | 重定向 |
| `title` | varchar(64) | not null | 菜单标题 |
| `icon` | varchar(64) | | 图标名（Element Plus 图标） |
| `type` | varchar(16) | default `menu` | `dir` / `menu` / `button` |
| `permission` | varchar(128) | index | 权限码，如 `system:user:create` |
| `sort` | int | default 0 | 排序 |
| `hidden` | boolean | default false | 是否在侧边栏隐藏 |
| `keep_alive` | boolean | default false | 是否缓存页面 |
| `is_external` | boolean | default false | 是否外链 |
| `status` | smallint | not null | 状态，**不使用数据库默认值** |

### sys_depts

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `parent_id` | bigint | index, default 0 | 父部门，0 为根 |
| `name` | varchar(64) | not null | 部门名称 |
| `leader` | varchar(64) | | 负责人 |
| `phone` | varchar(32) | | 联系电话 |
| `email` | varchar(128) | | 邮箱 |
| `sort` | int | default 0 | 排序 |
| `status` | smallint | not null | 状态，**不使用数据库默认值** |

### 关联表

```sql
sys_user_roles (user_id bigint PK, role_id bigint PK)
sys_role_menus (role_id bigint PK, menu_id bigint PK)
```

> GORM 的 `many2many` 标签必须写**完整表名**（`many2many:sys_user_roles`）。
> 若写成 `many2many:user_roles`，GORM 会推导出另一张不存在的表，导致权限集合为空、
> 路由接口返回空数组、只读账号一路 403。

### 日志表

`sys_operation_logs`：`user_id`、`username`、`module`、`action`、`method`、`path`、`ip`、`user_agent`、`request`(text，已脱敏)、`status`、`latency_ms`、`error_msg`、`created_at`

`sys_login_logs`：`user_id`、`username`、`ip`、`user_agent`、`success`、`message`、`created_at`

---

## 三、索引设计

### 3.1 唯一约束用「部分唯一索引」

`username` / `code` / `menu.name` 的唯一性**不使用** GORM 的 `uniqueIndex` 标签，而是由
`database.EnsureIndexes()` 建立带 `WHERE deleted_at IS NULL` 的部分唯一索引：

| 索引名 | 表 | 列 | 条件 |
| --- | --- | --- | --- |
| `uk_sys_users_username` | `sys_users` | `username` | `deleted_at IS NULL` |
| `uk_sys_roles_code` | `sys_roles` | `code` | `deleted_at IS NULL` |
| `uk_sys_menus_name` | `sys_menus` | `name` | `deleted_at IS NULL` |
| `uk_sys_depts_parent_name` | `sys_depts` | `parent_id, name` | `deleted_at IS NULL` |

**为什么这么做**：普通唯一索引会把软删除的行也算进去。
一旦删除账号 `zhangsan`，`deleted_at` 被写入但仍留在表里，
再想新建同名用户就会撞上 `重复键违反唯一约束`。
加上 `WHERE deleted_at IS NULL` 后，只有「未删除」的行参与唯一性判断。

`EnsureIndexes()` 会在启动时先检查并删除历史遗留的普通唯一索引（如 `idx_sys_users_username`），再创建部分唯一索引，因此老库升级无需手工处理。

### 3.2 普通索引

- `sys_users.dept_id`、`sys_users.deleted_at`
- `sys_roles.deleted_at`
- `sys_menus.parent_id`、`sys_menus.permission`、`sys_menus.deleted_at`
- `sys_depts.parent_id`、`sys_depts.deleted_at`
- `sys_operation_logs.user_id`、`module`、`status`、`created_at`
- `sys_login_logs.user_id`、`username`、`created_at`

### 3.3 `status` 列为什么不给数据库默认值

四张表的 `status` 都是 **`smallint NOT NULL`，且没有 `DEFAULT`**。这是刻意的。

GORM 对「字段带 `default` 标签、且当前取值等于 Go 零值」的字段，**会把该列整列从 INSERT 语句里省略**，
转而在落库时使用数据库默认值。而 `model.StatusDisabled == 0` 恰好就是 Go 的零值，于是：

```
调用方明确传 status = 0（禁用）
  → model.User{Status: 0}
  → GORM 认为「零值 = 没设置」，INSERT 里不写 status
  → 数据库默认值 1 生效
  → 记录变成「启用」
```

这个缺陷极其隐蔽：编译通过、`go vet` 干净、接口返回 200、常规 CRUD 测试也发现不了，
**只有真正去读 `status` 字段才会暴露**。且它同时存在于单条 `Create` 与批量 `CreateInBatches` 两条路径。

因此约定：**`status` 由应用层显式写入，数据库不提供默认值**，
让「忘记设置状态」变成一眼可见的问题，而不是被默认值悄悄兜住。

> 仍然保留 `default` 标签的字段（`sort` / `parent_id` / `data_scope` / `token_epoch` / 布尔位）是安全的：
> 它们的零值语义与默认值一致（`sort = 0` 就是排最前、`parent_id = 0` 就是根节点、
> `data_scope = 0` 本身不是合法取值，落到默认值 1 反而是期望行为、
> `token_epoch = 0` 就是「从未改过密码」）。
>
> 新增「零值是合法业务值」的字段时，参考 `model.User.Status` 的写法。
>
> `token_epoch` 上的 `default:0` 还有第二个必要理由：给**已有数据**的表新增
> `NOT NULL` 列时，如果没有默认值，PostgreSQL 会因为存量行取到 NULL 而直接拒绝迁移
> （`SQLSTATE 23502: column contains null values`）。

回归测试见 `backend/test/status_test.go`（用户 / 部门 / 角色 / 菜单四条创建路径 + 导入路径）。

---

## 四、初始数据

种子数据在 `internal/seed/seed.go`，**幂等**：可重复执行，已存在则跳过。

| 类型 | 内容 |
| --- | --- |
| 部门 | 总公司 → 技术部、运营部 |
| 菜单 | 26 条（1 个目录 + 6 个菜单 + 19 个按钮权限点） |
| 角色 | `super_admin`（超管，26 条授权）、`system_admin`（21 条）、`viewer`（只读，7 条），共 54 条授权 |
| 用户 | `admin / admin123456`、`demo / demo123456` |

### 4.1 内置菜单主键与幂等键

**内置菜单不再手工分配 ID** —— 主键由数据库 `bigserial` 自增分配，
幂等键换成「权限码（按钮）/ 路由名（目录、页面）」：
菜单声明在各模块自己的 `module.go`（`Menus` 字段），`seedMenus` 遍历模块登记表播种。

> `1–999 保留给内置菜单、自增菜单从 1000 起`（`seed.MenuIDFloor`）那套手工分段已作废。

`ensureNode` 按幂等键分三种情况处理：

| 情况 | 处理 |
| --- | --- |
| 命中**存活**记录 | 复用原 ID，**不覆盖任何字段**（管理员改过的文案不被冲掉） |
| 命中**已软删**记录（`db.Unscoped()` 查得到） | 打 `Warn("内置菜单已被删除，跳过该节点及其子节点")`，整棵子树跳过 |
| 都不命中 | 新建，主键自增 |

软删除仍会让问题变隐蔽：**被软删除的记录依然占着主键**，
所以「表里查不到某行」不等于「那个主键可以安全复用」。
排查真实占用要显式绕过软删除：
`SELECT id, title, deleted_at FROM sys_menus WHERE deleted_at IS NOT NULL`。

### 4.2 幂等补齐与授权清理

`seedMenus` 是**按幂等键逐条补齐**（而不是「表为空才整段写入」），
因此给已初始化的老库新增权限点时，重启服务即可自动补上。

授权不再抄一份菜单 ID 清单：`grantRules` 按「命名空间 + 节点类型 + 权限码后缀」描述
内置角色应有的范围，`resolveGrants` 在运行库现有菜单上求解出具体 ID。

`syncBuiltinRoleMenus` 除补授权外，还会清理**孤儿授权**——
即 `role_id` / `menu_id` 指向已软删除记录的 `sys_role_menus` 行。
不清理的话，`超级管理员授权数 == 存活菜单数` 这个不变量会被破坏。

> 新增权限点时只需要：① 在所属模块的 `module.go` 里加一个 `MenuNode`（按钮给 `Permission`）；
> ② 若这是**框架模块**的权限点，`grantRules` 会自动覆盖它；若是**业务模块**，
> 内置角色默认不获得（避免静默越权）——需要时把命名空间加进 `frameworkNamespaces`，
> 或到「角色管理」页手工分配。
> 注意只读角色**不给**任何按钮权限（能看列表与能批量取数是两个风险等级）。

种子数据写完后会同步自增序列（`resetSequences`），避免手工插入过 ID 导致后续插入主键冲突。

重建数据：

```bash
cd backend
go run ./cmd/server -config config.yaml -seed
```

彻底重来：

```sql
DROP DATABASE nova_admin_v5;
```

再启动服务即可自动重建。

---

## 五、常用排查 SQL

```sql
-- 当前生效的唯一索引（确认部分唯一索引已建立）
SELECT indexname, indexdef FROM pg_indexes
WHERE tablename IN ('sys_users','sys_roles','sys_menus','sys_depts');

-- 某用户实际拥有的权限码
SELECT m.permission FROM sys_menus m
JOIN sys_role_menus rm ON rm.menu_id = m.id
JOIN sys_user_roles ur ON ur.role_id = rm.role_id
WHERE ur.user_id = 2 AND m.permission <> '' AND m.deleted_at IS NULL;

-- 软删除但未清理的数据
SELECT id, username, deleted_at FROM sys_users WHERE deleted_at IS NOT NULL;

-- 最近 20 条失败请求
SELECT created_at, username, method, path, status, error_msg
FROM sys_operation_logs WHERE status >= 400 ORDER BY created_at DESC LIMIT 20;

-- 确认 status 列没有 DEFAULT（应为 NULL），且是 NOT NULL
SELECT table_name, column_default, is_nullable
FROM information_schema.columns
WHERE column_name = 'status'
  AND table_name IN ('sys_users','sys_roles','sys_menus','sys_depts');

-- 孤儿授权：menu_id 指向已软删除或已不存在的菜单
SELECT rm.role_id, rm.menu_id
FROM sys_role_menus rm
LEFT JOIN sys_menus m ON m.id = rm.menu_id AND m.deleted_at IS NULL
WHERE m.id IS NULL;

-- 超管授权数应恰好等于存活菜单数（不等说明有孤儿授权或漏补）
SELECT (SELECT count(*) FROM sys_role_menus WHERE role_id = 1) AS super_grants,
       (SELECT count(*) FROM sys_menus WHERE deleted_at IS NULL) AS live_menus;

-- 被软删除但仍占用主键的内置菜单 ID（1–999 区间冲突排查）
SELECT id, title, deleted_at FROM sys_menus
WHERE id < 1000 AND deleted_at IS NOT NULL ORDER BY id;
```
