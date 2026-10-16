package config

import (
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
