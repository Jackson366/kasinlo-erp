# Kasinlo ERP Java 迁移

Java 21、Spring Boot 4.1.2、COLA 5.0.0、MyBatis-Plus 3.5.18。

| 模块 | 职责 |
| --- | --- |
| client | 库存查询和开放平台访问的应用契约 |
| domain | 单据审核资格、库存基本数量、结清规则与仓储接口 |
| app | 查询与审核用例，管理事务 |
| infrastructure | MyBatis-Plus Mapper、旧库表映射、行锁与条件更新 |
| adapter | HTTP 响应和开放平台令牌验证 |
| start | Spring Boot 装配、Flyway 测试表结构和 H2 集成测试 |

## 构建与测试

在仓库根目录运行：

```bash
mvn -f backend-java/pom.xml verify
```

默认测试使用 H2 内存库，Flyway 从 `start/src/test/resources/db/h2` 初始化本阶段
读写涉及的表。测试覆盖库存查询、平台授权、审核过账和结清、库存不足、审计写入
失败回滚和并发重复审核。

## 在旧库副本上运行开放平台库存接口

构建后用本地 Spring 配置指定 PostgreSQL 只读账号和开放平台的签名参数。
`erp.platform.secret`、`erp.platform.issuer`、`erp.platform.audience`
与 Go ERP 接收开放平台令牌时的配置相同。签名密钥需要至少 32 字节。

```yaml
spring:
  datasource:
    url: jdbc:postgresql://localhost:5432/nova_erp
    username: erp_readonly
    password: "替换为本地口令"
erp:
  platform:
    secret: "替换为实际开放平台签名密钥"
    issuer: "替换为实际签发方"
    audience: "替换为实际受众"
```

把配置存到仓库外，例如 `/path/to/erp-java-local.yml`：

```bash
java -jar backend-java/start/target/erp-start-0.1.0-SNAPSHOT.jar \
  --spring.config.additional-location=file:/path/to/erp-java-local.yml
```

接口 `GET /api/v1/internal/open-platform/inventory` 使用现有开放平台的短期 ERP 令牌，
接受 `keyword`、`page`、`pageSize`。响应沿用现有分页格式和规格库存字段。
运行配置默认使用只读连接，并关闭 Flyway。旧库的结构和数据仍由 Go 管理。

审核用例当前由 H2 集成测试直接调用。正式对外接管库存查询前，需要在旧 PostgreSQL
库副本验证 Go 与 Java 的库存响应和权限。核心写入切换还需要接入 Web 鉴权及单据数据
范围，并在 PostgreSQL 验证并发过账、旧单据兼容和回切。
