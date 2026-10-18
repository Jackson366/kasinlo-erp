// Package ops 是「系统运维」模块：数据备份、数据清理、系统巡检、系统升级。
//
// 它不实现任何运维能力本身 —— 那些在 internal/ops（框架层）里，
// 由独立命令 nova-ops 与本模块共用。本模块只做三件事：
// 把能力接成 HTTP 接口、按权限收口、把结果整理成前端能直接渲染的形态。
//
// 为什么界面里也要有一套：运维动作不能只在运维人员的命令行里。
// 「今晚要不要升个级」这种判断往往是不懂命令行的人做的，
// 而升级前备份、升级后核对这两步恰恰最不能省。
package ops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/logger"
	"nova-erp/internal/model"
	fwops "nova-erp/internal/ops"

	"go.uber.org/zap"
)

// 真删 / 真升级的确认口令。故意是中文短句而不是 YES：
// 命令行里 YES 是惯例，但界面里要的是「看清楚自己在确认什么」。
const (
	confirmCleanup = "确认清理"
	confirmUpgrade = "确认升级"
)

// bizAnchorTables 是巡检与备份锚点关心的业务表。
//
// 放在应用层（本模块）而不是框架层（internal/ops）：框架层不 import 业务模块，
// 也不该硬编码业务表名 —— 那是「业务模块永远删不掉」的另一种形式。
// 本模块知道这是一个进销存系统，由它把清单喂给框架层的 Check / Backup。
var bizAnchorTables = []string{
	"inv_orders", "inv_order_items", "inv_products", "inv_customers",
}

// Service 运维服务。
type Service struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewService 创建运维服务。db 由调用方传入，便于测试时换库。
func NewService(cfg *config.Config, db *gorm.DB) *Service {
	return &Service{cfg: cfg, db: db}
}

// ── 备份 ────────────────────────────────────────────────

// ListBackups 列出备份目录里的全部备份，新的在前。
func (s *Service) ListBackups() ([]BackupItem, error) {
	dir := s.backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupItem{}, nil
		}
		return nil, fmt.Errorf("读取备份目录失败：%w", err)
	}
	items := make([]BackupItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".dump") {
			continue
		}
		item, err := s.backupItem(dir, e.Name())
		if err != nil {
			continue // 单个文件读不到不该让整个列表失败
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Time.After(items[j].Time) })
	return items, nil
}

// backupItem 读一个备份文件与它的元数据。
func (s *Service) backupItem(dir, name string) (BackupItem, error) {
	full := filepath.Join(dir, name)
	info, err := os.Stat(full)
	if err != nil {
		return BackupItem{}, err
	}
	item := BackupItem{
		Name: name,
		Path: full,
		Size: info.Size(),
		// 时间取文件名里的时间戳，取不到才回落到 mtime —— 拷贝过的备份
		// mtime 会变成拷贝时间，而名字里的时间戳才是「这个备份是哪一刻的库」。
		// 解析规则统一走框架层的 BackupTimeFromName，CLI 与列表只有一份实现。
		Time: fwops.BackupTimeFromName(name, info.ModTime()),
	}
	if meta, err := readBackupMeta(full + ".json"); err == nil {
		item.HasMeta = true
		item.SHA256 = meta.SHA256
		item.Database = meta.Database
		item.ServerVersion = meta.Stats.ServerVersion
		item.Tables = meta.Stats.Tables
		item.Verified = meta.Verified
	}
	return item, nil
}

// RunBackup 执行一次备份。
func (s *Service) RunBackup(req RunBackupReq) (*BackupItem, error) {
	res, err := fwops.Backup(s.db, s.cfg.Database, fwops.BackupOptions{
		Dir:          s.backupDir(),
		KeepCount:    s.cfg.Backup.KeepCount,
		KeepMonths:   s.cfg.Backup.KeepMonths,
		PgDumpPath:   s.cfg.Backup.PgDumpPath,
		AnchorTables: append([]string{"sys_users"}, bizAnchorTables...),
		SkipPrune:    req.SkipPrune,
	})
	if err != nil {
		return nil, err
	}
	item, err := s.backupItem(s.backupDir(), filepath.Base(res.Path))
	if err != nil {
		return nil, err
	}
	item.SHA256 = res.SHA256
	return &item, nil
}

// DeleteBackup 删除一个备份（连同它的 .sha256 与 .json）。
func (s *Service) DeleteBackup(file string) error {
	name, err := s.resolveBackupFile(file)
	if err != nil {
		return err
	}
	dir := s.backupDir()
	for _, suffix := range []string{"", ".sha256", ".json"} {
		if err := os.Remove(filepath.Join(dir, name+suffix)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 %s 失败：%w", name+suffix, err)
		}
	}
	return nil
}

// BackupPath 返回备份文件的绝对路径，供下载用。
func (s *Service) BackupPath(file string) (string, error) {
	name, err := s.resolveBackupFile(file)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.backupDir(), name), nil
}

// resolveBackupFile 校验文件名：必须是纯文件名、以 .dump 结尾、且真的在备份目录里。
func (s *Service) resolveBackupFile(file string) (string, error) {
	name := filepath.Base(strings.TrimSpace(file))
	if name == "" || name == "." || name == ".." || name != strings.TrimSpace(file) {
		return "", errors.New("备份文件名不合法")
	}
	if !strings.HasSuffix(name, ".dump") {
		return "", errors.New("只允许操作 .dump 备份文件")
	}
	full := filepath.Join(s.backupDir(), name)
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("备份文件不存在：%s", name)
	}
	return name, nil
}

func (s *Service) backupDir() string {
	dir := strings.TrimSpace(s.cfg.Backup.Dir)
	if dir == "" {
		dir = "backups"
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// ── 恢复 ────────────────────────────────────────────────

// Restore 把备份恢复到一个新库（恢复演练 / 找回误删数据）。
//
// 接口层面**不提供**覆盖生产库的能力：命令行还要 --force 双确认，
// 而 HTTP 接口一旦放开，一次误点就是生产库被盖掉。要覆盖生产库，
// 请停机后用 nova-ops restore --force 执行。
func (s *Service) Restore(req RestoreReq) (*fwops.RestoreResult, error) {
	file, err := s.BackupPath(req.File)
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(req.Target)
	if target != "" && strings.EqualFold(target, s.cfg.Database.DBName) {
		return nil, errors.New("不允许把备份恢复到生产库；如需覆盖生产库，请停机后用 nova-ops restore --force")
	}
	return fwops.Restore(s.cfg, fwops.RestoreOptions{
		File:          file,
		Target:        target,
		Yes:           true,
		Force:         false,
		PgRestorePath: s.cfg.Backup.PgRestorePath,
	})
}

// ── 清理 ────────────────────────────────────────────────

// CleanupPreview 预演：只统计不删。
func (s *Service) CleanupPreview() (*CleanupView, error) {
	rep, err := fwops.Cleanup(s.db, s.cleanupOptions(0, 0, 0, false))
	if err != nil {
		return nil, err
	}
	return toCleanupView(rep, ""), nil
}

// CleanupApply 真删。确认口令不对时返回带 Blocked 的预演结果，不删任何数据。
func (s *Service) CleanupApply(req CleanupApplyReq) (*CleanupView, error) {
	if strings.TrimSpace(req.Confirm) != confirmCleanup {
		view, err := s.CleanupPreview()
		if err != nil {
			return nil, err
		}
		view.Blocked = fmt.Sprintf("确认口令不正确（需要输入「%s」），已按预演处理，未删除任何数据", confirmCleanup)
		return view, nil
	}
	rep, err := fwops.Cleanup(s.db, s.cleanupOptions(req.OpLogDays, req.LoginLogDays, req.IdempotencyDays, true))
	if err != nil {
		return nil, err
	}
	return toCleanupView(rep, ""), nil
}

func (s *Service) cleanupOptions(opDays, loginDays, idemDays int, apply bool) fwops.CleanupOptions {
	// 页面没填的项取配置默认，框架层还会再兜一次（见 fwops.Cleanup 的默认值）。
	// 三层里任意一层漏了都不会退化成「0 天 = 删掉全部」。
	return fwops.CleanupOptions{
		OpLogDays:        firstPositive(opDays, s.cfg.Maintenance.OpLogDays),
		LoginLogDays:     firstPositive(loginDays, s.cfg.Maintenance.LoginLogDays),
		IdempotencyDays:  firstPositive(idemDays, s.cfg.Maintenance.IdempotencyDays),
		LoginLockSeconds: s.cfg.Security.LoginLockSeconds,
		BatchSize:        5000,
		Apply:            apply,
	}
}

// firstPositive 返回 a（>0 时）或 b。0 与负数都视为「没给」。
func firstPositive(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

// ── 巡检 ────────────────────────────────────────────────

// Check 做一次体检。
func (s *Service) Check() (*fwops.CheckReport, []string, error) {
	return fwops.Check(s.db, s.cfg, fwops.CheckOptions{
		BackupDir:    s.backupDir(),
		MaxBackupAge: 48 * time.Hour,
		KeyTables:    append([]string{"sys_users"}, bizAnchorTables...),
	})
}

// ── 升级 ────────────────────────────────────────────────

// UpgradeStatus 汇总升级页需要的一切：版本三元组、预检、升级记录。
func (s *Service) UpgradeStatus() (*UpgradeStatus, error) {
	rep, warns, err := s.Check()
	if err != nil {
		return nil, err
	}
	records, err := s.recentRecords(10)
	if err != nil {
		return nil, err
	}
	binVer, note := database.Current()

	st := &UpgradeStatus{
		Release:        s.cfg.App.Release,
		SchemaDB:       rep.SchemaDB,
		SchemaBinary:   rep.SchemaBinary,
		SchemaKnown:    rep.SchemaKnown,
		SchemaNote:     note,
		LatestBackup:   rep.LatestBackup,
		LatestBackupAt: rep.LatestBackupA,
		Database:       rep.Database,
		ServerVersion:  rep.ServerVersion,
		SizeBytes:      rep.SizeBytes,
		Records:        records,
		Warnings:       warns,
		Preflight: []PreflightItem{
			{
				Key:   "schema",
				Title: "结构版本一致",
				Pass:  rep.SchemaKnown && rep.SchemaDB <= rep.SchemaBinary,
				Hint:  fmt.Sprintf("库 v%d / 程序 v%d", rep.SchemaDB, rep.SchemaBinary),
			},
			{
				Key:   "backup",
				Title: "存在 48 小时内的备份",
				Pass:  !rep.LatestBackupA.IsZero() && time.Since(rep.LatestBackupA) < 48*time.Hour,
				Hint:  backupAgeHint(rep.LatestBackupA),
			},
			{
				Key:   "pending",
				Title: "没有未确认的升级",
				Pass:  !hasPendingUpgrade(records),
				Hint:  "上一次升级必须已是 success 或 failed",
			},
			{
				Key:   "release",
				Title: "程序版本号已注入",
				Pass:  s.cfg.App.Release != "" && s.cfg.App.Release != "dev",
				Hint:  fmt.Sprintf("当前 %s；为 dev 时升级后无法自动核对是否生效", s.cfg.App.Release),
			},
		},
	}
	_ = binVer
	return st, nil
}

// UpgradeRecords 升级记录列表。
func (s *Service) UpgradeRecords() ([]UpgradeRecord, error) {
	return s.recentRecords(50)
}

func (s *Service) recentRecords(limit int) ([]UpgradeRecord, error) {
	var rows []model.UpgradeRecord
	if err := s.db.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取升级记录失败：%w", err)
	}
	out := make([]UpgradeRecord, 0, len(rows))
	for i := range rows {
		out = append(out, *toUpgradeRecord(&rows[i]))
	}
	return out, nil
}

// UploadPackage 上传并校验升级包，生成计划。
func (s *Service) UploadPackage(name string, data []byte) (*fwops.UpgradePackageResult, error) {
	const maxPackageSize = 512 << 20 // 512 MB：一个后端二进制 + 一份前端产物远小于此
	if int64(len(data)) > maxPackageSize {
		return nil, fmt.Errorf("升级包超过 %d MB", maxPackageSize>>20)
	}
	return fwops.ParseUpgradePackage(s.db, s.cfg, data, s.cfg.App.Release)
}

// UpgradeBackup 升级前强制留档备份，并把备份路径记进升级记录。
//
// 「没有当天备份不允许升级」这条不变量就在这里落地：apply 会检查
// record.BackupPath 指向的文件真实存在，绕不过去。
func (s *Service) UpgradeBackup(req UpgradeBackupReq) (*BackupItem, error) {
	var rec model.UpgradeRecord
	if err := s.db.Where("plan_id = ?", req.PlanID).First(&rec).Error; err != nil {
		return nil, errors.New("升级计划不存在")
	}
	if rec.Status != model.UpgradePending {
		return nil, fmt.Errorf("该计划当前状态为「%s」，只有待执行的计划才能留档", upgradeStatusText(rec.Status))
	}
	item, err := s.RunBackup(RunBackupReq{SkipPrune: true, Note: "升级前留档：" + req.PlanID})
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(&rec).Update("backup_path", item.Path).Error; err != nil {
		return nil, fmt.Errorf("记录升级前备份失败：%w", err)
	}
	return item, nil
}

// ApplyUpgrade 执行升级计划。
func (s *Service) ApplyUpgrade(req ApplyUpgradeReq) (*UpgradeRecord, error) {
	if strings.TrimSpace(req.Confirm) != confirmUpgrade {
		return nil, fmt.Errorf("确认口令不正确（需要输入「%s」）", confirmUpgrade)
	}
	// 硬性前置：升级前必须留过档，且那份备份还在。
	var rec model.UpgradeRecord
	if err := s.db.Where("plan_id = ?", req.PlanID).First(&rec).Error; err != nil {
		return nil, errors.New("升级计划不存在")
	}
	if strings.TrimSpace(rec.BackupPath) == "" {
		return nil, errors.New("尚未执行升级前备份：请先点「升级前备份」，没有回滚点不允许升级")
	}
	if _, err := os.Stat(rec.BackupPath); err != nil {
		return nil, fmt.Errorf("升级前备份文件不存在（%s）：请重新备份", rec.BackupPath)
	}
	done, err := fwops.ExecuteUpgradePlan(s.db, s.cfg, req.PlanID)
	if err != nil {
		return nil, err
	}
	return toUpgradeRecord(done), nil
}

// ── 小工具 ──────────────────────────────────────────────

// readBackupMeta 读备份的 .json 元数据。
func readBackupMeta(path string) (fwops.BackupMeta, error) {
	var meta fwops.BackupMeta
	raw, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func toCleanupView(rep *fwops.CleanupReport, blocked string) *CleanupView {
	steps := make([]CleanupStep, 0, len(rep.Steps))
	for _, s := range rep.Steps {
		steps = append(steps, CleanupStep{
			Name: s.Name, Table: s.Table, Cutoff: s.Cutoff,
			Rows: s.Rows, Batches: s.Batches, Note: s.Note,
		})
	}
	return &CleanupView{DryRun: rep.DryRun, Steps: steps, Vacuumed: rep.Vacuumed, Blocked: blocked}
}

func toUpgradeRecord(r *model.UpgradeRecord) *UpgradeRecord {
	return &UpgradeRecord{
		ID: r.ID, PlanID: r.PlanID, FromVersion: r.FromVersion, ToVersion: r.ToVersion,
		PackageSHA256: r.PackageSHA256, PackageSize: r.PackageSize, BackupPath: r.BackupPath,
		Status: r.Status, Actions: parseActions(r.Plan), Message: r.Message,
		CreatedAt: r.CreatedAt, AppliedAt: r.AppliedAt, FinishedAt: r.FinishedAt,
	}
}

func parseActions(planJSON string) []UpgradeAction {
	if strings.TrimSpace(planJSON) == "" {
		return []UpgradeAction{}
	}
	var actions []UpgradeAction
	if err := json.Unmarshal([]byte(planJSON), &actions); err != nil || actions == nil {
		return []UpgradeAction{}
	}
	return actions
}

func hasPendingUpgrade(records []UpgradeRecord) bool {
	for _, r := range records {
		// executing（正在执行）与 applied（已执行未确认）都算未收尾
		if r.Status != model.UpgradeSuccess && r.Status != model.UpgradeFailed {
			return true
		}
	}
	return false
}

// upgradeStatusText 给报错信息用的状态中文名。
func upgradeStatusText(status string) string {
	m := map[string]string{
		model.UpgradePending:   "待执行",
		model.UpgradeExecuting: "执行中",
		model.UpgradeApplied:   "已执行待重启",
		model.UpgradeSuccess:   "已完成",
		model.UpgradeFailed:    "未生效",
	}
	if t, ok := m[status]; ok {
		return t
	}
	return status
}

func backupAgeHint(at time.Time) string {
	if at.IsZero() {
		return "没有找到任何备份"
	}
	d := time.Since(at).Round(time.Hour)
	return fmt.Sprintf("最近一份：%s（%s 前）", at.Format("2006-01-02 15:04"), d)
}

// ── 业务数据清理 ─────────────────────────────────────────

// bizCleanupMaxBackupAge 业务清理要求的回滚点新鲜度。
// 与升级共用一把尺子：24 小时内必须有备份，否则不许动业务数据。
const bizCleanupMaxBackupAge = 24 * time.Hour

// BizScopes 返回全部清理范围及各表当前行数，供界面做预演。
func (s *Service) BizScopes() ([]BizScopeView, error) {
	// 同一张表可能出现在多档里，行数只查一次。
	counts := map[string]int64{}
	count := func(table string) error {
		if _, ok := counts[table]; ok {
			return nil
		}
		var n int64
		if err := s.db.Table(table).Count(&n).Error; err != nil {
			return fmt.Errorf("统计 %s 行数失败：%w", table, err)
		}
		counts[table] = n
		return nil
	}

	out := make([]BizScopeView, 0, 3)
	for _, sc := range fwops.BizCleanupScopes() {
		view := BizScopeView{
			Key: sc.Key, Title: sc.Title, Desc: sc.Desc, Keep: sc.Keep,
			Confirm: sc.Confirm, ResetStockable: sc.ResetStockable,
			Tables: make([]BizTableView, 0, len(sc.Tables)),
		}
		for _, t := range sc.Tables {
			if err := count(t); err != nil {
				return nil, err
			}
			view.Tables = append(view.Tables, BizTableView{Name: t, Rows: counts[t]})
			view.TotalRows += counts[t]
		}
		out = append(out, view)
	}
	return out, nil
}

// BusinessCleanup 执行业务数据清理。
//
// 与日志清理不同，这里是**不可逆**的硬删，所以多两道闸门：
//  1. 24 小时内必须有备份（没有回滚点不许动账）；
//  2. 确认口令必须与该档完全一致（每档口令不同，手滑跳不了档）。
func (s *Service) BusinessCleanup(req BizCleanupReq) (*fwops.BizCleanupReport, error) {
	scope, err := fwops.FindBizCleanupScope(req.Scope)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Confirm) != scope.Confirm {
		return nil, fmt.Errorf("确认口令不正确（「%s」需要输入「%s」）", scope.Title, scope.Confirm)
	}

	// 闸门一：回滚点。取最近一份备份的时间，不拷文件、不校验内容 ——
	// 那份备份在创建时已经回读校验过（见 ops.Backup）。
	items, err := s.ListBackups()
	if err != nil {
		return nil, err
	}
	var latest time.Time
	for _, it := range items {
		if it.Time.After(latest) {
			latest = it.Time
		}
	}
	if ok, hint := fwops.BizCleanupGate(latest, time.Now(), bizCleanupMaxBackupAge); !ok {
		return nil, errors.New(hint)
	}

	rep, err := fwops.BizCleanup(s.db, fwops.BizCleanupOptions{
		Scope:      req.Scope,
		ResetStock: req.ResetStock,
		Apply:      true,
	})
	if err != nil {
		return nil, err
	}
	// 审计：业务清理会连日志一起删掉（database 档），
	// 所以除了操作日志，再往应用日志里写一条摘要 ——
	// 「谁在什么时候清了多少行」必须留得下来。
	logger.Info("业务数据清理完成",
		zap.String("scope", rep.Scope),
		zap.String("title", rep.Title),
		zap.Int64("totalDeleted", rep.TotalDeleted),
		zap.Bool("stockReset", rep.StockReset),
		zap.Int64("stockResetRows", rep.StockResetRows),
	)
	return rep, nil
}
