# Nova ERP

一套「简洁够用」的现代化后台管理系统：**Go + Vue 3 + PostgreSQL**，模块化分层，专为 AI 辅助开发做了目录与约定设计。

当前仓库里跑的就是一套**进销存后台**（`inv` 模块，10 个页面 43 个接口）：
数据概览 / 采购管理 / 销售管理 / 库存管理 / 应收应付 / 报表中心 / 打印模板 / 商品档案（含左侧分类树）/ 客户管理 / 供应商管理。
登录后直接落在顶级菜单「数据概览」，框架自带的仪表盘已下线。

- 后端：Gin + GORM + PostgreSQL，分层清晰（`config / logger / database / authz / middleware / model / module / seed`）
- 前端：Vue 3 + Vite + TypeScript + Element Plus，菜单驱动动态路由
- 进销存：采购与销售共用一张单据表（`biz_type` 区分）；单据「完成」时**同事务**生成出入库单并原子调整库存（出库校验库存充足）；应收应付由已完成单据推导，**结算状态不落库**；业务线按实体 `bizType` 二次鉴权，仓库权限看不到成本金额
- 商品模型：`商品 → 规格 → 计量单位` 三层。**库存只记在规格上（按基本单位计），单价记在单位上** —— 「`φ20` 剩 30 根 = 180 米」永远只有一个数，「1 箱 100 元 vs 1 个 10.5 元」也不会混。单据明细选到「规格 + 单位」后按 `qty × rate` 折算扣减
- 商品分类：多级父子字典表（`parent_id`），与商品档案同页（左侧树）；同级名称唯一、禁止挂到自己子孙下（成环会让前端组装树时无限递归）
- 主题：内置**亮色 / 暗色**双主题，跟随系统偏好 + 手动切换 + 本地记忆，首屏无闪烁
- 权限：JWT 双令牌 + RBAC（用户-角色-菜单）+ 按钮级权限指令（不摘 DOM、运行期权限变化可重新判定）+ **数据范围**（全部 / 本部门 / 本部门及以下 / 仅本人，读写同时生效，用户 / 部门 / 仪表盘统一收口）
- 账号安全：连续失败锁定、JWT 锁死 HS256 且校验 issuer、限流记账原子且 key 数有硬上界、改密码即吊销旧令牌（`token_epoch`）、审计 IP 不可被 `X-Forwarded-For` 伪造、清空登录日志不解除账号锁定
- 健壮性：客户端可控整数（页码）统一收敛、聚合查询错误一律上报而非静默返回空、审计日志组件未初始化会显式报错
- 导入导出：用户 / 角色 / 操作日志 / 登录日志四类 Excel 导出，用户**两阶段批量导入**（先校验预览、再确认写库）
- 依赖全部为 MIT / Apache-2.0 / BSD-3-Clause 等宽松许可，无 GPL 传染风险
- 本项目自身以 [MIT 许可](LICENSE) 发布

> **关于名字**：`Nova ERP` 是这个项目的自有命名（Go module 名与目录名），
> 不是某个现成开源框架。代码是按需求从零写的，仅在依赖层面使用了上述成熟开源库。

---

## 一、快速开始

### 1. 环境要求

| 组件 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.25+ | 实测 go1.26.4 |
| Node.js | 18+ | 实测 v22 |
| PostgreSQL | 13+ | 实测 17.6，监听 `127.0.0.1:5432` |

### 2. 初始化数据库

只需保证 PostgreSQL 正常运行、`postgres` 账号可用。
**数据库 `nova_inv_v5` 与全部数据表会由程序自动创建**，无需手工执行 DDL。

连接信息写在 `backend/config.yaml`。**该文件不进版本库**（含数据库口令），
先从模板复制一份再改：

```bash
cd backend
cp config.example.yaml config.yaml
```

```yaml
database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: CHANGE_ME      # ← 改成你本机的口令
  dbname: nova_inv_v5
```

任意配置项都可用环境变量覆盖，规则为 `NOVA_<SECTION>_<KEY>`（全大写），例如：

```bash
NOVA_DATABASE_PASSWORD=xxxx      # 数据库口令
NOVA_APP_PORT=9090               # 服务端口
NOVA_JWT_SECRET=change-me        # JWT 密钥
```

### 3. 启动后端

```bash
cd backend
go run ./cmd/server -config config.yaml          # 首次启动会自动建库、迁移、灌入种子数据
go run ./cmd/server -config config.yaml -seed    # 强制重跑种子数据（幂等）
```

启动后：

- 服务地址：`http://127.0.0.1:8080`
- 接口前缀：`/api/v1`
- 健康检查：`GET http://127.0.0.1:8080/health`

### 4. 启动前端

```bash
cd frontend
npm install
npm run dev            # http://localhost:5173
```

Vite 已把 `/api` 与 `/swagger` 代理到 `http://127.0.0.1:8080`，无需处理跨域；
在线接口文档在开发时可直接访问 `http://localhost:5173/swagger/index.html`。

### 5. 切换主题

顶栏右侧的太阳 / 月亮按钮即可切换亮色与暗色，**登录页右上角**也有一个（避免暗色用户每次进登录页都被闪一下）。

| 行为 | 说明 |
| --- | --- |
| 首次访问 | 未保存过选择时跟随操作系统偏好（`prefers-color-scheme`） |
| 手动切换后 | 选择写入 `localStorage.nova_theme`，此后不再跟随系统 |
| 首屏 | `index.html` 内联脚本在样式生效前就写好 `<html class="dark">`，无白闪 |
| 图表 | ECharts 的坐标轴 / 渐变 / tooltip 会同步换色（见 `utils/theme.ts`） |

### 6. 导入导出

| 入口 | 位置 | 说明 |
| --- | --- | --- |
| 导出 | 用户 / 角色 / 操作日志 / 登录日志 列表右上角 | 导出内容与当前筛选条件一致；用户页支持「导出选中项」 |
| 下载模板 | 用户页 → 「导入」→ 第 1 步 | 模板含「用户导入」+「填写说明」两个工作表 |
| 批量导入 | 用户页 → 「导入」→ 第 2 步 | 先校验预览（含逐行失败原因），确认后才写库 |

导入规则要点：

- 表头列顺序必须与模板一致，否则直接拒绝
- 合法行照常写入、非法行跳过并给出原因，**不会因为一行出错整批失败**
- 密码列留空使用默认密码 `123456`
- 单文件上限 5 MB、单次最多 1000 行

### 7. 内置账号

| 账号 | 密码 | 角色 | 说明 |
| --- | --- | --- | --- |
| `admin` | `admin123456` | 超级管理员 | 拥有全部权限 |
| `demo` | `demo123456` | 只读账号 | 仅可查看，所有写操作被后端拒绝 |

> 生产环境请第一时间修改默认口令与 `NOVA_JWT_SECRET`。

### 8. 进销存业务

登录后落在「数据概览」，侧边栏第一组就是进销存。

| 页面 | 主要能力 |
| --- | --- |
| 数据概览 | 客户 / 销售 / 应收 / 库存 / 待处理单据五张指标卡 + 趋势图 + 品类占比 + 客户排行 |
| 采购管理 / 销售管理 | 开单（明细行内编辑，选商品 → 规格 → 单位，单价留 0 时自动取所选单位的价格）、审核（生成出入库单）、取消、删除 |
| 库存管理 | 出入库单据（可打印、可作废回滚）+ 库存查询（**按规格出账**、基本单位呈现 / 预警标红 / 库存总值） |
| 应收应付 | 应收 / 应付明细与汇总（总额 / 已结算 / 未结 / 逾期）+ 登记收付款 |
| 报表中心 | 五个时间范围、营业收入 / 成本 / 毛利 / 毛利率、趋势、品类占比、客户排行 |
| 打印模板 | 可视化设计打印版式：**明细列可增删 / 改名 / 调顺序 / 设对齐**、**表头公司信息与 Logo**、**页边距 / 行高 / 字号 / 表头底色**、**签收栏 / 备注 / 合计大写 / 附加文字**，右侧实时预览，A4/A5 |
| 商品档案 | 左栏分类树（多级，可增改删）+ 右栏商品表格：**一个商品多规格、每个规格多计量单位**（含换算率与各自价格）；库存只读（由单据驱动）；录入时自动带出同名商品防重；规格可单独停用 |
| 报表中心 | 时间范围预设 + **自定义起止日期** + **品类筛选（含子分类）**；汇总 / 趋势 / 排行 / 品类占比四张图 |
| 客户管理 / 供应商管理 | 档案 CRUD，行内含余额与单据数；被单据引用过则不可删 |

**业务规则的五个要点**（详见 [docs/GUIDE.md](docs/GUIDE.md) 与 [AGENTS.md](AGENTS.md) §5.62–5.63）：

1. **出库不能出成负库存**。校验不通过返回 40900 并带上商品名与现有库存，不是含糊的「数据库操作失败」。
2. **结算状态是推导值**（`总额 − 已结算` → 未结清 / 部分结算 / 已结清），不落库，避免两份口径。
3. **仓库权限看不到成本**。出入库单的行结构里没有金额字段 —— 能看数量不等于能看钱。
4. **到期日是内部结算字段**（选填，只用于逾期判定），不进打印纸面；未填的永远不算逾期，
   明细与汇总走同一个口径（冒烟脚本断言「汇总 == 逐行之和」）。
5. **库存记规格、价格记单位**。库存按基本单位折算后落在规格上（`qty_base = qty × rate`），
   价格落在计量单位上；明细落库前校验「商品 → 规格 → 单位」三元组链，防止下拉串了把库存记错地方。

---

## 二、目录结构

```
nova-erp/
├── LICENSE                             # MIT
├── backend/
│   ├── cmd/server/main.go              # 进程入口：装配依赖 + 优雅关停
│   ├── config.example.yaml             # 配置模板（复制为 config.yaml 后填写）
│   ├── config.yaml                     # 本地配置，含口令，已在 .gitignore 中
│   ├── internal/
│   │   ├── bootstrap/bootstrap.go      # 初始化流程（测试与 main 共用）
│   │   ├── config/                     # 配置加载
│   │   ├── logger/                     # zap 日志
│   │   ├── database/                   # 连接、自动迁移、部分唯一索引
│   │   ├── authz/                      # 身份与权限集合（30s 缓存）
│   │   ├── middleware/                 # TraceID / Logger / Recovery / CORS / Auth / RequirePerm / 操作日志
│   │   ├── model/                      # GORM 模型（框架表 + 进销存业务表）
│   │   ├── module/inv/                 # 进销存业务模块（本仓库的主线功能）
│   │   ├── module/<业务模块>/           # ★ 业务代码都放这里
│   │   │   ├── dto.go                  #    请求/响应结构体
│   │   │   ├── service.go              #    业务逻辑
│   │   │   ├── handler.go              #    HTTP 入参出参
│   │   │   ├── route.go                #    路由注册（含权限码）
│   │   │   └── excel.go                #    可选：该模块的导入导出
│   │   ├── oplog/                      # 操作日志异步落库
│   │   ├── pkg/                        # 通用工具：errcode / response / bind / jwt / hash / page / sheet
│   │   ├── router/router.go            # 路由总装配
│   │   └── seed/seed.go                # 幂等种子数据
│   └── test/                           # 进程内 E2E 测试（独立测试库）
├── frontend/
│   └── src/
│       ├── api/                        # 按模块拆分的接口封装
│       ├── components/                 # AppIcon / EChart / Pagination 等通用组件
│       ├── directives/permission.ts    # v-perm 按钮级权限指令
│       ├── layout/                     # 布局：Sidebar / Navbar / TagsView / Breadcrumb
│       ├── router/                     # 常量路由 + 守卫 + 动态路由转换
│       ├── store/                      # user / permission / app（含主题状态）
│       ├── styles/                     # 设计令牌（亮/暗两套）+ 全局样式 + Element Plus 覆写
│       ├── types/api.ts                # 与后端一一对应的类型定义
│       ├── utils/                      # auth / request / download / format / theme
│       └── views/                      # 页面：system / inv（进销存）/ dashboard
├── scripts/
│   ├── api-smoke.mjs                   # 零依赖接口冒烟脚本（214 项）
│   ├── browser-verify.mjs              # 零依赖浏览器运行时验证（80 项 / 15 个页面，含双主题）
│   ├── interaction-verify.mjs          # 零依赖受控组件交互验证（9 项）
│   ├── gen-swagger.sh                  # swag init 封装：重新生成 OpenAPI 文档
│   └── fixtures/                       # 测试样例文件（含生成脚本）
├── docs/                               # GUIDE.md / API.md / DATABASE.md
└── AGENTS.md                           # ★ AI 开发约定（改代码前先读）
```

---

## 三、验证方式

### 后端：进程内 E2E 测试

```bash
cd backend
go vet ./...
go test ./test/... -count=1        # 68 个测试、139 条用例
```

测试使用独立库 `nova_inv_v5_test`，通过 `httptest` 在进程内跑完整路由，不依赖外部服务。
测试配置同样不进版本库，首次跑测试前复制一份：

```bash
cd backend
cp test/testdata/config.example.yaml test/testdata/config.yaml
```

### 后端：对运行实例的接口冒烟

```bash
node scripts/api-smoke.mjs                      # 默认 http://127.0.0.1:8080
node scripts/api-smoke.mjs http://host:port     # 指定实例
```

共 90 项断言，覆盖登录/令牌刷新/CRUD/权限隔离/参数校验/日志/导入导出/数据范围/令牌吊销。

### 前端：类型检查与构建

```bash
cd frontend
npm run typecheck      # vue-tsc --noEmit
npm run build          # 类型检查 + 产出 dist/
npm run preview        # 本地预览生产产物（已配置 /api 代理）
```

### 前端：真实浏览器运行时验证

编译通过不等于运行时正确。本项目曾出现过「类型检查全绿、但冷启动直接落 404」这类缺陷，
因此提供了一份零依赖的浏览器验证脚本（Node 原生 `fetch` + `WebSocket` 驱动系统已装的 Chrome）：

```bash
# 前提：后端与前端都已启动
node scripts/browser-verify.mjs
APP_URL=http://localhost:4173 node scripts/browser-verify.mjs    # 验证生产产物
```

80 项断言，覆盖登录链路、F5 深链、菜单遍历白屏检测、树选择器渲染、**商品多规格编辑弹窗（结构 + 嵌套校验）**、**分类树操作按钮可见性**、**开单选商品的关键字模糊查找**、**商品表单分类下拉含全部层级且可搜索**、**明细商品列宽度下限**、**弹窗尺寸分档（1280 / 1100）**、**echarts 6 下的图表实际渲染（canvas 像素尺寸）**、按钮级权限与导入导出入口、账号切换、控制台干净度。
退出码非 0 表示存在失败项，可直接接入 CI。

脚本另外覆盖了**双主题**：跟随系统偏好、登录页与顶栏切换、刷新后持久化、
暗色下无浅色底残留（按亮度阈值扫描）、关键文字与主按钮的对比度（WCAG 算法）、
以及暗色下弹窗 / 下拉弹层 / 输入框的底色。

> 注意：无头 Chrome 的 `prefers-color-scheme` 默认是 **dark**，脚本会在连接后显式固定为 `light`，
> 否则主题初始值会随浏览器实现漂移。详见 `AGENTS.md` §5.14。

---

## 四、下一步去哪看

- **新接手？先读 [`docs/GUIDE.md`](docs/GUIDE.md)** —— 项目说明、使用步骤、新增业务全流程
  （同内容的 Word 分发版：[`docs/Nova-Admin-项目说明与开发指南.docx`](docs/Nova-Admin-项目说明与开发指南.docx)）
- 接口清单与响应约定 → [`docs/API.md`](docs/API.md)
  （在线版：后端开启 `app.swagger` 后访问 `/swagger/index.html`，由 swag 注解自动生成）
- 数据表结构与索引设计 → [`docs/DATABASE.md`](docs/DATABASE.md)
- 后台系统独立升级方案（目录契约 / 迁移分段 / 升级 SOP / 回滚） → [`docs/UPGRADE.md`](docs/UPGRADE.md)
- 版本变更明细（改了什么、动了哪些表、要不要手工步骤） → [`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- 新增模块 / 新增页面的标准做法、踩坑记录 → [`AGENTS.md`](AGENTS.md)

---

## 五、依赖许可

| 层 | 主要依赖 | 许可 |
| --- | --- | --- |
| 后端 | gin, gorm, pgx, golang-jwt/v5, validator/v10, zap | MIT |
| 后端 | golang.org/x/crypto | BSD-3-Clause |
| 前端 | vue, vue-router, pinia, element-plus, axios, dayjs, nprogress, @vueuse/core, vite, sass | MIT |
| 前端 | echarts | Apache-2.0 |
| 前端 | typescript | Apache-2.0 |
| 后端 | github.com/xuri/excelize/v2 | BSD-3-Clause |

均为宽松许可，可商用、可闭源分发。本项目自身以 **MIT** 许可发布，全文见 [`LICENSE`](LICENSE)。
