// Package seed 负责写入系统初始化数据。
// 所有种子函数都是幂等的：目标表已有数据时直接跳过，不会重复写入。
package seed

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/logger"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/hash"
)

// 默认账号，仅用于首次初始化，上线前请务必修改密码。
const (
	DefaultAdminUser     = "admin"
	DefaultAdminPassword = "admin123456"
	DefaultDemoUser      = "demo"
	DefaultDemoPassword  = "demo123456"
)

// Run 执行全部种子数据初始化。
func Run() error {
	db := database.DB()
	steps := []struct {
		name string
		fn   func(*gorm.DB) error
	}{
		{"部门", seedDepts},
		{"菜单", seedMenus},
		{"角色", seedRoles},
		{"用户", seedUsers},
	}
	for _, s := range steps {
		if err := s.fn(db); err != nil {
			return fmt.Errorf("初始化%s数据失败: %w", s.name, err)
		}
	}
	if err := resetSequences(db); err != nil {
		return err
	}
	authz.InvalidateAll()
	return nil
}

func seedDepts(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.Dept{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	depts := []model.Dept{
		{BaseModel: model.BaseModel{ID: 1}, Name: "总公司", Leader: "管理员", Sort: 1, Status: model.StatusEnabled},
		{BaseModel: model.BaseModel{ID: 2}, ParentID: 1, Name: "技术部", Leader: "张工", Sort: 1, Status: model.StatusEnabled},
		{BaseModel: model.BaseModel{ID: 3}, ParentID: 1, Name: "运营部", Leader: "李经理", Sort: 2, Status: model.StatusEnabled},
	}
	if err := db.Create(&depts).Error; err != nil {
		return err
	}
	logger.Info("已初始化部门数据", zap.Int("count", len(depts)))
	return nil
}

// menuDefs 返回内置菜单定义（含按钮级权限点）。
func menuDefs() []model.Menu {
	on := model.StatusEnabled
	menu := func(id, parent int64, name, path, component, title, icon, perm string, sort int, keepAlive bool) model.Menu {
		return model.Menu{
			BaseModel: model.BaseModel{ID: id}, ParentID: parent, Name: name, Path: path,
			Component: component, Title: title, Icon: icon, Type: model.MenuTypeMenu,
			Permission: perm, Sort: sort, KeepAlive: keepAlive, Status: on,
		}
	}
	button := func(id, parent int64, name, title, perm string, sort int) model.Menu {
		return model.Menu{
			BaseModel: model.BaseModel{ID: id}, ParentID: parent, Name: name, Title: title,
			Type: model.MenuTypeButton, Permission: perm, Sort: sort, Status: on,
		}
	}

	return []model.Menu{
		// 顶层目录与菜单
		{
			BaseModel: model.BaseModel{ID: 1},
			ParentID:  0, Name: "Dashboard", Path: "/dashboard", Component: "dashboard/index",
			Title: "仪表盘", Icon: "Odometer", Type: model.MenuTypeMenu,
			Permission: "dashboard:view", Sort: 1, KeepAlive: true, Status: on,
		},
		{
			BaseModel: model.BaseModel{ID: 10},
			ParentID:  0, Name: "System", Path: "/system", Title: "系统管理",
			Icon: "Setting", Type: model.MenuTypeDir, Redirect: "/system/user", Sort: 2, Status: on,
		},
		menu(11, 10, "SystemUser", "/system/user", "system/user/index", "用户管理", "User", "system:user:list", 1, true),
		menu(12, 10, "SystemRole", "/system/role", "system/role/index", "角色管理", "Key", "system:role:list", 2, true),
		menu(13, 10, "SystemMenu", "/system/menu", "system/menu/index", "菜单管理", "Menu", "system:menu:list", 3, false),
		menu(14, 10, "SystemDept", "/system/dept", "system/dept/index", "部门管理", "OfficeBuilding", "system:dept:list", 4, false),
		menu(15, 10, "SystemLog", "/system/log", "system/log/index", "操作日志", "Document", "system:log:list", 5, false),

		// 用户管理按钮权限
		button(101, 11, "SystemUserCreate", "新增用户", "system:user:create", 1),
		button(102, 11, "SystemUserUpdate", "修改用户", "system:user:update", 2),
		button(103, 11, "SystemUserDelete", "删除用户", "system:user:delete", 3),
		button(104, 11, "SystemUserReset", "重置密码", "system:user:reset", 4),
		button(105, 11, "SystemUserExport", "导出用户", "system:user:export", 5),
		button(106, 11, "SystemUserImport", "导入用户", "system:user:import", 6),

		// 角色管理按钮权限
		button(111, 12, "SystemRoleCreate", "新增角色", "system:role:create", 1),
		button(112, 12, "SystemRoleUpdate", "修改角色", "system:role:update", 2),
		button(113, 12, "SystemRoleDelete", "删除角色", "system:role:delete", 3),
		button(114, 12, "SystemRoleAssign", "分配权限", "system:role:assign", 4),
		button(115, 12, "SystemRoleExport", "导出角色", "system:role:export", 5),

		// 菜单管理按钮权限
		button(121, 13, "SystemMenuCreate", "新增菜单", "system:menu:create", 1),
		button(122, 13, "SystemMenuUpdate", "修改菜单", "system:menu:update", 2),
		button(123, 13, "SystemMenuDelete", "删除菜单", "system:menu:delete", 3),

		// 部门管理按钮权限
		button(131, 14, "SystemDeptCreate", "新增部门", "system:dept:create", 1),
		button(132, 14, "SystemDeptUpdate", "修改部门", "system:dept:update", 2),
		button(133, 14, "SystemDeptDelete", "删除部门", "system:dept:delete", 3),

		// 日志管理按钮权限
		button(141, 15, "SystemLogDelete", "清空日志", "system:log:delete", 1),
		button(149, 15, "SystemLogExport", "导出日志", "system:log:export", 2),
	}
}

// seedMenus 写入内置菜单。
//
// 采用「按 ID 补齐」而非「表为空才写入」：这样后续新增的权限点
// 能自动补进已经初始化过的数据库，不必手工执行 SQL 或重建库。
// 已存在的记录一律不覆盖，避免冲掉管理员对菜单文案的调整。
func seedMenus(db *gorm.DB) error {
	menus := menuDefs()

	// 正常存在的记录
	var activeIDs []int64
	if err := db.Model(&model.Menu{}).Pluck("id", &activeIDs).Error; err != nil {
		return err
	}
	active := make(map[int64]bool, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = true
	}

	// 含软删除的全部 ID：软删除的记录仍然占用主键，
	// 直接插入同一个 ID 会触发唯一冲突。
	var allIDs []int64
	if err := db.Unscoped().Model(&model.Menu{}).Pluck("id", &allIDs).Error; err != nil {
		return err
	}
	occupied := make(map[int64]bool, len(allIDs))
	for _, id := range allIDs {
		occupied[id] = true
	}

	missing := make([]model.Menu, 0, len(menus))
	conflicts := 0
	for _, m := range menus {
		if active[m.ID] {
			continue
		}
		if occupied[m.ID] {
			// ID 被软删除的历史记录占住。此时既不能直接插入（主键冲突），
			// 也不该擅自硬删用户数据，只能显式告警 —— 静默跳过会让权限点凭空消失，
			// 表现为「菜单里看不到、接口 403」，极难排查。
			conflicts++
			logger.Warn("内置菜单 ID 被已软删除的历史记录占用，该权限点未能写入",
				zap.Int64("id", m.ID),
				zap.String("name", m.Name),
				zap.String("permission", m.Permission),
			)
			continue
		}
		missing = append(missing, m)
	}

	if len(missing) > 0 {
		if err := db.Create(&missing).Error; err != nil {
			return err
		}
		logger.Info("已补齐缺失的内置菜单", zap.Int("count", len(missing)))
	}
	if conflicts > 0 {
		return fmt.Errorf(
			"有 %d 个内置菜单因主键 ID 被占用而未能写入，请清理 sys_menus 表中对应的历史数据后重启",
			conflicts)
	}
	return nil
}

// allMenuIDs 返回全部菜单 ID。
func allMenuIDs() []int64 {
	menus := menuDefs()
	ids := make([]int64, 0, len(menus))
	for _, m := range menus {
		ids = append(ids, m.ID)
	}
	return ids
}

func seedRoles(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.Role{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := createBuiltinRoles(db); err != nil {
			return err
		}
	}
	// 无论内置角色是新建还是早已存在，都要补齐缺失的菜单授权：
	// 新增权限点后，已初始化过的数据库同样需要补上授权关系。
	return syncBuiltinRoleMenus(db)
}

func createBuiltinRoles(db *gorm.DB) error {
	roles := []model.Role{
		{
			BaseModel: model.BaseModel{ID: 1}, Name: "超级管理员", Code: authz.SuperRoleCode,
			Sort: 1, Status: model.StatusEnabled, DataScope: model.DataScopeAll,
			IsBuiltin: true, Remark: "内置角色，拥有全部权限",
		},
		{
			BaseModel: model.BaseModel{ID: 2}, Name: "系统管理员", Code: "system_admin",
			Sort: 2, Status: model.StatusEnabled, DataScope: model.DataScopeDeptAndSub,
			Remark: "可维护用户、角色、菜单、部门，不具备删除权限",
		},
		{
			BaseModel: model.BaseModel{ID: 3}, Name: "只读访客", Code: "viewer",
			Sort: 3, Status: model.StatusEnabled, DataScope: model.DataScopeSelf,
			Remark: "仅可查看列表与仪表盘",
		},
	}
	if err := db.Create(&roles).Error; err != nil {
		return err
	}
	logger.Info("已初始化角色数据", zap.Int("roles", len(roles)))
	return nil
}

// builtinRoleGrants 返回各内置角色应有的菜单授权。
//
// 注意：user:export / role:export / log:export 这类「批量取数」权限
// 刻意不授予「只读访客」—— 能查看列表与能批量导出属于两个风险等级。
func builtinRoleGrants() map[int64][]int64 {
	readOnly := []int64{1, 10, 11, 12, 13, 14, 15}
	adminMenus := append([]int64{}, readOnly...)
	adminMenus = append(adminMenus,
		101, 102, 104, 105, 106, // 用户：新增/修改/重置/导出/导入（无删除）
		111, 112, 114, 115, // 角色：新增/修改/分配/导出（无删除）
		121, 122, // 菜单：新增/修改
		131, 132, // 部门：新增/修改
		149, // 日志：导出
	)
	return map[int64][]int64{
		1: allMenuIDs(),
		2: adminMenus,
		3: readOnly,
	}
}

// syncBuiltinRoleMenus 为内置角色补齐缺失的菜单授权。
// 只增不减（管理员对授权做过的裁剪不会被覆盖回去），但会清理两类无意义记录：
// 指向「不存在或已软删除菜单」的孤儿授权，以及菜单已被删除后残留的授权。
func syncBuiltinRoleMenus(db *gorm.DB) error {
	grants := builtinRoleGrants()

	// 现有菜单 ID：授权只能指向真实存在且未删除的菜单，
	// 否则会产生孤儿记录 —— 在内置菜单 ID 被软删除的历史记录占用时就会发生。
	var liveIDs []int64
	if err := db.Model(&model.Menu{}).Pluck("id", &liveIDs).Error; err != nil {
		return err
	}
	live := make(map[int64]bool, len(liveIDs))
	for _, id := range liveIDs {
		live[id] = true
	}

	roleIDs := make([]int64, 0, len(grants))
	for roleID := range grants {
		roleIDs = append(roleIDs, roleID)
	}

	// 清理孤儿授权。它们没有任何有效含义，留着会污染授权统计，
	// 极端情况下（菜单 ID 被复用时）还会让新菜单凭空继承旧权限。
	if len(liveIDs) > 0 {
		res := db.Where("role_id IN ?", roleIDs).
			Where("menu_id NOT IN ?", liveIDs).
			Delete(&model.RoleMenu{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			logger.Warn("已清理内置角色指向不存在菜单的授权记录",
				zap.Int64("count", res.RowsAffected))
		}
	}

	added := 0
	for roleID, menuIDs := range grants {
		var exists int64
		if err := db.Model(&model.Role{}).Where("id = ?", roleID).Count(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			// 内置角色被删除过（正常流程不允许），跳过即可
			continue
		}

		var current []int64
		if err := db.Model(&model.RoleMenu{}).Where("role_id = ?", roleID).
			Pluck("menu_id", &current).Error; err != nil {
			return err
		}
		has := make(map[int64]bool, len(current))
		for _, id := range current {
			has[id] = true
		}

		links := make([]model.RoleMenu, 0, len(menuIDs))
		for _, mid := range menuIDs {
			if live[mid] && !has[mid] {
				links = append(links, model.RoleMenu{RoleID: roleID, MenuID: mid})
			}
		}
		if len(links) == 0 {
			continue
		}
		if err := db.Create(&links).Error; err != nil {
			return err
		}
		added += len(links)
	}

	if added > 0 {
		logger.Info("已为内置角色补齐菜单授权", zap.Int("count", added))
	}
	return nil
}

func seedUsers(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	adminPwd, err := hash.Password(DefaultAdminPassword)
	if err != nil {
		return err
	}
	demoPwd, err := hash.Password(DefaultDemoPassword)
	if err != nil {
		return err
	}
	dept1, dept2 := int64(1), int64(2)

	users := []model.User{
		{
			BaseModel: model.BaseModel{ID: 1}, Username: DefaultAdminUser, Password: adminPwd,
			Nickname: "超级管理员", Email: "admin@example.com", DeptID: &dept1,
			Status: model.StatusEnabled, Remark: "系统内置管理员",
		},
		{
			BaseModel: model.BaseModel{ID: 2}, Username: DefaultDemoUser, Password: demoPwd,
			Nickname: "演示账号", Email: "demo@example.com", DeptID: &dept2,
			Status: model.StatusEnabled, Remark: "只读演示账号",
		},
	}
	links := []model.UserRole{
		{UserID: 1, RoleID: 1},
		{UserID: 2, RoleID: 3},
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&users).Error; err != nil {
			return err
		}
		if err := tx.Create(&links).Error; err != nil {
			return err
		}
		logger.Info("已初始化用户数据",
			zap.String("admin", DefaultAdminUser),
			zap.String("demo", DefaultDemoUser),
		)
		return nil
	})
}

// MenuIDFloor 是用户自建菜单的起始 ID。
//
// 内置菜单使用硬编码 ID，约定占用 1-999 这一段；自增生成的菜单必须从 1000 起，
// 两者不能混在一起。否则新建菜单会占掉内置菜单预留的 ID，此后新增的权限点
// 就再也补不进来了 —— 软删除的记录会一直占着那个主键，
// 而这类冲突既不会报错也不会留下线索。这条约束是踩过坑之后才加上的。
const MenuIDFloor = 1000

// resetSequences 同步自增序列，避免手工指定 ID 后新增数据主键冲突。
func resetSequences(db *gorm.DB) error {
	targets := []struct {
		table string
		floor int64
	}{
		{"sys_users", 1},
		{"sys_roles", 1},
		{"sys_depts", 1},
		{"sys_menus", MenuIDFloor},
	}
	for _, t := range targets {
		stmt := fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s','id'), GREATEST(COALESCE((SELECT MAX(id) FROM %s), 1) + 1, %d), false)`,
			t.table, t.table, t.floor,
		)
		if err := db.Exec(stmt).Error; err != nil {
			// 表不存在时忽略（例如尚未启用菜单表）
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return fmt.Errorf("重置 %s 自增序列失败: %w", t.table, err)
		}
	}
	return nil
}
