# 前端独立 Mock 模式

用于界面开发、需求评审和本地演示。只启动前端即可使用，不需要 Go/Java 后端或 PostgreSQL。

```bash
cd frontend
npm ci
npm run dev:mock
```

打开 http://localhost:6083。浏览器标题含「Mock 演示」。如 6083 被占用，先停止原前端，或添加 `-- --port 6084`。

| 账号 | 密码 | 权限 |
| --- | --- | --- |
| admin | admin123456 | 可操作模拟业务数据 |
| demo | demo123456 | 只读，写请求会明确拒绝 |

普通 `npm run dev` 继续连接真实后端。Mock 以 Vite 的 `mock` mode 显式启用；普通生产构建不加载或打包 Mock 实现。需要静态演示产物时执行 `npm run build:mock`，然后 `npm run preview`，或把 dist 部署到静态服务器。

## 已支持

- 登录、退出、刷新令牌、个人资料、动态路由和全部现有菜单。
- 商品、分类、客户、供应商、用户、角色等模拟列表及基础新增、编辑、删除；分页、关键词、日期、状态筛选。
- 采购/销售单据详情、新增、编辑、删除、取消、审核、反审核。
- 审核联动库存和出入库记录；库存不足时报错；反审核回滚库存；已收付款单据不允许反审核。
- 应收应付列表、单据收付款、批量结算；失败时整体回滚模拟数据。
- 合同列表、详情、录入、编辑、审核、反审核与基础验收演示。
- 数据概览、基础报表、打印模板列表、系统列表与运维只读预览。

数据存放在当前浏览器 localStorage；刷新和重新启动前端后保留，同一浏览器同一站点的 Mock 账号共用一套演示数据。真实模式与 Mock 模式使用不同的令牌键。不会向真实后端发送业务、刷新令牌或文件请求。

## 演示边界

Mock 是面向页面开发的简化模型，不替代后端的业务规则和集成测试。报表使用演示汇总口径，不模拟全部统计维度与完整历史成本；系统菜单、角色维护不模拟完整 RBAC 授权变更；合同验收不模拟完整质保日历。

文件导入导出、附件上传下载、修改密码、合同期次收付款、数据库备份恢复/清理/升级暂不支持。点击这些操作会明确提示当前边界；未知接口返回 Mock 未实现错误，不会返回虚假的成功，也不会自动回退到真实服务。打印预览使用现有前端组件。

## 重置演示数据

在浏览器开发者工具 Console 执行，随后刷新：

```js
localStorage.removeItem('kasinlo_mock_data_v1')
localStorage.removeItem('kasinlo_mock_access_token')
localStorage.removeItem('kasinlo_mock_refresh_token')
location.reload()
```

只删除 Mock 数据与会话，不影响真实模式的令牌。

## 扩展与验证

- `frontend/src/mock/seed.ts`：演示种子和持久化数据版本键。
- `frontend/src/mock/routes.json`：当前后端菜单对应的页面快照；新增页面时同步更新，测试会检查后端页面覆盖和组件存在性。
- `frontend/src/mock/service.ts`：与传输层无关的模拟请求、数据变更和回滚。
- `frontend/src/mock/adapter.ts`：Axios 适配器，复用现有响应拦截与错误提示。
- `frontend/src/mock/index.ts`：加载数据并安装适配器；由 `main.ts` 在挂载路由前动态加载。

```bash
npm run typecheck
npm test
npm run build
npm run build:mock
```

同源标签页的每次请求会读取最新持久数据，并通过 Web Locks 串行执行事务。新增供应商/商品后，回到原采购单点击“刷新可选资料”即可选用；无需整页刷新，不丢失正在填写的单据。并发保存不会用旧页面快照覆盖另一标签页的新增内容。需要支持 Web Locks 的浏览器及 localhost/HTTPS；缺少能力时明确报错，不静默写入。

完整采购路线浏览器回归：`cd frontend && npm run test:purchase:mock`（先执行 `npx playwright install chromium`，或通过 `CHROME_PATH` 指定浏览器）。脚本启动独立 Mock 前端及全新浏览器上下文，验证跨标签页补资料、精确关联、付款返回、暗色窄屏和并发写入，不拦截业务接口。
