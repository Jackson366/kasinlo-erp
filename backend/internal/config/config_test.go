package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDSNStatementTimeout 验证 statement_timeout 只在配置为正数时才写进连接串。
//
// 这一点必须成立：pgx 会把连接串里未识别的键当作运行时参数下发给**每一条**新连接，
// 所以拼错或漏拼都不会报错，只会静默地不生效（或把参数发成非法值导致连接失败）。
func TestDSNStatementTimeout(t *testing.T) {
	base := DatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "u", DBName: "d", TimeZone: "Asia/Shanghai"}

	with := base
	with.StatementTimeout = 5000
	if dsn := with.DSN(); !strings.Contains(dsn, "statement_timeout=5000") {
		t.Fatalf("配置了 statement_timeout 却未写进 DSN: %s", dsn)
	}

	if dsn := base.DSN(); strings.Contains(dsn, "statement_timeout") {
		t.Fatalf("未配置 statement_timeout 却写进了 DSN: %s", dsn)
	}
}

// TestApplyDefaultsStatementTimeout 验证默认值兜底：未配置时单条 SQL 不得超过 30s。
func TestApplyDefaultsStatementTimeout(t *testing.T) {
	var c Config
	applyDefaults(&c)
	if c.Database.StatementTimeout != 30000 {
		t.Fatalf("statement_timeout 默认值应为 30000，实际 %d", c.Database.StatementTimeout)
	}
}

// TestMetricsDisabledByDefault 验证监控端点默认关闭 —— 它暴露运行信息，
// 不能像 swagger 那样随运行模式自动放开。
func TestMetricsDisabledByDefault(t *testing.T) {
	var a AppConfig
	if a.MetricsEnabled() {
		t.Fatal("/metrics 默认应为关闭")
	}
	on := true
	a.Metrics = &on
	if !a.MetricsEnabled() {
		t.Fatal("显式开启后 /metrics 应为启用")
	}
}

// TestValidateUpgradeDirsRejectsSelfDestructivePaths 钉住「升级会把目录整个替换」这个前提：
// frontend_dir 指向工作目录、文件系统根、或可执行文件所在目录（及其祖先）时，
// 升级的「改名旧目录」一步就会把程序自己脚下的东西搬走。这类配置错误
// 必须在**启动时**拦下，而不是等某次升级把系统搞挂。
func TestValidateUpgradeDirsRejectsSelfDestructivePaths(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录失败: %v", err)
	}
	exeDir, err := executableDir()
	if err != nil {
		t.Skipf("取可执行文件目录失败，跳过: %v", err)
	}

	bad := []struct {
		name string
		dir  string
	}{
		{"工作目录字面量", "."},
		{"工作目录绝对路径", wd},
		{"可执行文件所在目录", exeDir},
		{"文件系统根", string(filepath.Separator)},
	}
	for _, c := range bad {
		var cfg Config
		cfg.Upgrade.FrontendDir = c.dir
		if err := cfg.validateUpgradeDirs(); err == nil {
			t.Fatalf("frontend_dir = %q（%s）应被拒绝", c.dir, c.name)
		}
	}

	// 祖先目录：exeDir 的上级被替换，同样会带走可执行文件
	var cfg Config
	cfg.Upgrade.FrontendDir = filepath.Dir(exeDir)
	if err := cfg.validateUpgradeDirs(); err == nil {
		t.Fatalf("frontend_dir = %q（可执行文件目录的祖先）应被拒绝", filepath.Dir(exeDir))
	}
}

// 留空 = 只升级后端，是合法配置，不能因为加了校验就把这条路径堵死。
func TestValidateUpgradeDirsAllowsEmptyAndNormalDir(t *testing.T) {
	var emptyCfg Config
	if err := emptyCfg.validateUpgradeDirs(); err != nil {
		t.Fatalf("frontend_dir 留空应合法: %v", err)
	}
	cfg := Config{}
	cfg.Upgrade.FrontendDir = filepath.Join(t.TempDir(), "dist")
	if err := cfg.validateUpgradeDirs(); err != nil {
		t.Fatalf("正常的产物目录应合法: %v", err)
	}
}

// 暂存目录同样不能是根：解压到盘符根会把整个盘的根目录弄脏，且清理时极危险。
func TestValidateUpgradeStagingDirRejectsRoot(t *testing.T) {
	cfg := Config{}
	cfg.Upgrade.StagingDir = string(filepath.Separator)
	if err := cfg.validateUpgradeDirs(); err == nil {
		t.Fatal("staging_dir 为文件系统根应被拒绝")
	}
	cfg.Upgrade.StagingDir = filepath.Join(t.TempDir(), "staging")
	if err := cfg.validateUpgradeDirs(); err != nil {
		t.Fatalf("正常的暂存目录应合法: %v", err)
	}
}

func TestIsRootPath(t *testing.T) {
	if !isRootPath(string(filepath.Separator)) {
		t.Fatal("`/` 应判定为根")
	}
	vol := filepath.VolumeName(mustAbs(t, "."))
	if vol != "" {
		if !isRootPath(vol + string(filepath.Separator)) {
			t.Fatalf("%s 应判定为根", vol+string(filepath.Separator))
		}
	}
	if isRootPath(filepath.Join(t.TempDir(), "dist")) {
		t.Fatal("普通目录不应判定为根")
	}
}

func TestIsAncestorOf(t *testing.T) {
	root := string(filepath.Separator)
	if !isAncestorOf(root, filepath.Join(root, "a", "b")) {
		t.Fatal("根应是任意路径的祖先")
	}
	if isAncestorOf(filepath.Join(root, "a"), filepath.Join(root, "b")) {
		t.Fatal("兄弟目录不构成祖先关系")
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("Abs(%q) 失败: %v", p, err)
	}
	return abs
}
