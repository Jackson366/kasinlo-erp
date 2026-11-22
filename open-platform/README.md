# kasinlo-erp 开放平台

独立 Go 服务。平台绑定一个 ERP 实例，预注册公共客户端 `kas`。每份授权记录保存用户确认的只读权限，ERP 在每次查询时校验账号当前的功能权限与单据数据范围。访问凭据有效期为 8 小时，到期后重新授权。

## 运行

为平台准备一个独立 PostgreSQL 数据库。启动时自动迁移授权表。既有库存凭据与未完成的库存授权请求保持 `inventory.read`。升级时先更新 ERP 后端和前端，再更新开放平台与 CLI。准备两份不同的随机密钥，例如分别执行一次 `openssl rand -hex 32`。以下同一行的两个变量应取相同值。

| 开放平台环境变量 | ERP 环境变量 | 值 |
| --- | --- | --- |
| `PLATFORM_ERP_SIGNING_SECRET` | `NOVA_OPEN_PLATFORM_SECRET` | 签发内部库存 JWT 的独立密钥 |
| `PLATFORM_BRIDGE_SECRET` | `NOVA_OPEN_PLATFORM_BRIDGE_SECRET` | ERP 向平台提交用户授权决定的独立密钥 |
| `PLATFORM_ERP_ISSUER` | `NOVA_OPEN_PLATFORM_ISSUER` | 例如 `kasinlo-open-platform` |
| `PLATFORM_ERP_AUDIENCE` | `NOVA_OPEN_PLATFORM_AUDIENCE` | 当前 ERP 实例唯一标识 |

平台另需 `PLATFORM_DATABASE_URL`、`PLATFORM_ERP_API_URL`（含 ERP API 前缀，如 `http://erp:6085/api/v1`）和 `PLATFORM_VERIFICATION_URI`（如 `https://erp.example.com/#/open-platform/authorize`）。ERP 需设置 `NOVA_OPEN_PLATFORM_BRIDGE_URL`，例如 `http://open-platform:6090`。平台默认监听 6090，可通过 `PLATFORM_ADDR` 修改。

在本目录执行 `go run ./cmd/server`，或从本目录构建 Docker 镜像。公网反向代理为公开端点配置 HTTPS 和请求限流，只转发平台的 `/oauth/`、`/openapi/v1/` 和按需开放的健康检查。ERP 与平台的两个内部接口前缀只经私有网络访问。

## 联调

1. 向 `POST /oauth/device_authorization` 提交表单 `client_id=kas` 和 `scope`（多个权限以空格分隔，省略时默认 `inventory.read`），取得 `user_code`、`device_code` 和验证链接。
2. 用户打开链接，登录 ERP，输入 `user_code`，核对权限并确认。设备码和访问令牌只由调用端持有。
3. 按返回间隔向 `POST /oauth/token` 提交 `client_id=kas`、`grant_type=urn:ietf:params:oauth:grant-type:device_code` 和 `device_code`。收到 `slow_down` 时增加轮询间隔。
4. 用访问令牌作为 Bearer 凭据调用下表中的 GET 接口。
5. 从 ERP 授权页撤销该记录，或向 `POST /oauth/revoke` 提交 `client_id=kas` 和 `token`。后续受保护的请求应返回 401。

## 查询接口

| GET 路径（前缀 `/openapi/v1`） | 授权范围 | ERP 功能权限 | 数据 |
| --- | --- | --- | --- |
| `/inventory` | `inventory.read` | `inv:stock:list` | 规格库存 |
| `/product-categories`、`/product-categories/{id}` | `product.read` | `inv:product:list` | 分类 ID、父分类 ID、名称、排序与状态 |
| `/products`、`/products/{id}` | `product.read` | `inv:product:list` | 商品标识、分类、状态；详情含规格与计量单位 |
| `/customers`、`/customers/{id}` | `customer.read` | `inv:customer:list` | 客户 ID、编号、名称和状态 |
| `/suppliers`、`/suppliers/{id}` | `supplier.read` | `inv:supplier:list` | 供应商 ID、编号、名称和状态 |
| `/purchase-orders`、`/purchase-orders/{id}` | `purchase.read` | `inv:purchase:list` | 采购单号、往来单位、日期、状态、金额；详情含商品明细 |
| `/sales-orders`、`/sales-orders/{id}` | `sales.read` | `inv:sales:list` | 销售单号、往来单位、日期、状态、金额；详情含商品明细 |

列表返回 `list`、`total`、`page`、`pageSize`，支持 `page`、`pageSize` 和 `keyword`。商品列表另支持 `status`（0 停用、1 启用）和 `categoryId`。单据列表另支持 `status`（1 待处理、2 已审核、3 已取消、4 已红冲、5 红冲单）、`settleStatus`（`unpaid`、`partial`、`paid`）以及 `startDate`、`endDate`（`YYYY-MM-DD`，含边界）。

客户和供应商列表的 `keyword` 匹配编号及名称，`status=0|1` 筛选停用或启用的档案。两类档案分别授权，详情使用对应列表返回的 ID，响应只包含基础档案字段。档案按 ERP 的共享主数据规则查询。

响应错误为 `{"error":"..."}`。授权缺少所需范围返回 403 `insufficient_scope`，ERP 账号缺少对应功能权限返回 403 `insufficient_permission`，不存在或超出数据范围的单据详情返回 404 `not_found`。

设置 `PLATFORM_TEST_DATABASE_URL` 后执行 `go test ./...` 可运行 PostgreSQL 联调测试。测试使用独立 schema。

### 出入库和往来款

| GET 路径（前缀 `/openapi/v1`） | 授权范围 | ERP 功能权限 |
| --- | --- | --- |
| `/stock-orders`、`/stock-orders/{id}` | `stock.read` | `inv:stock:list` |
| `/receivables`、`/payables`、`/order-settlements/{id}` | `finance.read` | `inv:finance:list` |

出入库列表支持 `direction=in|out`、`includeVoided=true` 和单号关键词。用 `sourceType=purchase|sales` 与 `sourceId` 成对查询关联单据，该查询同时需要对应采购或销售授权及 ERP 查看权限；列表只在来源筛选生效时返回来源标识。详情返回状态、红冲标记及带计量单位的数量明细，支持查看作废历史。

应收应付列表支持 `settle=unpaid|partial|paid|overdue`、`source=order|plan` 和关键词，返回同一数据范围下的分页结果与汇总。流水接口的 ID 对应采购或销售单 ID，返回同一查看范围内的带符号流水、已结算金额与未结金额。金额沿用 ERP 口径，已经红冲的原单不计入待结算余额。所有这些接口均为只读。

### 商品分类

分类列表支持 `keyword`（分类名称字面匹配）、`status=0|1`、`parentId` 和分页。省略 `parentId` 查询各层分类；`parentId=0` 只查根分类，正整数只查该父级的直接子分类。结果为平表，包含 `id`、`parentId`、`name`、`sort`、`status`，按父分类、排序值、ID 升序排列。分页或筛选结果可能不含父节点，可通过分类详情补查。

分类与商品均为共享主数据，不按创建人收窄。内部备注和商品统计不对外返回。已有 `product.read` 凭据可直接查询，库存凭据仍只有库存权限。查询商品时把分类列表的 `id` 用作 `/products?categoryId=...`；商品筛选只匹配该分类，不自动包含后代。负数、非整数父分类及非法状态返回 400；不存在或已删除分类返回 404。

## 合同查询

合同列表与详情新增独立授权 `contract.read`，对应 ERP 权限 `inv:contract:list`。旧凭据不会自动增加权限，请使用独立 profile 重新授权。

```sh
kas --profile contracts auth login --scope "contract.read"
kas --profile contracts contract list --biz-type sales --status 1 --stage pending --page-size 20
kas --profile contracts contract list --start-date 2026-11-01 --end-date 2026-11-30 --party-id 123
# 将 456 替换为列表返回的合同 ID。
kas --profile contracts contract get --id 456
```

公开 GET 接口为 `/openapi/v1/contracts` 和 `/openapi/v1/contracts/{id}`。列表支持 `keyword`、`page`、`pageSize`、`bizType=purchase|sales`、`status=1|2|3`、`partyId`、`stage`、`startDate` 和 `endDate`。日期按签订日期筛选，包含两端；`partyId=0` 不过滤，往来单位 ID 应与业务类型配合使用。阶段支持 `unaudited`（待审核）、`pending`（待验收）、`accepted`（已验收）、`warranty`（质保中）、`warranty_due`（质保到期）。待审核合同用 `stage=unaudited` 查询，不传 `status`；`status` 沿用 ERP 的执行中、已完成、已终止三态筛选。

列表返回分页对象，含合同编号、往来单位、状态、履约阶段、金额和计划收付汇总；详情另含收付款计划（ID、期次、名称、类型、金额、到期日、已结和未结金额、状态）。无计划时不返回 `plans`。所有列表、总数、详情与计划均受当前账号的数据范围约束，越界或已删除合同返回 404。内部备注、经办人、业务员、审核人及附件信息不对外返回；采购/销售合同以合同查看权限为准，与采购单/销售单授权分别管理。接口均为只读，授权撤销与账号停用沿用现有即时校验。

### 合同报表与流水

| GET 路径（前缀 `/openapi/v1`） | 授权范围 | ERP 功能权限 | 返回数据 |
| --- | --- | --- | --- |
| `/contract-report` | `contract.report.read` | `inv:report:view` | 合同执行报表及当前结果的合计，包含合同金额、计划金额、已收付、未收付、质保金与履约阶段 |
| `/contract-activities/{id}` | `contract.history.read` | `inv:contract:list` | `contractId` 与最近 50 条已落库的操作记录，含动作、请求方式、HTTP 状态和操作时间 |
| `/contract-plan-settlements/{id}` | `contract.history.read` | `inv:contract:list` | `planId` 与该期次的收付款流水，含金额、结算方式和日期 |

`GET /openapi/v1/contract-report` 支持 `keyword`、`bizType`、`status`、`stage`、`partyId`、`startDate`、`endDate`。日期区间按合同签订日筛选。结果按签订日和合同 ID 倒序，最多返回 1000 行，通过 `truncated` 标识超限。合计仅覆盖返回的 `rows`。调用前需申请 `contract.report.read`。

合同操作记录和期次收付款流水均按时间倒序返回 `rows`，没有记录时返回空数组。合同、期次不存在或超出账号数据范围时返回 404。操作记录随 ERP 日志异步落库，返回字段排除操作者账号、IP 与请求内容，收付款流水排除内部备注。申请 `contract.read contract.history.read` 可同时查询合同详情与流水，已有合同凭据需要重新授权才能调用这两个接口。
