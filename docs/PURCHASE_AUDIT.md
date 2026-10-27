# 采购审核优化（2026-10-27）

本次在现有单级审核上增加责任留痕、下游摘要与一致的撤销入口。权限码和原有审核/反审核的库存、结算事务保持原有业务语义；Java 与 Go 后端提供相同新增字段，Mock 同步支持。

## 本期业务决定

- 仍允许有权限且在数据范围内的制单人自审，页面与历史标识“自审”，不增加硬性拦截。
- 未引入指派：“我可审核”表示具有对应审核权限、在数据范围内的待处理单据，不代表唯一责任人。“我发起的”按账号 createdBy 匹配，不匹配业务员自由文本。
- “反审核后待处理”仅表示撤销审核，不表示审核驳回。
- 确认要求“已取得收货确认依据”，不要求审核人亲自收货。仍然是审核即入库，没有新增仓管收货状态。
- 等待时长从最近进入待处理开始；24 小时为视觉提示阈值（orderPresentation.ts），不代表业务承诺。历史 pendingSince 为空时不推断等待时长。
- 入库摘要沿用出入库查看权限及其数据范围，未改为跟随采购制单人。当前可见范围内无关联单据不等于仓库没有入库；界面明确该范围。
- 采购详情内的应付摘要来自该采购单已有金额/已结算字段，不开放付款明细或新增财务权限。

## 使用路径

1. 采购工作台默认继续显示待处理队列；交接视图可筛选我可审核、我发起的、反审核后待处理。
2. 核对区或全部单据详情审核后保留本单，显示本次审核人、自审、入库单状态和应付摘要。
3. 展开“审核历史”查看历次审核与反审核，显示操作人、操作和本地时间；分页展示。
4. 在核对区或详情中执行反审核。已有付款时禁用并解释原因；库存不足等服务端失败保留确认窗和错误。
5. 出入库列表中，关联业务单据不再出现直接作废按钮，提供“原单反审核”跳转到对应源单据详情；服务端限制继续保留。手工单据仍按原规则作废。
6. 批量确认说明库存/应付后果，明确批量不结清、各单独立提交。结果保留全部失败单号与原因，分页并可复制全部；浏览器拒绝复制时提供可手工复制文本。

## 数据与迁移

inv_orders 新增可空 auditor_id、audited_at、reversed_by、reversed_at、pending_since；前四项保留最近一次摘要。反审核不会清除审核摘要。inv_order_audits 逐次追加 order_id、order_no、action、actor_id、actor_name、occurred_at、stock_order_id。操作人名称保留事件发生时的快照。无事件更新/删除业务 API。

成功事件和状态、库存一起提交；业务失败或历史写入失败全部回滚，不产生成功事件。再次审核生成新入库单，历史作废入库单继续保留。失败原因通过现有接口错误反馈；本期没有增加独立埋点平台或指标看板，不能把失败计入成功事件表。

- Java：Flyway V3 增量迁移；V1/V2 脚本不变。旧备份恢复允许缺少新增的审核事件表，历史字段保持空。
- Go：AutoMigrate 增量同步，结构版本升至 v5，旧二进制连接新库会被版本闸门阻止。
- 历史审核信息不从入库单或操作日志推断补录。
- 原单即使后来删除，事件快照仍保留；管理员明确执行业务数据清理时，审核事件随单据范围清理，备份/恢复包含新表。

## 契约

GET /inv/orders 新增 handoff=auditable|created|reversed，非法值拒绝；导出沿用同一筛选。原有权限码不变。

列表新增 createdBy、auditorId、auditedAt、auditorName、reversedBy、reversedAt、reversedByName、pendingSince、selfAudit；详情增加 auditHistory 与 downstream={visibility, stockOrders:[{id,orderNo,status}]}。出入库 status=1 已过账，2 已作废。无权查看返回 restricted 和空数组。摘要不返回出入库商品明细、价格或付款流水。

## 验证

- 前端：typecheck、Vitest、production build。
- 浏览器：test:purchase（既有工作台回归）与 test:purchase-audit（真实 Vue + Mock 的审核、反审核、三次历史、付款拦截、十条失败复制、交接筛选、窄屏暗色）。截图只证明界面，Mock 不证明真实库存事务。
- Java：mvn verify，新增 TradeAuditTest 与 AuditMigrationTest，覆盖并发只写一次、失败整体回滚、权限/范围、审核历史与 V2 老数据迁移。review-smoke.py 复测真实 HTTP。
- Go：gofmt、vet、build、internal 包测试与模块边界校验。当前环境没有可启动的 PostgreSQL 服务，未运行 Go test 包的 PostgreSQL 集成测试，也未进行 Java PostgreSQL 验证。
- UI 静态审计：新增范围无发现；仓库原有 TagsView 的 ElDropdown 触发按钮仍被扫描器误报为无动作，见 UX-CONTRACT.md Verification boundary。

## 评审仍需确认

收货确认依据由谁提供、是否禁止自审、是否需要金额分级、仓管的数据范围是否跟随制单人，均未在本次扩展为新业务规则。独立遥测采集与支持工单指标尚未建设。
