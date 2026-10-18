// Command ops 是 nova-erp 的运维命令：备份、恢复、数据清理、体检。
//
// 为什么是独立命令而不是脚本：
//   - 连接参数、库名、口令只在 config.yaml 里维护一份，脚本不用再抄一遍；
//   - Windows 与 Linux 行为一致（本项目实际部署在 Windows 机器上，
//     .sh + Git Bash 是最容易被漏装的一环）；
//   - 该命令**不跑迁移、不写种子数据** —— 备份的时候顺手改表结构是最不该发生的事。
//
// 所有子命令默认都站在「安全侧」：cleanup 默认 dry-run；restore 必须 --yes
// 且默认恢复到新库；backup 写完立刻回读归档，确认文件真能恢复。
//
// 用法：
//
//	nova-ops backup  --config config.yaml
//	nova-ops restore --file backups/nova_inv_v5-20261018-220000.dump --yes
//	nova-ops cleanup --config config.yaml            # 只统计，不删
//	nova-ops cleanup --config config.yaml --apply    # 真删
//	nova-ops check   --config config.yaml --strict   # 有告警时退出码非 0
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/ops"
)

// bizAnchorTables 是备份/巡检/恢复的锚点业务表。
//
// nova-ops 是进销存系统的运维命令，知道这些表；框架层 internal/ops 不硬编码
// 业务表名，由本命令与 HTTP 模块（module/ops）各自传入。两边清单必须保持一致，
// 否则同一份能力在 CLI 与页面上覆盖的业务表不同（已有教训：CLI 曾退化到只锚定 sys_users）。
var bizAnchorTables = []string{
	"inv_orders", "inv_order_items", "inv_products", "inv_customers",
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "backup":
		err = runBackup(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	case "cleanup":
		err = runCleanup(os.Args[2:])
	case "reset":
		err = runReset(os.Args[2:])
	case "check":
		err = runCheck(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `nova-ops —— nova-erp 运维命令

用法：nova-ops <backup|restore|cleanup|check> [选项]

  backup   全量备份（pg_dump 自定义格式）+ 校验 + 按保留策略轮转
           --config config.yaml   --dir backups   --keep 30   --keep-months 6
           --no-prune             本次不删旧备份

  restore  把备份恢复到新库（默认 <db>_restore_<时间戳>），恢复后核对表数与行数
           --file 备份文件         --yes（必填）
           --db 目标库名            --force（允许指向生产库，危险）

  cleanup  按保留期清理日志与过期幂等键；默认 dry-run，加 --apply 才真删
           --op-log-days 180   --login-log-days 180   --idempotency-days 7
           --batch 5000

  reset   业务数据清理（不可逆，硬删）。范围：inv_documents / inv_all / database
           --scope inv_documents   --confirm 确认清理单据   --reset-stock
           要求 24 小时内有备份（--force 可跳过，慎用）

  check    体检：结构版本 / 数据库体积 / 日志与软删除堆积 / 备份新鲜度
           --json                  --max-backup-age 48（小时）
           --strict                有告警时退出码 1（给监控用）

所有子命令都支持 --config（默认 config.yaml）。
`)
}

// newFlags 建一个带公共参数的 FlagSet。
func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	cfgPath := fs.String("config", "config.yaml", "配置文件路径")
	return fs, cfgPath
}

// loadConfig 解析配置。
func loadConfig(path string) (*config.Config, error) {
	return config.Load(path)
}

// openDB 连接数据库（不迁移、不播种）。
func openDB(cfg *config.Config) error {
	if _, err := database.Init(cfg.Database); err != nil {
		return err
	}
	return nil
}

func runBackup(args []string) error {
	fs, cfgPath := newFlags("backup")
	dir := fs.String("dir", "", "备份输出目录（默认取配置 backup.dir）")
	keep := fs.Int("keep", 0, "保留最近多少份（默认取配置 backup.keep_count）")
	keepMonths := fs.Int("keep-months", 0, "每月至少保留一份，覆盖最近多少个月")
	dumpPath := fs.String("pg-dump", "", "pg_dump 路径（默认自动探测）")
	noPrune := fs.Bool("no-prune", false, "本次不清理旧备份")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	// 命令行优先，未给则用配置默认（0 / 空串表示「没给」）
	opt := ops.BackupOptions{
		Dir:          firstNonEmpty(*dir, cfg.Backup.Dir),
		KeepCount:    firstPositive(*keep, cfg.Backup.KeepCount),
		KeepMonths:   firstPositive(*keepMonths, cfg.Backup.KeepMonths),
		PgDumpPath:   firstNonEmpty(*dumpPath, cfg.Backup.PgDumpPath),
		AnchorTables: append([]string{"sys_users"}, bizAnchorTables...),
		SkipPrune:    *noPrune,
	}

	if err := openDB(cfg); err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	res, err := ops.Backup(database.DB(), cfg.Database, opt)
	if err != nil {
		return err
	}

	fmt.Printf("✓ 备份完成 %s\n", res.Path)
	fmt.Printf("  大小 %s · 耗时 %s · sha256 %s\n",
		humanSize(res.Size), res.Duration.Round(time.Millisecond), shortSum(res.SHA256))
	fmt.Printf("  数据库 %s（PostgreSQL %s）· %d 张表 · %s\n",
		cfg.Database.DBName, res.Stats.ServerVersion, res.Stats.Tables, rowSummary(res.Stats.Rows))
	if len(res.Removed) > 0 {
		fmt.Printf("  已清理 %d 份过期备份\n", len(res.Removed))
	}
	return nil
}

func runRestore(args []string) error {
	fs, cfgPath := newFlags("restore")
	file := fs.String("file", "", "备份文件路径（.dump）")
	target := fs.String("db", "", "目标库名（默认自动生成 <db>_restore_<时间戳>）")
	yes := fs.Bool("yes", false, "确认执行（必填）")
	force := fs.Bool("force", false, "允许目标库指向生产库（危险）")
	restore := fs.String("pg-restore", "", "pg_restore 路径（默认自动探测）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("必须用 --file 指定备份文件")
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}

	res, err := ops.Restore(cfg, ops.RestoreOptions{
		File:          *file,
		Target:        *target,
		Yes:           *yes,
		Force:         *force,
		PgRestorePath: firstNonEmpty(*restore, cfg.Backup.PgRestorePath),
		AnchorTables:  append([]string{"sys_users"}, bizAnchorTables...),
	})
	if err != nil {
		return err
	}

	fmt.Printf("✓ 已恢复到 %s（耗时 %s）\n", res.Target, res.Duration.Round(time.Millisecond))
	fmt.Printf("  %d 张表 · 关键表行数 %s\n", res.Tables, rowSummary(res.Rows))
	if len(res.Warn) > 0 {
		fmt.Println("  需要留意：")
		for _, w := range res.Warn {
			fmt.Printf("    ! %s\n", w)
		}
	}
	fmt.Printf("  确认无误后可删除该库：DROP DATABASE \"%s\";\n", res.Target)
	return nil
}

func runCleanup(args []string) error {
	fs, cfgPath := newFlags("cleanup")
	apply := fs.Bool("apply", false, "真正执行删除（默认只统计）")
	opDays := fs.Int("op-log-days", 0, "操作日志保留天数")
	lgDays := fs.Int("login-log-days", 0, "登录日志保留天数")
	idDays := fs.Int("idempotency-days", 0, "幂等键过期后再保留天数")
	batch := fs.Int("batch", 5000, "每批删除行数")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}

	if err := openDB(cfg); err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	rep, err := ops.Cleanup(database.DB(), ops.CleanupOptions{
		OpLogDays:        firstPositive(*opDays, cfg.Maintenance.OpLogDays),
		LoginLogDays:     firstPositive(*lgDays, cfg.Maintenance.LoginLogDays),
		IdempotencyDays:  firstPositive(*idDays, cfg.Maintenance.IdempotencyDays),
		LoginLockSeconds: cfg.Security.LoginLockSeconds,
		BatchSize:        *batch,
		Apply:            *apply,
	})
	if err != nil {
		return err
	}

	if rep.DryRun {
		fmt.Println("清理模式：预演（不会删除任何数据）")
	} else {
		fmt.Println("清理模式：已执行")
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "对象\t表\t删除边界\t行数\t批次\t说明")
	for _, s := range rep.Steps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Name, s.Table, s.Cutoff.Format("2006-01-02"), humanCount(s.Rows), batchText(s.Batches), s.Note)
	}
	_ = w.Flush()
	if rep.DryRun {
		fmt.Println("确认无误后加 --apply 执行删除。")
	}
	if len(rep.Vacuumed) > 0 {
		fmt.Printf("已 ANALYZE：%v\n", rep.Vacuumed)
	}
	return nil
}

// runReset 业务数据清理。与 cleanup 的区别：这里是整表硬删、不可逆，
// 因此多两道闸门 —— 24 小时内必须有备份，且必须输入该范围专属的确认口令。
func runReset(args []string) error {
	fs, cfgPath := newFlags("reset")
	scope := fs.String("scope", "", "清理范围：inv_documents / inv_all / database")
	confirm := fs.String("confirm", "", "确认口令（必须与该范围完全一致）")
	resetStock := fs.Bool("reset-stock", false, "同时清零规格库存（仅 inv_documents）")
	apply := fs.Bool("apply", false, "真正执行（默认只预演）")
	force := fs.Bool("force", false, "跳过「24 小时内有备份」这道闸门（危险）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scope == "" {
		return fmt.Errorf("必须用 --scope 指定清理范围（inv_documents / inv_all / database）")
	}
	sc, err := ops.FindBizCleanupScope(*scope)
	if err != nil {
		return err
	}
	if *apply && *confirm != sc.Confirm {
		return fmt.Errorf("确认口令不正确：%s 需要输入「%s」", sc.Title, sc.Confirm)
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	if err := openDB(cfg); err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	if *apply && !*force {
		dir := cfg.Backup.Dir
		if dir == "" {
			dir = "backups"
		}
		var latest time.Time
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".dump") {
					continue
				}
				if info, err := e.Info(); err == nil && info.ModTime().After(latest) {
					latest = info.ModTime()
				}
			}
		}
		if ok, hint := ops.BizCleanupGate(latest, time.Now(), 24*time.Hour); !ok {
			return errors.New(hint + "；确认要继续请加 --force")
		}
	}

	rep, err := ops.BizCleanup(database.DB(), ops.BizCleanupOptions{
		Scope:      *scope,
		ResetStock: *resetStock,
		Apply:      *apply,
	})
	if err != nil {
		return err
	}

	if rep.DryRun {
		fmt.Printf("预演模式：%s（不会删除任何数据）\n", rep.Title)
	} else {
		fmt.Printf("已执行：%s\n", rep.Title)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "表\t清理前行数\t删除行数\t序列重置")
	for _, t := range rep.Tables {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, humanCount(t.Rows), humanCount(t.Deleted), yesNo(t.SeqReset))
	}
	_ = w.Flush()
	fmt.Printf("合计删除 %s 行\n", humanCount(rep.TotalDeleted))
	if rep.StockReset {
		fmt.Printf("已清零规格库存 %s 行\n", humanCount(rep.StockResetRows))
	}
	if rep.DryRun {
		fmt.Println("确认无误后加 --apply 执行。")
	}
	return nil
}

func runCheck(args []string) error {
	fs, cfgPath := newFlags("check")
	asJSON := fs.Bool("json", false, "以 JSON 输出（给监控用）")
	maxAge := fs.Int("max-backup-age", 48, "备份多久未更新算告警（小时）")
	strict := fs.Bool("strict", false, "有告警时退出码 1")
	backupDir := fs.String("backup-dir", "", "备份目录（默认取配置 backup.dir）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}

	if err := openDB(cfg); err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	rep, warns, err := ops.Check(database.DB(), cfg, ops.CheckOptions{
		BackupDir:    firstNonEmpty(*backupDir, cfg.Backup.Dir),
		MaxBackupAge: time.Duration(*maxAge) * time.Hour,
		KeyTables:    append([]string{"sys_users"}, bizAnchorTables...),
	})
	if err != nil {
		return err
	}

	if *asJSON {
		buf, _ := json.MarshalIndent(map[string]any{"report": rep, "warnings": warns}, "", "  ")
		fmt.Println(string(buf))
	} else {
		printCheck(rep, warns)
	}
	if *strict && len(warns) > 0 {
		os.Exit(1)
	}
	return nil
}

func printCheck(rep *ops.CheckReport, warns []string) {
	fmt.Printf("数据库 %s（PostgreSQL %s）· %s · %d 张表\n",
		rep.Database, rep.ServerVersion, humanSize(rep.SizeBytes), rep.Tables)
	if rep.SchemaKnown {
		fmt.Printf("结构版本：库 v%d / 程序 v%d\n", rep.SchemaDB, rep.SchemaBinary)
	} else {
		fmt.Printf("结构版本：库未登记（老库首次启动会自动登记）/ 程序 v%d\n", rep.SchemaBinary)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "表\t存活行\t软删除\t体积")
	for _, t := range rep.KeyTables {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, humanCount(t.Rows), humanCount(t.SoftDeleted), humanSize(t.SizeBytes))
	}
	_ = w.Flush()

	fmt.Printf("操作日志 %s 行（最早 %s）· 登录日志 %s 行 · 幂等键 %s 行（已过期 %s）\n",
		humanCount(rep.OpLogRows), orDash(rep.OpLogOldest),
		humanCount(rep.LoginLogRows), humanCount(rep.IdemRows), humanCount(rep.IdemExpired))
	if rep.LatestBackup == "" {
		fmt.Println("最近备份：无")
	} else {
		fmt.Printf("最近备份：%s（%s 前）\n", rep.LatestBackup,
			time.Since(rep.LatestBackupA).Round(time.Minute))
	}

	if len(warns) == 0 {
		fmt.Println("✓ 未发现需要处理的问题")
		return
	}
	fmt.Println("需要处理：")
	for _, x := range warns {
		fmt.Printf("  ! %s\n", x)
	}
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	for _, u := range units {
		v /= unit
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, u)
		}
	}
	return fmt.Sprintf("%.1f PB", v/unit)
}

func humanCount(n int64) string {
	if n < 10000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

func shortSum(s string) string {
	if len(s) > 16 {
		return s[:16] + "…"
	}
	return s
}

func rowSummary(rows map[string]int64) string {
	if len(rows) == 0 {
		return "-"
	}
	out := ""
	for _, k := range []string{"sys_users", "inv_products", "inv_orders", "inv_order_items"} {
		v, ok := rows[k]
		if !ok {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += fmt.Sprintf("%s %s", k, humanCount(v))
	}
	return out
}

func batchText(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstPositive(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

func yesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}
