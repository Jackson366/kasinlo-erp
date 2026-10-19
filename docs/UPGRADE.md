# nova-erp 升级方案

> 版本：v1.1（2026-10-13，随进销存落地与仪表盘下线更新）
> 目标：让后台管理系统（框架 + 系统模块）可以**独立于业务模块单独升级**，
> 业务代码的堆叠不抬高框架升级的成本，框架升级也不强迫业务同步改造。
>
> 本文是「架构级方案 + 操作手册」二合一：§1–§5 讲边界与约定（为什么这样升），
> §6 起讲每次升级的具体动作、验证、回滚（怎么升）。版本明细见 [CHANGELOG.md](CHANGELOG.md)。

---

## 1. 问题定义

nova-erp 的定位是「可复用的后台管理底座」，业务（当前即进销存 `inv`）以模块形式往上加。
没有边界约束时，升级会遭遇三类耦合：

| 耦合类型 | 典型症状 |
| --- | --- |
| **代码耦合** | 业务 handler 直接改 `internal/pkg` 里的通用函数；`pkg` 反过来 import 业务模块 |
| **数据耦合** | 业务迁移改了系统表结构；系统迁移假设业务表存在 |
| **依赖耦合** | 业务需要一个新库版本，与框架锁定的旧版本冲突，升级被业务绑架 |

本方案的目标：**升级框架时，只要系统回归套件全绿就能发布，不需要业务代码参与；升级业务时，不触碰框架目录。**

## 2. 边界划分：什么是「后台系统」，什么是「业务」

### 2.1 目录契约（硬边界）

```
backend/
├── cmd/server/              # 框架：组装入口
├── internal/
│   ├── module/contract.go   # 框架：模块契约 MenuNode / Module / Build / Group（纯类型，不 import 具体模块）
│   ├── assembly/            # 框架：组装层（composition root）——全项目唯一 import 具体模块的地方
│   ├── authz/               # 框架：授权、数据范围（业务只准调用，不准修改语义）
│   ├── middleware/          # 框架：认证、限流、oplog
│   ├── oplog/               # 框架：操作日志基础设施
│   ├── pkg/                 # 框架：errcode/page/response/sheet/textutil/jwt/limiter/bind/dberr
│   ├── module/
│   │   ├── auth|profile|user|role|menu|dept|log/  # 框架：系统模块（每个含 module.go 自描述清单）
│   │   └── inv/             # 业务：进销存模块（本仓库的主线业务）
│   ├── database/            # 框架：连接、时区、自动建库、AutoMigrate 入口
│   ├── router/              # 框架：遍历模块登记表挂路由（不 import 具体模块）
│   └── seed/                # 框架：内置角色/菜单/账号 + 各业务演示数据分文件（seed/inv.go）
├── scripts/                 # 框架：check-module-boundary.sh（依赖方向门禁）等
└── test/                    # 框架回归套件（系统测试）+ 业务测试分文件
frontend/
├── src/
│   ├── api/                 # 系统 api 文件与业务 api 文件并列（inv.ts）
│   ├── components/          # 框架：通用组件（EChart、Pagination、AppIcon…）
│   ├── router/              # 框架：守卫与注册机制；业务路由全部走后端动态下发
│   ├── stores/ utils/ directives/  # 框架
│   └── views/
│       ├── system/ profile/ # 框架：系统页面
│       └── inv/             # 业务：进销存页面（overview/purchase/sales/stock/finance/report/print/customer/supplier）
```

**判定规则（一句话版）：**

- 会出现在「内置菜单 seed」里的模块 = 后台系统；
- 只通过「业务菜单管理」挂上来的模块 = 业务。
- `internal/pkg`、`internal/authz`、`middleware` 只服务系统语义；业务要扩展行为时**提钩子**，不直接改。

**边界是怎么被编译器强制的（本方案的落地形态）：**

系统模块与业务模块都只实现同一份契约（`module.Module`），由 `internal/assembly` 汇总登记；
`router` 与 `seed` 只读登记表。于是「删掉一个业务模块」= 删目录 + 在组装层删一行 + 记一笔退役清单，
框架其余部分 diff 为零 —— **边界不是靠自觉，是删不掉的反面：删得掉**。
（v1.1 下线 `dashboard` 就是一次完整演练，见 §4.3。）

### 2.2 三条铁律

1. **依赖方向单向**：`业务 → 框架` 永远合法；`框架 → 业务` 一次都不许出现。
   已经落地成一条命令（`scripts/check-module-boundary.sh`，改动框架层后必跑）：
   ```bash
   # 框架包不许 import 具体模块（internal/module/<任意子包>），出现即视为破坏
   bash scripts/check-module-boundary.sh
   ```
   它用「框架路径白名单 + grep `internal/module/<子包>`」判定，比逐个枚举业务模块名更强 ——
   **新增模块不必维护白名单**；排除 `internal/assembly/`（组装层本就是唯一该 import 具体模块的地方）
   与 `_test.go`。
2. **系统表与业务表不互相外键**：系统表（`sys_*`）只允许被业务表**引用**（业务表存
   `user_id`/`dept_id`），系统表永远不存业务表主键、不加指向业务表的 FK。
3. **框架不改业务行为**：框架升级 diff 里出现业务模块的文件 = 违规，
   反过来，业务 PR diff 里出现 `internal/pkg`、`middleware` 等框架目录 = 违规。

## 3. 业务模块的「准出清单」

业务模块要能随框架升级零成本搬家（甚至整个删掉），创建时必须满足：

- [ ] 只 import 框架层（`internal/pkg/*`、`authz`、`model` 基础类型），不 import 其他业务模块
      ——确需跨业务调用时走**接口注入**（在组装处把 A 业务的能力作为接口传给 B），不直接包引用
- [ ] 有 `module.go` 自描述清单（`Namespace` + `Register` + `Menus`），并在 `internal/assembly` 登记一行；
      框架其余包对它零引用（`bash scripts/check-module-boundary.sh` 必须无输出）
- [ ] 路由全部经 `route.go` 注册、由清单的 `Register` 带出，权限码走菜单表（动态下发），
      **不在前端写死路由**；菜单写在清单的 `Menus` 里，框架 `seed.go` 不出现业务菜单
- [ ] 数据表名带业务前缀（`inv_*`、`crm_*`）
- [ ] 权限码用业务命名空间（`inv:order:create`），不冒用 `system:*`（契约校验会拦）
- [ ] 业务演示数据放独立文件 `internal/seed/<name>.go`，由 `seed.Run` 显式调用；
      框架 seed 文件不出现业务名
- [ ] 有独立的业务回归测试文件，可整体删除而不破坏 `go test ./...`

> `inv` 模块（11 页面 / 43 接口 / 12 张表）已按本清单准出，可作为新业务的参照模板。

## 4. 数据库分层升级

### 4.1 结构迁移：AutoMigrate + 启动时订正

| 场景 | 机制 |
| --- | --- |
| 加表 / 加列 / 加索引 | `database.AutoMigrate()` 启动时自动完成（只增不改） |
| 改列类型、收窄列、删列 | AutoMigrate **不做**。写进模块的启动迁移函数（按版本/条件执行），
| 数据订正（历史脏值） | 同上，参考 `test/upgrade_test.go` 的 `token_epoch` 回填用例 |
| 删菜单 / 下线模块 | 登记 `seed.retiredModules`（§4.3） |

新增「启动时执行」的迁移时，在 `seed.Run()` 的 steps 里按序追加，
并在 `CHANGELOG.md` 写清楚这一步对存量数据做了什么。
**迁移红线：禁止 `DROP COLUMN` / 破坏性 `ALTER`**——字段废弃走「停写 → 下个 MAJOR 再删」，
这是旧版二进制能在新 schema 上运行（可回滚）的前提。

> **唯一的例外（已记录在案）：`inv_products` 的七列遗留字段**。
> 商品从「一档一规格一单位」升级为 `商品 → 规格 → 计量单位` 三层时，
> 旧的 `spec` / `unit` / `category` / `purchase_price` / `sale_price` / `stock` / `warn_stock`
> 被 `migrateLegacyProducts()` **删除**（见 §4.5）。
> 破例的理由：这七列与新表**语义完全重叠**，留着就是两套真相 ——
> `inv_products.stock` 与 `inv_product_specs.stock` 并存时，
> 后续任何一个查询都不知道该信谁，且无从判断哪边是最新的。
> 保守做法（保留列）在这里换不来可回滚性，只换来一个必然算错的数。
>
> **代价要说清楚**：这个库不再支持「回滚到旧版二进制」——旧代码读不到那七列会直接报错。
> 回滚必须走备份恢复（§6.4），不能只换二进制。

### 4.2 seed 分层

框架 seed（内置角色、内置菜单、admin/demo 账号）与业务 seed（业务演示数据）分开：
**内置菜单不再来自框架 seed 的手工清单，而是各模块 `module.go` 的 `Menus`**，
由 `seed` 遍历模块登记表统一播种；业务菜单树天然跟着业务模块走。

幂等键是「权限码（按钮）/ 路由名（目录、页面）」，兼容三态：
命中未删除记录 → 复用原 ID 且**不覆盖字段**（管理员改过的标题/图标/排序必须保留）；
命中已软删记录 → 告警并跳过整棵子树（不复活）；都不命中 → 新建（主键自增）。

> **「不覆盖字段」的直接后果**：菜单的**结构变更**（挪到顶级、改 sort）对存量行无效，
> 必须写显式校准代码（现为 `seedMenus` 末尾的 `ensureLandingOrder`），
> 只校准「声明值要求、存量行没满足」的关系，其余一律不碰。
> v1.1「数据概览升为顶级菜单」走的就是这条路（AGENTS.md §5.62）。

### 4.3 下线一个模块 = 摘装配 + 记退役清单 + 删目录，三步缺一不可

1. **摘装配**：`internal/assembly/assembly.go` 删掉 `Module()` 调用与 import；
2. **记退役清单**：`internal/seed/seed.go` 的 `retiredModules` 登记命名空间
   （权限码前缀 + 无权限码节点的路由名）。下次启动自动：
   - 软删除该命名空间的菜单（`deleted_at`，保留恢复余地）；
   - 清理 `sys_role_menus` 里指向这些菜单的授权（**先清授权再软删菜单**，
     否则留下孤儿行，破坏「超管授权数 == 存活菜单数」不变量）；
3. **删代码**：`internal/module/<名字>/`、前端 `views/<名字>/`、`api/<名字>.ts`、
   仅它使用的图标注册项，同步删掉冒烟/浏览器脚本里的对应断言。

配套检查：`seed.frameworkNamespaces` 里有没有它的命名空间（有就删）、
`AGENTS.md` 的验证数字（断言数、页面数）要不要更新。

**为什么退役清单用权限码前缀而不是给 `sys_menus` 加 `namespace` 列**：
每个模块的权限码都以 `namespace:` 开头（`module.Build` 校验），前缀即命名空间，
零 schema 成本；目录节点没有权限码，用路由名兜底。

### 4.4 改了表结构，pgx 的语句缓存会延迟爆炸（SQLSTATE 0A000）

升级测试里 `ALTER TABLE ... DROP COLUMN` 之后，毫不相关的接口可能报
`已缓冲的计划不能改变结果类型 (SQLSTATE 0A000)`——pgx 默认把语句计划按**连接**缓存，
表结构一变，同一连接复用同一 SQL 就炸，且**哪个连接踩到取决于池调度，表现为随机失败**。

对策已内建于 `config.DatabaseConfig.DSN()`：固定带
`default_query_exec_mode=describe_exec`（每次执行前重新 Describe 拿结果类型）。
注意这是 pgx **v5** 的参数名；v4 的 `statement_cache_mode` 会被透传给服务器报 42704，
`describe` 也不是合法值（合法值见 AGENTS.md §5.65）。

### 4.5 商品「一档一规格一单位」→ 三层结构的一次性迁移

`database.migrateLegacyProducts()` 在启动时执行，**幂等**（用「`inv_products.spec`
列是否存在」判断是否已跑过）。步骤固定为「先把数据搬完，最后才删列」：

| 序 | 动作 | 说明 |
| --- | --- | --- |
| 1 | 旧品类文本收成分类字典 | 把历史 `inv_products.category` 去重写入 `inv_categories`（`NOT EXISTS` 兜住，不产生重名） |
| 2 | 回填商品的分类外键 | 按分类名关联回 `category_id` |
| 3 | 商品升级出规格（库存随之迁移） | 每个商品生成一条 `inv_product_specs`（规格取原 `spec`，空则用 `默认`），`stock` / `warn_stock` 原样带过来 |
| 4 | 规格升级出基本单位（沿用旧单位与双价格） | 每个规格生成一条 `inv_product_units`（`unit` 取原 `unit`，空则用 `个`；`rate = 1`、`is_base = true`；采购价 / 销售价从原商品列带下来）。`NOT EXISTS` 同时兜住「商品漏建规格」的情况 |
| 5 | 历史单据明细补规格与基本数量 | `inv_order_items` 的 `spec_id` / `unit_id` / `qty_base` 按商品回填（`qty_base = qty`，因为老结构下单位即基本单位） |
| 6 | 历史出入库明细补规格与基本数量 | 同上，作用于 `inv_stock_items` |
| 7 | **最后删列** | `ALTER TABLE inv_products DROP COLUMN IF EXISTS ...` |

步骤之间全部用 `NOT EXISTS` / 条件判断兜住，**中途失败后重跑不会产出重复规格与单位**。

**为什么删列放最后**：中途任何一步失败，旧列还在，下次启动会从头重跑一遍上面的幂等步骤，
而不是留下一个「数据搬了一半、列已经没了」的半迁移库。

**为什么这些列必须删**（这是 §4.1 迁移红线的唯一例外）：它们与新表语义完全重叠 ——
`inv_products.stock` 与 `inv_product_specs.stock` 并存，后续任何查询都不知道该信谁。
保留列换不来可回滚性，只换来一个必然算错的数。**代价**：此库不再支持直接回滚到旧二进制，
回滚须走备份恢复（§6.4）。

**迁移完别忘了补种子**：老库的幂等口径是「客户表非空就整体跳过」，
所以多单位示例（`1 根 = 6 米` 等）不会被补上。`seed.Run()` 里的
`seedInvProductTopUp` 专门做这件事：按演示编码定位、**只补缺的、不覆盖已有的**
（详见 `DATABASE.md` §4）。

## 5. API 与前端分层升级

### 5.1 API 兼容策略

- 路由前缀 `/api/v1` 不变；**框架接口**（`/auth/*`、`/system/*`、`/profile*`）
  的请求/响应结构变更属破坏性变更，必须：加字段不删字段、旧字段保留一个版本、
  `swagger_test.go` 断言不回退。
- **业务接口**（`/inv/*` 等）由业务自行版本化，框架不感知。
- 破坏性 API 变更的出口是**主版本号**（见 §6.1），小版本内只做向后兼容演进。

### 5.2 前端

- `src/api/` 系统 api 与业务 api 并列（`inv.ts`）；api 类型定义与后端 DTO 一一对应。
- 业务页面全部通过动态路由下发，框架升级**不会**触碰业务页面文件；
  业务页面删除后框架照常启动（动态路由 404 兜底已有）。
- 通用组件（`Pagination`、`EChart`、`sheet` 导出等）是框架的公开契约：
  升级时 props/事件不破坏性变更；要破坏时先发 deprecation 警告一个版本。

## 6. 版本策略与升级操作手册

### 6.1 版本号

版本号与 git tag 统一管理（`config.yaml` 里的 `version: v1` 是 **API 版本段**，
最终前缀 `/api/v1`——动它等于全站接口换前缀，属 MAJOR，与发版号是两件事）：

```
vMAJOR.MINOR.PATCH
- MAJOR：框架 API / 系统表结构破坏性变更（业务需要适配）
- MINOR：框架新能力（新权限点、新通用组件、数据范围增强），业务零适配
- PATCH：缺陷修复，业务零感知
```

框架升级（MINOR/PATCH）时业务代码**零改动**是硬承诺；做不到的改动进 MAJOR。

**MAJOR 判例表：**

| 改动 | 归档 |
| --- | --- |
| 删响应字段 / 改响应结构 | **MAJOR**（加字段是 MINOR） |
| 系统表 `DROP COLUMN` / 破坏性 `ALTER` | **MAJOR** |
| 权限码格式、错误码语义变化 | **MAJOR** |
| swagger 契约不兼容 | **MAJOR** |
| 新增接口 / 新增权限点 / 新组件 / 新页面 | MINOR |
| 缺陷修复、文档、测试 | PATCH |

边界情况：**有行为变化但修的是错误行为 → PATCH**（如把本应 409 的并发冲突
从 50100 纠正过来）；若业务已依赖旧行为做分支判断，按 MAJOR 处理并在
CHANGELOG 给过渡期。

**版本路线：** v1.0.0 = 进销存主线落地（已完成）；此后 MINOR = 新能力（业务零适配），
PATCH = 修复。MAJOR 的节拍就是业务痛苦的节拍，能用 MINOR 演进的绝不进 MAJOR。

### 6.2 升级 SOP（每次发布的完整动作）

```
1. 开分支 chore/upgrade-<ver>；先读 CHANGELOG.md 确认改动范围
2. 依赖升级（分两步，各自独立可回滚）：
   a. Go：go get -u ./...（只升级直接依赖）
   b. npm：npm up element-plus vue vite 等（大版本逐个升，不一把梭）
3. 全量回归门禁（必须全绿才允许合并）：
   cd backend  && go build ./... && go vet ./... && gofmt -l internal cmd test
   cd backend  && go test ./... -count=1
   bash scripts/check-module-boundary.sh
   cd frontend && npm run typecheck && npm run build
4. 发布到环境：
   a. 备份数据库（§6.4）
   b. 部署后端：启动时自动完成 AutoMigrate + seed 幂等补齐，
      日志确认「已补齐缺失的内置菜单」「已为内置角色补齐菜单授权」
   c. 前端 npm run build，替换静态产物
   ——— 也可以走在线升级（§6.2.1），让管理页自己做「备份 → 换程序 → 换前端 → 重启核对」
5. 三层验证（§6.3）
6. 写 CHANGELOG → 打 tag → 合并
```

### 6.2.1 在线升级包：一条命令打出来

管理页的「系统运维 → 系统升级」吃的是一个 zip。打包不用手搓：

```bash
# 在仓库根目录执行（用当前分支的代码构建后端 + 前端）
node scripts/build-upgrade-package.mjs --version 1.3.0

# 只发后端（比如只改了 Go）
node scripts/build-upgrade-package.mjs --version 1.3.0 --backend-only

# 只发前端（纯前端修复，不需要重启）
node scripts/build-upgrade-package.mjs --version 1.3.0 --frontend-only

# 带上要求的最低结构版本与升级说明
node scripts/build-upgrade-package.mjs --version 1.3.0 --min-schema 3 --notes "修复报表导出"
```

产物：`build/upgrade/nova-erp-<version>.zip`（已 gitignore）。

**包结构（只有一层，嵌套目录会被后端直接拒绝）**

```
manifest.json            # 版本 / 最低结构版本 / 说明 / 两个文件的 sha256
nova-erp-server.exe      # 后端二进制（--backend-only 或默认都有）
dist.zip                 # 前端产物，zip 内顶层就是 index.html 与 assets/
```

`manifest.json` 长这样（脚本自动生成，不用手写）：

```json
{
  "version": "1.3.0",
  "minSchemaVersion": 3,
  "notes": "修复报表导出",
  "backend":  { "file": "nova-erp-server.exe", "sha256": "0c9f66…" },
  "frontend": { "file": "dist.zip",            "sha256": "366169…" }
}
```

后端只认「`file` 是纯文件名 + `sha256` 对得上」，文件名不必和线上二进制同名 ——
执行时它会落位成当前 exe 的 `.new`。

**打包前必须对齐的两个配置**（脚本会检查并打印告警，但不替你改）：

| 配置 | 为什么必须对 | 不对会怎样 |
| --- | --- | --- |
| `app.release: 1.3.0` | 重启后 `FinalizeUpgrades` 拿**新程序读到的**这个值去比对目标版本 | 为 `dev` 或旧值时，升级永远停在「未生效」 |
| `upgrade.frontend_dir` | 前端 zip 解压到这个目录 | 不配则只换后端，`dist.zip` 被静默忽略（有 warning） |

```yaml
app:
  release: 1.3.0        # 与升级包 version 一致；也可用 NOVA_APP_RELEASE 覆盖

upgrade:
  frontend_dir: "D:/go/nova-erp/frontend/dist"   # 线上真正被 Nginx/静态服务读到的目录
  # staging_dir: "D:/go/nova-erp/backend/upgrade/staging"  # 可选，默认 <exe目录>/upgrade/staging
```

**投放流程**：系统运维 → 系统升级 → 上传 zip（自动校验结构与摘要）→ 升级前备份 →
执行升级 → 重启服务 → 启动日志里核对「升级已生效」。

**常见报错对照**

| 提示 | 原因 |
| --- | --- |
| 升级包版本 x 与当前运行版本相同，无需升级 | 包版本 = 当前 `app.release`；改版本号或先改配置 |
| 后端文件 … 摘要不匹配 | 包在传输中被改过，或 manifest 不是脚本生成的 |
| 升级包不允许嵌套目录 | zip 里套了一层文件夹；用脚本打，别手工压 |
| 未配置 upgrade.frontend_dir | 见上表，前端不会替换 |
| 该升级包已有待执行计划 | 同一个包 + 同一个 sha256 已经传过，去记录里继续执行或等它失效 |


### 6.3 三层验证清单（缺一层不发布）

| 层 | 命令 | 抓什么 |
| --- | --- | --- |
| 静态 | `go build` / `go vet` / `gofmt -l` / `npm run typecheck` | 编译、类型、格式 |
| 单元 + E2E | `cd backend && go test ./... -count=1` | 协议层全链路（含老库升级路径 `upgrade_test.go`） |
| 接口冒烟 | `node scripts/api-smoke.mjs` | 真实 HTTP：鉴权、越权矩阵、二进制导入导出（当前 187 项） |
| 浏览器 | `APP_URL=http://localhost:4173 node scripts/browser-verify.mjs` | 真实渲染：逐页白屏、双主题、控制台零报错（当前 73 项 / 16 页面） |
| 交互 | `APP_URL=http://localhost:4173 node scripts/interaction-verify.mjs` | 受控组件交互 |

新增页面时，`browser-verify.mjs` 的 `expected` 表要登记 `hash + 页面根 class`。
**断言不许硬编码数据库主键**——菜单 ID 由库分配，种子顺序一变全表翻新
（v1.1 删 dashboard 后全表 ID 都变过一轮；`api-smoke.mjs` 因此改为
`menuId(token, '权限码')` 现查）。

### 6.4 备份与回滚

```bash
# 备份（升级前必做）
pg_dump -h 127.0.0.1 -U postgres -Fc nova_inv_v5 > backup_$(date +%Y%m%d_%H%M).dump

# 恢复（先停服务，再动数据，再滚代码，最后启动）
pg_restore -h 127.0.0.1 -U postgres -d nova_inv_v5 --clean --if-exists backup_xxx.dump
```

| 情况 | 回滚方式 |
| --- | --- |
| 代码问题，数据未改 | `git revert` 对应提交，重启（AutoMigrate 只增不改，旧代码兼容新表） |
| 迁移写坏了（删列/订正值错） | 停服务 → `pg_restore` 恢复备份 → 回滚代码 → 重启 |
| 菜单/授权被 seed 改坏 | 软删可恢复：清掉 `deleted_at` 即可（授权行还在，模块回归即复活） |

### 6.5 换库 / 换环境

库名只出现在 `backend/config.yaml`（与 `config.example.yaml`、`test/testdata/config.yaml`），
**代码里没有任何地方写死库名**。库不存在时 `database.Init` 会自动创建并建表，
所以换环境 = 改 `dbname` + 启动一次：

```bash
go run ./cmd/server -config config.yaml -seed   # 首次：写入演示数据
go run ./cmd/server -config config.yaml         # 日常：不带 -seed
```

测试库与开发库必须隔离（`nova_inv_v5_test`）：E2E 会在测试库上做 DDL
（见 `test/upgrade_test.go`），不能指向开发库。

## 7. 已具备的基础（现状盘点）

| 机制 | 状态 | 对独立升级的贡献 |
| --- | --- | --- |
| 动态路由 + 菜单表 | ✅ | 业务页面与框架解耦 |
| 权限码全部走菜单表 | ✅ | 业务权限不写死在前端 |
| 模块契约 + 组装层 | ✅ | 业务模块可整块增删，框架 diff 为零 |
| 模块自描述清单 | ✅ | 加菜单/权限不必再改框架 seed |
| seed 幂等键 = 权限码 / 路由名（三态兼容） | ✅ | 重复执行安全，老库 ID 原样保留 |
| 结构变更显式校准（`ensureLandingOrder`） | ✅ | 「不覆盖字段」与「要改结构」之间的矛盾有着落 |
| 退役模块自动清理（`retiredModules`） | ✅ | 删模块对存量部署也干净，无需手工 SQL |
| 依赖方向门禁 `check-module-boundary.sh` | ✅ | 框架反向依赖被脚本拦下 |
| pgx describe_exec | ✅ | 改表不再随机炸 0A000 |
| upgrade_test 旧库升级用例 | ✅ | 升级路径有回归 |
| 三层验证脚本 | ✅ | 升级门禁现成 |
| git tag + CHANGELOG | ✅ | 版本号落地处（`config.yaml` 的 version 是 API 段，另论） |

## 8. 风险与限制

| 风险 | 说明 | 缓解 |
| --- | --- | --- |
| 边界逐渐被侵蚀 | 赶工时业务顺手改框架最常见 | CI grep 门禁 + PR diff 检查清单 |
| 数据范围被业务绕过 | 业务自写查询漏套 scope | 新业务模板里带 scope 示例；code review 红线 |
| swagger 契约漂移 | 框架接口改了注解没同步 | swagger_test 已钉；升级 SOP 必跑 |
| seed 幂等破坏 | 业务 seed 写成「表空才插」 | 沿用按 ID 补齐模式，进准出清单 |
| 大版本升级仍需业务适配 | MAJOR 无法完全避免 | 变更日志列「业务适配指南」章节 |

## 9. 落地顺序

1. [x] **立规矩**：目录契约、三条铁律
2. [x] **补门禁**：依赖方向门禁 + 契约测试
3. [x] **模块自描述清单 + 组装层**：「加一个菜单要改 5 处」收敛成「在模块里写一段」
4. [x] **首个业务入驻**：`inv` 模块按准出清单落地（11 页面 / 43 接口 / 12 张表）
5. [x] **演练一次下线**：v1.1 完整演练「下线 `dashboard`」——
   摘装配 + 记退役清单 + 删前后端代码 + 同步断言与文档，
   验证了 §4.3 三步法在老库（`nova_admin_v5`）与新库（`nova_inv_v5`）上都成立

## 10. 复杂性权衡（这套方案到底贵不贵）

**结论：运行时零成本，流程少量成本，换来的是升级解耦——净效果是降复杂性。**

| 维度 | 增量 | 说明 |
| --- | --- | --- |
| 运行时代码 | ≈ 0 | 方案只有约定与门禁，无新服务、新中间件 |
| 构建/CI | 一条脚本 | 秒级；误报（注释命中）人工判断 |
| 心智负担 | 少量、持续 | 业务模块对照准出清单；两条新约束（不碰种子数据、不硬编码主键） |
| 升级流程 | 6 步 SOP | 换来「依赖升级不被业务绑架」 |
| 换回来的 | 大幅下降 | 业务与框架纠缠后的解耦成本、全量回归范围 |
