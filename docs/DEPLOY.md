# Windows 单机部署

面向「一台 Windows 机器 + 一个 PostgreSQL」的交付形态。全程不需要额外装 Nginx：
后端自带静态托管，前端产物放在 `web/` 目录即可。

> 交付包由 `node scripts/build-release.mjs --version 1.1.0` 生成，
> 产物是 `build/release/nova-erp-<版本>-win64.zip`。

---

## 0. 交付包里有什么

```
nova-erp-server.exe     主程序（API + 前端静态托管）
nova-ops.exe            运维命令行（备份 / 恢复 / 清理 / 巡检）
config.yaml             配置文件（已生成随机 jwt.secret，**只需改 database.password**）
config.example.yaml     配置模板（参考用，不动它）
web/                    前端产物（index.html + assets/）
start.bat               前台启动（纯 ASCII，带配置检查）
DEPLOY.md               本文件

> start.bat 全程纯 ASCII 英文，避开中文 Windows 把中文批处理按 GBK 误读、
> 把 echo 文案当成命令执行的坑。如仍想看中文提示，去看本文件即可。
```

---

## 1. 装 PostgreSQL

1. 下载安装 PostgreSQL 13 及以上（本项目在 17 上验证），记住安装时设的 `postgres` 口令。
2. 确认服务在跑、端口 5432 可连：

   ```bat
   "C:\Program Files\PostgreSQL\17\bin\psql.exe" -h 127.0.0.1 -U postgres -c "select version();"
   ```

3. 建库不需要手工做：主程序启动时会自动 `CREATE DATABASE`。
   前提是所用账号有 **CREATEDB** 权限（用 `postgres` 直接满足）。
   如果要给系统单独建账号：

   ```sql
   CREATE USER nova_erp WITH PASSWORD '强口令' CREATEDB;
   ```

   建库完成后可以收回 CREATEDB：`ALTER USER nova_erp NOCREATEDB;`

---

## 2. 放程序

解压到一个**固定目录**，例如 `D:\nova-erp`。目录里应当直接看到
`nova-erp-server.exe`、`config.example.yaml`、`web\`。

> 路径含空格或中文都可以：程序内部全部走系统路径 API。
> 但**不要放在 `C:\Program Files` 下**——那里默认需要管理员权限才能写日志和备份。

---

## 3. 写配置

交付包里已经有一份 `config.yaml`，且 `jwt.secret` 已在打包时填好随机串，
**唯一要改的就是数据库口令**：

```bat
notepad config.yaml
```

把这一行的 `CHANGE_ME` 换成你的 PostgreSQL 口令（其余保持默认）：

```yaml
database:
  password: CHANGE_ME     # ← 改成你本机 PostgreSQL 的口令
```

> 如果 `config.yaml` 被误删，直接再 `copy config.example.yaml config.yaml` 也行；
> 但那样 `jwt.secret` 会是占位串，启动会被拦下并提示你去换（见第 9 节）。
> 正常随包附带的 `config.yaml` 不会出现这个问题。

其余可保持默认：

| 配置 | 说明 |
| --- | --- |
| `database.password` | 上一步的 PostgreSQL 口令（**必改**） |
| `jwt.secret` | 随包已填随机串，**不用动**；若自己从模板复制才需要换（见上） |

出厂默认值已经是交付形态，确认这几项没被改回去：

```yaml
app:
  mode: release          # debug 会打开接口文档与 gin 详细日志
  swagger: false         # 生产不要暴露全部路由
  demo_data: false       # ★ 关键：不灌演示客户/商品/单据
  web_dir: web           # 前端产物目录（后端托管）
cors:
  allow_origins: []      # 同源部署不需要跨域头
```

`demo_data` 只在**空库首次启动**时生效。如果已经带着演示数据建过库，
需要清空业务数据：管理页「系统运维 → 数据清理 → 业务数据重置」，
或 `nova-ops reset --scope inv_all --yes`。

相对路径（`./logs`、`./backups`、`web`）一律按**可执行文件所在目录**解析，
与从哪个目录启动无关 —— 所以用计划任务/服务方式跑也不会写错位置。

---

## 4. 首次启动

```bat
start.bat
```

看控制台输出，正常会依次出现：

```
已自动创建数据库           ← 库不存在时
自动迁移表结构完成
已补齐缺失的内置菜单 count: N
已启用前端静态托管 dir=...\web
HTTP 服务已启动 addr=0.0.0.0:6802
```

第一次启动会写入内置角色、菜单、权限，以及管理员账号。

---

## 5. 登录并立刻改密

浏览器打开 `http://<服务器IP>:6802`。

```
初始账号：admin
初始口令：admin123456
```

**登录后第一件事：右上角个人中心 → 修改密码。**
初始口令是公开的，未改密等于没有鉴权。启动日志里每次用默认口令登录都会打一条 WARN。

验证清单（5 分钟）：

- [ ] 左侧菜单齐全：数据概览 / 采购 / 销售 / 出入库 / 应收应付 / 报表 / 打印模板 / 商品 / 客户 / 供应商 / 系统管理
- [ ] 刷新任意页面不白屏（hash 深链回退正常）
- [ ] 新建一张采购单 → 审核 → 库存增加 → 反审核 → 库存回到原点
- [ ] 系统运维 → 系统巡检 → 点「重新体检」有提示且时间刷新
- [ ] 系统运维 → 执行备份 → 备份列表出现一条且「校验」列不是「未校验」

---

## 6. 开机自启（可选）

`start.bat` 是前台运行，关掉窗口就停了。要常驻，用「任务计划程序」：

1. 任务计划程序 → 创建任务
2. 常规：勾选「不管用户是否登录都要运行」「使用最高权限运行」
3. 触发器：启动时
4. 操作：程序填 `D:\nova-erp\nova-erp-server.exe`，
   **起始于**填 `D:\nova-erp`（这一项别漏，虽然程序自己按 exe 目录找配置，
   但运维脚本仍按工作目录解析部分路径）
5. 设置：勾选「如果任务失败，每隔 1 分钟重新启动」

也可以用 NSSM 注册成服务（`nssm install NovaERP D:\nova-erp\nova-erp-server.exe`，
AppDirectory 同样要填 `D:\nova-erp`）。

---

## 7. 备份与恢复

界面：系统运维 → 备份管理 → 立即备份。
命令行：

```bat
nova-ops.exe backup
nova-ops.exe restore --file backups\nova_inv_v5-20261019-103000.dump --yes
nova-ops.exe check
```

- 备份默认落在 `backups\`（相对 exe 目录）。**必须同步到第二块盘**：
  备份和数据同盘等于没有备份。
- PostgreSQL 客户端版本低于服务器版本会报 `server version mismatch`，
  在配置里显式指定 `backup.pg_dump_path` / `pg_restore_path`。

---

## 8. 升级

1. 在开发机打包：`node scripts/build-upgrade-package.mjs --version 1.2.0`
2. 把 zip 传到服务器，系统运维 → 系统升级 → 上传
3. 升级前备份 → 执行升级 → 重启服务
4. 重启后日志出现「升级已生效」，状态变为「已完成」

注意：`config.yaml` 的 `app.release` 必须等于升级包版本，否则重启核对会停在
「已执行」。也可以把版本写进构建（`-ldflags`），这样配置文件不用动。

---

## 9. 常见问题

| 现象 | 原因与处理 |
| --- | --- |
| 启动报「读取配置文件失败」 | 没有 `config.yaml`，或用了 `-config` 指到别的路径。默认找 exe 同目录下的 `config.yaml`（随包已带，除非被删） |
| 启动报 `jwt.secret 仍是占位值` | 只有自己从 `config.example.yaml` 手动 `copy` 成 `config.yaml` 才会出现（随包那份不会）。按提示换随机串：`openssl rand -base64 48` |
| `start.bat` 弹出一堆 `'xxx' 不是内部或外部命令` 乱码 | 旧版 `start.bat` 含中文、被中文 Windows 按 GBK 误读破坏了批处理结构。升级到 v1.1.1 起的纯 ASCII `start.bat` 即可解决；或自己用纯英文重写启动脚本 |
| 启动报 `new encoding (UTF8) is incompatible ... (GBK)` | 老版本程序在中文 Windows 上建库的问题。当前版本已显式用 `template0`；若仍报错，手工建库：`CREATE DATABASE nova_inv_v5 WITH ENCODING 'UTF8' TEMPLATE template0;` |
| 启动报「连接数据库失败」 | 检查 host/port/口令；确认 PostgreSQL 服务已启动。程序只做一次连接，失败即退出 |
| 打开页面是 JSON 404 | `web\` 目录不在 exe 旁边，或里面没有 `index.html`。日志里没有「已启用前端静态托管」就是没找到 |
| 登录后左侧菜单是空的 | 启动时模块未装配或种子失败。看日志有没有「模块装配失败」/「初始化XX数据失败」 |
| 局域网其他机器打不开 | 防火墙放行 6802；`app.host` 保持 `0.0.0.0` |
| 忘记管理员密码 | 用另一个管理员重置；都不行就停服后改库（bcrypt 哈希）或联系开发 |

---

## 10. 目录与技术栈（交付说明）

| 项 | 值 |
| --- | --- |
| 后端 | Go + Gin + GORM，单 exe，无外部运行时依赖 |
| 前端 | Vue 3 + Element Plus，构建产物静态托管 |
| 数据库 | PostgreSQL 13+（验证于 17） |
| 默认端口 | 6802（`app.port`） |
| 数据文件 | 全部在 PostgreSQL 里，程序目录只有日志、备份、升级暂存 |
