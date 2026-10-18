package model

import "time"

// 升级记录状态。一次升级走完的完整生命周期：
//
//	pending ──apply(抢占)──> executing ──┬──> applied ──(重启后核对)──> success
//	                                   ├──> success（纯前端包：切换即生效）
//	                                   └──> failed（执行中断 / 回滚 / 版本不一致）
//
// 为什么要有 applied 这个中间态：升级的**执行**与**生效**是两件事 ——
// 后端二进制落位成 `.new` 之后必须重启才生效，而这之间进程还跑着旧代码。
// 少了这个状态，「执行成功」会被误读成「升级完成」。
//
// 为什么要有 executing 这个抢占态：执行入口是 HTTP，双击 / 超时重试会产生
// 并发请求。先「抢占」再执行（条件更新 status=pending→executing），
// 第二个请求抢不到就退出 —— 没有它，两个请求都能通过「状态是 pending」
// 的检查然后各执行一遍（TOCTOU）。进程执行中途崩溃会停在 executing，
// 由启动流程判定为 failed，不会卡死状态机。
const (
	UpgradePending   = "pending"   // 计划已生成，等待执行
	UpgradeExecuting = "executing" // 已抢占，正在执行（并发第二个请求会被拒）
	UpgradeApplied   = "applied"   // 已执行，等待重启生效
	UpgradeSuccess   = "success"   // 生效核对通过（或纯前端包切换即生效），升级完成
	UpgradeFailed    = "failed"    // 执行失败 / 中断 / 重启后核对不一致
)

// UpgradeRecord 是一次系统升级的登记。
//
// 存在的意义：升级是少数几个「做对了没人知道、做错了要能复盘」的操作。
// 每次升级留下了「从哪版到哪版、升级包摘要、升级前备份在哪、执行了哪些动作、
// 结果如何」，回滚时才知道该拿哪份备份。
type UpgradeRecord struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// PlanID 是升级计划标识，同时是 staging 目录名。
	PlanID string `gorm:"size:64;uniqueIndex" json:"planId"`

	FromVersion string `gorm:"size:32" json:"fromVersion"` // 升级前程序版本
	ToVersion   string `gorm:"size:32" json:"toVersion"`   // 升级包目标版本

	PackageSHA256 string `gorm:"size:64" json:"packageSha256"` // 升级包摘要
	PackageSize   int64  `json:"packageSize"`

	// BackupPath 是升级前强制留档的备份（.dump 绝对路径），回滚就靠它。
	BackupPath string `gorm:"size:512" json:"backupPath"`

	Status string `gorm:"size:32;index" json:"status"`
	// Plan 是动作清单的 JSON（前端展示与执行都读它）。
	Plan string `gorm:"type:text" json:"-"`
	// Message 是执行结果说明或失败原因，给人看的一行字。
	Message string `gorm:"size:512" json:"message"`

	CreatedAt  time.Time  `json:"createdAt"`
	AppliedAt  *time.Time `json:"appliedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

// TableName 表名。不带 BaseModel：升级记录是审计痕迹，不该被软删除，
// 也不该有 updated_at —— 「这条记录被改过」本身就该是一次新记录。
func (UpgradeRecord) TableName() string { return "sys_upgrade_records" }
