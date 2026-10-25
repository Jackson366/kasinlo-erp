# Kasinlo ERP Java

Java 17+（推荐 JDK 21）、Spring Boot、MyBatis-Plus 3.5.17、Flyway、PostgreSQL / H2。
运行时完全由 Java 执行业务，不调用 Go 服务。`backend/` 暂保留为迁移参考。

## 本地评审

```bash
cd backend-java
mvn package
java -jar target/kasinlo-erp-2.0.0-java-SNAPSHOT.jar
```

另开终端执行 `cd frontend && npm ci && npm run dev`，访问 http://localhost:6083。
默认 H2 文件数据库保存到启动目录的 `data/`，无需另装数据库。
初始管理员 `admin / admin123456`，只读账号 `demo / demo123456`。

仓库根目录执行 `docker compose -f compose.java.yml up --build`，则可在
http://localhost:6085 使用打包在 Java 服务内的 Vue 页面。
首次构建需要下载 Maven、npm 及容器依赖，耗时取决于网络。

## PostgreSQL

设置 `ERP_DATABASE_URL=jdbc:postgresql://127.0.0.1:5432/kasinlo_erp`、
`ERP_DATABASE_USER`、`ERP_DATABASE_PASSWORD`，使用预先创建的空库启动。
Flyway 执行建表；`ERP_DEMO_DATA=false` 关闭业务示例数据。
`ERP_JWT_SECRET` 可配置至少32字符密钥；未配置时每次启动生成随机密钥，旧登录失效。

## 代码组织

- `module/system`：账号、权限、菜单、数据范围与个人资料。
- `module/inv`：基础资料、单据、库存、合同、收付款、报表与文件。
- `module/ops`：Java 格式备份、恢复、清理与巡检。
- `persistence/entity`、`persistence/mapper`：实体映射与 `BaseMapper` CRUD；库存扣减采用专用 Mapper SQL。
- `persistence/ErpRepository`：适配现有业务接口，统一使用 MyBatis-Plus 执行数据库操作；不接收客户端表名或列名。
- 逻辑删除由 `@TableLogic` 维护；主键通过 `@TableId(AUTO)` 回填。显式 null、空串、false、0 的更新使用 `UpdateWrapper.set` 保留语义。
- 用户角色、角色菜单保持原复合主键，通过 Wrapper 条件操作，不伪造单列主键。
- 备份恢复及清理使用明确的物理操作 Mapper，和日常逻辑删除分开。
- `web`：HTTP 路由适配、统一响应、文件下载与 Vue 入口。
- `src/main/resources/db/migration`：数据库初始化脚本。
- `tools/extract-baseline.py`：一次性提取 Go 接口、表与菜单契约；运行 Java 无需执行。

## 验证

`python tools/review-smoke.py target/kasinlo-erp-2.0.0-java-SNAPSHOT.jar`
启动隔离数据库，检查页面接口、审核与反审核、并发防超卖、失败事务回滚、
收付款幂等、合同分期、质保日期、只读权限和打印模板。
`mvn verify` 另包含 MyBatis-Plus 持久层集成测试，覆盖主键、软删除、空值清理、复合关联表、库存精度、幂等键与恢复。
GitHub Actions 进一步在 PostgreSQL 上执行相同业务测试。

## 当前迁移边界

这是一份待完整等价验收的迁移候选，不能把129条路由清单等同于129条业务已验收。
Go 二进制升级包在 Java 版明确拒绝；Java 使用 JAR / 容器发布。
恢复到新数据库的演练需启动独立 Java 实例，原地恢复仅接受 Java JSON 备份。
列表过滤目前集中在 Java 内存中，数据量较大时需迁移为 SQL 分页。
Excel列标题、复杂筛选、角色授权边界、登录防爆破和打印纸面仍需逐项对照上游测试。
业务评审发现的问题应登记并修复后再扩展需求。
