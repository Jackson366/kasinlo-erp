# Kasinlo ERP · Java 迁移分支

当前分支新增独立 Java 后端，保留现有 Vue 3 + TypeScript 页面。

- [Java 启动、验证与迁移边界](backend-java/README.md)
- 一体化评审启动：`docker compose -f compose.java.yml up --build`，访问 http://localhost:6085。
- 本地开发：`cd backend-java && mvn spring-boot:run`，前端按原方式运行。
- 管理员：`admin / admin123456`；只读演示：`demo / demo123456`。
- 下方为原 Go 版本文档，保留作迁移参考；Java 操作以以上文档为准。

---

# Kasinlo ERP

面向中小企业日常经营的轻量 ERP，围绕采购、销售、库存、合同履约与收付款组织业务，采用 **Go + Vue 3 + TypeScript + PostgreSQL** 技术栈。

本仓库基于 [Nova ERP](https://gitee.com/xy_qcdz/nova-erp) 建立，用于独立维护与后续业务扩展。保留上游代码、提交历史和 MIT 许可证；当前功能继承自上游，仓库重新命名不代表已完成业务改造。

## 功能范围

| 模块 | 主要能力 |
| --- | --- |
| 商品与往来单位 | 多级商品分类、多规格、多计量单位及价格，客户与供应商档案 |
| 采购与销售 | 单据录入、审核、反审核、取消、附件与打印 |
| 库存管理 | 审核生成出入库单、库存查询与预警、出库库存校验、作废回滚 |
| 合同管理 | 合同审核、分期付款计划、验收、质保金跟踪、期次收付款与操作记录 |
| 应收应付 | 单据及合同期次应收应付、部分结算、单据批量收付款、逾期统计 |
| 报表与打印 | 经营概览、采购销售排行与明细、库存分析、合同执行与台账、Excel 导出、打印模板设计 |
| 系统管理 | 用户、部门、角色、菜单及按钮权限、数据范围、操作日志、登录日志 |
| 系统运维 | 数据库备份恢复、数据清理、巡检与系统升级 |

## 技术架构

| 层次 | 技术与组织方式 |
| --- | --- |
| 前端 | Vue 3、TypeScript、Vite、Element Plus、Pinia、Vue Router、Axios、ECharts |
| 后端 | Go、Gin、GORM，按业务模块组织 DTO、Handler、Service 与路由 |
| 数据库 | PostgreSQL，启动时执行结构迁移与基础数据初始化 |
| 认证授权 | JWT 双令牌、RBAC、部门及本人数据范围 |
| 接口文档 | Swagger / OpenAPI，由后端注释生成 |

后端采用模块化单体：模块声明路由、菜单与权限，组装层统一注册。前端通过后端菜单生成动态路由，公共列表查询与弹窗逻辑由组合式函数复用。

### 核心业务约定

- **库存按规格、按基本单位记账**：商品、规格、计量单位分层，单据保留单位换算快照。
- **审核与过账同事务完成**：生成出入库单、调整库存和推进单据状态一起提交；出库通过条件更新防止并发超扣。
- **反审核撤销关联影响**：按业务约束回滚库存与出入库记录，涉及已收付款等情况时拒绝不允许的操作。
- **结算状态由金额推导**：根据总额与已结算金额计算未结清、部分结算、已结清。
- **后端执行权限校验**：菜单和按钮展示与接口权限配合，单据读写同时受数据范围约束。

## 快速开始

### 1. 准备环境

| 组件 | 要求 |
| --- | --- |
| Go | 1.26.0 或更高，以 `backend/go.mod` 为准 |
| Node.js | 推荐 22 LTS，并使用 npm 安装前端依赖 |
| PostgreSQL | 13 或更高 |

```bash
git clone https://github.com/Jackson366/kasinlo-erp.git
cd kasinlo-erp
```

### 2. 配置后端

```bash
cd backend
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填写数据库连接信息，并替换 JWT 密钥占位值：

```yaml
app:
  name: kasinlo-erp
  mode: debug
  host: 127.0.0.1
  demo_data: true

database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: YOUR_DATABASE_PASSWORD
  dbname: kasinlo_erp

jwt:
  secret: REPLACE_WITH_A_RANDOM_LONG_SECRET
```

以上仅展示需要修改的字段，**不要用此片段覆盖完整配置文件**。可通过 `openssl rand -base64 48` 生成密钥。配置模板中的默认密钥占位值会被程序拒绝。

数据库不存在时，程序会尝试自动创建，数据库账号需具备 `CREATEDB` 权限；也可以提前建好数据库并授权。`demo_data: true` 用于本地体验，正式环境保持 `false`。

当前代码沿用上游的 Go module 名 `nova-erp`、`NOVA_*` 环境变量前缀和部分界面名称。配置覆盖项以 [配置模板](backend/config.example.yaml) 为准。

### 3. 启动后端

在 `backend` 目录执行：

```bash
go run ./cmd/server -config config.yaml
```

默认服务为 `http://127.0.0.1:6085`，接口前缀为 `/api/v1`，健康检查为 `/health`。需要在线接口文档时，将配置中的 `app.swagger` 改为 `true`，再访问 `/swagger/index.html`；配置模板默认关闭此入口。

### 4. 启动前端

另开终端，在仓库根目录执行：

```bash
cd frontend
npm ci
npm run dev
```

访问 `http://localhost:6083`。Vite 默认代理后端请求到 `http://127.0.0.1:6085`。

### 5. 首次登录

| 账号 | 初始密码 | 用途 |
| --- | --- | --- |
| `admin` | `admin123456` | 系统管理员 |
| `demo` | `demo123456` | 启用演示数据时初始化的只读账号 |

正式使用前修改初始密码与 JWT 密钥，关闭演示数据，并按实际部署环境配置访问地址。业务附件保存在文件目录中，备份时需同时保存数据库与附件目录。

## 目录结构

| 目录 | 内容 |
| --- | --- |
| `backend/cmd` | 服务端与运维命令入口 |
| `backend/internal/module/inv` | 进销存、合同、收付款与报表业务 |
| `backend/internal/assembly` | 模块组装 |
| `backend/internal/model` | 数据模型 |
| `backend/internal/authz`、`middleware` | 授权与请求中间件 |
| `backend/test` | 后端接口及回归测试 |
| `frontend/src/views` | 业务页面 |
| `frontend/src/api`、`types` | 接口封装与类型定义 |
| `frontend/src/composables` | 列表查询、弹窗等公共逻辑 |
| `scripts` | 冒烟验证、浏览器验证与打包脚本 |
| `docs` | 使用、接口、数据结构、部署及运维文档 |

## 开发与验证

后端静态检查与测试：

```bash
cd backend
cp test/testdata/config.example.yaml test/testdata/config.yaml
# 编辑测试配置，填写数据库连接与独立测试库，并替换 JWT 密钥占位值。
go vet ./...
go test ./... -count=1
```

前端类型检查、测试与生产构建：

```bash
cd frontend
npm ci
npm run typecheck
npm test
npm run build
```

系统启动后，可在仓库根目录运行现有验证脚本；浏览器验证还需本机具备脚本支持的 Chrome 环境：

```bash
node scripts/api-smoke.mjs
node scripts/browser-verify.mjs
node scripts/interaction-verify.mjs
```

脚本与测试继承自上游，验证结果以实际运行输出为准。

## 当前边界

- 当前面向单组织使用，尚未实现完整的 SaaS 多租户隔离与订阅体系。
- 库存以商品规格为核心，尚未建立完整的多仓库、库位与批次库存模型。
- 尚未提供 BOM、MRP、生产工单等制造业务。
- 收付款与应收应付属于业务往来管理，尚未形成总账、凭证与会计期间体系。
- 经营报表中的销售成本使用当前采购价计算，不能直接作为历史出库成本核算结果。

## 文档导航

- [使用与开发指南](docs/GUIDE.md)
- [接口说明](docs/API.md)
- [数据库说明](docs/DATABASE.md)
- [部署说明](docs/DEPLOY.md)
- [运维说明](docs/OPS.md)
- [升级说明](docs/UPGRADE.md)
- [上游变更日志](docs/CHANGELOG.md)
- [开发约定与已知问题](AGENTS.md)

部分继承文档仍使用 Nova ERP 名称、默认数据库名和原打包文件名；执行命令时以实际配置与脚本为准。

## 来源与许可证

上游项目：[Nova ERP](https://gitee.com/xy_qcdz/nova-erp)。本仓库导入基于上游 `master` 分支的 `v1.3.1`，对应提交 [`ec0ab0c67c2ff6a6841fc902eb210c0b2f79a804`](https://gitee.com/xy_qcdz/nova-erp/commit/ec0ab0c67c2ff6a6841fc902eb210c0b2f79a804)。感谢上游作者与贡献者。

项目采用 [MIT License](LICENSE)，保留原始版权与许可声明。依赖库遵循各自许可证。
