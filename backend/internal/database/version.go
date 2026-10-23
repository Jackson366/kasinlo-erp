package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"nova-erp/internal/model"
)

// CurrentSchemaVersion 是「本二进制认识的最新结构版本」。
//
// 规则（改结构时照做，否则闸门形同虚设）：
//   - 新增表 / 新增列 / 新增索引 / 新增数据修补 → **必须 +1**
//   - 在 schemaChanges 里写清这一版改了什么（日志与 ops check 会打出来）
//   - 只加不删不改名：AutoMigrate 不会删列，硬删会留下「程序不认识但库里有」的
//     残留；真要删，先发一版不再读写该列的代码，下一版再动手。
//
// 版本号只用于「判断方向」，不做逐版本迁移脚本 —— 结构同步仍由 AutoMigrate
// 完成。这样既拿到了「旧程序不能连新库」这道保护，又不必把 20 张表的 DDL
// 抄成 20 个迁移文件（抄错一个就是事故）。
const CurrentSchemaVersion int32 = 3

// schemaChanges 是版本 → 说明，用于升级日志与运维排查。
var schemaChanges = map[int32]string{
	1: "基线：框架 10 张表 + 进销存 12 张表（含幂等表 sys_idempotencies、结构版本表 sys_schema_versions）",
	2: "新增 sys_upgrade_records：升级记录表（版本 / 升级前备份 / 动作清单 / 结果）",
	3: "合同履约：新增 inv_contract_plans（付款计划期次）；inv_contracts 加验收与质保 6 列（accepted_at / accept_result / warranty_months / warranty_end_at / retention_rate / retention_amount）；inv_settlements 加 plan_id / source",
}

// SchemaState 是库侧版本状态。
type SchemaState struct {
	// Version 为 0 表示还没有版本记录：新库，或引入版本表之前就存在的老库。
	Version int32
	// Known 为 false 表示版本表不存在（老库首次升级）。
	Known bool
}

// Current returns 本程序支持的结构版本与说明。
func Current() (int32, string) { return CurrentSchemaVersion, schemaChanges[CurrentSchemaVersion] }

// ReadSchemaVersion 读库里的结构版本。
//
// 用 to_regclass 判断表是否存在，而不是「查一次、把 not exists 当 0」：
// 后者在 PostgreSQL 里会让整个事务进入 aborted 状态，后续语句全部失败 ——
// 一个「库是新还是旧」的判断不该有这种副作用。
func ReadSchemaVersion(db *gorm.DB) (SchemaState, error) {
	var exists bool
	if err := db.Raw(`SELECT to_regclass('public.sys_schema_versions') IS NOT NULL`).Scan(&exists).Error; err != nil {
		return SchemaState{}, fmt.Errorf("读取结构版本失败: %w", err)
	}
	if !exists {
		return SchemaState{}, nil
	}
	var v int32
	if err := db.Raw(`SELECT COALESCE(MAX(version), 0) FROM sys_schema_versions`).Scan(&v).Error; err != nil {
		return SchemaState{Version: 0, Known: true}, fmt.Errorf("读取结构版本失败: %w", err)
	}
	return SchemaState{Version: v, Known: true}, nil
}

// CheckSchemaVersion 在 AutoMigrate **之前**把关。
//
// 顺序很重要：库比程序新时，AutoMigrate 本身可能做出错误判断（例如把新加的
// 多值列当成缺列去改类型），必须在它动手前就停下来。
func CheckSchemaVersion(db *gorm.DB) (SchemaState, error) {
	st, err := ReadSchemaVersion(db)
	if err != nil {
		return st, err
	}
	if err := compareSchemaVersion(st.Version, CurrentSchemaVersion); err != nil {
		return st, err
	}
	return st, nil
}

// compareSchemaVersion 只比较方向；抽成纯函数便于测试。
func compareSchemaVersion(dbVersion, binVersion int32) error {
	if dbVersion > binVersion {
		return fmt.Errorf(
			"数据库结构版本 v%d 高于当前程序支持的 v%d：请升级程序，或用升级前的备份恢复。"+
				"旧程序连新库会读错字段，这不是「先跑起来再说」的问题",
			dbVersion, binVersion)
	}
	return nil
}

// RecordSchemaVersion 把库版本推进到当前程序版本（幂等）。
//
// 首次运行（版本表刚建、或引入版本表之前的老库）会补上基线行；
// 之后每次 bump 只插入缺的那几行，重复执行不会产生重复记录。
func RecordSchemaVersion(db *gorm.DB) error {
	if err := checkVersionTable(db); err != nil {
		return err
	}
	rows := make([]model.SchemaVersion, 0, 2)
	st, err := ReadSchemaVersion(db)
	if err != nil {
		return err
	}
	for v := st.Version + 1; v <= CurrentSchemaVersion; v++ {
		note, ok := schemaChanges[v]
		if !ok {
			note = fmt.Sprintf("v%d（未登记说明）", v)
		}
		rows = append(rows, model.SchemaVersion{Version: v, AppliedAt: time.Now(), Note: note})
	}
	if len(rows) == 0 {
		return nil
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return fmt.Errorf("记录结构版本失败: %w", err)
	}
	return nil
}

// checkVersionTable 确认版本表存在（AutoMigrate 之后才可能写入）。
func checkVersionTable(db *gorm.DB) error {
	ok := db.Migrator().HasTable(&model.SchemaVersion{})
	if !ok {
		return errors.New("表 sys_schema_versions 不存在，无法记录结构版本")
	}
	return nil
}
