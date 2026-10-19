package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/config"
	"nova-erp/internal/database"
)

// CheckOptions 是巡检参数。
type CheckOptions struct {
	BackupDir string
	// MaxBackupAge 备份「多久没更新」算异常，默认 48h（允许一天一次的备份
	// 偶尔晚几个小时，但连续两天没备份必须报出来）。
	MaxBackupAge time.Duration
	// KeyTables 是巡检要逐张统计的关键表清单。缺省只巡检框架表 sys_users ——
	// 框架层（本包）不 import 业务模块，也不该硬编码业务表名，
	// 业务表清单由应用层（module/ops、nova-ops）传入。
	KeyTables []string
	Now       time.Time
}

// TableStat 是单表统计。
type TableStat struct {
	Name        string `json:"name"`
	Rows        int64  `json:"rows"`
	SoftDeleted int64  `json:"softDeleted"`
	SizeBytes   int64  `json:"sizeBytes"`
}

// CheckReport 是巡检结果。字段都带 json tag，`--json` 直接输出给监控用。
type CheckReport struct {
	Database      string      `json:"database"`
	ServerVersion string      `json:"serverVersion"`
	SizeBytes     int64       `json:"sizeBytes"`
	Tables        int64       `json:"tables"`
	SchemaDB      int32       `json:"schemaVersionDb"`
	SchemaBinary  int32       `json:"schemaVersionBinary"`
	SchemaKnown   bool        `json:"schemaVersionKnown"`
	KeyTables     []TableStat `json:"keyTables"`
	OpLogRows     int64       `json:"opLogRows"`
	OpLogOldest   string      `json:"opLogOldest"`
	LoginLogRows  int64       `json:"loginLogRows"`
	IdemRows      int64       `json:"idempotencyRows"`
	IdemExpired   int64       `json:"idempotencyExpired"`
	LatestBackup  string      `json:"latestBackup"`
	LatestBackupA time.Time   `json:"latestBackupAt"`
	CheckedAt     time.Time   `json:"checkedAt"`
}

// Check 做一次「升级前 / 日常巡检」体检，返回报告与告警清单。
//
// 体检项刻意都在「出问题会疼」的地方，不做泛泛的统计：
// 结构版本（回滚风险）、备份新鲜度（真出事能不能救回来）、
// 日志膨胀（磁盘与查询变慢的源头）、软删除堆积（主数据是否被悄悄删掉）。
func Check(db *gorm.DB, cfg *config.Config, opt CheckOptions) (*CheckReport, []string, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.MaxBackupAge <= 0 {
		opt.MaxBackupAge = 48 * time.Hour
	}
	if opt.BackupDir == "" {
		opt.BackupDir = cfg.Backup.Dir
	}
	if len(opt.KeyTables) == 0 {
		opt.KeyTables = []string{"sys_users"}
	}

	rep := &CheckReport{Database: cfg.Database.DBName, CheckedAt: opt.Now}
	if err := db.Raw("SELECT current_setting('server_version')").Scan(&rep.ServerVersion).Error; err != nil {
		return nil, nil, fmt.Errorf("读取数据库版本失败: %w", err)
	}
	if err := db.Raw("SELECT pg_database_size(current_database())").Scan(&rep.SizeBytes).Error; err != nil {
		return nil, nil, fmt.Errorf("读取数据库体积失败: %w", err)
	}
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`).Scan(&rep.Tables).Error; err != nil {
		return nil, nil, fmt.Errorf("统计表数量失败: %w", err)
	}

	st, err := database.ReadSchemaVersion(db)
	if err != nil {
		return nil, nil, err
	}
	rep.SchemaDB, rep.SchemaKnown = st.Version, st.Known
	rep.SchemaBinary, _ = database.Current()

	for _, t := range opt.KeyTables {
		stat := TableStat{Name: t}
		if !isValidTableName(t) {
			// 非法表名不拼进 SQL
			stat.Rows = -1
			stat.SoftDeleted = -1
			rep.KeyTables = append(rep.KeyTables, stat)
			continue
		}
		// Rows 只数存活行：把软删除也算进去，「存活 163 / 软删除 161」这种
		// 数字会同时误导两件事 —— 以为数据还在用，以及看不出到底删了多少。
		if err := db.Raw("SELECT count(*) FROM " + t + " WHERE deleted_at IS NULL").Scan(&stat.Rows).Error; err != nil {
			// 表不存在（老库/精简部署）：记 -1 表示缺失，不让整次体检失败。
			// 体检的意义是「把所有该看的都摊出来」，缺一张表不构成失败。
			stat.Rows = -1
			stat.SoftDeleted = -1
			rep.KeyTables = append(rep.KeyTables, stat)
			continue
		}
		// 软删除计数对业务表同样重要：它们看不见却还在库里，
		// 会让人误判「数据怎么越用越多」。
		_ = db.Raw("SELECT count(*) FROM " + t + " WHERE deleted_at IS NOT NULL").Scan(&stat.SoftDeleted).Error
		_ = db.Raw(`SELECT COALESCE(pg_total_relation_size(?), 0)`, t).Scan(&stat.SizeBytes).Error
		rep.KeyTables = append(rep.KeyTables, stat)
	}

	if err := db.Raw("SELECT count(*) FROM sys_operation_logs").Scan(&rep.OpLogRows).Error; err != nil {
		return nil, nil, fmt.Errorf("统计操作日志失败: %w", err)
	}
	var oldest *time.Time
	if err := db.Raw("SELECT min(created_at) FROM sys_operation_logs").Scan(&oldest).Error; err == nil && oldest != nil {
		rep.OpLogOldest = oldest.Format("2006-01-02")
	}
	if err := db.Raw("SELECT count(*) FROM sys_login_logs").Scan(&rep.LoginLogRows).Error; err != nil {
		return nil, nil, fmt.Errorf("统计登录日志失败: %w", err)
	}
	if err := db.Raw("SELECT count(*) FROM sys_idempotencies").Scan(&rep.IdemRows).Error; err != nil {
		return nil, nil, fmt.Errorf("统计幂等键失败: %w", err)
	}
	_ = db.Raw("SELECT count(*) FROM sys_idempotencies WHERE expires_at < ?", opt.Now).Scan(&rep.IdemExpired).Error

	if latest, ok := latestBackup(opt.BackupDir); ok {
		// 只回文件名，不回绝对路径：巡检报告经 ops:list 权限即可见，
		// 绝对路径会把服务器的目录结构整个抖出去，而文件名已经足够判断「有没有备份」。
		rep.LatestBackup = filepath.Base(latest.Path)
		rep.LatestBackupA = latest.Time
	}

	return rep, checkWarnings(rep, opt), nil
}

// checkWarnings 把报告翻译成「该动手了」的句子。
//
// 告警要能直接指导动作，不写「数据量偏大」这种没法执行的话：
// 每条都点明对象、数字与建议做法。
func checkWarnings(rep *CheckReport, opt CheckOptions) []string {
	var warns []string

	if rep.SchemaDB > rep.SchemaBinary {
		warns = append(warns, fmt.Sprintf(
			"数据库结构版本 v%d 高于程序支持的 v%d：旧程序不能连新库，请升级程序或恢复备份", rep.SchemaDB, rep.SchemaBinary))
	}
	if rep.LatestBackup == "" {
		warns = append(warns, "备份目录里没有任何备份：先跑一次 nova-ops backup，并配好每日计划任务")
	} else if age := opt.Now.Sub(rep.LatestBackupA); age > opt.MaxBackupAge {
		warns = append(warns, fmt.Sprintf(
			"最近一份备份是 %s（%.0f 小时前），已超过 %.0f 小时的阈值：检查计划任务是否还在跑",
			rep.LatestBackupA.Format("2006-01-02 15:04"), age.Hours(), opt.MaxBackupAge.Hours()))
	}
	if rep.OpLogRows > 1_000_000 {
		warns = append(warns, fmt.Sprintf(
			"操作日志 %d 行，已超过 100 万：跑 nova-ops cleanup --apply 按保留期清理", rep.OpLogRows))
	}
	if rep.IdemExpired > 1000 {
		warns = append(warns, fmt.Sprintf(
			"过期幂等键 %d 条：跑 nova-ops cleanup --apply 清掉（它们已不可能被回放）", rep.IdemExpired))
	}
	for _, t := range rep.KeyTables {
		if t.Rows > 0 && t.SoftDeleted*5 > t.Rows && t.SoftDeleted > 100 {
			warns = append(warns, fmt.Sprintf(
				"%s 软删除 %d 行 / 存活 %d 行：已删数据占比过高，确认后归档", t.Name, t.SoftDeleted, t.Rows))
		}
	}
	return warns
}

// latestBackup 找备份目录里最新的一份 .dump。
func latestBackup(dir string) (BackupFile, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return BackupFile{}, false
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return BackupFile{}, false
	}
	var best BackupFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), backupFileSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f := BackupFile{
			Path: filepath.Join(abs, e.Name()),
			Time: BackupTimeFromName(e.Name(), info.ModTime()),
			Size: info.Size(),
		}
		if best.Path == "" || f.Time.After(best.Time) {
			best = f
		}
	}
	return best, best.Path != ""
}
