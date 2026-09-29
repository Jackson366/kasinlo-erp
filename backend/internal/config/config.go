package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config 是应用的完整配置树。
type Config struct {
	App      AppConfig      `yaml:"app"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Log      LogConfig      `yaml:"log"`
	CORS     CORSConfig     `yaml:"cors"`
	Security SecurityConfig `yaml:"security"`
}

type AppConfig struct {
	Name    string `yaml:"name"`
	Mode    string `yaml:"mode"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Prefix  string `yaml:"prefix"`
	Version string `yaml:"version"`
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
}

// DSN 返回 GORM 使用的 PostgreSQL 连接串。
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode, d.TimeZone,
	)
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
}

type CORSConfig struct {
	AllowOrigins []string `yaml:"allow_origins"`
}

type SecurityConfig struct {
	MaxLoginFail     int `yaml:"max_login_fail"`
	LoginLockSeconds int `yaml:"login_lock_seconds"`
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

	str("NOVA_DATABASE_HOST", &c.Database.Host)
	num("NOVA_DATABASE_PORT", &c.Database.Port)
	str("NOVA_DATABASE_USER", &c.Database.User)
	str("NOVA_DATABASE_PASSWORD", &c.Database.Password)
	str("NOVA_DATABASE_DBNAME", &c.Database.DBName)
	str("NOVA_DATABASE_SSLMODE", &c.Database.SSLMode)
	str("NOVA_DATABASE_TIMEZONE", &c.Database.TimeZone)
	num("NOVA_DATABASE_MAX_IDLE_CONNS", &c.Database.MaxIdleConns)
	num("NOVA_DATABASE_MAX_OPEN_CONNS", &c.Database.MaxOpenConns)
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
}
