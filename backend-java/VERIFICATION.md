# MyBatis-Plus 持久层改造（2026-10-25）

ORM 已统一改为 MyBatis-Plus 3.5.17（Spring Boot 3 Starter）。26 张既有表有显式实体和 BaseMapper，幂等请求表另有专用实体。常规 CRUD、关联表条件操作、主键回填和软删除均使用 MyBatis-Plus；库存条件扣减与物理恢复使用 MyBatis Mapper 专用 SQL。Flyway 建表与既有 HTTP 契约保持不变。

本轮新增 7 项持久层集成测试：BaseMapper 主键/禁用值、逻辑删除保留历史、显式 null 与空串、复合关联表、库存精度及防超扣、幂等键、恢复原 ID 与删除时间并拒绝非法标识符。GitHub Actions 继续执行 H2 和 PostgreSQL 各 58 项业务检查。实际结果以本次提交 Actions 为准。

以下保留首次 Java 迁移的历史验证记录，不代表本次 ORM 改造的执行结果。

---

# Java 迁移验证记录

提交基准：`3f65f68`。2026-10-24。

## 已执行

Java 17 环境使用 Eclipse Java Compiler 编译全部 Java 源码，无编译错误；启动 Spring Boot、Flyway 和真实 H2 数据库，通过 HTTP 执行业务检查。

结果：**58 项通过，0 项失败**。测试每次启动独立内存数据库，包含并发审核。

- 通过：管理员登录
- 通过：菜单动态路由
- 通过：未登录访问被拒绝
- 通过：读取 /profile
- 通过：读取 /system/users
- 通过：读取 /system/roles
- 通过：读取 /system/menus
- 通过：读取 /system/depts
- 通过：读取 /system/logs/operations
- 通过：读取 /system/logs/logins
- 通过：读取 /system/logs/modules
- 通过：读取 /inv/overview
- 通过：读取 /inv/customers
- 通过：读取 /inv/suppliers
- 通过：读取 /inv/categories
- 通过：读取 /inv/categories/options
- 通过：读取 /inv/products
- 通过：读取 /inv/products/pick
- 通过：读取 /inv/products/duplicates?name=螺栓
- 通过：读取 /inv/orders?bizType=purchase
- 通过：读取 /inv/orders?bizType=sales
- 通过：读取 /inv/contracts
- 通过：读取 /inv/stock-orders
- 通过：读取 /inv/inventory
- 通过：读取 /inv/finance/receivables
- 通过：读取 /inv/finance/payables
- 通过：读取 /inv/print-templates
- 通过：读取 /inv/report/summary
- 通过：读取 /inv/report/trend
- 通过：读取 /inv/report/ranking
- 通过：读取 /inv/report/category
- 通过：读取 /inv/report/inventory
- 通过：读取 /inv/report/stock-ranking
- 通过：读取 /inv/report/detail
- 通过：读取 /inv/report/contract
- 通过：读取 /inv/report/contract-ledger
- 通过：读取 /ops/check
- 通过：读取 /ops/backups
- 通过：读取 /ops/cleanup/preview
- 通过：读取 /ops/cleanup/scopes
- 通过：读取 /ops/upgrade
- 通过：读取 /ops/upgrade/records
- 通过：采购审核入库
- 通过：重复审核拒绝
- 通过：采购反审核回滚库存
- 通过：并发销售审核只有一笔成功
- 通过：并发扣减不超卖
- 通过：失败审核事务回滚
- 通过：金额按分计算
- 通过：重复收款幂等
- 通过：超额收款拒绝
- 通过：已收款单据反审核拒绝
- 通过：合同创建为待审核
- 通过：待审核合同收款拒绝
- 通过：合同分期及质保计算
- 通过：打印模板持久化
- 通过：只读账号拒绝写操作
- 通过：接口契约清单

## 未完成的验收

- 本地 Maven 标准打包：当前执行环境没有 javac，依赖下载受限；编译和运行使用上述明确列出的替代路径。
- GitHub Actions 已配置 JDK 21 标准 Maven 构建、原 Vue 类型检查/测试/构建、H2 与 PostgreSQL 业务验证；结果以 Actions 实际状态为准。
- 尚未运行上游完整 Go/Java 差分测试及浏览器页面回归，不能宣称全部行为等价。
- Go 二进制升级改为 Java JAR/容器发布，独立数据库恢复演练改为独立实例执行。
- 复杂筛选、Excel列标题与逐字段校验、登录限流及更细的授权边界仍需完整对照验收。

当前交付为业务迁移候选，尚未达到“所有功能完整迁移并验收”的标准。
