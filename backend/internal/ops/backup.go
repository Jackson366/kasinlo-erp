package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/config"
	"nova-erp/internal/logger"

	"go.uber.org/zap"
)

// backupFileSuffix 是备份文件后缀。用 pg_dump 的自定义格式（-Fc）而不是纯 SQL：
//
//   - 自带压缩，几十 MB 的库落成几 MB；
//   - 能**选择性恢复**（只恢复某张表 / 只恢复数据），SQL 文本做不到；
//   - 恢复时 pg_restore 会并行建索引，比 psql 逐条执行快。
//
// 代价是「不能用文本编辑器打开看一眼」，但对这类系统没人会去读 dump，
// 需要看内容时用 `pg_restore --list`。
const backupFileSuffix = ".dump"

// BackupOptions 是备份参数；零值字段由调用方用配置默认值补齐。
type BackupOptions struct {
	Dir        string
	KeepCount  int
	KeepMonths int
	PgDumpPath string
	// AnchorTables 是恢复后用来「对得上账」的锚点表清单（几张关键表的行数）。
	// 缺省只锚定框架表 sys_users —— 框架层不 import 业务模块，业务表清单
	// 由应用层（module/ops、nova-ops）传入。
	AnchorTables []string
	// SkipPrune 为 true 时不做轮转（首次接入、或手工备份时不想删旧的）。
	SkipPrune bool
	Now       time.Time
}

// BackupStats 是备份时顺带采集的「当时库里有什么」，恢复后用来比对。
//
// 只采表数与几张关键表的行数，不做全库行数统计：全量 count(*) 在大表上会把
// 备份时间拖长，而「对得上账」只需要几个锚点。
type BackupStats struct {
	Tables        int64            `json:"tables"`
	ServerVersion string           `json:"serverVersion"`
	Rows          map[string]int64 `json:"rows"`
}

// BackupMeta 写在 `<file>.json` 里，与 dump 同名同目录。
//
// 单独存一份元数据而不是只靠日志：半年后要判断「这份备份能不能用」时，
// 日志早就轮转没了，元数据还在。
type BackupMeta struct {
	Database string      `json:"database"`
	File     string      `json:"file"`
	Started  time.Time   `json:"startedAt"`
	Duration string      `json:"duration"`
	Size     int64       `json:"size"`
	SHA256   string      `json:"sha256"`
	Stats    BackupStats `json:"stats"`
	// Verified 表示写完是否用 pg_restore --list 回读过。false 时这份备份
	// 「没坏，但也没验过」—— 通常是缺 pg_restore 的那类精简安装。
	Verified bool `json:"verified"`
}

// BackupResult 是备份结果。
type BackupResult struct {
	Path     string
	Size     int64
	SHA256   string
	Duration time.Duration
	Stats    BackupStats
	Verified bool
	Removed  []string
	Meta     BackupMeta
}

// Backup 执行一次全量备份，并按保留策略轮转旧文件。
func Backup(db *gorm.DB, dbc config.DatabaseConfig, opt BackupOptions) (*BackupResult, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.Dir == "" {
		opt.Dir = "backups"
	}
	if len(opt.AnchorTables) == 0 {
		opt.AnchorTables = []string{"sys_users"}
	}
	dir, err := filepath.Abs(opt.Dir)
	if err != nil {
		return nil, fmt.Errorf("解析备份目录失败: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %s: %w", dir, err)
	}
	dumpPath, err := locateTool("pg_dump", opt.PgDumpPath)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s-%s%s", dbc.DBName, opt.Now.Format("20060102-150405"), backupFileSuffix)
	target := filepath.Join(dir, name)
	// 文件名只到秒，两次备份落在同一秒就会写同一个 target：
	// 两个 pg_dump 交替截断同一个文件，最终留下的是一份**看起来存在、
	// 恢复时才发现是坏的**备份 —— 比没有备份更危险（没人会去补做）。
	// 用 O_EXCL 原子占位：抢不到就明确报错，而不是默默覆盖。
	if err := reserveBackupFile(target); err != nil {
		return nil, err
	}
	// 占位之后任何一步失败，都要把这个空壳删掉：一个 0 字节的 .dump
	// 会照常出现在备份列表里，看着像「备份过」，真要恢复时才发现是空的。
	// 半截文件同理 —— 比没有文件更危险。
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(target)
		}
	}()

	stats, err := collectStats(db, opt.AnchorTables)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	// 口令走环境变量而不是命令行参数：命令行在 ps / 任务管理器里对同机所有用户可见，
	// 备份是最容易被顺手跑的一条命令，不该把口令顺手泄出去。
	cmd := exec.Command(dumpPath,
		"--format=custom",
		"--compress=6",
		"--no-owner",
		"--no-privileges",
		"--file="+target,
		"--host="+dbc.Host,
		fmt.Sprintf("--port=%d", dbc.Port),
		"--username="+dbc.User,
		"--dbname="+dbc.DBName,
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+dbc.Password)
	if dbc.SSLMode != "" && dbc.SSLMode != "disable" {
		cmd.Env = append(cmd.Env, "PGSSLMODE="+dbc.SSLMode)
	}
	out, err := cmd.CombinedOutput()
	duration := time.Since(started)
	if err != nil {
		// 空壳文件由上面的 defer 统一清理
		return nil, fmt.Errorf("pg_dump 失败: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	// 立刻回读一遍归档清单：dump 进程退出码为 0 不等于文件可恢复
	// （磁盘写满时可能截断），而「备份是坏的」要到真出事那天才发现。
	verified, err := verifyArchive(dumpPath, target, opt.PgDumpPath)
	if err != nil {
		return nil, err
	}

	sum, size, err := fileSHA256(target)
	if err != nil {
		return nil, err
	}
	meta := BackupMeta{
		Database: dbc.DBName,
		File:     name,
		Started:  opt.Now,
		Duration: duration.Round(time.Millisecond).String(),
		Size:     size,
		SHA256:   sum,
		Stats:    stats,
		Verified: verified,
	}
	if err := writeSidecars(target, meta); err != nil {
		return nil, err
	}
	committed = true // dump、校验、摘要、元数据都齐了，这份备份才算成立

	res := &BackupResult{
		Path: target, Size: size, SHA256: sum, Duration: duration, Stats: stats,
		Verified: verified, Meta: meta,
	}
	if !opt.SkipPrune {
		removed, err := pruneBackups(dir, opt)
		if err != nil {
			// 轮转失败不该让备份本身算作失败：文件已经落好了，告警让人去清
			res.Removed = nil
			return res, fmt.Errorf("备份已完成，但清理旧备份失败: %w", err)
		}
		res.Removed = removed
	}
	return res, nil
}

// reserveBackupFile 以 O_EXCL 原子占位备份文件。
//
// 选「占位 + 报错」而不是「自动改名 / 等一秒重试」：
// 备份是低频人工动作，撞名意味着有人在并发触发（页面连点、CLI 与服务同时跑），
// 这种时候让后一次**失败并说清楚**比悄悄产出第二份文件更可控 ——
// 后者会让保留策略里的「最近 N 份」失去意义。
func reserveBackupFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("备份文件 %s 已存在：同一秒内已有备份在生成，请稍后重试", filepath.Base(path))
		}
		return fmt.Errorf("创建备份文件失败: %s: %w", path, err)
	}
	return f.Close()
}

// collectStats 采集恢复后用于比对的锚点。
func collectStats(db *gorm.DB, anchorTables []string) (BackupStats, error) {
	stats := BackupStats{Rows: map[string]int64{}}
	if err := db.Raw("SELECT current_setting('server_version')").Scan(&stats.ServerVersion).Error; err != nil {
		return stats, fmt.Errorf("读取数据库版本失败: %w", err)
	}
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`).Scan(&stats.Tables).Error; err != nil {
		return stats, fmt.Errorf("统计表数量失败: %w", err)
	}
	// 锚点表由调用方传入：一张系统表 + 若干业务表（单据主表、明细表、主数据表）。
	// 覆盖多类数据，任何一类丢了都能从计数上看出来。
	for _, t := range anchorTables {
		var n int64
		if err := db.Raw("SELECT count(*) FROM " + t).Scan(&n).Error; err != nil {
			return stats, fmt.Errorf("统计 %s 行数失败: %w", t, err)
		}
		stats.Rows[t] = n
	}
	return stats, nil
}

// verifyArchive 用 pg_restore --list 回读归档目录，确认文件结构完整。
//
// 返回 (是否真的校验了, 错误)。「找不到 pg_restore」不是错误（不该卡住备份），
// 但要通过日志与元数据的 verified=false 让用户知道「这次没验」——
// 静默放行会让「没验过」的备份看起来和「验过」的一样可信。
func verifyArchive(pgDumpPath, file, configuredRestore string) (bool, error) {
	restorePath, err := locateTool("pg_restore", configuredRestore)
	if err != nil {
		// 只有 pg_dump 没有 pg_restore 的安装（极少见）不该卡住备份，
		// 但要明确告诉用户「这次没有校验」。
		logger.Warn("本次备份未做归档校验（找不到 pg_restore）",
			zap.String("file", filepath.Base(file)))
		return false, nil
	}
	_ = pgDumpPath
	out, err := exec.Command(restorePath, "--list", file).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("备份文件校验失败（归档不可读）: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return false, fmt.Errorf("备份文件校验失败：归档清单为空，文件可能被截断")
	}
	return true, nil
}

// writeSidecars 写 sha256 与元数据。
func writeSidecars(target string, meta BackupMeta) error {
	if err := os.WriteFile(target+".sha256", []byte(fmt.Sprintf("%s  %s\n", meta.SHA256, filepath.Base(target))), 0o640); err != nil {
		return fmt.Errorf("写校验文件失败: %w", err)
	}
	buf, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化备份元数据失败: %w", err)
	}
	if err := os.WriteFile(target+".json", append(buf, '\n'), 0o640); err != nil {
		return fmt.Errorf("写备份元数据失败: %w", err)
	}
	return nil
}

// pruneBackups 按保留策略删除旧备份（连同 .sha256 / .json 一起删）。
func pruneBackups(dir string, opt BackupOptions) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]BackupFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), backupFileSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		files = append(files, BackupFile{
			Path: path,
			Time: BackupTimeFromName(e.Name(), info.ModTime()),
			Size: info.Size(),
		})
	}
	victims := PlanRetention(files, opt.Now, opt.KeepCount, opt.KeepMonths)
	removed := make([]string, 0, len(victims))
	for _, v := range victims {
		for _, p := range []string{v.Path, v.Path + ".sha256", v.Path + ".json"} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return removed, fmt.Errorf("删除旧备份失败 %s: %w", p, err)
			}
		}
		removed = append(removed, v.Path)
	}
	return removed, nil
}

// backupTimeRe 从 <库名>-20060102150405.dump 里提取时间戳。
// 用「最后一个 - 后面的 14 位数字 + .dump 结尾」定位：库名里无论有没有横线、
// 下划线都不影响 —— 时间戳永远在文件名末尾、紧贴 .dump 之前。
var backupTimeRe = regexp.MustCompile(`-(\d{14})\.dump$`)

// BackupTimeFromName 从备份文件名解析时间戳；解析不到就回落到 mtime。
//
// 导出给应用层（module/ops）复用：备份时间的解析规则只该有一份。
// 之前 CLI（internal/ops 用 Split）与列表（module/ops 的自有正则）各写一遍，
// 而 Split 版本对「下划线库名」（如 nova_inv_v5）会解析失败、静默回落到 mtime
// —— 同一份备份在两边显示不同时间。统一收敛到正则这一份。
func BackupTimeFromName(name string, fallback time.Time) time.Time {
	m := backupTimeRe.FindStringSubmatch(name)
	if len(m) != 2 {
		return fallback
	}
	if t, err := time.ParseInLocation("20060102150405", m[1], time.Local); err == nil {
		return t
	}
	return fallback
}

// fileSHA256 计算文件摘要与大小。
func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("计算备份摘要失败: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
