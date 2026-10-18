package model

import "time"

// 升级记录状态。一次升级走完的完整生命周期：
//
//	pending ──apply──> applied ──(重启后由启动流程核对)──> success
//	                       └──(换了回二进制 / 长时间没重启)──> failed
//
// 为什么要有 applied 这个中间态：升级的**执行**与**生效**是两件事 ——
// 后端二进制落位成 `.new` 之后必须重启才生效，而这之间进程还跑着旧代码。
// 少了这个状态，「执行成功」会被误读成「升级完成」。
const (
	UpgradePending = "pending" // 计划已生成，等待执行
	UpgradeApplied = "applied" // 已执行，等待重启生效
	UpgradeSuccess = "success" // 重启后核对版本一致，升级完成
	UpgradeFailed  = "failed"  // 重启后核对不一致（回滚了或没换成）
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
