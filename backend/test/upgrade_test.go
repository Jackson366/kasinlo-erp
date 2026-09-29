// 本文件收录「老库升级路径」用例。
//
// 为什么单独测升级：本项目用 GORM AutoMigrate 增量同步表结构，种子数据也只增不覆盖，
// 因此「新库能跑」和「老库能平滑升上来」是两条不同的路径 —— 新增列、纠正历史脏值
// 这两件事只在老库上才会触发，全新初始化的库永远走不到那些分支。
//
// 本文件在包内按文件名排序最后执行，用例会在共享测试库上做 DDL（删列再加回来），
// 放在最后可以避免影响其它用例；结束时表结构会被 AutoMigrate 恢复原样。
package test

import (
	"net/http"
	"testing"

	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/seed"
)

// columnInfo 描述 information_schema 中的一列。
type columnInfo struct {
	IsNullable string  `gorm:"column:is_nullable"`
	Default    *string `gorm:"column:column_default"`
}

// lookupColumn 查询指定表的列定义；列不存在时返回 nil。
func lookupColumn(t *testing.T, table, column string) *columnInfo {
	t.Helper()
	db := database.DB()

	var cols []columnInfo
	err := db.Raw(`
		SELECT is_nullable, column_default
		FROM information_schema.columns
		WHERE table_name = ? AND column_name = ?`,
		table, column).Scan(&cols).Error
	if err != nil {
		t.Fatalf("查询列定义 %s.%s 失败: %v", table, column, err)
	}
	if len(cols) == 0 {
		return nil
	}
	return &cols[0]
}

// TestUpgradeAddsTokenEpochToLegacyUserTable 老库补列必须成功，且存量行要被填成 0。
//
// 曾经的缺陷：token_epoch 声明为 `not null` 却没给默认值，AutoMigrate 生成的
// ALTER TABLE 在老库（表里有数据）上直接报
// `column "token_epoch" of relation "sys_users" contains null values` (SQLSTATE 23502)，
// 服务起不来。加上 default:0 后 PG 会在补列时把存量行填成 0 ——
// 零值与默认值语义一致，所以这里用默认值兜底是安全的。
func TestUpgradeAddsTokenEpochToLegacyUserTable(t *testing.T) {
	setup(t)
	db := database.DB()

	// 前置：确认库里有存量数据，否则本用例证明不了「补列时有行」这个关键场景
	var before int64
	if err := db.Model(&model.User{}).Count(&before).Error; err != nil {
		t.Fatalf("统计存量用户失败: %v", err)
	}
	if before == 0 {
		t.Fatalf("前置条件不成立：用户表为空，无法验证补列时的存量行回填")
	}

	// 把表退回老结构
	if err := db.Exec(`ALTER TABLE sys_users DROP COLUMN IF EXISTS token_epoch`).Error; err != nil {
		t.Fatalf("模拟老库结构（删列）失败: %v", err)
	}
	if col := lookupColumn(t, "sys_users", "token_epoch"); col != nil {
		t.Fatalf("前置条件不成立：token_epoch 列仍存在")
	}

	// 走真实的迁移入口
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("老库补列失败（这正是升级会踩的坑）: %v", err)
	}

	col := lookupColumn(t, "sys_users", "token_epoch")
	if col == nil {
		t.Fatalf("迁移后 token_epoch 列仍未创建")
	}
	if col.IsNullable != "NO" {
		t.Fatalf("token_epoch 应为 NOT NULL，实际 is_nullable=%s", col.IsNullable)
	}
	if col.Default == nil {
		t.Fatalf("token_epoch 缺少默认值 —— 老库补列时存量行会被填成 NULL 而报 23502，默认值就是为此而加")
	}

	// 存量行必须被回填为 0（等价于「令牌从未被吊销」）
	var bad int64
	if err := db.Raw(`SELECT count(*) FROM sys_users WHERE token_epoch IS DISTINCT FROM 0`).Scan(&bad).Error; err != nil {
		t.Fatalf("检查存量行回填结果失败: %v", err)
	}
	if bad != 0 {
		t.Fatalf("补列后有 %d 行 token_epoch 不是 0 —— 存量用户的令牌会被误判为已吊销", bad)
	}

	// 升级后系统仍可用
	if token := loginAs(t, adminAccount, adminSecret); token == "" {
		t.Fatalf("升级后管理员无法登录")
	}
}

// TestUpgradeRepairsLegacyRoleDataScope 老库里「只读访客」的错误数据范围要在启动时自动纠正。
//
// 启动即可完成升级，意味着运维不需要手工执行 SQL —— 手工步骤一旦被漏掉，
// 表现是「这个角色只能看到自己」，而排查者很难联想到是数据范围的历史脏值。
func TestUpgradeRepairsLegacyRoleDataScope(t *testing.T) {
	setup(t)
	db := database.DB()

	// 还原成历史错误值：早期版本写的就是「仅本人」
	if err := db.Model(&model.Role{}).Where("code = ?", "viewer").
		Update("data_scope", model.DataScopeSelf).Error; err != nil {
		t.Fatalf("模拟老库脏值失败: %v", err)
	}
	if got := viewerDataScope(t); got != model.DataScopeSelf {
		t.Fatalf("前置条件不成立：期望 viewer 数据范围为 %d，实际 %d", model.DataScopeSelf, got)
	}

	// 走真实的启动路径
	if err := seed.Run(); err != nil {
		t.Fatalf("启动升级失败: %v", err)
	}

	if got := viewerDataScope(t); got != model.DataScopeAll {
		t.Fatalf("升级后 viewer 数据范围应为「全部」(%d)，实际 %d —— 老库未被纠正",
			model.DataScopeAll, got)
	}

	// 幂等：再跑一次不应出错，值也不变
	if err := seed.Run(); err != nil {
		t.Fatalf("重复启动升级失败（应幂等）: %v", err)
	}
	if got := viewerDataScope(t); got != model.DataScopeAll {
		t.Fatalf("重复升级后数据范围被改动: %d", got)
	}
}

// TestUpgradeKeepsDeliberateRoleDataScope 管理员主动改过的数据范围不能被启动逻辑覆盖。
//
// 这条与上一条是一对：纠正要「只认历史错误值」，不能变成每次启动都强制回写，
// 否则管理员把「只读访客」调成「本部门」的配置会在重启后莫名失效。
func TestUpgradeKeepsDeliberateRoleDataScope(t *testing.T) {
	setup(t)
	db := database.DB()
	defer func() {
		if err := db.Model(&model.Role{}).Where("code = ?", "viewer").
			Update("data_scope", model.DataScopeAll).Error; err != nil {
			t.Fatalf("还原 viewer 数据范围失败: %v", err)
		}
	}()

	// 「本部门」既不是默认值也不是历史错误值，属于管理员的显式选择
	if err := db.Model(&model.Role{}).Where("code = ?", "viewer").
		Update("data_scope", model.DataScopeDept).Error; err != nil {
		t.Fatalf("设置 viewer 数据范围失败: %v", err)
	}

	if err := seed.Run(); err != nil {
		t.Fatalf("启动升级失败: %v", err)
	}

	if got := viewerDataScope(t); got != model.DataScopeDept {
		t.Fatalf("启动逻辑覆盖了管理员显式设置的数据范围：期望 %d，实际 %d",
			model.DataScopeDept, got)
	}
}

// TestUpgradeKeepsDisabledUserLockedOut 老库升级后，已禁用的账号仍不能登录。
// 升级过程会重置令牌版本号，必须确认它不会顺带把禁用状态一起抹掉。
func TestUpgradeKeepsDisabledUserLockedOut(t *testing.T) {
	setup(t)
	adminToken := loginAs(t, adminAccount, adminSecret)
	db := database.DB()

	// 清理上一轮异常中断可能残留的同名账号，避免用户名列的唯一约束让本用例假失败
	if err := db.Exec(`DELETE FROM sys_users WHERE username = ?`, "upgrade_probe").Error; err != nil {
		t.Fatalf("清理历史探针账号失败: %v", err)
	}

	// 造一个禁用账号
	created := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": "upgrade_probe", "nickname": "升级探针",
		"password": "probe123456", "deptId": 1, "roleIds": []int64{3},
		"status": model.StatusDisabled,
	})
	assertCode(t, created, 0, "创建禁用账号")
	probeID := num(created.obj(t), "id")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(probeID), adminToken, nil)
	}()

	// 前置：确认禁用确实写进去了（StatusDisabled 是零值，曾被 GORM 默认值机制吃掉过）
	if got := lookupUserStatus(t, probeID); got != model.StatusDisabled {
		t.Fatalf("前置条件不成立：新建账号状态应为禁用(%d)，实际 %d", model.StatusDisabled, got)
	}

	// 升级流程
	if err := seed.Run(); err != nil {
		t.Fatalf("启动升级失败: %v", err)
	}

	// 禁用状态必须还在
	if got := lookupUserStatus(t, probeID); got != model.StatusDisabled {
		t.Fatalf("升级后禁用状态被改动：期望 %d，实际 %d", model.StatusDisabled, got)
	}

	// 且仍然登录不进来
	res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "upgrade_probe", "password": "probe123456",
	})
	if res.Code == 0 {
		t.Fatalf("升级后已禁用账号竟然能登录")
	}
}

// viewerDataScope 直接读库取「只读访客」的数据范围。
// 用 Raw 而不是链式 API：这里要的就是绕过软删除作用域之外的任何加工，读到原始值。
func viewerDataScope(t *testing.T) int16 {
	t.Helper()
	var rows []struct {
		DataScope int16 `gorm:"column:data_scope"`
	}
	err := database.DB().Raw(
		`SELECT data_scope FROM sys_roles WHERE code = ? AND deleted_at IS NULL`, "viewer").
		Scan(&rows).Error
	if err != nil {
		t.Fatalf("读取 viewer 数据范围失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("内置角色 viewer 应恰好有 1 条，实际 %d 条", len(rows))
	}
	return rows[0].DataScope
}

// lookupUserStatus 直接读库取用户状态（绕过接口的数据范围限制）。
func lookupUserStatus(t *testing.T, id int64) int16 {
	t.Helper()
	var rows []struct {
		Status int16 `gorm:"column:status"`
	}
	err := database.DB().Raw(
		`SELECT status FROM sys_users WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&rows).Error
	if err != nil {
		t.Fatalf("读取用户 %d 状态失败: %v", id, err)
	}
	if len(rows) != 1 {
		t.Fatalf("用户 %d 应恰好有 1 条，实际 %d 条", id, len(rows))
	}
	return rows[0].Status
}
