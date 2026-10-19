package ops

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"nova-erp/internal/config"
)

// dbNamePattern 限定目标库名。库名要拼进 `CREATE DATABASE "x"` 的标识符位置，
// 而标识符不能用占位符传参 —— 唯一可靠的做法是**先严格校验再用**。只允许
// 字母数字下划线，长度对齐 PostgreSQL 的 63 字节上限。
var dbNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// RestoreOptions 是恢复参数。
type RestoreOptions struct {
	File          string
	Target        string // 为空则自动生成 <db>_restore_<时间戳>
	Yes           bool   // 必须显式确认：恢复是覆盖性操作
	Force         bool   // 允许把目标指向生产库本身
	PgRestorePath string
	// AnchorTables 是恢复后核对的锚点表清单（与备份时的 AnchorTables 对齐）。
	// 缺省只锚定框架表 sys_users；业务表由应用层（module/ops、nova-ops）传入。
	AnchorTables []string
	Now          time.Time
}

// RestoreResult 是恢复结果。
type RestoreResult struct {
	Target   string
	Tables   int64
	Rows     map[string]int64
	Duration time.Duration
	// Warn 是需要人看一眼但不影响成功的事实（例如备份时与恢复后的行数不一致）
	Warn []string
}

// Restore 把备份恢复到一个**新库**。
//
// 三条刻意的安全设计：
//
//  1. **默认不落到生产库**。目标库名自动生成 `<db>_restore_<时间戳>`：
//     恢复的主要用途是「验备份能不能用」和「找回误删的几行」，两者都不该
//     直接盖掉正在跑的库。要覆盖生产库必须同时给 --force 且手工改配置。
//  2. **目标库必须不存在**。已存在就报错而不是 DROP —— 一条命令删掉一个库
//     这种能力，不该藏在运维脚本的默认行为里。
//  3. **先校验摘要再动手**。dump 文件在传输/拷贝中最容易半截，
//     sha256 对不上时 pg_restore 会写出一个「部分成功」的库，比直接失败更难查。
func Restore(cfg *config.Config, opt RestoreOptions) (*RestoreResult, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if !opt.Yes {
		return nil, fmt.Errorf("恢复会写入一个数据库，请确认后加 --yes 再执行")
	}
	if len(opt.AnchorTables) == 0 {
		opt.AnchorTables = []string{"sys_users"}
	}
	file, err := filepath.Abs(opt.File)
	if err != nil {
		return nil, fmt.Errorf("解析备份文件路径失败: %w", err)
	}
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("备份文件不可用: %s: %w", file, err)
	}
	if err := verifySHA256(file); err != nil {
		return nil, err
	}

	target := opt.Target
	if target == "" {
		target = fmt.Sprintf("%s_restore_%s", cfg.Database.DBName, opt.Now.Format("20060102_150405"))
	}
	if !dbNamePattern.MatchString(target) {
		return nil, fmt.Errorf("目标库名非法: %q（只允许字母数字下划线，且以字母或下划线开头）", target)
	}
	if target == cfg.Database.DBName && !opt.Force {
		return nil, fmt.Errorf("目标库 %s 就是生产库，拒绝覆盖；确需覆盖请加 --force（并先停服务）", target)
	}

	restorePath, err := locateTool("pg_restore", opt.PgRestorePath)
	if err != nil {
		return nil, err
	}

	// 1) 建库（连 postgres 维护库）
	if err := createDatabase(cfg.Database, target); err != nil {
		return nil, err
	}

	// 2) 恢复
	started := time.Now()
	cmd := exec.Command(restorePath,
		"--no-owner",
		"--no-privileges",
		"--exit-on-error",
		"--dbname="+target,
		"--host="+cfg.Database.Host,
		fmt.Sprintf("--port=%d", cfg.Database.Port),
		"--username="+cfg.Database.User,
		file,
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.Database.Password)
	if cfg.Database.SSLMode != "" && cfg.Database.SSLMode != "disable" {
		cmd.Env = append(cmd.Env, "PGSSLMODE="+cfg.Database.SSLMode)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pg_restore 失败（目标库 %s 已创建，可保留排查）: %w\n%s", target, err, strings.TrimSpace(string(out)))
	}
	duration := time.Since(started)

	// 3) 核对：表数与锚点行数
	tables, rows, err := queryCounts(cfg.Database, target, opt.AnchorTables)
	if err != nil {
		return nil, err
	}
	res := &RestoreResult{Target: target, Tables: tables, Rows: rows, Duration: duration}

	if meta, err := readMeta(file + ".json"); err == nil {
		if meta.Stats.Tables > 0 && meta.Stats.Tables != tables {
			res.Warn = append(res.Warn,
				fmt.Sprintf("表数不一致：备份时 %d 张，恢复后 %d 张", meta.Stats.Tables, tables))
		}
		for name, want := range meta.Stats.Rows {
			if got := rows[name]; got != want {
				res.Warn = append(res.Warn,
					fmt.Sprintf("%s 行数不一致：备份时 %d，恢复后 %d", name, want, got))
			}
		}
	}
	return res, nil
}

// createDatabase 在维护库里建目标库；已存在则报错（不自动删除）。
func createDatabase(dbc config.DatabaseConfig, name string) error {
	admin, err := sql.Open("pgx", dbc.AdminDSN())
	if err != nil {
		return fmt.Errorf("连接维护库失败: %w", err)
	}
	defer func() { _ = admin.Close() }()

	var exists bool
	if err := admin.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		return fmt.Errorf("检查目标库是否存在失败: %w", err)
	}
	if exists {
		return fmt.Errorf("目标库 %s 已存在：请换一个库名，或先手工确认后删除它（本命令不会自动删库）", name)
	}
	// name 已通过 dbNamePattern 校验，可以安全地作为标识符拼进语句
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		return fmt.Errorf("创建目标库 %s 失败: %w", name, err)
	}
	return nil
}

// queryCounts 统计恢复后的表数与锚点行数。
func queryCounts(dbc config.DatabaseConfig, dbName string, anchorTables []string) (int64, map[string]int64, error) {
	clone := dbc
	clone.DBName = dbName
	conn, err := sql.Open("pgx", clone.DSN())
	if err != nil {
		return 0, nil, fmt.Errorf("连接目标库失败: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var tables int64
	if err := conn.QueryRow(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`).Scan(&tables); err != nil {
		return 0, nil, fmt.Errorf("统计表数量失败: %w", err)
	}
	rows := map[string]int64{}
	for _, t := range anchorTables {
		if !isValidTableName(t) {
			rows[t] = -1
			continue
		}
		var n int64
		if err := conn.QueryRow("SELECT count(*) FROM " + t).Scan(&n); err != nil {
			// 老备份里可能没有某张表：记 -1 表示缺失，而不是让整个恢复判定为失败
			rows[t] = -1
			continue
		}
		rows[t] = n
	}
	return tables, rows, nil
}

// verifySHA256 对同目录的 .sha256 校验文件；没有校验文件时只告警（由调用方打印）。
func verifySHA256(file string) error {
	raw, err := os.ReadFile(file + ".sha256")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取校验文件失败: %w", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return fmt.Errorf("校验文件为空: %s.sha256", file)
	}
	got, _, err := fileSHA256(file)
	if err != nil {
		return err
	}
	if !strings.EqualFold(fields[0], got) {
		return fmt.Errorf("备份文件校验失败：sha256 不匹配（文件可能损坏或被截断），期望 %s 实际 %s", fields[0], got)
	}
	return nil
}

// readMeta 读取备份元数据；没有就返回错误，由调用方决定是否忽略。
func readMeta(path string) (BackupMeta, error) {
	var meta BackupMeta
	raw, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}
