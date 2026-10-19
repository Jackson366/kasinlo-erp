// Package seed 负责写入系统初始化数据。
// 所有种子函数都是幂等的：目标表已有数据时直接跳过，不会重复写入。
package seed

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/logger"
	"nova-erp/internal/model"
	"nova-erp/internal/module"
	"nova-erp/internal/pkg/hash"
)

// 默认账号，仅用于首次初始化，上线前请务必修改密码。
const (
	DefaultAdminUser     = "admin"
	DefaultAdminPassword = "admin123456"
	DefaultDemoUser      = "demo"
	DefaultDemoPassword  = "demo123456"
)

// IsDefaultPassword 判断该账号是否仍在用内置的默认口令。
//
// 供登录流程在成功后告警：默认口令是公开的，只要部署者没改密，
// 任何知道口令的人都能以 admin 登录。代码层面目前没有「首次登录强制改密」，
// 所以至少要在每次默认口令登录时留一条 WARN，让运维在日志里看得到这个隐患。
func IsDefaultPassword(username, password string) bool {
	switch username {
	case DefaultAdminUser:
		return password == DefaultAdminPassword
	case DefaultDemoUser:
		return password == DefaultDemoPassword
	}
	return false
}

// Run 执行全部种子数据初始化。
//
// 前置条件是模块清单已登记（由 bootstrap.Init 里的 module.Build 完成）。
// 登记表为空时直接报错而不是静默跳过：静默跳过只会写出一张空菜单表，
// 而服务照样能起来，问题要等到「侧边栏什么都没有」时才被发现。
func Run() error {
	if len(module.All()) == 0 {
		return fmt.Errorf("模块登记表为空：请先经 bootstrap.Init（会调用 module.Build）再播种")
	}
	db := database.DB()
	steps := []struct {
		name string
		fn   func(*gorm.DB) error
	}{
		{"部门", seedDepts},
		{"菜单", seedMenus},
		{"角色", seedRoles},
		{"用户", seedUsers},
		{"进销存", seedInv},
		// 演示档案补齐：老库由单规格结构迁移而来，多计量单位示例需要单独补，
		// 只补缺、不覆盖（见 seedInvProductTopUp 的说明）
		{"进销存档案补齐", seedInvProductTopUp},
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

// seedMenus 按模块清单补齐内置菜单。
//
// 幂等键是「权限码」（有权限码的节点）或「路由名」（纯目录节点），不再是手工
// 分配的菜单 ID —— 主键交给数据库自增，本来就不该由清单操心。这也顺带解决了
// 老库的平滑迁移：历史记录带着同样的权限码/路由名，照样被认出来并复用，
// 既不重复插入，也不会改动它原有的 ID 与角色授权关系。
func seedMenus(db *gorm.DB) error {
	added, skipped := 0, 0
	for _, m := range module.All() {
		for i, node := range m.Menus {
			a, s, err := insertNode(db, node, 0, nodeSort(node, i))
			if err != nil {
				return err
			}
			added += a
			skipped += s
		}
	}
	if added > 0 {
		logger.Info("已补齐缺失的内置菜单", zap.Int("count", added))
	}
	if skipped > 0 {
		logger.Warn("有内置菜单因曾被删除而未补齐，如确需恢复请手工处理",
			zap.Int("count", skipped))
	}
	if err := retireModules(db); err != nil {
		return err
	}
	// 合并迁移要放在 ensureNode 之后：新库里 InvProduct 已经带上了分类的按钮权限点，
	// 老库里则靠这一步把旧 InvCategory 的子节点移挂过来再软删它。
	// 顺序反了会在新库上找不到 InvCategory（本就不是问题），老库上却会漏移子节点。
	if err := mergeMenuNodes(db); err != nil {
		return err
	}
	return ensureLandingOrder(db)
}

// retiredModules 登记**已下线**的模块。
//
// 模块从 assembly.Modules() 摘掉之后，老库里它的菜单、按钮权限点和角色授权
// 都还躺着：侧边栏会多一块点不动的区域，`/role` 授权列表里也还看得见。
// 在这里登记命名空间，播种时就会把它的菜单软删除、并清掉指向这些菜单的授权 ——
// 删模块因此对存量部署也是干净的，不需要任何人手工执行 SQL。
//
// 删除用软删除（deleted_at）：既留了恢复的余地，也保证「模块哪天回来了」
// 不会和残留记录抢唯一索引（name / permission 都是部分唯一索引，
// WHERE deleted_at IS NULL，软删后即可复用，见 model.Menu 注释）。
var retiredModules = []struct {
	Namespace string   // 权限码命名空间，如 dashboard
	Names     []string // 无权限码的节点按路由名兜底（目录节点没有权限码）
}{
	{Namespace: "dashboard", Names: []string{"Dashboard"}},
}

// mergedMenuNodes 登记**被合并进别的菜单**的页面节点。
//
// 与 retiredModules 的区别：那个是「整个模块下线」，这个是「模块还在，
// 只是其中一个页面被并进另一个页面」。商品分类页并进商品档案页（左侧树）就是这种。
//
// 只软删父节点会留下一个很难发现的问题：它的**按钮子节点还挂在被删节点下**，
// 而 ensureNode 按幂等键「命中即沿用、不重建」，那些按钮于是永远挂在一条
// 已软删的菜单底下 —— 角色授权列表里看不见，前端按菜单树渲染时也渲染不出来，
// 权限等于白给。所以这里要把子节点**移挂**到新父节点，再软删旧父节点。
var mergedMenuNodes = []struct {
	Name   string // 被合并掉的页面节点（幂等键 = 路由名）
	MoveTo string // 子节点移挂到的目标页面节点
}{
	{Name: "InvCategory", MoveTo: "InvProduct"},
}

// mergeMenuNodes 执行上面的合并迁移（幂等）。
func mergeMenuNodes(db *gorm.DB) error {
	for _, m := range mergedMenuNodes {
		var old model.Menu
		if err := db.Where("name = ?", m.Name).First(&old).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // 新库 / 已迁过
			}
			return err
		}
		var target model.Menu
		if err := db.Where("name = ?", m.MoveTo).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("合并菜单失败：目标节点 %s 不存在", m.MoveTo)
			}
			return err
		}
		// 子节点移挂：授权按 menu_id 关联，换父节点不影响任何授权
		var moved int64
		if err := db.Model(&model.Menu{}).
			Where("parent_id = ? AND type = ?", old.ID, model.MenuTypeButton).
			Update("parent_id", target.ID).Error; err != nil {
			return err
		}
		moved = db.RowsAffected
		// 再软删旧父节点（先清它的授权，避免孤儿行破坏「超管授权数 == 存活菜单数」）
		if err := db.Where("menu_id = ?", old.ID).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if err := db.Delete(&model.Menu{}, old.ID).Error; err != nil {
			return err
		}
		logger.Info("菜单已合并",
			zap.String("merged", m.Name),
			zap.String("into", m.MoveTo),
			zap.Int64("childrenMoved", moved))
	}
	return nil
}

// retireModules 清理已下线模块的菜单与角色授权。
func retireModules(db *gorm.DB) error {
	for _, r := range retiredModules {
		conds := db.Where("permission LIKE ?", r.Namespace+":%")
		if len(r.Names) > 0 {
			conds = db.Where(conds.Or("name IN ?", r.Names))
		}
		var menus []model.Menu
		if err := db.Where(conds).Find(&menus).Error; err != nil {
			return err
		}
		if len(menus) == 0 {
			continue
		}
		ids := make([]int64, 0, len(menus))
		for _, m := range menus {
			ids = append(ids, m.ID)
		}
		// 先数后删：Count 放在 Delete 之后永远是 0
		var grants int64
		if err := db.Model(&model.RoleMenu{}).Where("menu_id IN ?", ids).Count(&grants).Error; err != nil {
			return err
		}
		// 先清授权再软删菜单：顺序反了的话，角色授权表会留下指向
		// 已软删菜单的孤儿行，而「超管授权数 == 存活菜单数」这个
		// 不变量就是被这些行破坏的（见 docs/DATABASE.md 排查小节）
		if err := db.Where("menu_id IN ?", ids).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if err := db.Where("id IN ?", ids).Delete(&model.Menu{}).Error; err != nil {
			return err
		}
		logger.Info("已下线模块的菜单与授权已清理",
			zap.String("namespace", r.Namespace),
			zap.Int("menus", len(ids)),
			zap.Int64("grants", grants))
	}
	return nil
}

// ensureLandingOrder 校准顶层入口结构，保证「数据概览」是顶级菜单且排最前。
//
// 前端用「菜单树里第一个可访问的叶子」当登录落地页（AGENTS.md §5.62）。
// 全新库由模块清单直接建好，无需校准；这里兜的是**老库升级**：
//   - v1.0 里「数据概览」是「进销存」目录的子项（parent_id ≠ 0），
//     升级后要被提到顶级，否则落地页推导不出它；
//   - 顶层 sort 平局按 id ASC 决胜，老库里先建的节点会赢，
//     所以显式把数据概览压到最前，其余节点一律不动 ——
//     管理员在菜单管理里对其它顺序的调整仍然是权威。
func ensureLandingOrder(db *gorm.DB) error {
	var overview model.Menu
	err := db.Where("name = ? AND deleted_at IS NULL", "InvOverview").Take(&overview).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // 这套部署没有进销存菜单，无需校准
	}
	if err != nil {
		return err
	}

	if overview.ParentID != 0 {
		// 从「进销存」目录下提取为顶级菜单
		if err := db.Model(&model.Menu{}).Where("id = ?", overview.ID).
			Updates(map[string]any{"parent_id": 0, "sort": 1}).Error; err != nil {
			return err
		}
		logger.Info("已将「数据概览」提升为顶级菜单（老库结构迁移）",
			zap.Int64("id", overview.ID))
	}
	// 「进销存」目录退到第二位：不覆盖管理员显式调大的值，只在小了的时候抬
	if err := db.Model(&model.Menu{}).Where("name = ? AND parent_id = 0", "Inv").
		Where("sort < ?", 2).Update("sort", 2).Error; err != nil {
		return err
	}
	return nil
}

// insertNode 递归补齐一个节点及其子节点，返回 (新增条数, 跳过条数)。
//
// 必须自顶向下插入：子节点的 parent_id 要用父节点刚落库的主键，
// 而主键现在由数据库分配，清单里拿不到。
func insertNode(db *gorm.DB, node module.MenuNode, parentID int64, sort int) (int, int, error) {
	id, created, omitted, err := ensureNode(db, node, parentID, sort)
	if err != nil {
		return 0, 0, err
	}
	if omitted {
		// 父节点曾被删除，整棵子树一起跳过：挂到一个不存在的父级下，
		// 子节点在侧边栏里就会散架，比不创建更难排查
		return 0, countNodes(node), nil
	}

	added, skipped := 0, 0
	if created {
		added = 1
	}
	for i, child := range node.Children {
		a, s, err := insertNode(db, child, id, nodeSort(child, i))
		if err != nil {
			return added, skipped, err
		}
		added += a
		skipped += s
	}
	return added, skipped, nil
}

// ensureNode 保证节点存在，返回 (菜单 ID, 是否新建, 是否跳过, 错误)。
func ensureNode(db *gorm.DB, node module.MenuNode, parentID int64, sort int) (int64, bool, bool, error) {
	var existing model.Menu
	err := keyQuery(db.Model(&model.Menu{}), node).Take(&existing).Error
	if err == nil {
		// 已存在：沿用原 ID，字段一律不覆盖 ——
		// 管理员对标题、图标、排序的调整必须保留
		return existing.ID, false, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, false, err
	}

	// 未被软删除的记录里没有，再看一眼是不是被管理员删掉了。
	// 直接重建会让「删掉的菜单下次重启又冒出来」，与旧版一致：告警并跳过。
	var removed model.Menu
	err = keyQuery(db.Unscoped().Model(&model.Menu{}), node).Take(&removed).Error
	if err == nil {
		logger.Warn("内置菜单已被删除，跳过该节点及其子节点",
			zap.String("name", node.Name),
			zap.String("title", node.Title),
			zap.String("permission", node.Permission),
		)
		return 0, false, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, false, err
	}

	row := model.Menu{
		ParentID:   parentID,
		Name:       node.Name,
		Path:       node.Path,
		Component:  node.Component,
		Redirect:   node.Redirect,
		Title:      node.Title,
		Icon:       node.Icon,
		Type:       node.TypeOf(),
		Permission: node.Permission,
		Sort:       sort,
		Hidden:     node.Hidden,
		KeepAlive:  node.KeepAlive,
		IsExternal: node.IsExternal,
		Status:     model.StatusEnabled,
	}
	if err := db.Create(&row).Error; err != nil {
		return 0, false, false, fmt.Errorf("写入内置菜单 %q 失败: %w", node.Name, err)
	}
	return row.ID, true, false, nil
}

// keyQuery 按幂等键定位菜单：有权限码用权限码，否则用路由名。
func keyQuery(db *gorm.DB, node module.MenuNode) *gorm.DB {
	if node.Permission != "" {
		return db.Where("permission = ?", node.Permission)
	}
	return db.Where("name = ?", node.Name)
}

// nodeSort 取节点排序：声明了就用声明的，否则按同级声明顺序递增。
// 于是「写清单」不必再为每个节点算一个数字，声明顺序就是展示顺序。
func nodeSort(node module.MenuNode, index int) int {
	if node.Sort > 0 {
		return node.Sort
	}
	return index + 1
}

// countNodes 统计一棵子树的节点数，用于统计被跳过的条数。
func countNodes(node module.MenuNode) int {
	total := 1
	for _, child := range node.Children {
		total += countNodes(child)
	}
	return total
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
	if err := syncBuiltinRoleMenus(db); err != nil {
		return err
	}
	// 同理，历史库里错误的角色数据范围也要在启动时自动纠正，
	// 否则已上线的库必须手工执行 SQL 才能升级。
	return syncBuiltinRoleDataScope(db)
}

// historicalRoleDataFixes 记录内置角色历史上曾是「错误值」的数据范围。
//
// 只在当前值恰好等于 from 时才纠正为 to：
//   - 幂等：重复启动不会产生额外写入；
//   - 不越权：管理员若已把这个角色显式改成别的范围（例如「本部门」），
//     说明是有意为之，不会被启动逻辑覆盖回去。
//
// 为什么需要它：早期版本把「只读访客」写成「仅本人」(4)。当时 data_scope
// 只被计算、入库、展示，从未参与任何查询，这个自相矛盾的值因此一直没暴露；
// 数据范围真正生效后，「只读访客」会连用户列表都只剩自己一行，
// 与它「只读浏览各列表」的用途完全相悖。
var historicalRoleDataFixes = []struct {
	code   string
	from   int16
	to     int16
	reason string
}{
	{
		code: "viewer", from: model.DataScopeSelf, to: model.DataScopeAll,
		reason: "只读访客的用途是浏览各列表，将其数据范围纠正为「全部」",
	},
}

// syncBuiltinRoleDataScope 纠正历史库中内置角色的错误数据范围（幂等）。
func syncBuiltinRoleDataScope(db *gorm.DB) error {
	for _, fix := range historicalRoleDataFixes {
		res := db.Model(&model.Role{}).
			Where("code = ? AND data_scope = ?", fix.code, fix.from).
			Update("data_scope", fix.to)
		if res.Error != nil {
			return fmt.Errorf("纠正角色 %s 的数据范围失败: %w", fix.code, res.Error)
		}
		if res.RowsAffected > 0 {
			logger.Warn("已纠正内置角色的历史数据范围",
				zap.String("role", fix.code),
				zap.Int16("from", fix.from),
				zap.Int16("to", fix.to),
				zap.String("reason", fix.reason),
			)
		}
	}
	return nil
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
			// 数据范围必须是「全部」。这个角色的用途是只读地浏览各个列表，
			// 若配成「仅本人」，它会连用户列表都只剩自己一行 —— 与 Remark 自相矛盾。
			// 数据范围此前从未参与任何查询，这个矛盾因此一直没暴露；现在它真正生效了。
			BaseModel: model.BaseModel{ID: 3}, Name: "只读访客", Code: "viewer",
			Sort: 3, Status: model.StatusEnabled, DataScope: model.DataScopeAll,
			Remark: "仅可查看列表与仪表盘，不可导出",
		},
	}
	if err := db.Create(&roles).Error; err != nil {
		return err
	}
	logger.Info("已初始化角色数据", zap.Int("roles", len(roles)))
	return nil
}

// frameworkNamespaces 是「内置角色自动授权」覆盖的权限命名空间。
//
// 这里同时包含框架自带模块与随本项目一起交付的业务模块（inv 进销存）。
// 为什么不把 inv 排除在外：本项目就是一套进销存后台，进销存是它的主线功能，
// 而非「临时插入的敏感业务」——把整条业务线从内置角色里隐去，会让
// 系统管理员登录后看不到任何业务菜单。若将来引入与主线无关的敏感模块，
// 请不要加进这里，而是到「角色管理」里手工分配。
//
// ops（系统运维）也在列：备份、清理、升级都是「维护框架」的一部分，
// 与清空日志同类（系统管理员同样拿不到 *:delete，而 ops 五个权限点里
// 没有一个是 :delete）。若现场要求把升级权单独收给超管，
// 从这里摘掉 ops 即可 —— 菜单与权限点仍在，只是不再自动授权。
var frameworkNamespaces = []string{"system", "profile", "auth", "inv", "ops"}

// grantRule 描述一个内置角色应有的菜单授权。
type grantRule struct {
	roleID     int64
	name       string
	reason     string
	namespaces []string                               // 为空表示不限命名空间
	match      func(menuType, permission string) bool // 按节点语义判定
}

// grantRules 是内置角色的授权规则。
//
// 不再抄一份菜单 ID 清单。原先那份（1,10,11,…,101,102,104,…）有两个毛病：
// 读不出意图，而且必须与 menuDefs() 里的 ID 手工保持对齐 ——
// 两边改歪一处就是静默的权限错配。改成按节点语义判定后，规则本身就是文档。
//
// 下面的规则与旧清单逐条等价：
//   - 只读访客   = 所有非按钮节点（即各列表菜单）
//   - 系统管理员 = 除 *:delete 外的全部节点
//
// 刻意不授予「只读访客」任何导出/导入：能查看列表与能批量取数
// 属于两个风险等级。
var grantRules = []grantRule{
	{
		roleID: 1, name: "超级管理员",
		reason: "拥有全部菜单与按钮权限",
		match:  func(string, string) bool { return true },
	},
	{
		roleID: 2, name: "系统管理员",
		reason:     "可维护框架模块，但不具备任何删除类权限（清空日志同属删除）",
		namespaces: frameworkNamespaces,
		match:      func(_, permission string) bool { return !strings.HasSuffix(permission, ":delete") },
	},
	{
		roleID: 3, name: "只读访客",
		reason:     "只浏览各列表；导出/导入属于批量取数，故不授予任何按钮权限",
		namespaces: frameworkNamespaces,
		match:      func(menuType, _ string) bool { return menuType != model.MenuTypeButton },
	},
}

// covers 判断某个菜单是否落在该规则的授权范围内。
func (r grantRule) covers(menuType, permission string) bool {
	if !r.match(menuType, permission) {
		return false
	}
	ns := namespaceOf(permission)
	if ns == "" {
		// 没有权限码的节点只可能是目录：它只是分组，授予它不放大任何权限；
		// 而不授予的话，其下的子菜单会失去父级、在侧边栏里散架
		return true
	}
	if len(r.namespaces) == 0 {
		return true
	}
	for _, allowed := range r.namespaces {
		if allowed == ns {
			return true
		}
	}
	return false
}

// namespaceOf 取权限码的命名空间段，如 system:user:list -> system。
func namespaceOf(permission string) string {
	if permission == "" {
		return ""
	}
	if i := strings.Index(permission, ":"); i > 0 {
		return permission[:i]
	}
	return permission
}

// resolveGrants 在当前库里真实存在的菜单上求解授权规则，得到 角色 -> 菜单 ID。
//
// 「求解」而不是「硬编码」的意义：老库的 ID、新库的 ID、管理员重建过的 ID
// 各不相同，但语义相同。规则只在语义层描述，落库前才解析成具体 ID。
func resolveGrants(db *gorm.DB) (map[int64][]int64, error) {
	var menus []model.Menu
	if err := db.Model(&model.Menu{}).Find(&menus).Error; err != nil {
		return nil, err
	}
	grants := make(map[int64][]int64, len(grantRules))
	for _, rule := range grantRules {
		ids := make([]int64, 0, len(menus))
		for _, m := range menus {
			if rule.covers(m.Type, m.Permission) {
				ids = append(ids, m.ID)
			}
		}
		grants[rule.roleID] = ids
	}
	return grants, nil
}

// syncBuiltinRoleMenus 为内置角色补齐缺失的菜单授权。
// 只增不减（管理员对授权做过的裁剪不会被覆盖回去），但会清理两类无意义记录：
// 指向「不存在或已软删除菜单」的孤儿授权，以及菜单已被删除后残留的授权。
func syncBuiltinRoleMenus(db *gorm.DB) error {
	grants, err := resolveGrants(db)
	if err != nil {
		return err
	}

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

func resetSequences(db *gorm.DB) error {
	// 内置菜单不再使用硬编码 ID（清单只声明权限码与路由名，主键交给自增），
	// 于是「用户自建菜单必须从 1000 起」那条约束随之作废 —— 它当初存在的唯一
	// 理由是避免自增主键侵占内置菜单预留的 1-999 段，现在没有预留段了。
	for _, table := range []string{"sys_users", "sys_roles", "sys_depts", "sys_menus"} {
		stmt := fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s','id'), GREATEST(COALESCE((SELECT MAX(id) FROM %s), 1) + 1, 1), false)`,
			table, table,
		)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("重置 %s 自增序列失败: %w", table, err)
		}
	}
	return nil
}
