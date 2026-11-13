# kas CLI

在 `cli/` 目录运行 `go build -o kas ./cmd/kas`。Windows 下可用 `go build -o kas.exe ./cmd/kas`。

```sh
export KAS_SERVER=https://platform.example.com

kas auth login
kas auth status
kas inventory list --keyword "螺纹钢" --page-size 20
kas auth logout
```

登录时，在浏览器打开终端给出的 ERP 授权页，输入验证码并确认。CLI 会等待领取凭据。公网地址使用 HTTPS，本地回环地址可用 HTTP。

智能体可以用两次命令完成授权：

```sh
kas auth login --no-wait
# 把结果中的 verification_uri 和 user_code 提供给用户确认。
kas auth login --session <session_id>
```

## 查询商品和单据

`kas auth login` 默认请求库存只读权限。使用 `--scope` 请求所需权限；浏览器会列出本次请求的权限。已有库存凭据可以继续使用。用独立的 profile 可以保留原授权，或先退出当前 profile 再重新授权。

```sh
export KAS_PROFILE=erp-agent
kas auth login --scope "inventory.read product.read purchase.read sales.read"
kas product list --keyword "螺纹钢"
kas product get --id 123
kas purchase list --status 2 --start-date 2026-11-01 --end-date 2026-11-14
kas purchase get --id 456
kas sales list --settle-status unpaid
kas sales get --id 789
```

异步授权时，把 `--scope` 加在第一次 `auth login --no-wait` 上。列表返回分页对象，详情按列表返回的 `id` 查询。商品列表可用 `--category-id` 和 `--status 0|1`。采购与销售列表可按关键词、日期、单据状态和结算状态过滤，查询结果受 ERP 账号的数据范围约束。完整参数可运行 `kas --help`。

成功结果以 JSON 写到 stdout，错误以 JSON 写到 stderr。退出码 `2` 表示参数错误，`3` 表示需要授权，`4` 表示授权范围或 ERP 权限不足，`1` 表示未找到记录或其他错误。授权状态查询成功但没有有效凭据时，返回 `"authenticated": false`。库存命令输出规格库存分页数据。

同一操作系统账号下，使用相同平台 URL、配置目录和 profile 的进程共用凭据。profile 默认为 `default`，可用全局参数 `--profile NAME` 或 `KAS_PROFILE` 区分不同 ERP 用户。全局参数放在子命令之前。凭据保存在系统用户配置目录的 `kas` 子目录，可用 `KAS_CONFIG_DIR` 指定目录；Linux 和 macOS 的凭据文件权限为 `0600`，Windows 使用用户目录的访问控制。凭据及设备码不出现在命令输出中。平台凭据有效期为 8 小时，到期后重新授权。

`auth logout` 会先在平台撤销，再清理本地凭据。浏览器也可以撤销同一份授权，共用凭据的进程随后会收到未授权响应。若客户端在保存凭据时异常退出，可能留下配置目录中的 `credential.lock` 空目录。确认没有 `kas` 进程在更新登录后，可删除该空目录再重试。
