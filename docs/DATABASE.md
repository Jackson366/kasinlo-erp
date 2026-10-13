# 数据库说明

- 数据库：PostgreSQL（实测 17.6）
- 库名：`nova_inv_v5`（测试库 `nova_inv_v5_test`）
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

进销存业务表（前缀 `inv_`，共 12 张）：

| 表名 | 说明 | 软删除 |
| --- | --- | --- |
| `inv_customers` | 客户档案 | 是 |
| `inv_suppliers` | 供应商档案 | 是 |
| `inv_categories` | 商品分类字典（名称唯一） | 是 |
| `inv_products` | 商品**身份**（编码 / 名称 / 分类 / 状态 / 备注） | 是 |
| `inv_product_specs` | 商品规格 = **库存桶**（`stock` / `warn_stock`） | 是 |
| `inv_product_units` | 计量单位（`rate` / `is_base` / 采购价 / 销售价） | 是 |
| `inv_orders` | 采购 **与** 销售单据主表（`biz_type` 区分） | 是 |
| `inv_order_items` | 单据明细行（含 `spec_id` / `unit_id` / `rate` / `qty_base`） | 是 |
| `inv_settlements` | 收付款记录（应收应付共用） | 是 |
| `inv_stock_orders` | 出入库单据主表 | 是 |
| `inv_stock_items` | 出入库明细行（同样带折算列，不存金额） | 是 |
| `inv_print_templates` | 单据打印模板 | 是 |

**采购与销售共用一张单据表**：两者的字段、明细、金额与结算语义完全一致，
拆成两张表只会让「明细、结算、报表」三块各写两遍。交易对手由 `biz_type` 推导：
`purchase` → 供应商，`sales` → 客户。

**商品拆成三张表**是这套模型的核心：`商品 → 规格 → 计量单位`。三层各司其职 ——

- `inv_products` 只放**身份**，不放任何数量与价格。
- `inv_product_specs` 是**库存桶**，`stock` / `warn_stock` 落在这里，且数量一律按**基本单位**计。
- `inv_product_units` 放单位与**价格**，`rate` 的口径是「1 个该单位 = rate 个基本单位」。

**库存记在规格、价格记在单位**，两条线不能合并：合并到商品上，多规格多单位场景下
就没有地方分别存「每个规格的库存」与「每个单位的价格」，模型只能退化成一档一规格一单位。
合并到规格上，同一规格的「1 箱」与「1 个」就无法给不同价格。详见 [`API.md`](API.md) §11.2。

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

### 进销存业务表

#### inv_customers / inv_suppliers

两表结构完全相同，用两套模型是为了让「客户」与「供应商」在业务上各自独立演进
（客户要开票信息、供应商要账期，将来可能分叉）。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `code` | varchar(32) | not null | 编号，如 `KH001` / `GYS001` |
| `name` | varchar(128) | not null | 名称 |
| `contact` | varchar(64) | | 联系人 |
| `phone` | varchar(32) | | 联系电话 |
| `address` | varchar(255) | | 地址 |
| `tax_no` | varchar(64) | | 开票信息（统一社会信用代码） |
| `status` | smallint | not null | 1 启用 / 0 停用 |
| `remark` | varchar(255) | | |

> 列表里的「余额」不落库：客户余额 = 该客户所有**已完成**销售单据的未结金额之和，
> 由 `partyStats()` 一次聚合算出（避免逐行 N+1）。

#### inv_categories

商品分类字典。从「商品表上的一列自由文本」（旧 `inv_products.category`）升级而来 ——
自由文本下「钢材」与「钢材 」会各算一类，报表的品类占比会凭空多出一块。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `name` | varchar(64) | not null | 分类名称，部分唯一索引 |
| `sort` | int | default 0 | 下拉排序 |
| `status` | smallint | not null | 1 启用 / 0 停用，**不使用数据库默认值** |
| `remark` | varchar(255) | | |

删除前必须校验商品引用（`inv_products.category_id`）：**仍被引用返回 40900，只能停用**。
否则 `category_id` 悬空 —— 商品还在，分类没了，品类统计会凭空少一块。

#### inv_products

**只放商品身份，不放任何数量与价格**。库存与价格分别下沉到规格表与单位表。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `code` | varchar(32) | not null | 商品编码，部分唯一索引 |
| `name` | varchar(128) | not null | 商品名称（**允许重名**，只做录入提示不拦截） |
| `category_id` | bigint | index | 所属分类（`inv_categories.id`） |
| `status` | smallint | not null | 1 启用 / 0 停用 |
| `remark` | varchar(255) | | |

> 早期版本的 `inv_products` 自带 `spec` / `unit` / `category` / `purchase_price` /
> `sale_price` / `stock` / `warn_stock` 七列（一档一规格一单位）。升级为三层后这七列
> 已由 `database.migrateLegacyProducts()` **迁移并 DROP**：老商品被升级成
> 「一条规格 + 一条基本单位」，`category` 文本回填成 `category_id`。
> 迁移用「`inv_products.spec` 列是否存在」判断是否已跑过，因此**幂等**。

#### inv_product_specs

商品规格 = **库存桶**。库存只记在这里，数量一律按**基本单位**计。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `product_id` | bigint | index, not null | 所属商品 |
| `spec` | varchar(64) | not null | 规格名，如 `φ20` / `M12×40` |
| `stock` | numeric(14,2) | default 0 | 当前库存（基本单位），过账时用 `stock = stock + ?` 原子增减 |
| `warn_stock` | numeric(14,2) | default 0 | 库存预警下限 |
| `sort` | int | default 0 | 同商品内的规格顺序 |
| `status` | smallint | not null | 1 启用 / 0 停用 |

> 「被单据引用过的规格只能停用不能删除」（`specInUse`）：历史明细的 `spec_id`
> 还指着它，硬删会让那些单据失去规格归属。

#### inv_product_units

计量单位与**价格**。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `spec_id` | bigint | index, not null | 所属规格 |
| `unit` | varchar(16) | not null | 单位名，如 `根` / `米` / `箱` |
| `rate` | numeric(14,4) | not null | 换算率：**1 个该单位 = rate 个基本单位** |
| `is_base` | boolean | default false | 是否该规格的基本单位（每个规格**恰有一个**） |
| `purchase_price` | numeric(14,2) | default 0 | 该单位的采购价 |
| `sale_price` | numeric(14,2) | default 0 | 该单位的销售价 |
| `sort` | int | default 0 | 单位顺序 |
| `status` | smallint | not null | 1 启用 / 0 停用 |

写入约束（服务层 `validateSpecs()` 校验，违反返回 40000）：同一规格下单位名不重复、
**恰有一个 `is_base`**、基本单位 `rate` 必须为 1、`rate > 0`。

> **单位改名要沿 ID，不能按名全删重建。** `saveUnits()` 先按单位名比对、
> 命中则 `UPDATE` 保留原 ID，未命中才 `INSERT`。若全删重建，单位 ID 每次都会漂移，
> 历史明细的 `unit_id` 随即失效，那张单再保存就会被三元组校验打回。
> 规格同理（`saveSpecs()` 优先按回传的 `id` 匹配）。

#### inv_orders


| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `order_no` | varchar(32) | not null | 单据编号，`CG` 采购 / `XS` 销售 + 日期 + 4 位流水 |
| `biz_type` | varchar(16) | not null, index | `purchase` / `sales` |
| `party_id` | bigint | index | 供应商 / 客户 ID |
| `party_name` | varchar(128) | | 冗余名称，列表免联表 |
| `order_date` | date | | 单据日期 |
| `due_date` | date | **可空** | 结算到期日（仅逾期判定用，选填；未填为 `NULL`） |
| `status` | smallint | not null, index | 1 待处理 / 2 已完成 / 3 已取消 |
| `total_amount` | numeric(14,2) | default 0 | 单据总额 |
| `settled_amount` | numeric(14,2) | default 0 | 已结算金额 |
| `remark` | varchar(255) | | |
| `created_by` | bigint | | 制单人 |

> **结算状态不是列**：`unpaid` / `partial` / `paid` 由「`total_amount` − `settled_amount`」
> 推导（`settleStatus()`）。多存一列就要在每次登记收付款时保持同步，纯属自找不一致。
>
> **`due_date` 必须是可空列**：它是选填字段，「没填」在数据层要有唯一表达。
> 早期用非指针 `time.Time` 承载，空串会解析出 Go 零值并把 `0001-01-01` 原样写进库 ——
> 这个值与「已过期两千年」不可区分，逾期汇总会多算、打印纸面会多打一行。
> 现在指针 + `NULL`，并在启动时做一次幂等修补
> （`database.PatchData()`：`UPDATE inv_orders SET due_date = NULL WHERE due_date < '1900-01-01'`）。
> 逾期判定另在 SQL 侧加了 `due_date IS NOT NULL AND due_date > '1900-01-01'`
> 作为第二道防线 —— 修补没跑成，汇总也不会凭空多出一笔逾期。

#### inv_order_items

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `order_id` | bigint, index, not null | 所属单据 |
| `product_id` | bigint, index | 商品 |
| `spec_id` | bigint, index | **规格**（必填，库存扣减的归属） |
| `unit_id` | bigint | **计量单位**（必填，价格与数量口径的来源） |
| `rate` | numeric(14,4) | 下单时的换算率快照 |
| `qty_base` | numeric(14,2) | 折合基本单位数量 = `qty × rate`，库存按此扣减 |
| `product_name` / `spec` / `unit` | varchar | 下单时的商品快照，商品改名不影响历史单据 |
| `qty` | numeric(14,2) | 数量（按所选单位计） |
| `price` | numeric(14,2) | 单价（填 0 时取**所选单位**的采购价 / 销售价） |
| `amount` | numeric(14,2) | 金额 = 数量 × 单价，`round2()` 保留两位 |

> **`rate` 与 `qty_base` 是快照，不是 join 出来的。** 单位换算率日后被改
> （「1 箱 = 10 个」改成 12 个），历史单据的折算数量不该跟着变 ——
> 单据记的是当时的事实。`spec_id` / `unit_id` 落库前经 `resolveSpecUnits`
> 校验三元组链（单位属于规格、规格属于商品），否则下拉串了会把库存记到
> 不属于它的规格上，账面上完全看不出来。

#### inv_settlements

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `order_id` | bigint, index, not null | 对应单据 |
| `biz_type` | varchar(16), index | 方向：purchase 为付款、sales 为收款 |
| `party_id` | bigint, index | 往来单位 |
| `amount` | numeric(14,2) | 本次结算金额 |
| `method` | varchar(32) | 结算方式，缺省「银行转账」 |
| `settled_at` | date | 结算日期，缺省当天 |
| `remark` | varchar(255) | |
| `created_by` | bigint | 经办人 |

登记时在**同一事务**内写本表并 `settled_amount = settled_amount + ?`。

#### inv_stock_orders

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `order_no` | varchar(32) | 出入库单号，`CK` + 日期 + 4 位流水 |
| `direction` | varchar(8), not null, index | `in` 入库 / `out` 出库 |
| `source_type` | varchar(16) | `purchase` / `sales` / `manual` |
| `source_id` | bigint | 来源单据 ID（手工单为 0） |
| `order_date` | date | 单据日期 |
| `status` | smallint, not null | 1 已过账 / 2 已作废 |
| `remark` | varchar(255) | |
| `created_by` | bigint | |

#### inv_stock_items

`stock_order_id`（bigint, index, not null）、`product_id`（bigint, index）、
`spec_id`（bigint, index）、`unit_id`（bigint）、`rate`（numeric(14,4)）、
`qty_base`（numeric(14,2)）、`product_name`、`spec`、`unit`、`qty`（numeric(14,2)）。

与 `inv_order_items` 同构：规格 / 单位必填，`qty_base = qty × rate` 是库存实际增减的量。

> 出入库明细**不存成本与金额**：有 `inv:stock:list` 权限能看数量，不等于能看到成本。
> 这是与 `ProductRow` / `OrderRow` 有意的结构差异。

#### inv_print_templates

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | varchar(32), not null | 模板编码，如 `BG01` |
| `name` | varchar(64), not null | 模板名称 |
| `doc_type` | varchar(32) | 关联单据类型 |
| `paper` | varchar(8) | `A4` / `A5`，缺省 A4 |
| `status` | smallint, not null | 1 启用 / 0 停用 |
| `remark` | varchar(255) | |

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

进销存业务表同样使用部分唯一索引：

| 索引名 | 表 | 列 | 条件 |
| --- | --- | --- | --- |
| `uk_inv_customers_code` | `inv_customers` | `code` | `deleted_at IS NULL` |
| `uk_inv_suppliers_code` | `inv_suppliers` | `code` | `deleted_at IS NULL` |
| `uk_inv_categories_name` | `inv_categories` | `name` | `deleted_at IS NULL` |
| `uk_inv_products_code` | `inv_products` | `code` | `deleted_at IS NULL` |
| `uk_inv_orders_no` | `inv_orders` | `order_no` | `deleted_at IS NULL` |
| `uk_inv_stock_orders_no` | `inv_stock_orders` | `order_no` | `deleted_at IS NULL` |
| `uk_inv_print_templates_code` | `inv_print_templates` | `code` | `deleted_at IS NULL` |

> 规格 `spec` 与单位 `unit` **不做唯一索引**：它们的作用域是「同一商品」/「同一规格」内，
> 跨商品的 `φ20` 完全合法。唯一性由服务层 `validateSpecs()` 在事务内校验。

好处是「删掉一个有历史单据的客户编号还能重新用」；代价是**并发下的先查后插存在
TOCTOU 窗口**，要靠数据库兜底，所以写入路径统一走 `saveErr()` 把 SQLSTATE `23505`
翻译成 40900 冲突，而不是 50100「数据库操作失败」。

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
- `inv_orders.biz_type`、`inv_orders.status`、`inv_orders.party_id`
- `inv_products.category_id`
- `inv_product_specs.product_id`、`inv_product_units.spec_id`
- `inv_order_items.order_id`、`inv_order_items.product_id`、`inv_order_items.spec_id`
- `inv_settlements.order_id`、`inv_settlements.biz_type`、`inv_settlements.party_id`
- `inv_stock_orders.direction`、`inv_stock_items.stock_order_id`、`inv_stock_items.product_id`、`inv_stock_items.spec_id`

### 3.3 `status` 列为什么不给数据库默认值

带 `status` 的表（`sys_users` / `sys_roles` / `sys_depts` / `sys_menus` /
`inv_customers` / `inv_suppliers` / `inv_categories` / `inv_products` /
`inv_product_specs` / `inv_product_units` / `inv_stock_orders` / `inv_print_templates`）
都是 **`smallint NOT NULL`，且没有 `DEFAULT`**。这是刻意的。

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
| 菜单 | 62 条（2 个目录 + 16 个菜单 + 44 个按钮权限点） |
| 角色 | `super_admin`（超管，62 条授权）、`system_admin`（49 条）、`viewer`（只读，18 条） |
| 用户 | `admin / admin123456`、`demo / demo123456` |
| 进销存演示数据 | 5 分类（钢材 / 五金 / 机电 / 包装 / 化工）、5 客户（KH001~KH005）、5 供应商（GYS001~GYS005）、8 商品（SP001~SP008，含多规格多单位示例，其中 SP007 刻意低于预警线）、8 打印模板（BG01~BG08，BG08 已停用）、14 张采购/销售单据（9 销售 + 5 采购，日期落在近 30 天，含部分结算）、6 张手工出入库单 |

**多规格多单位示例**（`demoProducts` 里刻意留了几条换算率 ≠ 1 的）：

| 商品 | 规格 | 计量单位 | 说明 |
| --- | --- | --- | --- |
| SP002 不锈钢管 | `φ20` | 根（基本）/ 米 | 1 根 = 6 米 |
| SP003 轴承 | 单项 | 个（基本）/ 盒 | 1 盒 = 10 个 |
| SP007 润滑脂 | 单项 | 桶（基本）/ 箱 | 1 箱 = 4 桶 |
| SP008 钢丝绳 | 单项 | 米（基本）/ 卷 | 1 卷 = 100 米 |

> 从旧的单规格结构迁移来的库，上面这些多单位一条都不会有 ——
> 所以 `seed.Run()` 里多了一步 **`seedInvProductTopUp`**（演示档案补齐）：
> 按**演示编码**定位商品、按名比对规格与单位，**只补缺的、不覆盖已有的**，
> 客户改过的单位名与价格不会被种子冲回去。补基本单位前先查该规格是否已有 `is_base`，
> 避免补出两个基本单位。老库的种子口径是「客户表非空就整体跳过」，
> 没有这一步，迁移后示例永远是「一规格一单位」。

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

### 4.3 顶层入口顺序校准

`seedMenus` 末尾会调 `ensureLandingOrder`：**保证 `inv` 目录的 `sort` 严格小于
`Dashboard` 菜单的 `sort`**，不足时把 `Dashboard` 顺延一位。

原因见 AGENTS.md §5.62：前端登录后的落地页由「菜单树里第一个可访问的叶子」推导，
而菜单树按 `sort ASC, id ASC` 取，顶级 sort 平局时 `id` 小的赢。
「数据概览」是顶级菜单且 sort=1，严格小于其它顶级节点，落地页稳定落在它上面。
这段校准只动这两个顶层入口的相对顺序，其余节点一律不碰 ——
管理员在「菜单管理」里对其它的调整仍然是权威。

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
DROP DATABASE nova_inv_v5;
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

-- 顶层入口顺序（数据概览的 sort 必须严格小于其它顶级节点，否则落地页会漂移）
SELECT name, title, sort FROM sys_menus
WHERE parent_id = 0 AND deleted_at IS NULL ORDER BY sort, id;

-- 未完成单据不会形成应收应付；这条应返回 0 行
SELECT o.order_no, o.biz_type, o.status FROM inv_orders o
WHERE o.deleted_at IS NULL AND o.status <> 2 AND o.settled_amount > 0;

-- 单据已结算金额与收付款记录之和对不上（正常应返回 0 行）
SELECT o.id, o.order_no, o.settled_amount, COALESCE(s.sum, 0) AS recorded
FROM inv_orders o
LEFT JOIN (
  SELECT order_id, SUM(amount) AS sum FROM inv_settlements WHERE deleted_at IS NULL GROUP BY order_id
) s ON s.order_id = o.id
WHERE o.deleted_at IS NULL AND ABS(o.settled_amount - COALESCE(s.sum, 0)) > 0.01;

-- 库存为负（出库校验被绕过）应返回 0 行
SELECT s.id, p.code, p.name, s.spec, s.stock
FROM inv_product_specs s
JOIN inv_products p ON p.id = s.product_id
WHERE s.deleted_at IS NULL AND s.stock < 0;

-- 每个规格必须恰有一个基本单位（返回 0 行才正常）
SELECT s.id, s.spec, count(u.id) AS base_units
FROM inv_product_specs s
LEFT JOIN inv_product_units u ON u.spec_id = s.id AND u.deleted_at IS NULL AND u.is_base
WHERE s.deleted_at IS NULL
GROUP BY s.id, s.spec HAVING count(u.id) <> 1;

-- 基本单位的换算率必须为 1（返回 0 行才正常）
SELECT u.id, u.unit, u.rate FROM inv_product_units u
WHERE u.deleted_at IS NULL AND u.is_base AND u.rate <> 1;

-- 规格没有挂任何单位（库存无法计量，返回 0 行才正常）
SELECT s.id, s.spec FROM inv_product_specs s
WHERE s.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM inv_product_units u WHERE u.spec_id = s.id AND u.deleted_at IS NULL);

-- 明细的折算数量与「数量 × 换算率」对不上（返回 0 行才正常）
SELECT i.id, i.qty, i.rate, i.qty_base FROM inv_order_items i
WHERE i.deleted_at IS NULL AND ABS(i.qty_base - i.qty * i.rate) > 0.01;
```
