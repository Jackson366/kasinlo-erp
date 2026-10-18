package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config 是应用的完整配置树。
type Config struct {
	App         AppConfig         `yaml:"app"`
	Database    DatabaseConfig    `yaml:"database"`
	JWT         JWTConfig         `yaml:"jwt"`
	Log         LogConfig         `yaml:"log"`
	CORS        CORSConfig        `yaml:"cors"`
	Security    SecurityConfig    `yaml:"security"`
	Backup      BackupConfig      `yaml:"backup"`
	Maintenance MaintenanceConfig `yaml:"maintenance"`
}

// BackupConfig 是备份/恢复的默认参数，由独立命令 nova-ops 读取。
//
// 放在配置文件里而不是写死在脚本中：连接参数、库名、pg_dump 路径都跟着
// 环境走，运维只需要维护一份 config.yaml，不用在两个地方各改一遍口令。
type BackupConfig struct {
	// Dir 备份输出目录；相对路径按**进程工作目录**解析（与 log.dir 一致）。
	Dir string `yaml:"dir"`
	// KeepCount 保留最近多少份全量备份。
	KeepCount int `yaml:"keep_count"`
	// KeepMonths 另外保证每个月至少留一份，覆盖最近多少个月（月档）。
	KeepMonths int `yaml:"keep_months"`
	// PgDumpPath / PgRestorePath 留空则依次探测 PATH、Windows 安装目录、
	// Linux/macOS 常见包管理路径。
	PgDumpPath    string `yaml:"pg_dump_path"`
	PgRestorePath string `yaml:"pg_restore_path"`
}

// MaintenanceConfig 是数据清理的默认保留期，由 nova-ops cleanup 读取。
//
// 一律按「保留天数」而不是「删多少行」：按行删会随数据量漂移，
// 按时间删的语义是「超过这个年限的日志我不需要了」，一眼可读也能审计。
type MaintenanceConfig struct {
	// OpLogDays 操作日志保留天数，<=0 走默认 180。
	OpLogDays int `yaml:"op_log_days"`
	// LoginLogDays 登录日志保留天数，<=0 走默认 180。
	// 实际删除时还会叠加账号锁定保护窗口，见 ops.Cleanup。
	LoginLogDays int `yaml:"login_log_days"`
	// IdempotencyDays 幂等键**过期之后**再保留多少天，<=0 走默认 7。
	// 留一段缓冲是为了让「过期前一秒提交、过期后才重试」的请求仍能回放结果。
	IdempotencyDays int `yaml:"idempotency_days"`
}

type AppConfig struct {
	Name    string `yaml:"name"`
	Mode    string `yaml:"mode"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Prefix  string `yaml:"prefix"`
	Version string `yaml:"version"`
	// Swagger 控制是否挂载 /swagger 在线接口文档。
	//
	// 默认跟随 mode：debug 打开，release 关闭 —— 接口文档会把全部路由与参数结构
	// 暴露给任何能访问服务的人，生产环境不该默认开着。
	Swagger *bool `yaml:"swagger"`
	// Metrics 控制是否挂载 /metrics 监控端点。
	//
	// 默认**关闭**：该端点会暴露路由表、请求量与状态码分布等运行信息，
	// 属于「内网都未必该公开」的东西，因此不像 swagger 那样按 mode 自动放开，
	// 需要接入 Prometheus 时才显式打开，并建议同时配置 metrics_token。
	Metrics *bool `yaml:"metrics"`
	// MetricsToken 非空时，访问 /metrics 必须带 Authorization: Bearer <token>。
	MetricsToken string `yaml:"metrics_token"`
}

// Addr 返回监听地址。
func (a AppConfig) Addr() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

// BasePath 返回 API 根前缀，例如 /api/v1。
func (a AppConfig) BasePath() string {
	p := strings.TrimRight(a.Prefix, "/")
	if a.Version == "" {
		return p
	}
	return p + "/" + strings.Trim(a.Version, "/")
}

func (a AppConfig) IsDebug() bool { return strings.EqualFold(a.Mode, "debug") }

// SwaggerEnabled 返回是否挂载在线接口文档。未显式配置时跟随运行模式。
func (a AppConfig) SwaggerEnabled() bool {
	if a.Swagger != nil {
		return *a.Swagger
	}
	return a.IsDebug()
}

// MetricsEnabled 返回是否挂载 /metrics 端点。默认关闭，需显式开启。
func (a AppConfig) MetricsEnabled() bool {
	return a.Metrics != nil && *a.Metrics
}

type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	DBName          string `yaml:"dbname"`
	SSLMode         string `yaml:"sslmode"`
	TimeZone        string `yaml:"timezone"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
	LogLevel        string `yaml:"log_level"`
	// StatementTimeout 是单条 SQL 在 PostgreSQL 侧的执行上限（毫秒），<=0 走默认值 30000。
	//
	// 必须放在数据库侧而不是应用侧：应用层的 context 超时只能让调用方不再等待，
	// 那条查询在数据库里照样跑完，连接也照样被占着；只有服务端 statement_timeout
	// 能真正把它掐断。默认 30s，略小于 HTTP 服务的 60s 写超时，保证请求先于连接超时结束。
	StatementTimeout int `yaml:"statement_timeout"`
}

// DSN 返回 GORM 使用的 PostgreSQL 连接串。
//
// statement_cache_mode=describe 是针对「运行时可能改表结构」的刻意选择：
// AutoMigrate 会在升级时补列/改列（见 test/upgrade_test.go 的老库迁移用例），
// 而 pgx 默认的 prepare 模式会把语句计划按连接缓存，表结构一变，
// 同一连接上复用同一 SQL 就会报
// `已缓冲的计划不能改变结果类型 (SQLSTATE 0A000)` —— 报错点离真正的
// 变更点很远（表现为登录失败、查询 500），极难排查。
// describe_exec 模式每次执行前重新 Describe 拿结果类型，代价是一次往返，
// 换来的是「改表不需要重启所有连接」。
//
// 参数名注意：`default_query_exec_mode=describe_exec` 是 pgx v5 的写法，
// v4 的 `statement_cache_mode` 在 v5 会被当成未知启动参数透传给服务器，
// 表现是连库时报「未认可的配置参数」(SQLSTATE 42704)。
func (d DatabaseConfig) DSN() string {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s default_query_exec_mode=describe_exec",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode, d.TimeZone,
	)
	// statement_timeout 以「运行时参数」形式下发：pgx 会把连接串里未识别的键
	// 转成 RuntimeParams（TimeZone 走的就是同一条路），于是连接池里**每一条**新连接
	// 都自带这个上限；若改用连接后执行一次 SET，则只对当时那一条连接生效。
	if d.StatementTimeout > 0 {
		dsn += fmt.Sprintf(" statement_timeout=%d", d.StatementTimeout)
	}
	return dsn
}

// AdminDSN 返回连接到 postgres 默认库的连接串，用于自动建库。
func (d DatabaseConfig) AdminDSN() string {
	clone := d
	clone.DBName = "postgres"
	return clone.DSN()
}

type JWTConfig struct {
	Secret        string `yaml:"secret"`
	Issuer        string `yaml:"issuer"`
	AccessExpire  int    `yaml:"access_expire"`
	RefreshExpire int    `yaml:"refresh_expire"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Dir    string `yaml:"dir"`
	File   string `yaml:"file"`
	// 落盘轮转参数，0 走默认值（单文件 100MB / 保留 7 个 / 30 天）。
	MaxSizeMB  int `yaml:"max_size_mb"`
	MaxBackups int `yaml:"max_backups"`
	MaxAgeDays int `yaml:"max_age_days"`
}

type CORSConfig struct {
	AllowOrigins []string `yaml:"allow_origins"`
}

type SecurityConfig struct {
	MaxLoginFail     int `yaml:"max_login_fail"`
	LoginLockSeconds int `yaml:"login_lock_seconds"`
	// IPMaxLoginFail 是「同一个 IP 在一个锁定窗口内的登录失败上限」。
	//
	// 按账号锁定拦不住「同一 IP 撞库大量不同账号」，这一项用来补这个缺口。
	// 用指针是为了区分「没配」（走默认值）与「显式配成 0」（关闭限流）：
	// 若用普通 int，测试环境把 0 写进配置会被 applyDefaults 又改回默认值。
	IPMaxLoginFail *int `yaml:"ip_max_login_fail"`
	// RefreshRateLimit 是 /auth/refresh 每 IP 每分钟的请求上限，0 表示关闭。
	// 刷新接口没有凭据猜测面（要验签名的 JWT），这里只做防刷，阈值可以放宽。
	RefreshRateLimit *int `yaml:"refresh_rate_limit"`
	// TrustedProxies 列出可信反向代理的 IP 或网段。
	//
	// 为空表示**不信任任何代理**，此时 ClientIP 只取 TCP 对端地址。
	// 这一点很重要：gin 默认信任所有代理，会直接采用 X-Forwarded-For 的值，
	// 而该头完全由客户端控制 —— 于是任何人都能把审计日志里的来源 IP 写成任意值。
	// 部署在 Nginx 等反向代理后面时，才把代理的地址填进来。
	TrustedProxies []string `yaml:"trusted_proxies"`
}

// IPFailLimit 返回单 IP 登录失败上限，<=0 表示关闭该限制。
func (s SecurityConfig) IPFailLimit() int {
	if s.IPMaxLoginFail == nil {
		return 0
	}
	return *s.IPMaxLoginFail
}

// RefreshRateLimitPerMin 返回 /auth/refresh 每 IP 每分钟的请求上限，<=0 表示关闭。
func (s SecurityConfig) RefreshRateLimitPerMin() int {
	if s.RefreshRateLimit == nil {
		return 0
	}
	return *s.RefreshRateLimit
}

var (
	once   sync.Once
	global *Config
)

// Get 返回全局配置单例。
func Get() *Config { return global }

// Load 从文件加载配置并应用环境变量覆盖。
func Load(path string) (*Config, error) {
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	applyEnvOverrides(&cfg)
	applyDefaults(&cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	once.Do(func() { global = &cfg })
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.App.Port <= 0 {
		return fmt.Errorf("app.port 必须大于 0")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname 不能为空")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 不能为空")
	}
	if c.Backup.KeepCount < 0 || c.Backup.KeepMonths < 0 {
		return fmt.Errorf("backup.keep_count / keep_months 不能为负数（0 表示用默认值）")
	}
	if c.Maintenance.OpLogDays < 0 || c.Maintenance.LoginLogDays < 0 || c.Maintenance.IdempotencyDays < 0 {
		return fmt.Errorf("maintenance.*_days 不能为负数（0 表示用默认值）")
	}
	// 代理地址写错会让 ClientIP 取不到真实来源，宁可启动时就报错
	for _, p := range c.Security.TrustedProxies {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			if _, _, err := net.ParseCIDR(p); err != nil {
				return fmt.Errorf("security.trusted_proxies 中的 %q 不是合法网段: %w", p, err)
			}
			continue
		}
		if net.ParseIP(p) == nil {
			return fmt.Errorf("security.trusted_proxies 中的 %q 既不是 IP 也不是网段", p)
		}
	}
	return nil
}

func applyDefaults(c *Config) {
	if c.App.Prefix == "" {
		c.App.Prefix = "/api"
	}
	if c.App.Host == "" {
		c.App.Host = "0.0.0.0"
	}
	if c.JWT.AccessExpire <= 0 {
		c.JWT.AccessExpire = 7200
	}
	if c.JWT.RefreshExpire <= 0 {
		c.JWT.RefreshExpire = 604800
	}
	if c.JWT.Issuer == "" {
		c.JWT.Issuer = c.App.Name
	}
	if c.Database.TimeZone == "" {
		c.Database.TimeZone = "Asia/Shanghai"
	}
	if c.Database.SSLMode == "" {
		c.Database.SSLMode = "disable"
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 10
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 100
	}
	if c.Database.StatementTimeout <= 0 {
		c.Database.StatementTimeout = 30000
	}
	if c.Backup.Dir == "" {
		c.Backup.Dir = "backups"
	}
	if c.Backup.KeepCount == 0 {
		c.Backup.KeepCount = 30
	}
	if c.Backup.KeepMonths == 0 {
		c.Backup.KeepMonths = 6
	}
	if c.Maintenance.OpLogDays == 0 {
		c.Maintenance.OpLogDays = 180
	}
	if c.Maintenance.LoginLogDays == 0 {
		c.Maintenance.LoginLogDays = 180
	}
	if c.Maintenance.IdempotencyDays == 0 {
		c.Maintenance.IdempotencyDays = 7
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "console"
	}
	if len(c.CORS.AllowOrigins) == 0 {
		c.CORS.AllowOrigins = []string{"*"}
	}
	if c.Security.MaxLoginFail <= 0 {
		c.Security.MaxLoginFail = 5
	}
	if c.Security.LoginLockSeconds <= 0 {
		c.Security.LoginLockSeconds = 300
	}
	if c.Security.IPMaxLoginFail == nil {
		// 默认开启：30 次失败 / 窗口。只统计失败，NAT 后的正常用户不会受影响。
		def := 30
		c.Security.IPMaxLoginFail = &def
	}
	if c.Security.RefreshRateLimit == nil {
		def := 120
		c.Security.RefreshRateLimit = &def
	}
}

// applyEnvOverrides 用环境变量覆盖配置，规则 NOVA_<SECTION>_<KEY>。
func applyEnvOverrides(c *Config) {
	str := func(key string, target *string) {
		if v := os.Getenv(key); v != "" {
			*target = v
		}
	}
	num := func(key string, target *int) {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*target = n
			}
		}
	}

	str("NOVA_APP_NAME", &c.App.Name)
	str("NOVA_APP_MODE", &c.App.Mode)
	str("NOVA_APP_HOST", &c.App.Host)
	num("NOVA_APP_PORT", &c.App.Port)
	str("NOVA_APP_PREFIX", &c.App.Prefix)
	str("NOVA_APP_VERSION", &c.App.Version)
	if v := os.Getenv("NOVA_APP_SWAGGER"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.App.Swagger = &b
		}
	}
	if v := os.Getenv("NOVA_APP_METRICS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.App.Metrics = &b
		}
	}
	str("NOVA_APP_METRICS_TOKEN", &c.App.MetricsToken)

	str("NOVA_DATABASE_HOST", &c.Database.Host)
	num("NOVA_DATABASE_PORT", &c.Database.Port)
	str("NOVA_DATABASE_USER", &c.Database.User)
	str("NOVA_DATABASE_PASSWORD", &c.Database.Password)
	str("NOVA_DATABASE_DBNAME", &c.Database.DBName)
	str("NOVA_DATABASE_SSLMODE", &c.Database.SSLMode)
	str("NOVA_DATABASE_TIMEZONE", &c.Database.TimeZone)
	num("NOVA_DATABASE_MAX_IDLE_CONNS", &c.Database.MaxIdleConns)
	num("NOVA_DATABASE_MAX_OPEN_CONNS", &c.Database.MaxOpenConns)
	num("NOVA_DATABASE_STATEMENT_TIMEOUT", &c.Database.StatementTimeout)
	str("NOVA_DATABASE_LOG_LEVEL", &c.Database.LogLevel)

	str("NOVA_JWT_SECRET", &c.JWT.Secret)
	str("NOVA_JWT_ISSUER", &c.JWT.Issuer)
	num("NOVA_JWT_ACCESS_EXPIRE", &c.JWT.AccessExpire)
	num("NOVA_JWT_REFRESH_EXPIRE", &c.JWT.RefreshExpire)

	str("NOVA_LOG_LEVEL", &c.Log.Level)
	str("NOVA_LOG_FORMAT", &c.Log.Format)
	str("NOVA_LOG_DIR", &c.Log.Dir)
	str("NOVA_LOG_FILE", &c.Log.File)

	num("NOVA_SECURITY_MAX_LOGIN_FAIL", &c.Security.MaxLoginFail)
	num("NOVA_SECURITY_LOGIN_LOCK_SECONDS", &c.Security.LoginLockSeconds)
	if v := os.Getenv("NOVA_SECURITY_IP_MAX_LOGIN_FAIL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Security.IPMaxLoginFail = &n
		}
	}
	if v := os.Getenv("NOVA_SECURITY_REFRESH_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Security.RefreshRateLimit = &n
		}
	}
	// 逗号分隔，例如 NOVA_SECURITY_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8
	if v := os.Getenv("NOVA_SECURITY_TRUSTED_PROXIES"); v != "" {
		out := make([]string, 0, 2)
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		c.Security.TrustedProxies = out
	}
}
