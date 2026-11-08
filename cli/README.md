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

成功结果以 JSON 写到 stdout，错误以 JSON 写到 stderr。退出码 `2` 表示参数错误，`3` 表示需要授权，`4` 表示 ERP 拒绝该操作，`1` 表示其他错误。授权状态查询成功但没有有效凭据时，返回 `"authenticated": false`。库存命令输出规格库存分页数据。

同一操作系统账号下，使用相同平台 URL、配置目录和 profile 的进程共用凭据。profile 默认为 `default`，可用全局参数 `--profile NAME` 或 `KAS_PROFILE` 区分不同 ERP 用户。全局参数放在子命令之前。凭据保存在系统用户配置目录的 `kas` 子目录，可用 `KAS_CONFIG_DIR` 指定目录；Linux 和 macOS 的凭据文件权限为 `0600`，Windows 使用用户目录的访问控制。凭据及设备码不出现在命令输出中。平台凭据有效期为 8 小时，到期后重新授权。

`auth logout` 会先在平台撤销，再清理本地凭据。浏览器也可以撤销同一份授权，共用凭据的进程随后会收到未授权响应。若客户端在保存凭据时异常退出，可能留下配置目录中的 `credential.lock` 空目录。确认没有 `kas` 进程在更新登录后，可删除该空目录再重试。
