# API 文档

- 基础地址：`http://<host>:<port>`
- 接口前缀：`/api/v1`（由 `config.yaml` 的 `app.prefix` + `app.version` 拼成）
- 数据格式：`application/json; charset=utf-8`
- 认证方式：`Authorization: Bearer <accessToken>`

> **在线版（由代码自动生成）**
>
> 服务启动后访问 **`/swagger/index.html`** 即可看到根据 handler 注解生成的 OpenAPI 文档，
> 支持在线调试。原始描述文件在 [`backend/api/docs/swagger.json`](../backend/api/docs/swagger.json)。
>
> - 开关：`app.swagger`（不写则跟随 `mode`：`debug` 打开、`release` 关闭），
>   也可用 `NOVA_APP_SWAGGER=true` 覆盖。文档会暴露全部路由与参数结构，生产环境默认不对外挂载。
> - 接口签名（路径、方法、参数、响应结构）以在线文档为准；
>   **业务语义、边界条件、错误码解释以本文档为准**，二者配合阅读。
> - 改动接口时：注解随代码一起改，再执行 `bash scripts/gen-swagger.sh` 重新生成
>   （生成物 `backend/api/docs/` 需要一并提交）。

---

## 一、通用约定

### 1.1 统一响应体

**成功**

```json
{
  "code": 0,
  "message": "success",
  "data": { },
  "traceId": "6f391a1d8455843c"
}
```

**失败**

```json
{
  "code": 40300,
  "message": "没有操作权限",
  "data": null,
  "traceId": "6f391a1d8455843c"
}
```

`traceId` 同时出现在响应头 `X-Trace-Id` 与日志中，排查线上问题时可据此串联整条请求链路。

### 1.2 分页响应

分页接口的 `data` 固定为：

```json
{
  "list": [],
  "total": 26,
  "page": 1,
  "pageSize": 10
}
```

### 1.3 请求头

| 请求头 | 是否必填 | 说明 |
| --- | --- | --- |
| `Authorization` | 需要登录的接口必填 | `Bearer <accessToken>` |
| `Content-Type` | 提交 JSON 时必填 | `application/json` |

### 1.4 分页与排序参数

| 参数 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `page` | int | 1 | 页码，从 1 开始 |
| `pageSize` | int | 10 | 每页条数，服务端上限 200 |
| `keyword` | string | - | 模糊搜索关键字 |
| `orderBy` | string | - | 排序字段，白名单校验，非法值自动回退 |
| `order` | string | desc | `asc` / `desc` |

### 1.5 业务错误码

业务码**前 3 位即 HTTP 状态码**，便于前后端一致理解。

| 业务码 | HTTP | 含义 |
| --- | --- | --- |
| `0` | 200 | 成功 |
| `40000` | 400 | 请求参数错误（提示信息为中文，直接可展示） |
| `40100` | 401 | 未登录或令牌缺失 |
| `40101` | 401 | 用户名或密码错误 |
| `40102` | 401 | 账号已被禁用 |
| `40103` | 401 | 账号已被锁定（连续失败超阈值） |
| `40104` | 401 | 令牌已过期 |
| `40105` | 401 | 令牌无效（含令牌版本号已失效，见 §1.6） |
| `40106` | 401 | 原密码错误 |
| `40300` | 403 | 已认证但无权限 |
| `40400` | 404 | 资源不存在 |
| `40900` | 409 | 资源冲突（重名、被引用等） |
| `42900` | 429 | 请求过于频繁 |
| `50000` | 500 | 服务器内部错误 |
| `50100` | 500 | 数据库操作失败 |

> **角色授予的 403**：分配角色时服务端会校验「可授予范围」——目标角色包含的每个权限点，
> 操作者自己必须也有；`super_admin` 只有超管本人能授予。越界返回 `40300`，
> message 会指明缺的是哪个权限点。角色下拉接口（`GET /roles/options`）同样只返回可授予的角色。

### 1.6 令牌策略

| 令牌 | 有效期 | 存放建议 |
| --- | --- | --- |
| `accessToken` | 2 小时 | 前端 `localStorage` |
| `refreshToken` | 7 天 | 前端 `localStorage` |

访问令牌过期时，前端会自动用 `refreshToken` 静默续期并重放原请求（并发请求只触发一次刷新）。

**令牌吊销**：令牌里带着签发时的用户版本号（`sys_users.token_epoch`）。
修改密码、管理员重置密码、禁用账号都会把该版本号 +1，于是**该用户此前签发的所有令牌
（访问令牌与刷新令牌都）立即失效**，返回 `40105`。这解决的是「改了密码，旧会话还能接着用」
最长 2 小时（访问令牌）甚至 7 天（刷新令牌）的问题。

> 前端把 `40102`（账号被禁用）与 `40100 / 40104 / 40105` 一视同仁地当成「会话失效」处理，
> 会清空令牌并跳回登录页。登录 / 刷新接口自身返回的这些码不触发该流程，只弹提示。

### 1.7 登录失败锁定

`config.yaml` 的 `security` 段控制：连续失败 `max_login_fail`（默认 5）次后锁定 `login_lock_seconds`（默认 300）秒。

**判定按「用户名 + 来源 IP」复合计数**：只按用户名计数的话，任何人乱打密码就能把 `admin`
锁在门外（锁定 DoS）；只按 IP 计数则 NAT 出口下的所有同事会互相连坐。
时间窗的起点取「最近一次同用户名同 IP 的成功登录」，因此**登录成功即重置计数**，
审计日志照常完整保留。因此**登录日志必须写得进去**。
客户端可控的 `User-Agent` 在入库前会按字符收敛（`pkg/textutil.Fit`），
不会因为超长或非法 UTF-8 让整行被数据库拒收 —— 否则攻击者只要伪造一个特殊 UA
就能让自己的失败尝试全部不留痕，从而绕过锁定。

**IP 维度限流（429）**：在查库判定之前还有一道内存滑动窗口 `ip_max_login_fail`
（默认 30，`0` 关闭），同 IP 在一分钟内的失败尝试超过阈值直接返回 `42900`，
不会再消耗数据库查询。登录与刷新接口各有独立的限流配额（刷新为 `refresh_rate_limit`，默认 120 次/分）。

### 1.8 来源 IP

审计日志（操作日志 / 登录日志）里的 `ip` 取 `c.ClientIP()`。只有在 `security.trusted_proxies`
里显式列出的代理地址才会被信任并采用 `X-Forwarded-For`；该配置默认为空，
此时记录的是 TCP 对端地址，客户端无法伪造。

### 1.9 二进制响应（导出接口）

**导出类接口不走统一响应体**，直接返回文件流：

| 响应头 | 取值 |
| --- | --- |
| `Content-Type` | `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` |
| `Content-Disposition` | `attachment; filename="<ASCII 回退名>"; filename*=UTF-8''<百分号编码名>` |
| `Content-Length` | 字节数 |
| `Cache-Control` | `no-store` |

中文文件名按 **RFC 5987** 放在 `filename*=UTF-8''` 里，`filename` 只放 ASCII 回退名
（`用户列表_20260929_205419.xlsx` → `_____20260929_205419.xlsx`）。
前端取值优先读 `filename*`，取不到再退回 `filename`。

> 因此**不能复用普通 axios 封装**：本项目的 `utils/request.ts` 会把响应体当 JSON 解析。
> 前端下载一律走 `utils/download.ts`（blob 直取 + `<a download>` 触发）。
> 若导出接口返回错误，响应体仍是 JSON，`request.ts` 已做 blob 错误体反解。

导出接口的 JSON 错误码与普通接口一致（如无权限返回 `40300`）。

---

## 二、健康检查

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/health` | 公开 |

> 注意：不带 `/api/v1` 前缀。

```json
{ "app": "nova-admin", "status": "ok", "version": "v1" }
```

---

## 三、认证

### 3.1 登录

`POST /api/v1/auth/login` · 公开

```json
{ "username": "admin", "password": "admin123456" }
```

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "accessToken": "eyJhbGci...",
    "refreshToken": "eyJhbGci...",
    "expiresIn": 7200,
    "tokenType": "Bearer",
    "user": { "id": 1, "username": "admin", "nickname": "超级管理员" }
  }
}
```

失败时返回 `40101 / 40102 / 40103`。

### 3.2 刷新令牌

`POST /api/v1/auth/refresh` · 公开

```json
{ "refreshToken": "eyJhbGci..." }
```

返回结构与登录一致。

### 3.3 退出登录

`POST /api/v1/auth/logout` · 需登录

---

## 四、个人中心

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/profile` | 当前用户资料 + 权限码集合 |
| PUT | `/api/v1/profile` | 修改昵称 / 邮箱 / 手机号 |
| PUT | `/api/v1/profile/password` | 修改密码（校验原密码） |
| GET | `/api/v1/profile/routes` | **当前用户的菜单树（驱动前端动态路由）** |

### 4.1 用户资料

```json
{
  "id": 1,
  "username": "admin",
  "nickname": "超级管理员",
  "email": "admin@example.com",
  "phone": "",
  "avatar": "",
  "deptId": 1,
  "deptName": "总公司",
  "status": 1,
  "roles": ["超级管理员"],
  "roleCodes": ["super_admin"],
  "isSuper": true,
  "permissions": []
}
```

> `isSuper = true` 时前端短路放行所有权限判断；后端也不查授权表，因此 `permissions` 可能为空数组，这是预期行为。

### 4.2 菜单树

```json
[
  {
    "id": 1,
    "name": "Dashboard",
    "path": "/dashboard",
    "component": "dashboard/index",
    "meta": { "title": "仪表盘", "icon": "Odometer", "keepAlive": true, "permission": "dashboard:view" }
  },
  {
    "id": 10,
    "name": "System",
    "path": "/system",
    "component": "",
    "meta": { "title": "系统管理", "icon": "Setting" },
    "children": [
      { "id": 11, "name": "SystemUser", "path": "/system/user", "component": "system/user/index", "meta": { "title": "用户管理" } }
    ]
  }
]
```

约定：

- **目录节点**（有 `children`）不注册路由，只用于侧边栏分组
- **叶子节点**的 `path` 一律是绝对路径
- `component` 是 `src/views/` 下的相对路径（不含 `.vue`），前端用 `import.meta.glob` 解析
- 非超管用户只能拿到自己角色被授权的菜单，且祖先目录会被自动补齐

---

## 五、用户管理

| 方法 | 路径 | 权限码 |
| --- | --- | --- |
| GET | `/api/v1/system/users` | `system:user:list` |
| GET | `/api/v1/system/users/options` | `system:user:list` 或 `system:role:list` 或 `system:dept:list` |
| GET | `/api/v1/system/users/export` | `system:user:export` |
| GET | `/api/v1/system/users/import-template` | `system:user:import` |
| POST | `/api/v1/system/users/import` | `system:user:import` |
| GET | `/api/v1/system/users/:id` | `system:user:list` |
| POST | `/api/v1/system/users` | `system:user:create` |
| PUT | `/api/v1/system/users/:id` | `system:user:update` |
| PUT | `/api/v1/system/users/:id/status` | `system:user:update` |
| PUT | `/api/v1/system/users/:id/password` | `system:user:reset` |
| DELETE | `/api/v1/system/users/:id` | `system:user:delete` |

> 三条导入导出路由注册在 `/:id` **之前**。这是必需的：静态路径段必须优先于
> 参数路由匹配，否则 `/export` 会被 `/:id` 吞掉并报「路径参数 id 非法」。

查询参数：`page`、`pageSize`、`keyword`（用户名/昵称）、`status`、`deptId`

新增 / 修改请求体：

```json
{
  "username": "zhangsan",
  "password": "123456",
  "nickname": "张三",
  "email": "z@example.com",
  "phone": "13800000000",
  "deptId": 2,
  "status": 1,
  "roleIds": [2],
  "remark": "备注"
}
```

内置保护：`admin`（ID=1）不可禁用、不可删除；用户不可删除自己。

---

## 六、角色管理

| 方法 | 路径 | 权限码 |
| --- | --- | --- |
| GET | `/api/v1/system/roles` | `system:role:list` |
| GET | `/api/v1/system/roles/options` | `system:role:list` 或 `system:user:list` |
| GET | `/api/v1/system/roles/export` | `system:role:export` |
| GET | `/api/v1/system/roles/:id` | `system:role:list` |
| POST | `/api/v1/system/roles` | `system:role:create` |
| PUT | `/api/v1/system/roles/:id` | `system:role:update` |
| PUT | `/api/v1/system/roles/:id/menus` | `system:role:assign` |
| DELETE | `/api/v1/system/roles/:id` | `system:role:delete` |

分配菜单：

```json
{ "menuIds": [1, 10, 11, 12] }
```

`dataScope` 取值：`1` 全部 · `2` 本部门 · `3` 本部门及以下 · `4` 仅本人。

内置角色（`isBuiltin = true`）不可删除；`super_admin` 的菜单不可裁剪，避免把自己锁死。

---

## 七、菜单管理

| 方法 | 路径 | 权限码 |
| --- | --- | --- |
| GET | `/api/v1/system/menus` | `system:menu:list` |
| GET | `/api/v1/system/menus/:id` | `system:menu:list` |
| POST | `/api/v1/system/menus` | `system:menu:create` |
| PUT | `/api/v1/system/menus/:id` | `system:menu:update` |
| DELETE | `/api/v1/system/menus/:id` | `system:menu:delete` |

`type` 取值：`dir` 目录 · `menu` 菜单 · `button` 按钮（仅作为权限点，不产生路由）。

含子节点的菜单不可直接删除，需先清理子节点。

---

## 八、部门管理

| 方法 | 路径 | 权限码 |
| --- | --- | --- |
| GET | `/api/v1/system/depts` | `system:dept:list` 或 `system:user:list` |
| GET | `/api/v1/system/depts/:id` | `system:dept:list` |
| POST | `/api/v1/system/depts` | `system:dept:create` |
| PUT | `/api/v1/system/depts/:id` | `system:dept:update` |
| DELETE | `/api/v1/system/depts/:id` | `system:dept:delete` |

`GET /system/depts` 返回树结构，每个节点带 `userCount`（在职人数）。
修改上级部门时会校验环路；有下属用户或有子部门的部门不可删除。

---

## 九、日志

| 方法 | 路径 | 权限码 |
| --- | --- | --- |
| GET | `/api/v1/system/logs/operations` | `system:log:list` |
| GET | `/api/v1/system/logs/operations/export` | `system:log:export` |
| GET | `/api/v1/system/logs/logins` | `system:log:list` |
| GET | `/api/v1/system/logs/logins/export` | `system:log:export` |
| GET | `/api/v1/system/logs/modules` | `system:log:list` |
| DELETE | `/api/v1/system/logs/operations` | `system:log:delete` |
| DELETE | `/api/v1/system/logs/logins` | `system:log:delete` |

操作日志查询参数支持 `module`、`status`、`startTime`、`endTime`（时间格式 `2006-01-02 15:04:05` 或 `2006-01-02`）。

请求体中的敏感字段（`password`、`oldPassword`、`refreshToken` 等）在落库前已被替换为 `***`。

导出会对超长字段做截断（错误信息 200 字符、User-Agent 120 字符），避免单行撑爆表格。

---

## 十、仪表盘

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/dashboard/overview` | 用户数/角色数/部门数/今日登录 四项概览 |
| GET | `/api/v1/dashboard/trend` | 近 N 天登录与操作趋势 |
| GET | `/api/v1/dashboard/modules` | 各业务模块操作量统计 |
| GET | `/api/v1/dashboard/recent-logins` | 最近登录记录 |

均需登录，无独立权限码。

---

## 十一、导入导出

### 11.1 导出

| 接口 | 权限码 | 工作表 | 说明 |
| --- | --- | --- | --- |
| `GET /api/v1/system/users/export` | `system:user:export` | 用户数据 | 12 列，含部门名、角色名、最后登录信息 |
| `GET /api/v1/system/roles/export` | `system:role:export` | 角色数据 | 10 列，`数据范围` 输出中文文案 |
| `GET /api/v1/system/logs/operations/export` | `system:log:export` | 操作日志 | 12 列 |
| `GET /api/v1/system/logs/logins/export` | `system:log:export` | 登录日志 | 7 列 |

**导出范围必须与列表筛选一致**：接口复用与列表相同的 `where` 构造
（用户模块的 `buildFilter`、日志模块的 `buildOperationFilter` / `buildLoginFilter`）。
否则用户在页面上看到 10 条、导出却是全表，属于数据泄漏级别的缺陷。

用户导出额外支持：

| 参数 | 说明 |
| --- | --- |
| `ids` | 逗号分隔的用户 ID，用于「导出选中项」。为空时按筛选条件导出全部 |

> **`ids` 解析不出合法 ID 时返回 `40000`，不会降级为导出全表。**
> 「没传 `ids`」与「传了 `ids` 但全是非法值」是两种语义，合并处理会让
> 「导出选中项」这个请求悄悄变成「导出整张表」，因此必须显式报错。

单次导出上限 `10000` 行（`sheet.MaxExportRows`），超出会截断并返回列头一致的文件。

表格特性：表头加粗浅底、数据单元格强制文本格式（**避免手机号被 Excel 转成科学计数法**）、
冻结首行、开启自动筛选、按内容估算列宽（中日韩字符按 2 列宽计）。

**导出会写入操作日志。** 导出接口走 `GET`，但属于数据外带风险最高的操作，
因此操作日志中间件的落库条件是「写操作 **或** handler 显式声明了 `opAction`」，
导出与模板下载都会留下 `导出用户` / `导出角色` / `导出操作日志` / `导出登录日志` /
`下载用户导入模板` 这类记录。上传接口的请求体是二进制，日志中记为
`(二进制请求体，未记录)`，不会把 xlsx 内容写进 `text` 列。

### 11.2 下载导入模板

`GET /api/v1/system/users/import-template` · 权限码 `system:user:import`

返回两个工作表：

| 工作表 | 内容 |
| --- | --- |
| `用户导入` | 仅表头，**数据区刻意留空** |
| `填写说明` | 字段含义、必填项、状态取值、部门/角色需先存在等规则 |

> 数据区不放示例行是刻意的：放了示例行，用户很可能不删就直接上传，
> 结果把示例数据导进正式库。格式示例统一放在说明页。

### 11.3 批量导入

`POST /api/v1/system/users/import` · 权限码 `system:user:import` · `multipart/form-data`

| 参数 | 位置 | 说明 |
| --- | --- | --- |
| `file` | form-data | `.xlsx` 文件，单文件上限 5 MB，数据行上限 1000 |
| `dryRun` | query | `1` 只校验不写库；缺省或 `0` 为真正导入 |

**两阶段导入**：前端先带 `dryRun=1` 让用户看到逐行校验结果，
用户确认后再发起一次不带 `dryRun` 的请求写库。

模板列顺序固定为：`用户名 *`、`昵称 *`、`密码`、`邮箱`、`手机号`、`部门`、`角色`、`状态`、`备注`。
表头必须完全一致，否则直接拒绝（防止把列顺序改错却悄悄导进去）。

响应体（成功，`code = 0`）：

```json
{
  "total": 10,
  "valid": 3,
  "invalid": 7,
  "created": 3,
  "rows": [
    { "row": 2, "username": "zhangsan", "nickname": "张三", "errors": [] },
    { "row": 5, "username": "import verify", "nickname": "非法用户名",
      "errors": ["用户名只能包含字母、数字、下划线、点和横线"] }
  ]
}
```

- `row` 是 **Excel 中的真实行号**（含表头，从 1 开始），便于用户直接定位到出问题的那一行
- `created` 在 `dryRun=1` 时恒为 `0`
- 逐行错误互不影响：**合法行照常写入，非法行跳过**，不会因为一行出错整批失败

校验规则：

| 校验项 | 规则 |
| --- | --- |
| 用户名 | 必填，3–64 字符，仅允许字母/数字/`_`/`.`/`-`；文件内不得重复；不得与库中现有用户名冲突 |
| 昵称 | 必填，≤ 64 字符 |
| 密码 | 选填；留空使用默认密码 `123456`；填写时 6–64 字符 |
| 邮箱 | 选填，格式校验 |
| 手机号 | 选填，≤ 32 字符 |
| 部门 | 选填，必须按名称匹配到已存在的部门 |
| 角色 | 选填，多个用 `,`/`，`/`、`/`/` 分隔，必须按名称匹配到已存在的角色 |
| 状态 | 选填，`启用` / `禁用`；留空视为启用 |

其它行为：

- 整批在**一个事务**内写入，任何一行写库失败就整批回滚
- 相同明文密码只计算一次 bcrypt 哈希（否则 1000 行会从毫秒级退化到分钟级）
- 上传文件必须能被解析成 xlsx，且必须有数据行，否则返回 `40000`

常见错误响应：

| 业务码 | 场景 |
| --- | --- |
| `40000` | 扩展名不是 `.xlsx` / 文件无法解析 / 表头被改动 / 没有可导入的数据行 |
| `40300` | 无 `system:user:import` 权限 |
