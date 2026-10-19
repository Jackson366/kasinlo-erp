package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // 注册 "pgx" database/sql 驱动
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"nova-erp/internal/config"
	"nova-erp/internal/logger"
	"nova-erp/internal/model"
)

var db *gorm.DB

// dbNamePattern 限定库名。库名要拼进 `CREATE DATABASE "x"` 的**标识符**位置，
// 而标识符不能用 $1 占位符传参 —— 唯一可靠的做法是先严格校验再用。
// 与 ops.restore 的目标库名校验同一口径：只允许字母数字下划线，长度对齐 PostgreSQL 63 字节上限。
var dbNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// loc 是数据库会话时区（配置 database.timezone，默认 Asia/Shanghai）。
// DSN 里的 TimeZone 决定了 SQL 侧 DATE(created_at) 等按哪个时区分桶，
// Go 侧拼日期键必须用同一时区，否则主机时区与数据库配置不一致时
// 趋势图等按天聚合的结果会整日错位。
var loc *time.Location

// Location 返回数据库会话时区；Init 未调用时回退本地时区。
func Location() *time.Location {
	if loc == nil {
		return time.Local
	}
	return loc
}

// DB 返回全局数据库句柄。
func DB() *gorm.DB { return db }

// Init 建立数据库连接：自动建库（如不存在）-> 连接 -> 连接池调优。
func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if l, err := time.LoadLocation(cfg.TimeZone); err == nil {
		loc = l
	} else {
		loc = time.Local
		logger.Warn("时区配置无效，回退使用服务器本地时区",
			zap.String("timezone", cfg.TimeZone), zap.Error(err))
	}
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
		// 进销存业务表
		&model.InvCustomer{},
		&model.InvSupplier{},
		&model.InvCategory{},
		&model.InvProduct{},
		&model.InvProductSpec{},
		&model.InvProductUnit{},
		&model.InvOrder{},
		&model.InvOrderItem{},
		&model.InvSettlement{},
		&model.InvStockOrder{},
		&model.InvStockItem{},
		&model.InvPrintTemplate{},
		&model.Idempotency{},
		// 结构版本表：记录库已升到哪一版，供启动闸门判断方向
		&model.SchemaVersion{},
		// 升级记录表：每次升级的版本、升级前备份、执行动作与结果
		&model.UpgradeRecord{},
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
	// 进销存业务唯一键：编号/单号在存活数据内唯一（软删除后允许复用）
	{name: "uk_inv_customers_code", table: "inv_customers", legacy: "", columns: "code", where: "deleted_at IS NULL"},
	{name: "uk_inv_suppliers_code", table: "inv_suppliers", legacy: "", columns: "code", where: "deleted_at IS NULL"},
	{name: "uk_inv_products_code", table: "inv_products", legacy: "", columns: "code", where: "deleted_at IS NULL"},
	{name: "uk_inv_categories_name", table: "inv_categories", legacy: "", columns: "name", where: "deleted_at IS NULL"},
	// 同一个商品下规格名唯一；同一个规格下单位名唯一。
	// 少了这两条，「φ20」可以被录两遍，库存被劈成两半且各自都不预警。
	{name: "uk_inv_product_specs_product_spec", table: "inv_product_specs", legacy: "", columns: "product_id, spec", where: "deleted_at IS NULL"},
	{name: "uk_inv_product_units_spec_unit", table: "inv_product_units", legacy: "", columns: "spec_id, unit", where: "deleted_at IS NULL"},
	{name: "uk_inv_orders_no", table: "inv_orders", legacy: "", columns: "order_no", where: "deleted_at IS NULL"},
	{name: "uk_inv_stock_orders_no", table: "inv_stock_orders", legacy: "", columns: "order_no", where: "deleted_at IS NULL"},
	{name: "uk_inv_print_templates_code", table: "inv_print_templates", legacy: "", columns: "code", where: "deleted_at IS NULL"},
}

// dataFixes 是幂等的历史数据修补清单，启动时统一执行。
//
// 与 AutoMigrate / EnsureIndexes 的分工：前两者管**结构**，这里管**已落库的值**。
// 有些缺陷只改代码修不掉 —— 错误的行还躺在库里。把这类修补集中登记在这里，
// 好处是：幂等（重复启动无副作用）、可审计（改了什么一眼可见）、
// 不需要运维手工执行 SQL。
var dataFixes = []struct {
	name string
	stmt string
}{
	{
		// 选填的到期日曾经用非指针 time.Time 承载，空串解析出 Go 零值后被原样
		// 写进 date 列，于是「没填到期日」在库里长成了 0001-01-01。这个值与
		// 「已过期两千年」无法区分：逾期汇总会多算、打印纸面会多打一行。
		// 归一成 NULL 后，「未填」才有唯一的表达方式。
		name: "到期日零值归一为 NULL",
		stmt: `UPDATE inv_orders SET due_date = NULL WHERE due_date IS NOT NULL AND due_date < '1900-01-01'`,
	},
}

// PatchData 执行历史数据修补（幂等）。
func PatchData() error {
	if err := migrateLegacyProducts(); err != nil {
		return err
	}
	for _, fix := range dataFixes {
		res := db.Exec(fix.stmt)
		if res.Error != nil {
			return fmt.Errorf("数据修补「%s」失败: %w", fix.name, res.Error)
		}
		if res.RowsAffected > 0 {
			logger.Info("已修补历史数据",
				zap.String("fix", fix.name),
				zap.Int64("rows", res.RowsAffected))
		}
	}
	return nil
}

// legacyProductColumns 是 inv_products 上被淘汰的列。
//
// 它们曾经把「一个商品 = 一个规格 + 一个单位 + 一份价格 + 一份库存」压在一行；
// 商品支持多规格多单位之后，这些列的语义已经无处安放。不删的话，
// 它们会一直留在库里被误读（读到一个早已不再维护的 stock 而已）。
var legacyProductColumns = []string{
	"spec", "unit", "category", "purchase_price", "sale_price", "stock", "warn_stock",
}

// migrateLegacyProducts 把旧结构的商品档案升级为「商品 → 规格 → 计量单位」三层。
//
// 幂等策略与 dataFixes 不同：它依赖的列在迁移完成后就不存在了，所以不能无脑重跑。
// 先探一次列是否存在，不存在即视为「已经迁过」直接返回；步骤之间全部用
// NOT EXISTS 兜住，中途失败后重跑也不会产出重复规格。
func migrateLegacyProducts() error {
	has, err := columnExists("inv_products", "spec")
	if err != nil {
		return err
	}
	if !has {
		return nil
	}

	steps := []struct {
		name string
		stmt string
	}{
		{
			name: "旧品类文本收成分类字典",
			stmt: `INSERT INTO inv_categories (created_at, updated_at, name, sort, status, remark)
SELECT NOW(), NOW(), c.name, 0, 1, ''
FROM (SELECT DISTINCT category AS name FROM inv_products WHERE deleted_at IS NULL AND category <> '') c
WHERE NOT EXISTS (SELECT 1 FROM inv_categories ic WHERE ic.name = c.name AND ic.deleted_at IS NULL)`,
		},
		{
			name: "回填商品的分类外键",
			stmt: `UPDATE inv_products p SET category_id = c.id
FROM inv_categories c
WHERE c.name = p.category AND c.deleted_at IS NULL
  AND p.deleted_at IS NULL AND (p.category_id IS NULL OR p.category_id = 0)`,
		},
		{
			name: "商品升级出规格（库存随之迁移）",
			stmt: `INSERT INTO inv_product_specs (created_at, updated_at, product_id, spec, stock, warn_stock, sort, status)
SELECT NOW(), NOW(), p.id, COALESCE(NULLIF(p.spec, ''), '默认'), p.stock, p.warn_stock, 0, p.status
FROM inv_products p
WHERE p.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM inv_product_specs s WHERE s.product_id = p.id AND s.deleted_at IS NULL)`,
		},
		{
			name: "规格升级出基本单位（沿用旧单位与双价格）",
			stmt: `INSERT INTO inv_product_units (created_at, updated_at, spec_id, unit, rate, is_base, purchase_price, sale_price, sort, status)
SELECT NOW(), NOW(), s.id, COALESCE(NULLIF(p.unit, ''), '个'), 1, TRUE, p.purchase_price, p.sale_price, 0, 1
FROM inv_product_specs s
JOIN inv_products p ON p.id = s.product_id AND p.deleted_at IS NULL
WHERE s.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM inv_product_units u WHERE u.spec_id = s.id AND u.deleted_at IS NULL)`,
		},
		{
			name: "历史单据明细补规格与基本数量",
			stmt: `UPDATE inv_order_items i SET spec_id = s.id, unit_id = u.id, qty_base = i.qty
FROM inv_product_specs s
JOIN inv_product_units u ON u.spec_id = s.id AND u.is_base AND u.deleted_at IS NULL
WHERE s.product_id = i.product_id AND s.deleted_at IS NULL
  AND i.deleted_at IS NULL AND (i.spec_id IS NULL OR i.spec_id = 0)`,
		},
		{
			name: "历史出入库明细补规格与基本数量",
			stmt: `UPDATE inv_stock_items i SET spec_id = s.id, unit_id = u.id, qty_base = i.qty
FROM inv_product_specs s
JOIN inv_product_units u ON u.spec_id = s.id AND u.is_base AND u.deleted_at IS NULL
WHERE s.product_id = i.product_id AND s.deleted_at IS NULL
  AND i.deleted_at IS NULL AND (i.spec_id IS NULL OR i.spec_id = 0)`,
		},
	}
	for _, st := range steps {
		res := db.Exec(st.stmt)
		if res.Error != nil {
			return fmt.Errorf("商品多层化迁移「%s」失败: %w", st.name, res.Error)
		}
		logger.Info("商品结构升级", zap.String("step", st.name), zap.Int64("rows", res.RowsAffected))
	}

	// 数据搬到新表之后，旧列才允许删。放在最后一步：中途失败时列还在，
	// 下次启动会重新走一遍上面的幂等步骤，而不是留下一个半迁移的库。
	drop := "ALTER TABLE inv_products"
	for i, col := range legacyProductColumns {
		if i > 0 {
			drop += ","
		}
		drop += " DROP COLUMN IF EXISTS " + col
	}
	if err := db.Exec(drop).Error; err != nil {
		return fmt.Errorf("清理 inv_products 遗留列失败: %w", err)
	}
	logger.Info("已清理 inv_products 遗留列", zap.Strings("columns", legacyProductColumns))
	return nil
}

// columnExists 判断当前 schema 下某张表是否有某一列。
func columnExists(table, column string) (bool, error) {
	var n int64
	err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, column).
		Scan(&n).Error
	if err != nil {
		return false, fmt.Errorf("检查列 %s.%s 是否存在失败: %w", table, column, err)
	}
	return n > 0, nil
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

// Stats 返回连接池运行状态，供 /metrics 抓取。
func Stats() (sql.DBStats, error) {
	if db == nil {
		return sql.DBStats{}, errors.New("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return sql.DBStats{}, err
	}
	return sqlDB.Stats(), nil
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
	// 库名会被拼进标识符位置（CREATE DATABASE 不支持占位符），必须先校验；
	// 环境变量注入的 DBName 里若含引号，不校验就是一条注入面。
	if !dbNamePattern.MatchString(cfg.DBName) {
		return fmt.Errorf("database.dbname %q 不是合法库名（只允许字母数字下划线，且以字母或下划线开头）", cfg.DBName)
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
	return gormZap{level: lvl, slow: 200 * time.Millisecond}
}

// gormZap 把 GORM 日志桥接到全局结构化日志器。
// gormlogger.Default 会绕过统一配置直接往 stdout 写裸文本，
// 桥接后 SQL 错误/慢查询与业务日志同一格式、同一输出、同一级别。
type gormZap struct {
	level gormlogger.LogLevel
	slow  time.Duration
}

func (g gormZap) LogMode(lvl gormlogger.LogLevel) gormlogger.Interface {
	g.level = lvl
	return g
}

func (g gormZap) Info(_ context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Info {
		logger.L().Info(fmt.Sprintf(msg, args...))
	}
}

func (g gormZap) Warn(_ context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Warn {
		logger.L().Warn(fmt.Sprintf(msg, args...))
	}
}

func (g gormZap) Error(_ context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Error {
		logger.L().Error(fmt.Sprintf(msg, args...))
	}
}

// maxTraceSQLLen 是单条 SQL 进日志前的长度上限。
// IN (?) 展开成上千个参数时，一行 SQL 能有几十 KB，不截断会把日志文件撑爆。
const maxTraceSQLLen = 4096

func (g gormZap) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sqlText, rows := fc()
	// 约定：敏感值永远不进 SQL 文本（全部走 $1 参数绑定），日志文件才对同机用户安全。
	// 这里再兜一层长度截断，避免超长 SQL 把日志撑爆。
	if len(sqlText) > maxTraceSQLLen {
		sqlText = sqlText[:maxTraceSQLLen] + "...(截断)"
	}
	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql", sqlText),
	}
	switch {
	// ErrRecordNotFound 是查询的正常结果，不是错误
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && g.level >= gormlogger.Error:
		logger.L().Error("SQL 执行失败", append(fields, zap.Error(err))...)
	case elapsed > g.slow && g.level >= gormlogger.Warn:
		logger.L().Warn("SQL 慢查询", fields...)
	case g.level >= gormlogger.Info:
		logger.L().Info("SQL", fields...)
	}
}
