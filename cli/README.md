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

## 查询客户和供应商

```sh
kas --profile parties auth login --scope "customer.read supplier.read"
kas --profile parties customer list --keyword "华东" --status 1
kas --profile parties supplier list --keyword "GYS" --page-size 20
kas --profile parties customer get --id 123
kas --profile parties supplier get --id 456
```

详情 ID 从各自列表获取。输出包括 ID、编号、名称、状态。只需查询其中一类时，可单独申请该权限。

## 查询出入库和往来款

```sh
kas --profile fulfillment auth login --scope "sales.read stock.read finance.read"
kas --profile fulfillment sales list --keyword "XS-001"
# 把 456 替换成销售单列表返回的 id。
kas --profile fulfillment stock list --source-type sales --source-id 456
# 把 789 替换成出入库单列表返回的 id。
kas --profile fulfillment stock get --id 789
kas --profile fulfillment finance settlements --order-id 456
kas --profile fulfillment finance receivables --settle-status overdue --source order
kas --profile fulfillment finance payables --settle-status overdue
```

采购单关联查询使用 `--source-type purchase` 和 `purchase.read`。出入库列表默认显示已过账单据，可用 `--include-voided` 查看作废历史。汇总商品数量时按明细的商品、规格和单位分别汇总。流水的调整记录保留带符号金额以及 `adjustsId`。

## 查询商品分类

```sh
kas --profile catalog auth login --scope "product.read"
kas --profile catalog category list --parent-id 0 --status 1
# 将 123 替换为分类列表返回的 id，逐级查询直接子分类。
kas --profile catalog category list --parent-id 123
kas --profile catalog category list --keyword "螺纹钢" --page-size 20
kas --profile catalog category get --id 123
kas --profile catalog product list --category-id 123
```

省略 `--parent-id` 查询所有层级；`--parent-id 0` 只查根分类。输出为分页平表，含 `id`、`parentId`、`name`、`sort`、`status`。筛选或分页结果可能不含父节点，使用 `category get` 补查。商品的 `--category-id` 精确匹配该分类，不包含子分类。分类与商品共用 `product.read`，已有该范围的凭据无需重新授权。

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

### 合同操作记录与收付款流水

使用包含 `contract.history.read` 的凭据查询流水，同时申请 `contract.read` 可从合同详情获取期次 ID。

```sh
kas --profile contract-history auth login --scope "contract.read contract.history.read"
kas --profile contract-history contract get --id 456
kas --profile contract-history contract activity --id 456
# 将 789 替换为合同详情 plans[].id 中的期次 ID。
kas --profile contract-history contract settlements --plan-id 789
```

两个命令以 JSON 输出 `rows`。操作记录最多返回最近 50 条，两项查询均遵循 ERP 账号的合同数据范围。
