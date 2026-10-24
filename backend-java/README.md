# Kasinlo ERP Java

Java 17+（推荐 JDK 21）、Spring Boot、Spring JDBC、Flyway、PostgreSQL / H2。
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
- `persistence`：基于固定表字段清单的参数化 SQL；不接收客户端表名或列名。
- `web`：HTTP 路由适配、统一响应、文件下载与 Vue 入口。
- `src/main/resources/db/migration`：数据库初始化脚本。
- `tools/extract-baseline.py`：一次性提取 Go 接口、表与菜单契约；运行 Java 无需执行。

## 验证

`python tools/review-smoke.py target/kasinlo-erp-2.0.0-java-SNAPSHOT.jar`
启动隔离数据库，检查页面接口、审核与反审核、并发防超卖、失败事务回滚、
收付款幂等、合同分期、质保日期、只读权限和打印模板。
GitHub Actions 进一步在 PostgreSQL 上执行相同业务测试。

## 当前迁移边界

这是一份待完整等价验收的迁移候选，不能把129条路由清单等同于129条业务已验收。
Go 二进制升级包在 Java 版明确拒绝；Java 使用 JAR / 容器发布。
恢复到新数据库的演练需启动独立 Java 实例，原地恢复仅接受 Java JSON 备份。
列表过滤目前集中在 Java 内存中，数据量较大时需迁移为 SQL 分页。
Excel列标题、复杂筛选、角色授权边界、登录防爆破和打印纸面仍需逐项对照上游测试。
业务评审发现的问题应登记并修复后再扩展需求。
