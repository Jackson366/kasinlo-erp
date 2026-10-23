package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	Upgrade     UpgradeConfig     `yaml:"upgrade"`
}

// UpgradeConfig 是系统升级的默认参数。
type UpgradeConfig struct {
	// FrontendDir 是前端产物目录（vite build 的输出）。升级包里的前端部分
	// 会原子替换这个目录。留空表示「本次升级只含后端」。
	FrontendDir string `yaml:"frontend_dir"`
	// StagingDir 是升级包的暂存目录（解压后的 manifest / 二进制 / 前端包）。
	// 留空取 <可执行文件所在目录>/upgrade/staging。
	StagingDir string `yaml:"staging_dir"`
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
	// Release 是**程序发布版本号**（如 1.2.0），与 Version（API 版本段 /api/v1）
	// 是两件事：API 版本段描述接口形状，发布版本号描述这一版程序本身。
	// 升级包校验、升级记录、启动后的生效核对都读它。
	//
	// 取值优先级：配置文件 app.release > 构建注入的 appRelease > dev。
	// 构建注入写法：go build -ldflags "-X nova-erp/internal/config.appRelease=1.2.0"。
	// 显式写进配置文件优先，是为了让「改配置就能核对版本」这种现场救急手段生效。
	Release string `yaml:"release"`
	// WebDir 是前端静态产物目录。非空且目录里存在 index.html 时，
	// 后端会直接托管它（含 SPA 深链回退），此时不需要额外的 Nginx。
	// 留空取 <可执行文件所在目录>/web。
	WebDir string `yaml:"web_dir"`
	// DemoData 控制首次初始化时是否灌入演示业务数据（示例客户/商品/单据）。
	//
	// 默认 true 是为了本地开发、冒烟与演示开箱可用；**交付真实客户时务必设 false**，
	// 否则对方第一次登录看到的是「华东制造」「远大贸易」和一堆不属于自己的单据。
	// 用指针区分「没配」与「显式关掉」。
	DemoData *bool `yaml:"demo_data"`
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
	// AttachmentDir 是业务附件（采购 / 销售单、合同的图片与文件）的存储根目录。
	// 留空取 <可执行文件所在目录>/data/attachments —— 与 web_dir 同一套基准，
	// 用「服务方式」启动（CWD 是 System32）时也不会把附件写进系统目录。
	//
	// 附件不进数据库：几百 KB 的扫描件塞进 bytea 会让备份变成几百 MB，
	// 而备份要能每天跑。库里只存相对路径与元数据。
	AttachmentDir string `yaml:"attachment_dir"`
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

// DemoDataEnabled 返回是否灌入演示业务数据（默认开，交付时关掉）。
func (a AppConfig) DemoDataEnabled() bool {
	return a.DemoData == nil || *a.DemoData
}

// appRelease 是构建期注入的发布版本号，缺省 dev。
//
// 单独放一个包级变量而不是只读 yaml：现场只需要一条
// -ldflags "-X nova-erp/internal/config.appRelease=1.2.0" 就能得到
// 「程序自带版本号」，不必在每台机器上改配置文件 —— 配置文件漏改正是
// 升级后核对不通过最常见的原因。
var appRelease = "dev"

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
	cfg.resolvePaths()
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
	// 签名固定 HS256，secret 就是整条鉴权链的根：弱密钥可被离线爆破，
	// 伪造任意身份（含超管）令牌。至少要求 32 字节，并拒绝纯弱口令，
	// 让「随手填个 secret」在启动时就被拦下，而不是等被人爆破。
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("jwt.secret 过短（%d 字节）：至少 32 字节，请用随机强密钥", len(c.JWT.Secret))
	}
	// 占位串必须硬拒：模板里的 CHANGE_ME_XXX 恰好超过 32 字节，
	// 只校验长度的话，「照抄模板即可启动」等于用一个公开密钥签发超管令牌。
	if looksLikePlaceholder(c.JWT.Secret) {
		return fmt.Errorf("jwt.secret 仍是占位值：请改成随机强密钥（例：openssl rand -base64 48）")
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
	if err := c.validateUpgradeDirs(); err != nil {
		return err
	}
	return nil
}

// validateUpgradeDirs 校验升级相关目录。
//
// frontend_dir 是要被**整目录替换**的目标：升级时先把旧目录改名，再把新目录顶上。
// 配成工作目录、盘符根、或可执行文件所在目录（及其祖先）的话，
// 「改名旧目录」这一步就把自己脚下的东西搬走了，进程当场失去可执行文件与配置。
// 这类错误在开发环境不会犯，但在现场「顺手改一下配置」时会犯，
// 所以宁可启动失败，也不能等升级那天才发现。
func (c *Config) validateUpgradeDirs() error {
	fe := strings.TrimSpace(c.Upgrade.FrontendDir)
	// 空表示本次升级只含后端，不替换前端。
	if fe != "" {
		abs, err := filepath.Abs(filepath.Clean(fe))
		if err != nil {
			return fmt.Errorf("upgrade.frontend_dir 解析失败 %q: %w", fe, err)
		}
		if isRootPath(abs) {
			return fmt.Errorf("upgrade.frontend_dir 不能是文件系统根 %q：升级会整目录替换它", fe)
		}
		if exeDir, err := executableDir(); err == nil && exeDir != "" {
			if abs == exeDir || isAncestorOf(abs, exeDir) {
				return fmt.Errorf("upgrade.frontend_dir 指向可执行文件所在目录 %q（或其祖先）：升级替换该目录会让程序删掉自己", abs)
			}
		}
		// 工作目录：配成 "." 时 Abs 会解析成真实路径，看不出是「顺手写的点」，
		// 所以单独按绝对路径再判一次，报错里也说人话。
		if wd, err := os.Getwd(); err == nil && wd != "" && abs == wd {
			return fmt.Errorf("upgrade.frontend_dir 不能是进程工作目录 %q：升级会整目录替换它", wd)
		}
	}

	sd := strings.TrimSpace(c.Upgrade.StagingDir)
	if sd != "" {
		sabs, err := filepath.Abs(filepath.Clean(sd))
		if err != nil {
			return fmt.Errorf("upgrade.staging_dir 解析失败 %q: %w", sd, err)
		}
		if isRootPath(sabs) {
			return fmt.Errorf("upgrade.staging_dir 不能是文件系统根 %q", sd)
		}
	}
	return nil
}

// isRootPath 判断是否为盘符根（Windows）或 / （Unix）。
func isRootPath(abs string) bool {
	if abs == string(filepath.Separator) {
		return true
	}
	if vol := filepath.VolumeName(abs); vol != "" {
		return abs == vol+string(filepath.Separator)
	}
	return false
}

// isAncestorOf 判断 parent 是否为 child 的祖先目录（含等于已在调用处判过）。
func isAncestorOf(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// executableDir 返回当前可执行文件所在目录；取不到时返回空（不因此阻断启动）。
func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// looksLikePlaceholder 判断密钥是否还是模板里的占位串。
//
// 只查关键词不查熵：目的是拦住「照抄模板直接上线」，而不是做密码强度评分
// （那会把现场自己定的可读密钥也误杀）。
func looksLikePlaceholder(secret string) bool {
	s := strings.ToLower(strings.TrimSpace(secret))
	for _, bad := range []string{"change_me", "change-me", "changeme", "please-change", "please_change", "replace_me", "your-secret", "your_secret"} {
		if strings.Contains(s, bad) {
			return true
		}
	}
	return false
}

// resolvePaths 把配置里的相对路径统一钉到**可执行文件所在目录**。
//
// 为什么不按进程工作目录：以 Windows 服务 / 计划任务方式启动时 CWD 是
// C:\Windows\System32，配置里的 "./logs"、"backups" 会静默落到系统目录 ——
// 表现为「日志找不到、备份不知写哪了」，而且不报任何错。
// 钉在 exe 目录后，从哪个目录启动，读写位置都一样。
//
// 绝对路径一律不动：现场常把备份放到另一块盘，那是显式意图。
func (c *Config) resolvePaths() {
	base, err := executableDir()
	if err != nil || base == "" {
		// 取不到 exe 目录（极少见）就保持原样，仍按 CWD 解析，不阻断启动
		return
	}
	abs := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" {
			return p
		}
		if filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Join(base, p)
	}
	c.Log.Dir = abs(c.Log.Dir)
	c.Backup.Dir = abs(c.Backup.Dir)
	c.App.WebDir = abs(c.App.WebDir)
	c.App.AttachmentDir = abs(c.App.AttachmentDir)
	c.Upgrade.FrontendDir = abs(c.Upgrade.FrontendDir)
	c.Upgrade.StagingDir = abs(c.Upgrade.StagingDir)
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
	// CORS 默认**留空**（同源部署不需要跨域头）。
	// 之前默认 "*" 是「任何网页都能代发请求」的出厂态：虽然身份走
	// Authorization 头而非 Cookie，但配上宽松的 allow_headers 就等于
	// 把接口暴露给任意站点。需要前后端分域部署时显式列域名。
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
	// 发布版本号：配置文件显式写的优先，其次构建注入值，最后 dev。
	// 升级核对拿它和升级包里的目标版本比，都没注入时停在 dev ——
	// 「升级后版本没变」是刻意显眼的信号，而不是静默显示成功。
	if strings.TrimSpace(c.App.Release) == "" {
		if strings.TrimSpace(appRelease) != "" {
			c.App.Release = appRelease
		} else {
			c.App.Release = "dev"
		}
	}
	if strings.TrimSpace(c.App.WebDir) == "" {
		c.App.WebDir = "web"
	}
	if strings.TrimSpace(c.App.AttachmentDir) == "" {
		c.App.AttachmentDir = filepath.Join("data", "attachments")
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
			} else {
				// 静默忽略会让运维以为设了连接池上限 / 锁定阈值、实际没生效，
				// 且无任何线索。这里不能依赖 logger（logger 反向依赖 config，会成环），
				// 所以直接写到 stderr：启动日志里能一眼看到这条警告。
				fmt.Fprintf(os.Stderr, "警告：环境变量 %s 的值 %q 不是合法数字，已忽略\n", key, v)
			}
		}
	}

	str("NOVA_APP_NAME", &c.App.Name)
	str("NOVA_APP_MODE", &c.App.Mode)
	str("NOVA_APP_HOST", &c.App.Host)
	num("NOVA_APP_PORT", &c.App.Port)
	str("NOVA_APP_PREFIX", &c.App.Prefix)
	str("NOVA_APP_VERSION", &c.App.Version)
	str("NOVA_APP_RELEASE", &c.App.Release)
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
