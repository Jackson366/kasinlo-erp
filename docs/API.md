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
| `page` | int | 1 | 页码，从 1 开始；服务端会按 OFFSET 上界（100 万）反推页码上限并夹住 |
| `pageSize` | int | 10 | 每页条数，服务端上限 200 |
| `keyword` | string | - | 模糊搜索关键字 |
| `orderBy` | string | - | 排序字段，白名单校验，非法值自动回退 |
| `order` | string | desc | `asc` / `desc` |

> **页码必须收敛**：`OFFSET = (page - 1) × pageSize`。`pageSize` 封顶了但页码没封顶时，
> `?page=99999999` 会算出约 2×10⁹ 的 OFFSET，PostgreSQL 必须先扫描丢弃这 2 亿行
> 才能返回 `LIMIT` 条 —— 单个请求即可占满连接池；更大的值还会让乘法**整型溢出**成负数。
> 超界时服务端收敛到 OFFSET 上界所在页，返回空列表而非报错。详见 `AGENTS.md` §5.41。

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
| `50200` | 500 | 服务器内部错误（通用） |
| `50300` | 503 | 服务不可用 |

> **角色授予的 403**：分配角色时服务端会校验「可授予范围」——目标角色包含的每个权限点，
> 操作者自己必须也有；数据范围也不得比操作者更宽；`super_admin` 只有超管本人能授予。
> 越界返回 `40300`，message 会指明缺的是哪个权限点或范围。
> **这条判定覆盖全部四条入口**：新建/修改用户的角色、Excel 批量导入、修改角色属性、给角色分配菜单。
> 角色下拉接口（`GET /roles/options`）同样只返回可授予的角色。

> **数据范围（`data_scope`）**：角色的数据范围有 4 档（1 全部 / 2 本部门 / 3 本部门及以下 / 4 仅本人），
> 多角色取最宽松者。它**同时约束读与写**，涉及「人」的接口一律生效：
> 用户列表与详情、部门树、登录与操作日志。
>
> | 越界场景 | 返回 |
> | --- | --- |
> | 读超出范围的部门 / 用户（列表、详情） | `40400`（按「不存在」处理，避免用返回信息探测其他部门有哪些数据） |
> | 写超出范围的部门（改、删、在其下新建） | `40300`「目标部门超出你的数据范围」 |
> | 把本部门挂到范围外的父部门下 | `40300`（只判目标部门会留下这条路） |
> | 把用户挪到范围外的部门 | `40300` |
>
> 注意 `login_logs.user_id` 可能为 0（未识别出账号的失败登录），这类记录不属于任何人的范围，
> 非超管看不到——否则等于把匿名爆破的来源 IP 全部公开。

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

**验签的三道校验**（`internal/pkg/jwt`）：

1. **算法白名单**——只接受 `HS256`。`HS384` / `HS512` / `alg=none` 一律拒绝
   （`jwt.WithValidMethods` + keyfunc 里显式比对 `Alg()`）。
2. **issuer 匹配**——签发时写入的 `iss` 必须等于配置的 `jwt.issuer`。
   多环境共用同一份 `jwt.secret` 时，这一条防止别的系统的令牌在本服务通过验签。
3. **`uid` 必须为正**——`uid <= 0` 的载荷直接判无效，避免 0 值落到「查不到用户」的静默分支。

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

> 两个限流维度的**记账是原子的**：单次加锁内完成「读窗口计数 → 判断 → 占一格」。
> 若拆成 `Allow` + `Fail` 两步调用，并发请求会同时通过判断再各自记账，阈值被成倍突破 ——
> 限流器恰好在爆破场景下失效。详见 `AGENTS.md` §5.38。
>
> 限流器的 key 数量有硬上界（默认 1 万）。超出后新来源 IP 不再记录、直接放行，
> 并计入 `evicted` 指标 —— **宁可漏放攻击也不能 OOM**，map 分配失败不可 recover。
> 详见 `AGENTS.md` §5.39。

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
{ "app": "nova-erp", "status": "ok", "version": "v1" }
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

> **前端对这组码的处理**：`40100 / 40102 / 40103 / 40104 / 40105` 归入「认证失效」，
> 会触发静默续期；续期失败后弹出「登录已过期」，且**并发请求只弹一次**
> （由 `utils/request.ts` 的 `sessionExpiredNotified` 闸门去重）。
> `40101` / `40106` 是业务凭证错误，直接提示、**不走**续期流程。
> `POST /auth/logout` 已在 `AUTH_ENDPOINTS` 里豁免——退出时令牌后端已作废，
> 否则会再弹一次「登录已过期」。

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
| PUT | `/api/v1/profile` | 修改昵称 / 邮箱 / 手机号（**PATCH 语义**，见下） |
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

> **`PUT /profile` 是 PATCH 语义**：`nickname` / `email` / `phone` / `avatar` 四个字段都是可选的，
> **不传的字段不会被改动**，显式传空串（`""`）即清空该字段。
> 一个字段都不传返回 `40000`。
> 之所以强调这一点：服务端用 map 形式的 `Updates` 写入，而 GORM 对 map **不做零值省略**，
> 若按「全量覆写」实现，只改昵称的请求会把邮箱、手机号、头像一并清空。
> 注意邮箱格式校验只在非空时生效——显式清空邮箱是合法的。

### 4.2 菜单树

```json
[
  {
    "id": 1,
    "name": "InvOverview",
    "path": "/inv/overview",
    "component": "inv/overview/index",
    "meta": { "title": "数据概览", "icon": "Odometer", "keepAlive": true, "permission": "inv:overview:view" }
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

`GET /system/depts` 返回树结构，每个节点带 `userCount`（含所有子部门的在职人数）。
修改上级部门时会校验环路；有下属用户或有子部门的部门不可删除。

**全部 5 个接口都受数据范围约束**（见 §1.5 的范围表）：非超管只能看到并管理
「本部门 / 本部门及以下」范围内的部门。越界读返回 `40400`，越界写返回 `40300`。
其中「把本部门挂到范围外的父部门下」也会被拒——只判目标部门会留下这条路，
因为挂上去之后该部门的实际管辖范围已经变了。`userCount` 同样按范围统计，
否则部门树裁剪了、人数还是全库的，等于换个字段继续泄露。

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

> **`DELETE /system/logs/logins` 有保护窗口**：`before` 为空（清空全部）时，
> 仍处于登录锁定期内的记录**一律保留**，实际删除范围被硬性限制在
> 「距今超过 `security.login_lock_seconds` 之前」。
>
> 原因：`login_logs` 不只是审计数据，它同时是账号锁定判定（`isLocked` 数窗口内失败条数）的输入。
> 若允许一并删掉，这个接口就等于「解除所有账号的锁定」——攻击者只要能触发锁定就能立刻恢复无限次尝试。
> `DELETE /system/logs/operations` 没有这层限制，操作日志不参与任何安全判定。

操作日志查询参数支持 `module`、`status`、`startTime`、`endTime`（时间格式 `2006-01-02 15:04:05` 或 `2006-01-02`）。

请求体中的敏感字段（`password`、`oldPassword`、`refreshToken` 等）在落库前已被替换为 `***`。

导出会对超长字段做截断（错误信息 200 字符、User-Agent 120 字符），避免单行撑爆表格。

---

## 十、进销存
## 十一、进销存

进销存是这套后台的主线业务，权限命名空间统一为 `inv`。接口全部挂在 `/api/v1/inv` 下，
分页、错误码、令牌策略与第一至三章完全一致。

> **业务线的二次鉴权**：采购与销售共用 `/inv/orders` 一条路由，路由上的
> `RequirePerm` 只要求「采购或销售任一读权限」。因此进入 handler 之后
> 还会按实体的 `bizType` 再做一次 `ensureBizPerm` 判定 ——
> 只有 `inv:sales:list` 的账号传 `bizType=purchase` 会被拒（403），
> 否则换个查询参数就能读到另一条业务线的单据。

### 11.1 数据概览

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/overview` | `inv:overview:view` | 客户数/活跃客户/本月新增、应收/应付总额、本月销售/采购、待处理单据、商品数与预警数 |

**这是登录后的落地页**。落地页由前端 `firstLeafPath(menus)` 按菜单树顺序推导 ——
「数据概览」是**顶级菜单**且 sort=1，第一个叶子就是它（见 AGENTS.md §5.62）。

### 11.2 基础资料（客户 / 供应商 / 商品 / 商品分类）

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/customers` | `inv:customer:list` | 客户列表（含 `balance` 应收余额、`orderCount` 单据数） |
| POST | `/api/v1/inv/customers` | `inv:customer:create` | 新增客户 |
| PUT | `/api/v1/inv/customers/{id}` | `inv:customer:update` | 修改客户 |
| DELETE | `/api/v1/inv/customers/{id}` | `inv:customer:delete` | 删除客户（被单据引用时 40900） |
| GET | `/api/v1/inv/suppliers` | `inv:supplier:list` | 供应商列表（`balance` 为应付余额） |
| POST/PUT/DELETE | `/api/v1/inv/suppliers[/{id}]` | `inv:supplier:*` | 供应商增删改，语义与客户一致 |

**商品是三层结构**：`商品 → 规格 → 计量单位`。

- 第一层 `inv_products` 只放**身份**：编码、名称、分类、状态、备注。
- 第二层 `inv_product_specs` 是**库存桶**：同一商品的每个规格（如 `φ20`、`φ25`）
  各自记 `stock` / `warn_stock`。
- 第三层 `inv_product_units` 是**计量单位**：同一规格下可以有多个单位
  （如 `根` / `米` / `捆`），各带 `rate` 换算率与采购价 / 销售价。

**库存只记在规格上，且按基本单位计**；**单价记在单位上**。
于是「`φ20` 剩 30 根 = 180 米」永远只有一个数，「1 箱 100 元 vs 1 个 10.5 元」
的价差也能被正确带出 —— 库存归属与价格归属分开，是这套模型的关键。

`rate` 的口径固定为「**1 个该单位 = rate 个基本单位**」，基本单位自身 `rate` 恒为 1。
单据按 `qtyBase = qty × rate` 折算扣减库存。

#### 商品分类（字典表）

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/categories` | `inv:category:list` | 分类列表（含 `productCount` 引用商品数） |
| GET | `/api/v1/inv/categories/options` | 同列表 | 启用分类的精简选项，供下拉 |
| POST | `/api/v1/inv/categories` | `inv:category:create` | 新增分类（名称重复 40900） |
| PUT | `/api/v1/inv/categories/{id}` | `inv:category:update` | 修改分类（名称 / 排序 / 状态 / 备注） |
| DELETE | `/api/v1/inv/categories/{id}` | `inv:category:delete` | 删除分类（**仍被商品引用时 40900**，可改为停用） |

分类从「商品表上的一列自由文本」升级为独立字典表，是为了让品类统计稳定 ——
自由文本下「钢材」「钢材 」会各算一类，报表的品类占比会凭空多出一块。
删除前必须校验引用：否则 `inv_products.category_id` 悬空，商品还在、分类没了。

#### 商品

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/products` | `inv:product:list` \| `inv:purchase:list` \| `inv:sales:list` \| `inv:stock:list` | 商品列表（商品管理页分页；含规格数 / 单位数 / 总库存 / 规格明细） |
| GET | `/api/v1/inv/products/pick` | 同商品列表 | **三级级联选项**：商品 → 规格 → 单位（只含启用且单位非空的规格），供单据明细下拉 |

#### `GET /inv/products/pick` 的返回体

```json
{ "items": [ { "id": 1, "code": "SP001", "name": "热轧钢板", "specs": [ … ] } ],
  "total": 9, "truncated": false }
```

级联选项一次最多给 `pickLimit`（500）个商品，`truncated` 为 `true` 表示**后面还有**。
这个标记是刻意暴露的：不给的话，商品多了以后下拉里就「少了几个」，
用户只会以为系统没录进去 —— 静默截断比报错更难排查。
前端拿到 `truncated=true` 要提示「请用关键字搜索」，并且搜索要走
`remote-method` 合并服务端结果（不能只靠本地 `filterable`，那是过滤不是搜索）。
| GET | `/api/v1/inv/products/duplicates` | `inv:product:list` | 按名称查同名商品（`name`、可选 `excludeId`），供录入防重提示 |
| GET | `/api/v1/inv/products/{id}` | 同商品列表 | 商品详情（含完整规格与单位树） |
| POST | `/api/v1/inv/products` | `inv:product:create` | 新增商品（编码重复 40900；规格与单位见下方约束） |
| PUT | `/api/v1/inv/products/{id}` | `inv:product:update` | 修改商品（联动保存规格与单位） |
| DELETE | `/api/v1/inv/products/{id}` | `inv:product:delete` | 删除商品（被单据明细引用时 40900，可改为停用） |

静态路径 `pick` / `duplicates` 注册在 `/{id}` **之前**，否则会被后者当成 id 吞掉。

商品列表支持 `keyword`（匹配编码 / 名称 / **规格**）、`status`（0/1）、`categoryId` 三个筛选。

**`stock` 是只读字段**：它由出入库单据驱动，新增 / 修改接口都不接受它 ——
否则档案上会多出一套与单据无关的库存账，盘点对不上时无从追溯。
`warn` 由 `warn_stock > 0 && stock <= warn_stock` 推导，不入库。

**规格与单位的写入约束**（任一不满足返回 40000）：

- 同一商品下规格名不得重复；同一规格下单位名不得重复。
- 每个规格**有且只有一个基本单位**（`isBase`），且它的 `rate` 必须等于 1。
- 每个规格至少带一个计量单位。
- `rate` 必须 `> 0`。

**改名不能产生僵尸规格**：规格被历史单据引用后是删不掉的（只能停用），
所以保存时回传规格 / 单位的 `id`，后端**优先按 ID 匹配**、否则按名称匹配，
再「比对后增删改」而不是全删重建 —— 全删重建会让规格与单位 ID 漂移，
历史单据明细的 `spec_id` / `unit_id` 随即失效，那张单再保存就会被校验打回。
被移除但已被引用的规格只做停用（`specInUse`），保住历史单据可编辑。

**明细里的三元组会被校验**：`resolveSpecUnits` 检查 `productId → specId → unitId`
是否构成真实存在的链（单位属于规格、规格属于商品），**并且三层都必须是启用状态**，
不成立则 40000（「商品「X」下不存在所选规格」/「规格「Y」下不存在所选计量单位」/
「规格「X Y」已停用」）。
不校验的话下拉串了会把库存记到不属于它的规格上，**账面上完全看不出来，要等盘点才发现**；
不校验状态的话，页面打开后管理员停用档案这段竞态里，停用就形同虚设。

**关键字里的 `%` 与 `_` 不当通配符**：所有模糊搜索统一转义（`ILIKE ? ESCAPE '\'`）。
否则搜「`M12_40`」会连「`M12X40`」一起出来，搜「`50%`」等于搜 `50` 开头的一切 ——
结果既不报错也不为空，只是**静默地不是你要的那批**。

**录入防重只提示不拦截**：同名不同规格是合法场景（两家供应商各自的「纸箱」），
硬拦会挡住正常录入。前端在名称停止输入 400ms 后拉一次 `duplicates`，
命中则列出「编码 / 名称 / 分类 / 规格摘要」并可一键跳转编辑。

列表行一次返回「余额 + 单据数」，避免前端逐行再发一次请求；余额只在
`status = 2`（已审核）的单据上累积，待处理与已取消的单据不形成应收应付。

### 11.3 采购 / 销售单据

`bizType` 取值 `purchase` / `sales`，非法值返回 40000（不静默忽略）。

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/orders?bizType=sales&status=1` | `inv:sales:list` | 单据列表，`status` 1 待处理 / 2 已审核 / 3 已取消 |
| POST | `/api/v1/inv/orders` | `inv:sales:create` | 新增单据（单价填 0 时取**所选计量单位**的销售价 / 采购价） |
| GET | `/api/v1/inv/orders/{id}` | 按实体 `bizType` | 单据详情（主表 + 明细；明细带 `productCode`、`specId` / `unitId` / `rate` / `qtyBase`，主表带 `operator` 制单人） |
| PUT | `/api/v1/inv/orders/{id}` | `inv:sales:update` | 修改单据（仅待处理，整体覆盖明细） |
| DELETE | `/api/v1/inv/orders/{id}` | `inv:sales:delete` | 删除单据（已审核不可删，返回 40900） |
| PUT | `/api/v1/inv/orders/{id}/status` | `inv:sales:finish` | 流转状态；置 2 时同事务生成出入库单并按 `qtyBase` 调整库存 |

**业务线二次鉴权要精确到动作**：`/inv/orders` 一条路由服务两条业务线，
路由上的 `RequirePerm` 只做粗筛（采购或销售任一）；handler 内还要按实体的 `bizType`
再判一次 —— **读**路径校验「该业务线任意权限」即可，**写**路径必须比对完整的
`inv:<bizType>:<action>`。只校验业务线的话，持有「采购只读 + 销售审核」的账号
就能审核采购单（AGENTS.md §5.74）。

**采购与销售共用同一张 `inv_orders` 表**，靠 `biz_type` 区分，避免两套结构
完全相同的表各自漂移。**明细行必须选到「规格 + 计量单位」**（`specId` / `unitId`
均为必填），落库时按 `qtyBase = qty × rate` 折算。单价缺省时按方向取**该单位**上的价格：

- `bizType=purchase` → `inv_product_units.purchase_price`
- `bizType=sales` → `inv_product_units.sale_price`

价格记在单位上而非商品上，是因为「1 箱 100 元」与「1 个 10.5 元」本就不是同一个数；
若只记商品一个价，按箱买与按个买就会有一个是错的。

明细里的三元组会被 `resolveSpecUnits` 校验（见 11.2），不成立返回 40000。

**结算状态是推导值，不入库**：`settleStatus` 由「总额 − 已结算」算出，
`unpaid` / `partial` / `paid` 三态。

`dueDate`（结算到期日）是**选填**字段，JSON 里可能为 `null`：

- 不传 / 传空串 → 落库 `NULL`，响应里是 `null`；已填写的也可以再传空串清回 `null`。
- 内部用途只有一个：判定应收应付是否逾期。
- **不进打印纸面**：打印版式（`printConfig`）里没有这一项，模板设计器也不提供，
  因此采购 / 销售单的纸面只有单据编号、单据日期与往来单位。

早期版本用非指针 `time.Time` 承载它，空串会解析出 Go 零值并把 `0001-01-01`
原样写进库 —— 于是「没填」与「过期两千年」不可区分，逾期汇总多算、
打印纸面还会多出一行「到期日：0001-01-01」。现在改指针 + 可空列，
并在启动时做一次幂等数据修补（`database.PatchData`）把存量零值归一成 `NULL`。

### 11.4 库存

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/inventory` | `inv:stock:list` | 库存查询（**按规格出账**，含 `warn` 预警标记；只看启用商品与启用规格） |
| GET | `/api/v1/inv/stock-orders` | `inv:stock:list` | 出入库单据（仅 `status = 1` 的有效单） |
| POST | `/api/v1/inv/stock-orders` | `inv:stock:create` | 手工出入库，**即时过账**（明细需选规格与单位，按 `qtyBase` 折算） |
| DELETE | `/api/v1/inv/stock-orders/{id}` | `inv:stock:delete` | 作废并按反方向回滚库存 |

库存查询的粒度是**规格**：一行 = 一个商品的一个规格，数量以该规格的
**基本单位**呈现（`unit` 列即基本单位名），另带 `purchasePrice` 供库存成本参考。
「一个商品多规格」时会出现多行，这是设计意图 —— 库存本就归属到规格。

**出库校验库存充足**（按 `qtyBase` 折算后比对），不足返回 40900 并带上商品名与现有库存，
禁止出现负库存。
`StockOrderRow` 结构里**刻意不含金额字段**：`inv:stock:list` 能看数量，
不等于能看到成本，这是与 `ProductRow` / `OrderRow` 的差异。

### 11.5 应收应付

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/finance/receivables?settle=overdue` | `inv:finance:list` | 应收明细 + 汇总 |
| GET | `/api/v1/inv/finance/payables` | `inv:finance:list` | 应付明细 + 汇总 |
| POST | `/api/v1/inv/finance/settlements` | `inv:finance:settle` | 登记收款 / 付款 |

`settle` 筛选支持 `unpaid` / `partial` / `paid` / `overdue`。响应把 `summary`
（`total` / `settled` / `outstanding` / `overdue`）与列表一起返回，前端不必再发一次请求。
只有**已审核**单据形成应收应付；登记金额超过未结余额会被拒（40900）。

**逾期口径（明细与汇总必须一致）**：一笔单据算逾期需同时满足
「未结余额 > 0」「`dueDate` 有值且不是零值时间（> 1900-01-01）」「`dueDate < 今天`」。
未填到期日的单据**永远不算逾期**。SQL 侧用 `dueDateOverdueCond` 常量表达这三条，
Go 侧用 `isOverdue()`，两处口径由冒烟脚本断言「汇总 == 逐行之和」守住 ——
历史缺陷正是汇总漏排零值、明细有 `IsZero` 守卫，同一页上出现两个自相矛盾的数字。

### 11.6 报表

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/report/summary?range=month` | `inv:report:view` | 营业收入 / 销售成本 / 毛利 / 毛利率 |
| GET | `/api/v1/inv/report/trend?days=30` | `inv:report:view` | 近 N 天销售与采购趋势（按天补齐，缺失日补 0） |
| GET | `/api/v1/inv/report/ranking?range=month&limit=5` | `inv:report:view` | 客户销售排行 |
| GET | `/api/v1/inv/report/category?range=month` | `inv:report:view` | 品类销售占比 |

`range` 取值 `today` / `week` / `month` / `quarter` / `year`，无法识别时回退本月。
**销售成本用商品当前采购价 join 计算**，`毛利 = 营业收入 − 销售成本`。

### 11.7 打印模板

| 方法 | 路径 | 权限码 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/inv/print-templates` | `inv:print:list` | 模板列表（一次 100 条，不分页） |
| POST | `/api/v1/inv/print-templates` | `inv:print:create` | 新增模板，`paper` 缺省 A4 |
| PUT | `/api/v1/inv/print-templates/{id}` | `inv:print:update` | 修改模板 |
| DELETE | `/api/v1/inv/print-templates/{id}` | `inv:print:delete` | 删除模板 |

`paper` 只接受 `A4` / `A5`，其它值返回 40000。

模板的 `content` 字段存**版式配置 JSON**（单据标题 / 明细列显隐 / 字号 / 页脚），
由前端「打印模板 → 设计」编辑，后端只存取不解释。打印时前端按配置渲染
纸面并调起系统打印；`content` 为空时按内置默认版式输出。

---

## 十二、导入导出

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
