// Package ops 是框架层的运维能力：备份、恢复、日志清理、业务数据清理、巡检、系统升级。
//
// 它被两条路径共用：独立命令 nova-ops（cmd/ops）与 HTTP 模块 internal/module/ops。
// 本包不 import 任何具体业务模块，也不硬编码业务表名 —— 业务表清单由应用层
// （internal/module/ops）通过 CheckOptions.KeyTables / BackupOptions.AnchorTables 传入。
//
// 注意：本包与 internal/module/ops 的**包名同为 ops**。应用模块（module/ops）引用本包时
// 必须加别名 fwops（framework ops），否则与本模块自身的包名 ops 冲突、无法编译。
// 这是有意的：目录名保持语义（internal/ops），别名负责在 module/ops 里消除歧义。
package ops
