package model

import "time"

// SchemaVersion 记录「数据库结构已经升到哪个版本」。
//
// 为什么需要它：本项目此前只靠 GORM AutoMigrate 同步表结构。AutoMigrate 是
// 幂等的「补齐」（缺表建表、缺列加列），但它**不知道方向** —— 库比程序新还是旧，
// 它一样安静地什么都不做。于是回滚时最容易出的事故是：把旧程序指回已经升级过的库，
// 程序照旧启动、照旧查询，直到某个页面按旧字段读出脏数据才被发现。
//
// 有了这张表，启动时就能显式判断：库版本 > 程序版本 → 拒绝启动并提示恢复备份。
//
// 一个版本一行（不是一行记当前值）：留下升级轨迹，出问题时能看出「哪天升到了 v3」。
type SchemaVersion struct {
	Version   int32     `gorm:"primaryKey;autoIncrement:false" json:"version"`
	AppliedAt time.Time `gorm:"not null" json:"appliedAt"`
	Note      string    `gorm:"size:255" json:"note"`
}

// TableName 指定表名。
func (SchemaVersion) TableName() string { return "sys_schema_versions" }
