package ops

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 业务数据清理。
//
// 与日志清理（Cleanup）是两套语义，刻意分开：
//   - 日志清理按**保留期**删，删的是「迟早要淘汰的流水」；
//   - 业务清理是**整表清空**，删的是「账」。前者可以放进计划任务无人值守，
//     后者必须是人看着确认的一次性动作。
//
// 因此这一层不做任何「自动」：范围只能是下面登记好的几档，
// 确认口令每档不同，且调用方必须先有 24 小时内的备份（闸门在模块层，
// 见 module/ops 的 BusinessCleanup）。

// 清理范围标识。写成常量而不是散落的字符串：范围 key 同时出现在
// 接口参数、确认口令映射、表白名单校验三处，拼错一个字母就变成
// 「校验放行了本档不该碰的表」。
const (
	ScopeInvDocuments = "inv_documents" // 只清单据
	ScopeInvAll       = "inv_all"       // 清全部进销存
	ScopeDatabase     = "database"      // 清全部进销存 + 日志 + 幂等键
)

// frameworkTables 是**任何业务清理都不许碰**的表。
//
// 它们不是「业务数据」而是「系统本身」：删掉账号/角色/菜单/权限之后，
// 正在跑的系统会立刻失去登录与鉴权依据，而重启虽然能重新播种，
// 现场看到的现象是「所有人都登不进去」—— 分不清是数据丢了还是系统坏了。
// 结构版本表与升级记录同理：前者是升级闸门的判据，后者是审计痕迹。
var frameworkTables = map[string]bool{
	"sys_users":           true,
	"sys_roles":           true,
	"sys_menus":           true,
	"sys_depts":           true,
	"sys_user_roles":      true,
	"sys_role_menus":      true,
	"sys_schema_versions": true,
	"sys_upgrade_records": true,
	"sys_operation_logs":  true, // 仅 database 档允许，见 scopeLogTables
	"sys_login_logs":      true,
	"sys_idempotencies":   true,
}

// scopeLogTables 是日志类表：只有「数据库重置」档允许清理。
// 拆出来是因为日志既是数据也是审计依据 —— 平时按保留期清（Cleanup），
// 整库重置时才整表清空。
var scopeLogTables = map[string]bool{
	"sys_operation_logs": true,
	"sys_login_logs":     true,
	"sys_idempotencies":  true,
}

// BizCleanupScope 是一档清理范围。
type BizCleanupScope struct {
	Key   string // 范围标识，如 inv_documents
	Title string // 展示名
	Desc  string // 一句话说明「删什么、留什么」
	Keep  string // 明确列出保留项
	// Confirm 是这一档的确认口令。每档不同是有意的：
	// 口令相同的时候，「手滑点到确认」就能从清单据跳到重置数据库。
	Confirm string
	// Tables 是删除顺序（子表在前）。库内进销存表之间没有外键，
	// 但顺序仍然按依赖排：将来若补上外键，顺序已经是正确的。
	Tables []string
	// ResetStockable 表示这一档支持「同时清零规格库存」。
	ResetStockable bool
}

// BizCleanupScopes 返回全部清理范围，顺序即界面展示顺序。
func BizCleanupScopes() []BizCleanupScope {
	return []BizCleanupScope{
		{
			Key:            ScopeInvDocuments,
			Title:          "进销存单据",
			Desc:           "删采购/销售/出入库单据与明细、收付款记录，商品与往来单位档案全部保留",
			Keep:           "商品、规格、单位、分类、客户、供应商、打印模板、系统框架",
			Confirm:        "确认清理单据",
			ResetStockable: true,
			Tables: []string{
				"inv_order_items", "inv_orders",
				"inv_stock_items", "inv_stock_orders",
				"inv_settlements",
			},
		},
		{
			Key:     ScopeInvAll,
			Title:   "进销存全部数据",
			Desc:    "删全部进销存业务表，包括商品、规格、单位、分类、客户、供应商与打印模板",
			Keep:    "系统框架（账号、角色、菜单、权限、部门、结构版本、升级记录）",
			Confirm: "确认清空进销存",
			Tables: []string{
				"inv_order_items", "inv_orders",
				"inv_stock_items", "inv_stock_orders",
				"inv_settlements",
				"inv_product_units", "inv_product_specs",
				"inv_customers", "inv_suppliers",
				"inv_products", "inv_categories",
				"inv_print_templates",
			},
		},
		{
			Key:     ScopeDatabase,
			Title:   "数据库重置",
			Desc:    "清空全部进销存业务表 + 日志 + 幂等键，相当于把系统交还给下一个使用者",
			Keep:    "系统框架（账号、角色、菜单、权限、部门、结构版本、升级记录）",
			Confirm: "确认重置数据库",
			Tables: []string{
				"inv_order_items", "inv_orders",
				"inv_stock_items", "inv_stock_orders",
				"inv_settlements",
				"inv_product_units", "inv_product_specs",
				"inv_customers", "inv_suppliers",
				"inv_products", "inv_categories",
				"inv_print_templates",
				"sys_operation_logs", "sys_login_logs", "sys_idempotencies",
			},
		},
	}
}

// FindBizCleanupScope 按 key 找范围，找不到时列出合法值
// （报错里带合法值，现场才不用回来翻文档）。
func FindBizCleanupScope(key string) (BizCleanupScope, error) {
	for _, s := range BizCleanupScopes() {
		if s.Key == key {
			return s, nil
		}
	}
	keys := make([]string, 0, 3)
	for _, s := range BizCleanupScopes() {
		keys = append(keys, s.Key)
	}
	return BizCleanupScope{}, fmt.Errorf("未知的清理范围 %q，合法值：%s", key, strings.Join(keys, " / "))
}

// BizCleanupOptions 是业务清理参数。
type BizCleanupOptions struct {
	Scope      string
	ResetStock bool // 同时把规格库存清零（仅 inv_documents 档有效）
	Apply      bool
}

// BizCleanupTable 是一张表的清理结果。
type BizCleanupTable struct {
	Name     string `json:"name"`
	Rows     int64  `json:"rows"`     // 清理前的总行数（含历史软删除行）
	Deleted  int64  `json:"deleted"`  // 实际删除行数
	SeqReset bool   `json:"seqReset"` // 表被清空后是否重置了自增序列
}

// BizCleanupReport 是整次业务清理的结果。
type BizCleanupReport struct {
	Scope          string            `json:"scope"`
	Title          string            `json:"title"`
	DryRun         bool              `json:"dryRun"`
	Tables         []BizCleanupTable `json:"tables"`
	StockReset     bool              `json:"stockReset"`     // 请求了清零
	StockResetRows int64             `json:"stockResetRows"` // 实际被清零的规格行数
	TotalDeleted   int64             `json:"totalDeleted"`
}

// BizCleanup 执行（或预演）一档业务数据清理。
//
// 三条硬约束：
//  1. **一个事务**：全部删除要么都成要么都不成。清到一半失败会留下
//     「单据没了但收付款还在」的库，比不清更难查。
//  2. **硬删**：业务清理的目的就是回收空间与重置状态，软删只会把行留着
//     继续占唯一索引与主键（AGENTS.md §5.16）。
//  3. **框架表免疫**：见 frameworkTables 的注释。
func BizCleanup(db *gorm.DB, opt BizCleanupOptions) (*BizCleanupReport, error) {
	scope, err := FindBizCleanupScope(opt.Scope)
	if err != nil {
		return nil, err
	}
	for _, t := range scope.Tables {
		if !isAllowedBizTable(t, scope.Key) {
			// 正常情况走不到：范围表是代码里写死的。这是给将来改表的人兜底的。
			return nil, fmt.Errorf("范围 %s 包含受保护的表 %s，已拒绝执行", scope.Key, t)
		}
	}

	rep := &BizCleanupReport{Scope: scope.Key, Title: scope.Title, DryRun: !opt.Apply, StockReset: opt.ResetStock}

	// 先数行：预演要给人看「会删多少」，真删也要留下清理前的基线。
	for _, t := range scope.Tables {
		var n int64
		if err := db.Table(t).Count(&n).Error; err != nil {
			return nil, fmt.Errorf("统计 %s 行数失败：%w", t, err)
		}
		rep.Tables = append(rep.Tables, BizCleanupTable{Name: t, Rows: n})
	}
	if !opt.Apply {
		return rep, nil
	}

	// 库存清零只在「规格表不会被删掉」时才有意义 —— 规格都删了，清零是空操作。
	resetStock := opt.ResetStock && scope.ResetStockable && !containsTable(scope.Tables, "inv_product_specs")

	err = db.Transaction(func(tx *gorm.DB) error {
		for i := range rep.Tables {
			t := &rep.Tables[i]
			res := tx.Exec("DELETE FROM " + t.Name)
			if res.Error != nil {
				return fmt.Errorf("删除 %s 失败：%w", t.Name, res.Error)
			}
			t.Deleted = res.RowsAffected
			// 表被清空才重置序列：留着行的表重置序列会撞主键。
			if t.Deleted > 0 {
				var left int64
				if err := tx.Table(t.Name).Count(&left).Error; err != nil {
					return err
				}
				if left == 0 {
					t.SeqReset = resetSequence(tx, t.Name)
				}
			}
		}
		if resetStock {
			res := tx.Exec("UPDATE inv_product_specs SET stock = 0")
			if res.Error != nil {
				return fmt.Errorf("清零规格库存失败：%w", res.Error)
			}
			rep.StockResetRows = res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, t := range rep.Tables {
		rep.TotalDeleted += t.Deleted
	}

	// 事务外做 VACUUM：它在事务内没有意义（看不到其它事务的提交），
	// 而且日志表再大也不值得为省几 MB 上 VACUUM FULL（会持排他锁）。
	for _, t := range rep.Tables {
		if t.Deleted == 0 {
			continue
		}
		if err := db.Exec("VACUUM (ANALYZE) " + t.Name).Error; err != nil {
			// VACUUM 失败不影响清理结果，只影响空间回收速度
			continue
		}
	}
	return rep, nil
}

// isAllowedBizTable 判断一张表在**给定范围**下是否允许被整表清空。
//
// 必须带 scope：日志表只在 database 档放行。之前这里只看「是不是框架表」，
// 于是「日志表属于 scopeLogTables」被当成豁免条件放行了任何档 ——
// 意味着「只清单据」这档如果哪天有人把日志表加进 Tables，校验也会点头。
// 白名单的意义就是**不给档位之间留错位的口子**，所以判据要连档位一起看。
func isAllowedBizTable(table string, scope string) bool {
	if scopeLogTables[table] {
		return scope == ScopeDatabase
	}
	return !frameworkTables[table]
}

func containsTable(tables []string, name string) bool {
	for _, t := range tables {
		if t == name {
			return true
		}
	}
	return false
}

// resetSequence 把表的自增序列重置到 1。没有序列（如 sys_idempotencies 的主键是 key）
// 时静默返回 false。
func resetSequence(db *gorm.DB, table string) bool {
	var seq *string
	if err := db.Raw(`SELECT pg_get_serial_sequence(?, 'id')`, table).Scan(&seq).Error; err != nil || seq == nil {
		return false
	}
	if err := db.Exec(`SELECT setval(?, 1, false)`, *seq).Error; err != nil {
		return false
	}
	return true
}

// BizCleanupGate 是「必须先有备份」这道闸门的判据。
// 返回 (最新备份时间, 是否通过, 说明)。
func BizCleanupGate(latest time.Time, now time.Time, maxAge time.Duration) (bool, string) {
	if latest.IsZero() {
		return false, "还没有任何备份：请先点「立即备份」，没有回滚点不允许清理业务数据"
	}
	age := now.Sub(latest)
	if age > maxAge {
		return false, fmt.Sprintf("最近一份备份已 %s 前：请先点「立即备份」刷新回滚点", age.Round(time.Hour))
	}
	return true, fmt.Sprintf("最近一份备份 %s 前，满足回滚点要求", age.Round(time.Hour))
}
