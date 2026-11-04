# kasinlo-erp 开放平台

独立 Go 服务。首版绑定一个 ERP 实例，预注册公共客户端 `kas`，只授予 `inventory.read`。授权后可查看 ERP 的规格库存。访问凭据有效期为 8 小时，到期后重新授权。

## 运行

为平台准备一个独立 PostgreSQL 数据库。启动时自动创建授权表。准备两份不同的随机密钥，例如分别执行一次 `openssl rand -hex 32`。以下同一行的两个变量应取相同值。

| 开放平台环境变量 | ERP 环境变量 | 值 |
| --- | --- | --- |
| `PLATFORM_ERP_SIGNING_SECRET` | `NOVA_OPEN_PLATFORM_SECRET` | 签发内部库存 JWT 的独立密钥 |
| `PLATFORM_BRIDGE_SECRET` | `NOVA_OPEN_PLATFORM_BRIDGE_SECRET` | ERP 向平台提交用户授权决定的独立密钥 |
| `PLATFORM_ERP_ISSUER` | `NOVA_OPEN_PLATFORM_ISSUER` | 例如 `kasinlo-open-platform` |
| `PLATFORM_ERP_AUDIENCE` | `NOVA_OPEN_PLATFORM_AUDIENCE` | 当前 ERP 实例唯一标识 |

平台另需 `PLATFORM_DATABASE_URL`、`PLATFORM_ERP_API_URL`（含 ERP API 前缀，如 `http://erp:6085/api/v1`）和 `PLATFORM_VERIFICATION_URI`（如 `https://erp.example.com/#/open-platform/authorize`）。ERP 需设置 `NOVA_OPEN_PLATFORM_BRIDGE_URL`，例如 `http://open-platform:6090`。平台默认监听 6090，可通过 `PLATFORM_ADDR` 修改。

在本目录执行 `go run ./cmd/server`，或从本目录构建 Docker 镜像。公网反向代理为公开端点配置 HTTPS 和请求限流，只转发平台的 `/oauth/`、`/openapi/v1/` 和按需开放的健康检查。ERP 与平台的两个内部接口前缀只经私有网络访问。

## 联调

1. 向 `POST /oauth/device_authorization` 提交表单 `client_id=kas&scope=inventory.read`，取得 `user_code`、`device_code` 和验证链接。
2. 用户打开链接，登录 ERP，输入 `user_code`，核对权限并确认。设备码和访问令牌只由调用端持有。
3. 按返回间隔向 `POST /oauth/token` 提交 `client_id=kas`、`grant_type=urn:ietf:params:oauth:grant-type:device_code` 和 `device_code`。收到 `slow_down` 时增加轮询间隔。
4. 用访问令牌作为 Bearer 凭据调用 `GET /openapi/v1/inventory`。可传 `page`、`pageSize`、`keyword`。
5. 从 ERP 授权页撤销该记录，或向 `POST /oauth/revoke` 提交 `client_id=kas` 和 `token`。后续库存请求应返回 401。

设置 `PLATFORM_TEST_DATABASE_URL` 后执行 `go test ./...` 可运行 PostgreSQL 联调测试。测试使用独立 schema。
