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
| `/products`、`/products/{id}` | `product.read` | `inv:product:list` | 商品标识、分类、状态；详情含规格与计量单位 |
| `/purchase-orders`、`/purchase-orders/{id}` | `purchase.read` | `inv:purchase:list` | 采购单号、往来单位、日期、状态、金额；详情含商品明细 |
| `/sales-orders`、`/sales-orders/{id}` | `sales.read` | `inv:sales:list` | 销售单号、往来单位、日期、状态、金额；详情含商品明细 |

列表返回 `list`、`total`、`page`、`pageSize`，支持 `page`、`pageSize` 和 `keyword`。商品列表另支持 `status`（0 停用、1 启用）和 `categoryId`。单据列表另支持 `status`（1 待处理、2 已审核、3 已取消、4 已红冲、5 红冲单）、`settleStatus`（`unpaid`、`partial`、`paid`）以及 `startDate`、`endDate`（`YYYY-MM-DD`，含边界）。

响应错误为 `{"error":"..."}`。授权缺少所需范围返回 403 `insufficient_scope`，ERP 账号缺少对应功能权限返回 403 `insufficient_permission`，不存在或超出数据范围的单据详情返回 404 `not_found`。

设置 `PLATFORM_TEST_DATABASE_URL` 后执行 `go test ./...` 可运行 PostgreSQL 联调测试。测试使用独立 schema。
