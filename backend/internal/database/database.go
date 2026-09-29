package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // 注册 "pgx" database/sql 驱动
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"nova-admin/internal/config"
	"nova-admin/internal/logger"
	"nova-admin/internal/model"
)

var db *gorm.DB

// DB 返回全局数据库句柄。
func DB() *gorm.DB { return db }

// Init 建立数据库连接：自动建库（如不存在）-> 连接 -> 连接池调优。
func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if err := ensureDatabase(cfg); err != nil {
		return nil, err
	}
	conn, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger:                 gormLogger(cfg.LogLevel),
		SkipDefaultTransaction: true,
		NowFunc:                func() time.Time { return time.Now().Local() },
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接池失败: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库 ping 失败: %w", err)
	}
	db = conn
	logger.Info("数据库连接成功", zap.String("database", cfg.DBName))
	return conn, nil
}

// AutoMigrate 自动同步表结构。
func AutoMigrate() error {
	return db.AutoMigrate(
		&model.Dept{},
		&model.User{},
		&model.Role{},
		&model.Menu{},
		&model.UserRole{},
		&model.RoleMenu{},
		&model.OperationLog{},
		&model.LoginLog{},
	)
}

// partialUniqueIndexes 定义业务唯一约束。
//
// 为什么用「部分唯一索引」而不是普通唯一索引 / GORM 的 uniqueIndex 标签：
// 本项目所有主数据表都启用了软删除（deleted_at）。普通唯一索引会把已软删除的
// 历史行也算进去，导致「删除后无法再创建同名记录」这一反直觉缺陷。
// 加上 WHERE deleted_at IS NULL 后，唯一性只约束存活数据。
var partialUniqueIndexes = []struct {
	name    string
	table   string
	legacy  string // 旧版 GORM uniqueIndex 生成的索引名，需要清理
	columns string
	where   string
}{
	{name: "uk_sys_users_username", table: "sys_users", legacy: "idx_sys_users_username", columns: "username", where: "deleted_at IS NULL"},
	{name: "uk_sys_roles_code", table: "sys_roles", legacy: "idx_sys_roles_code", columns: "code", where: "deleted_at IS NULL"},
	{name: "uk_sys_menus_name", table: "sys_menus", legacy: "idx_sys_menus_name", columns: "name", where: "deleted_at IS NULL"},
	{name: "uk_sys_depts_parent_name", table: "sys_depts", legacy: "", columns: "parent_id, name", where: "deleted_at IS NULL"},
}

// EnsureIndexes 创建部分唯一索引，并清理历史遗留的普通唯一索引（幂等）。
func EnsureIndexes() error {
	for _, idx := range partialUniqueIndexes {
		if idx.legacy != "" {
			if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, idx.legacy)).Error; err != nil {
				return fmt.Errorf("清理旧索引 %s 失败: %w", idx.legacy, err)
			}
		}
		stmt := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE %s`,
			idx.name, idx.table, idx.columns, idx.where)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("创建索引 %s 失败: %w", idx.name, err)
		}
	}
	return nil
}

// Close 关闭数据库连接。
func Close() error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Health 简单健康检查。
func Health() error {
	if db == nil {
		return errors.New("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// ensureDatabase 检查目标库是否存在，不存在则自动创建。
func ensureDatabase(cfg config.DatabaseConfig) error {
	admin, err := sql.Open("pgx", cfg.AdminDSN())
	if err != nil {
		return fmt.Errorf("初始化数据库连接失败: %w", err)
	}
	defer admin.Close()

	if err := admin.Ping(); err != nil {
		return fmt.Errorf("连接 PostgreSQL 失败，请检查 host/port/user/password: %w", err)
	}

	var exists bool
	row := admin.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.DBName)
	if err := row.Scan(&exists); err != nil {
		return fmt.Errorf("查询数据库是否存在失败: %w", err)
	}
	if exists {
		return nil
	}
	stmt := fmt.Sprintf(`CREATE DATABASE "%s" ENCODING 'UTF8'`, cfg.DBName)
	if _, err := admin.Exec(stmt); err != nil {
		return fmt.Errorf("创建数据库 %s 失败: %w", cfg.DBName, err)
	}
	logger.Info("已自动创建数据库", zap.String("database", cfg.DBName))
	return nil
}

func gormLogger(level string) gormlogger.Interface {
	var lvl gormlogger.LogLevel
	switch level {
	case "silent":
		lvl = gormlogger.Silent
	case "error":
		lvl = gormlogger.Error
	case "info":
		lvl = gormlogger.Info
	default:
		lvl = gormlogger.Warn
	}
	return gormlogger.Default.LogMode(lvl)
}
