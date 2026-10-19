package ops

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/config"
	"nova-erp/internal/logger"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"

	"go.uber.org/zap"
)

// 升级包（zip）内部结构约定：
//
//	manifest.json            必填，描述版本与文件清单
//	nova-erp.exe             后端二进制（名字由 manifest.backend.file 指定）
//	dist.zip                 前端产物压缩包（可选，名字由 manifest.frontend.file 指定）
//
// 为什么用 manifest 而不是约定死文件名：不同平台的二进制名不同
// （.exe / 无后缀），而「这个包里到底有没有前端」也必须能表达 ——
// 只更后端的补丁包很常见，硬要它带一份前端只会让包变大、让升级变慢。
type upgradeManifest struct {
	// Version 是升级包的目标发布版本号，如 1.3.0。
	Version string `json:"version"`
	// MinSchemaVersion 是升级包要求的最低结构版本：低于它说明库太旧，
	// 必须先升到中间版本，否则新程序的结构补丁会跑在错误的基线上。
	MinSchemaVersion int32 `json:"minSchemaVersion"`
	// Notes 是给人看的升级说明，会带进升级记录。
	Notes string `json:"notes"`

	Backend  *upgradeFile `json:"backend"`
	Frontend *upgradeFile `json:"frontend"`
}

// upgradeFile 是包内一个文件的清单项：文件名 + 摘要。
type upgradeFile struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// UpgradeAction 是升级计划里的一个动作。
//
// 拆成动作而不是「一个大方法」：升级要在执行前把「将要发生什么」摊给人看，
// 执行后还要逐项回报结果。藏在一个方法里，前端只能显示一个转圈的按钮。
type UpgradeAction struct {
	Key    string `json:"key"`    // backend / frontend
	Title  string `json:"title"`  // 动作名
	Detail string `json:"detail"` // 具体会动哪些路径
	Done   bool   `json:"done"`   // 执行后是否完成
	Error  string `json:"error"`  // 单项失败原因
}

// UpgradePlan 是一次升级的完整计划（生成后落库，执行时按它逐项做）。
type UpgradePlan struct {
	PlanID        string          `json:"planId"`
	FromVersion   string          `json:"fromVersion"`
	ToVersion     string          `json:"toVersion"`
	MinSchemaVer  int32           `json:"minSchemaVersion"`
	PackageSHA256 string          `json:"packageSha256"`
	PackageSize   int64           `json:"packageSize"`
	Notes         string          `json:"notes"`
	Actions       []UpgradeAction `json:"actions"`
	Warnings      []string        `json:"warnings"`
}

// UpgradePackageResult 是上传升级包的结果。
type UpgradePackageResult struct {
	Plan   *UpgradePlan         `json:"plan"`
	Record *model.UpgradeRecord `json:"record"`
}

// StagingRoot 返回升级包暂存根目录：<可执行文件目录>/upgrade/staging。
//
// 放在可执行文件旁边而不是系统临时目录：升级是跨重启的动作，
// 计划生成后要等运维点「执行」，期间系统重启 / 清理临时目录都会让它丢掉。
func StagingRoot(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Upgrade.StagingDir) != "" {
		return cfg.Upgrade.StagingDir
	}
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join("upgrade", "staging")
	}
	return filepath.Join(filepath.Dir(exe), "upgrade", "staging")
}

// ParseUpgradePackage 校验并解压升级包，生成升级计划。
//
// 三步：校验 zip 结构 → 逐项比对 sha256 → 落 staging 并建记录。
// 摘要校验放在解压之前算（读内存里的字节），因为「包在传输中半截」
// 与「包被人改过」必须区分开：前者是网络问题重传即可，后者要停下来查。
func ParseUpgradePackage(db *gorm.DB, cfg *config.Config, data []byte, fromVersion string) (*UpgradePackageResult, error) {
	if len(data) == 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "升级包为空")
	}
	sum := sha256.Sum256(data)
	pkgSHA := hex.EncodeToString(sum[:])

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errcode.Wrapf(errcode.ErrBadRequest, err, "升级包不是有效的 zip")
	}

	pkg, err := openPackage(zr)
	if err != nil {
		return nil, errcode.New(errcode.ErrBadRequest, err.Error())
	}
	raw := pkg.manifest
	if raw == nil {
		return nil, errcode.New(errcode.ErrBadRequest, "升级包缺少 manifest.json")
	}
	var mf upgradeManifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, errcode.Wrapf(errcode.ErrBadRequest, err, "manifest.json 解析失败")
	}
	if err := validateManifest(&mf); err != nil {
		return nil, errcode.New(errcode.ErrBadRequest, err.Error())
	}
	if mf.Version == fromVersion {
		return nil, errcode.Newf(errcode.ErrBadRequest, "升级包版本 %s 与当前运行版本相同，无需升级", mf.Version)
	}

	// 计划 ID 先算出来：它既是 staging 目录名，也要回给前端，
	// 后面的「升级前备份」「执行升级」都按它定位。
	planID := newPlanID(pkgSHA, mf.Version)

	// 同一份包（同 sha256）若已有一个待执行计划，拒绝重复上传 ——
	// 否则「传一次 → 传一次」会堆出一串指向同一份内容的 pending 计划，
	// staging 目录也随之增生。已失败（failed）的包可重新上传，不受此限制。
	var dup model.UpgradeRecord
	if err := db.Where("package_sha256 = ? AND status = ?", pkgSHA, model.UpgradePending).
		First(&dup).Error; err == nil {
		return nil, errcode.Newf(errcode.ErrConflict, "该升级包已有待执行计划 %s，请先执行或等待，无需重复上传", dup.PlanID)
	}

	plan := &UpgradePlan{
		PlanID:        planID,
		FromVersion:   fromVersion,
		ToVersion:     mf.Version,
		MinSchemaVer:  mf.MinSchemaVersion,
		PackageSHA256: pkgSHA,
		PackageSize:   int64(len(data)),
		Notes:         strings.TrimSpace(mf.Notes),
		Actions:       make([]UpgradeAction, 0, 2),
	}

	// 逐项校验摘要，并把文件写进 staging。
	staging := filepath.Join(StagingRoot(cfg), planID)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, fmt.Errorf("创建升级暂存目录失败：%w", err)
	}
	// 占位之后任何一步失败都要把 staging 目录整个删掉：失败重传一次就多一个目录，
	// 里面躺着半截文件，只会让人分不清「哪次上传是完整的」。
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()
	// manifest 也要落一份到 staging：执行阶段（可能是另一次进程、重启之后）
	// 只认 staging 里的文件，不认调用方传进来的字节 —— 上传时校验过的内容
	// 与执行时用的内容必须是同一份，否则「校验通过」与「实际执行」会脱节。
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), raw, 0o644); err != nil {
		return nil, fmt.Errorf("写入 manifest 失败：%w", err)
	}
	if mf.Backend != nil {
		zf, ok := pkg.files[mf.Backend.File]
		if !ok {
			return nil, errcode.Newf(errcode.ErrBadRequest, "manifest 声明的后端文件 %s 不在包里", mf.Backend.File)
		}
		got, err := writeZipEntry(zf, filepath.Join(staging, mf.Backend.File), 0o755)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(got, mf.Backend.SHA256) {
			_ = os.Remove(filepath.Join(staging, mf.Backend.File))
			return nil, errcode.Newf(errcode.ErrBadRequest, "后端文件 %s 摘要不匹配（manifest 写 %s，实际 %s）", mf.Backend.File, mf.Backend.SHA256, got)
		}
		plan.Actions = append(plan.Actions, UpgradeAction{
			Key:    "backend",
			Title:  "替换后端程序",
			Detail: fmt.Sprintf("%s → %s（新程序落位为 .new，需重启生效）", mf.Backend.File, currentExePath()),
		})
	}
	if mf.Frontend != nil {
		zf, ok := pkg.files[mf.Frontend.File]
		if !ok {
			return nil, errcode.Newf(errcode.ErrBadRequest, "manifest 声明的前端文件 %s 不在包里", mf.Frontend.File)
		}
		got, err := writeZipEntry(zf, filepath.Join(staging, mf.Frontend.File), 0o755)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(got, mf.Frontend.SHA256) {
			_ = os.Remove(filepath.Join(staging, mf.Frontend.File))
			return nil, errcode.Newf(errcode.ErrBadRequest, "前端文件 %s 摘要不匹配（manifest 写 %s，实际 %s）", mf.Frontend.File, mf.Frontend.SHA256, got)
		}
		dir := strings.TrimSpace(cfg.Upgrade.FrontendDir)
		if dir == "" {
			plan.Warnings = append(plan.Warnings, "未配置 upgrade.frontend_dir，本次只升级后端；前端产物需手工替换")
		} else {
			plan.Actions = append(plan.Actions, UpgradeAction{
				Key:    "frontend",
				Title:  "替换前端产物",
				Detail: fmt.Sprintf("%s → %s（原子替换，旧目录保留为 .old.<时间戳>）", mf.Frontend.File, dir),
			})
		}
	}
	if len(plan.Actions) == 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "升级包里既没有后端文件也没有前端文件，无可执行动作")
	}

	// 结构版本门槛：库比包要求的最低版本还旧，说明跳版本了。
	if dbVer, err := readSchemaVersion(db); err == nil && dbVer > 0 && dbVer < mf.MinSchemaVersion {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"当前库结构版本 v%d 低于升级包要求的 v%d：请先升级到中间版本", dbVer, mf.MinSchemaVersion))
	}

	rec := &model.UpgradeRecord{
		PlanID:        planID,
		FromVersion:   plan.FromVersion,
		ToVersion:     plan.ToVersion,
		PackageSHA256: plan.PackageSHA256,
		PackageSize:   plan.PackageSize,
		Status:        model.UpgradePending,
		CreatedAt:     time.Now(),
	}
	if err := db.Create(rec).Error; err != nil {
		return nil, fmt.Errorf("登记升级计划失败：%w", err)
	}
	rec.Plan = planJSON(plan)
	if err := db.Model(rec).Update("plan", rec.Plan).Error; err != nil {
		return nil, fmt.Errorf("保存升级计划失败：%w", err)
	}
	committed = true // 记录落库成功，staging 与计划才一起生效
	return &UpgradePackageResult{Plan: plan, Record: rec}, nil
}

// ExecuteUpgradePlan 执行升级计划，返回执行后的记录。
//
// 执行顺序刻意是「后端先落位、前端再替换」：后端落位只是写个 `.new`，
// 不影响正在跑的进程；前端替换会立刻改变用户看到的东西。
// 任何一个动作失败都会中止后续动作，并把已完成的动作留在记录里 ——
// 半途而废的升级最怕的是「不知道停在哪」。
func ExecuteUpgradePlan(db *gorm.DB, cfg *config.Config, planID string) (*model.UpgradeRecord, error) {
	var rec model.UpgradeRecord
	if err := db.Where("plan_id = ?", planID).First(&rec).Error; err != nil {
		return nil, fmt.Errorf("升级计划不存在：%w", err)
	}

	// 抢占：只有 pending 能进入执行，且必须是原子的条件更新。
	//
	// 先读状态再判断再写回的写法在并发下会双双通过（TOCTOU）：
	// 双击「执行升级」或客户端超时重试时，两个请求各执行一遍动作 ——
	// 后端 .new 写两次无害，但前端目录会被切换两次，第二次就找不到 .new 了。
	claimed := db.Model(&model.UpgradeRecord{}).
		Where("plan_id = ? AND status = ?", planID, model.UpgradePending).
		Update("status", model.UpgradeExecuting)
	if claimed.Error != nil {
		return nil, fmt.Errorf("占用升级计划失败：%w", claimed.Error)
	}
	if claimed.RowsAffected == 0 {
		// 报错里重新查一次当前状态：rec.Status 是最开头读出来的旧值，
		// 并发下另一路已把它改成 executing/applied，用旧值报错只会误导排查。
		cur := rec.Status
		var latest model.UpgradeRecord
		if err := db.Select("status").Where("plan_id = ?", planID).First(&latest).Error; err == nil {
			cur = latest.Status
		}
		return nil, errcode.Newf(errcode.ErrConflict, "该计划不在待执行状态（当前 %s）：不能重复执行", cur)
	}

	staging := filepath.Join(StagingRoot(cfg), planID)
	mf, actions, err := loadPlan(staging, &rec)
	if err != nil {
		// 抢占了却走不下去：必须把状态推进掉，否则这条记录永远卡在 executing。
		_ = db.Model(&model.UpgradeRecord{}).Where("plan_id = ?", planID).Updates(map[string]any{
			"status": model.UpgradeFailed, "message": "执行失败：" + err.Error(),
			"finished_at": time.Now(),
		})
		return nil, err
	}

	failed := ""
	for i := range actions {
		a := &actions[i]
		switch a.Key {
		case "backend":
			if err := stageBackend(staging, mf.Backend); err != nil {
				a.Error = err.Error()
				failed = err.Error()
			}
		case "frontend":
			if err := replaceFrontend(staging, mf.Frontend, cfg.Upgrade.FrontendDir); err != nil {
				a.Error = err.Error()
				failed = err.Error()
			}
		}
		a.Done = a.Error == ""
		if failed != "" {
			break
		}
	}

	now := time.Now()
	status, message := upgradeOutcome(actions, failed)
	updates := map[string]any{"plan": planJSONFromActions(actions), "applied_at": &now,
		"status": status, "message": message}
	if status != model.UpgradeApplied {
		updates["finished_at"] = &now
	}
	if err := db.Model(&model.UpgradeRecord{}).Where("plan_id = ?", planID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新升级记录失败：%w", err)
	}
	if err := db.Where("plan_id = ?", planID).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// upgradeOutcome 决定执行完成后的去向。
//
// 纯前端包是这里的关键分支：前端目录切换是**即时生效**的，不存在「等重启」，
// 所以执行成功即升级完成（success）。若仍按后端包的流程等重启比对版本，
// 二进制永远不会变成目标版本（它本来就不在包里），记录会被永久误标为
// 「未生效」—— 前端明明升好了，状态却说失败。
func upgradeOutcome(actions []UpgradeAction, execErr string) (status, message string) {
	if execErr != "" {
		return model.UpgradeFailed, "执行失败：" + execErr
	}
	for _, a := range actions {
		if a.Key == "backend" {
			return model.UpgradeApplied, "已执行，等待重启生效"
		}
	}
	return model.UpgradeSuccess, "前端产物已切换，即时生效，无需重启"
}

// FinalizeUpgrades 在启动时核对「已执行但未确认」的升级。
//
// 为什么放在启动流程里而不是执行完就算成功：后端二进制落位成 `.new` 后
// 必须重启才生效，而重启之后跑的到底是新二进制还是回滚后的旧二进制，
// 只有启动中的这个进程知道。以当前运行版本与记录的目标版本比对：
//
//   - 一致 → success，升级闭环；
//   - 不一致 → failed，说明换了回去或根本没换，需要人来看。
//
// 没有第三条路。若这里图省事直接标 success，「升级完成」就成了假消息。
func FinalizeUpgrades(db *gorm.DB, currentRelease string) error {
	// executing 也要一起捞：进程在执行中途崩溃 / 被杀时记录会停在 executing，
	// 启动是唯一能发现并收尾的时机 —— 不捞的话这条记录永远「卡死」。
	var recs []model.UpgradeRecord
	if err := db.Where("status IN ?", []string{model.UpgradeApplied, model.UpgradeExecuting}).
		Order("id ASC").Find(&recs).Error; err != nil {
		return fmt.Errorf("读取待确认的升级记录失败：%w", err)
	}
	now := time.Now()
	for i := range recs {
		r := &recs[i]
		var status, msg string
		switch {
		case r.Status == model.UpgradeExecuting:
			status, msg = model.UpgradeFailed,
				"执行中进程中断（崩溃或被停止），请重新上传升级包后再执行"
		case strings.EqualFold(r.ToVersion, currentRelease):
			status, msg = model.UpgradeSuccess, "重启后版本一致，升级完成"
		default:
			msg = fmt.Sprintf("当前运行版本 %s 与升级目标 %s 不一致：升级未生效（回滚或未替换二进制）",
				currentRelease, r.ToVersion)
			status = model.UpgradeFailed
		}
		if err := db.Model(r).Updates(map[string]any{
			"status":      status,
			"message":     msg,
			"finished_at": &now,
		}).Error; err != nil {
			return fmt.Errorf("更新升级记录 %s 失败：%w", r.PlanID, err)
		}
	}
	return nil
}

// ── 内部实现 ──────────────────────────────────────────────

// upgradePackage 是解包后的中间形态。
//
// manifest 很小，读进内存；backend / frontend 可能是几十 MB 的二进制/压缩包，
// **不读进内存**，只保留 zip 条目引用，写 staging 时再流式解压 ——
// 否则上传的 512 MB 上限会让「原始包 + 解压内容」同时驻留内存，峰值逼近 1 GB。
type upgradePackage struct {
	manifest []byte
	files    map[string]*zip.File // 包内普通文件（除 manifest 外），键为文件名
}

// openPackage 遍历 zip 条目：只收一层、拒绝嵌套目录；manifest 读入内存，
// 其余文件登记为 zip.File 供后续流式写出。
func openPackage(zr *zip.Reader) (*upgradePackage, error) {
	pkg := &upgradePackage{files: make(map[string]*zip.File, len(zr.File))}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// 只收一层：升级包不该有嵌套目录，出现就说明打包方式不对，
		// 宁可报错也不要按猜测的路径去解。
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("升级包不允许嵌套目录（发现 %s）", f.Name)
		}
		if name == "" || name == "." {
			continue
		}
		if name == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("读取 %s 失败：%w", f.Name, err)
			}
			body, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, fmt.Errorf("读取 %s 失败：%w", f.Name, err)
			}
			pkg.manifest = body
			continue
		}
		pkg.files[name] = f
	}
	return pkg, nil
}

// writeZipEntry 把 zip 条目流式写到 dest，边写边算 sha256，返回摘要。
//
// 刻意「边写边算」而不是「先算再写」：先算就要先把整个文件读进内存，
// 与 openPackage 的流式目标相悖。调用方拿到摘要后与 manifest 比对，
// 不匹配时由调用方删除已写出的文件。
func writeZipEntry(zf *zip.File, dest string, mode os.FileMode) (string, error) {
	rc, err := zf.Open()
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败：%w", zf.Name, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return "", fmt.Errorf("写出 %s 失败：%w", zf.Name, err)
	}
	defer func() { _ = out.Close() }()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), rc); err != nil {
		return "", fmt.Errorf("解压 %s 失败：%w", zf.Name, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// validateManifest 校验清单必填项与文件名安全性。
func validateManifest(mf *upgradeManifest) error {
	if strings.TrimSpace(mf.Version) == "" {
		return errors.New("manifest.json 缺少 version")
	}
	// planID 形如 upg-{version}-{12位摘要}-{14位时间戳}，总长受 PlanID 列宽（64）约束：
	// 4 + len(version) + 1 + 12 + 1 + 14 = len(version) + 32，故 version 最多 32。
	// 不限长的话，超长 version 会在落库时被截断或撞唯一索引，生成一个对不上的 planID。
	if len(mf.Version) > 32 {
		return errors.New("version 过长（最多 32 个字符）")
	}
	if strings.ContainsAny(mf.Version, `/\`) {
		return errors.New("version 含非法字符")
	}
	if mf.MinSchemaVersion < 0 {
		return errors.New("minSchemaVersion 不能为负")
	}
	check := func(f *upgradeFile, what string) error {
		if f == nil {
			return nil
		}
		if f.File == "" || f.SHA256 == "" {
			return fmt.Errorf("%s 必须同时声明 file 与 sha256", what)
		}
		// 文件名必须是纯 basename：带路径就可能越出 staging 目录写文件。
		if f.File != filepath.Base(f.File) || strings.ContainsAny(f.File, `/\`) || f.File == "." || f.File == ".." {
			return fmt.Errorf("%s 的文件名 %q 非法（只能是纯文件名）", what, f.File)
		}
		return nil
	}
	if err := check(mf.Backend, "backend"); err != nil {
		return err
	}
	if err := check(mf.Frontend, "frontend"); err != nil {
		return err
	}
	if mf.Backend == nil && mf.Frontend == nil {
		return errors.New("manifest.json 至少声明 backend 或 frontend 之一")
	}
	return nil
}

// newPlanID 生成计划标识：版本 + 包摘要前 12 位 + 秒级时间戳。
//
// 刻意含时间戳而不是「版本 + 摘要」：PlanID 是唯一索引，同一份包重传若得到
// 同一个 ID，会撞唯一索引、也没法区分「第一次上传」和「失败后的重传」。
// 时间戳保证每次上传都是独立计划；「同一份包已有 pending 计划」的重复上传
// 由 ParseUpgradePackage 按 package_sha256 兜住（拒绝而非再建一条）。
func newPlanID(pkgSHA, version string) string {
	short := pkgSHA
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("upg-%s-%s-%s", version, short, time.Now().Format("20060102150405"))
}

// currentExePath 返回正在运行的可执行文件路径，仅用于给人看的提示。
func currentExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "<当前程序>"
	}
	return exe
}

// readSchemaVersion 读库结构版本；表不存在时返回 0。
func readSchemaVersion(db *gorm.DB) (int32, error) {
	var exists bool
	if err := db.Raw(`SELECT to_regclass('public.sys_schema_versions') IS NOT NULL`).Scan(&exists).Error; err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var v int32
	if err := db.Raw(`SELECT COALESCE(MAX(version), 0) FROM sys_schema_versions`).Scan(&v).Error; err != nil {
		return 0, err
	}
	return v, nil
}

// loadPlan 从 staging 目录与记录里还原执行计划。
//
// 为什么不直接用记录里的 plan JSON：重启之后进程里的内存计划没了，
// 而 staging 目录里的文件是执行时真正要用的东西。以文件系统为准、
// 记录只用来取版本信息，避免「记录说成功了但文件早被删了」。
func loadPlan(staging string, rec *model.UpgradeRecord) (upgradeManifest, []UpgradeAction, error) {
	raw, err := os.ReadFile(filepath.Join(staging, "manifest.json"))
	if err != nil {
		return upgradeManifest{}, nil, fmt.Errorf("升级包暂存文件缺失（%s）：请重新上传升级包", staging)
	}
	var mf upgradeManifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return upgradeManifest{}, nil, fmt.Errorf("manifest.json 解析失败：%w", err)
	}
	if err := validateManifest(&mf); err != nil {
		return upgradeManifest{}, nil, err
	}
	if !strings.EqualFold(mf.Version, rec.ToVersion) {
		return upgradeManifest{}, nil, fmt.Errorf("升级包版本 %s 与记录中的目标版本 %s 不一致", mf.Version, rec.ToVersion)
	}
	actions := make([]UpgradeAction, 0, 2)
	if mf.Backend != nil {
		actions = append(actions, UpgradeAction{
			Key: "backend", Title: "替换后端程序",
			Detail: fmt.Sprintf("%s → %s", mf.Backend.File, currentExePath()),
		})
	}
	if mf.Frontend != nil {
		actions = append(actions, UpgradeAction{
			Key: "frontend", Title: "替换前端产物",
			Detail: "替换 upgrade.frontend_dir 指向的目录",
		})
	}
	return mf, actions, nil
}

// stageBackend 把新二进制落位为 <exe>.new。
//
// 刻意**不覆盖**正在运行的 exe：Windows 下「替换正在运行的可执行文件」
// 会直接被系统拒绝（ERROR_ACCESS_DENIED），而 Linux 上虽然能 unlink，
// 但运行中的进程仍持有旧 inode —— 两种平台的正确做法都一样：
// 落位成 .new，由重启动作完成替换。这也让回滚变得微不足道：删掉 .new 即可。
func stageBackend(staging string, f *upgradeFile) error {
	if f == nil {
		return errors.New("升级包没有后端文件")
	}
	src := filepath.Join(staging, f.File)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("升级包里的 %s 不存在：%w", f.File, err)
	}
	exe := currentExePath()
	if err := copyFile(src, exe+".new", 0o755); err != nil {
		return fmt.Errorf("落位新程序失败：%w", err)
	}
	return nil
}

// replaceFrontend 原子替换前端产物目录。
//
// 顺序：先解出 `.new` 目录 → 旧目录改名 `.old.<时间戳>` → `.new` 改名到位。
// 不能反过来：先删旧目录再挪新的，中间那一下服务是在提供一个不存在的前端，
// 用户刷新就是白屏。留 `.old` 而不是直接删，是为了回滚只要改个名。
func replaceFrontend(staging string, f *upgradeFile, frontendDir string) error {
	if f == nil {
		return nil
	}
	dir := strings.TrimSpace(frontendDir)
	if dir == "" {
		return errors.New("未配置 upgrade.frontend_dir")
	}
	src := filepath.Join(staging, f.File)
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("前端包 %s 不是有效的 zip：%w", f.File, err)
	}
	defer func() { _ = zr.Close() }()

	newDir := dir + ".new"
	oldDir := fmt.Sprintf("%s.old.%s", dir, time.Now().Format("20060102150405"))
	if err := os.RemoveAll(newDir); err != nil {
		return fmt.Errorf("清理旧的新目录失败：%w", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return fmt.Errorf("创建新前端目录失败：%w", err)
	}
	for _, zf := range zr.File {
		if err := extractZipFile(zf, newDir); err != nil {
			return err
		}
	}
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, oldDir); err != nil {
			return fmt.Errorf("备份旧前端目录失败：%w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查现有前端目录失败：%w", err)
	}
	if err := os.Rename(newDir, dir); err != nil {
		// 改名失败要把旧目录还回去，否则前端目录凭空消失。
		if oldDir != "" {
			_ = os.Rename(oldDir, dir)
		}
		return fmt.Errorf("切换到新前端目录失败：%w", err)
	}
	// 每次升级都留一份 .old 供回滚，但不轮转的话升级几次就会把磁盘吃掉
	// （前端产物动辄数百 MB）。只保留最近 N 份足够回滚。
	pruneOldFrontendDirs(dir, frontendOldKeep)
	return nil
}

// frontendOldKeep 是保留的旧前端目录份数。
const frontendOldKeep = 3

// pruneOldFrontendDirs 清理 <dir>.old.<时间戳> 里较旧的，保留最近 keep 份。
//
// 只删「同目录、同 base 名 + .old. + 时间戳」的目录：名字里的时间戳是
// 20060102150405，字典序即时间序，不需要读目录内容或 mtime。
func pruneOldFrontendDirs(dir string, keep int) {
	parent := filepath.Dir(dir)
	base := filepath.Base(dir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	prefix := base + ".old."
	var olds []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			olds = append(olds, filepath.Join(parent, e.Name()))
		}
	}
	if len(olds) <= keep {
		return
	}
	sort.Strings(olds)
	for _, p := range olds[:len(olds)-keep] {
		if err := os.RemoveAll(p); err != nil {
			logger.Warn("清理旧前端目录失败", zap.String("path", p), zap.Error(err))
		}
	}
}

// extractZipFile 解压单个文件到目标目录，带路径穿越防护。
func extractZipFile(zf *zip.File, destDir string) error {
	name := filepath.ToSlash(filepath.Clean(zf.Name))
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
		return fmt.Errorf("前端包含非法路径 %s", zf.Name)
	}
	target := filepath.Join(destDir, filepath.FromSlash(name))
	if zf.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%w", zf.Name, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("写出 %s 失败：%w", zf.Name, err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("解压 %s 失败：%w", zf.Name, err)
	}
	return nil
}

// copyFile 复制文件并设置权限。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// planJSON 序列化动作清单。
func planJSON(plan *UpgradePlan) string {
	return planJSONFromActions(plan.Actions)
}

func planJSONFromActions(actions []UpgradeAction) string {
	b, err := json.Marshal(actions)
	if err != nil {
		return "[]"
	}
	return string(b)
}
