package authz

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
)

// SuperRoleCode 是超级管理员角色标识，拥有全部权限。
const SuperRoleCode = "super_admin"

// SessionTTL 是身份缓存时长，超时后重新从数据库加载。
const SessionTTL = 30 * time.Second

// Identity 是一次请求的身份上下文。
type Identity struct {
	UserID    int64    `json:"userId"`
	Username  string   `json:"username"`
	Nickname  string   `json:"nickname"`
	Avatar    string   `json:"avatar"`
	DeptID    *int64   `json:"deptId"`
	RoleIDs   []int64  `json:"roleIds"`
	RoleCodes []string `json:"roleCodes"`
	DataScope int16    `json:"dataScope"`
	IsSuper   bool     `json:"isSuper"`
	// TokenEpoch 是签发令牌时要写进 Claims 的版本号，不对外输出。
	TokenEpoch  int64               `json:"-"`
	Permissions map[string]struct{} `json:"-"`
}

// HasAnyPerm 判断是否拥有任一权限。
func (i *Identity) HasAnyPerm(codes ...string) bool {
	if i == nil {
		return false
	}
	if i.IsSuper {
		return true
	}
	for _, c := range codes {
		if _, ok := i.Permissions[c]; ok {
			return true
		}
	}
	return false
}

// PermList 返回权限码列表，便于前端使用。
func (i *Identity) PermList() []string {
	out := make([]string, 0, len(i.Permissions))
	for k := range i.Permissions {
		out = append(out, k)
	}
	return out
}

type cacheEntry struct {
	identity *Identity
	expireAt time.Time
	version  int64
}

var (
	mu      sync.RWMutex
	entries = make(map[int64]cacheEntry)
	// version 用原子变量维护，不能用 RWMutex 保护：
	// 否则「已持有写锁时再读取版本号」会触发 RWMutex 自死锁。
	version atomic.Int64
)

func init() { version.Store(1) }

// Load 加载身份信息（带缓存）。用户不存在或已禁用返回错误。
func Load(userID int64) (*Identity, error) {
	now := time.Now()
	ver := version.Load()

	mu.RLock()
	e, ok := entries[userID]
	mu.RUnlock()
	if ok && now.Before(e.expireAt) && e.version == ver {
		return e.identity, nil
	}

	identity, err := build(userID)
	if err != nil {
		return nil, err
	}

	mu.Lock()
	entries[userID] = cacheEntry{identity: identity, expireAt: now.Add(SessionTTL), version: ver}
	mu.Unlock()
	return identity, nil
}

// Invalidate 清除单个用户的身份缓存（用户、角色、菜单变更时调用）。
func Invalidate(userID int64) {
	mu.Lock()
	delete(entries, userID)
	mu.Unlock()
}

// InvalidateAll 清空全部缓存（角色/菜单授权变更时调用）。
func InvalidateAll() {
	version.Add(1)
	mu.Lock()
	entries = make(map[int64]cacheEntry)
	mu.Unlock()
}

// build 从数据库加载用户及其角色、权限集合。
func build(userID int64) (*Identity, error) {
	db := database.DB()
	var user model.User
	if err := db.Preload("Roles", "status = ?", model.StatusEnabled).
		First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.ErrUnauthorized, "用户不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if user.Status != model.StatusEnabled {
		return nil, errcode.New(errcode.ErrAccountDisabled, "")
	}

	id := &Identity{
		UserID:      user.ID,
		Username:    user.Username,
		Nickname:    user.Nickname,
		Avatar:      user.Avatar,
		DeptID:      user.DeptID,
		TokenEpoch:  user.TokenEpoch,
		Permissions: map[string]struct{}{},
		DataScope:   model.DataScopeSelf,
	}

	if len(user.Roles) == 0 {
		return id, nil
	}

	roleIDs := make([]int64, 0, len(user.Roles))
	minScope := model.DataScopeSelf
	for _, r := range user.Roles {
		roleIDs = append(roleIDs, r.ID)
		id.RoleCodes = append(id.RoleCodes, r.Code)
		if r.Code == SuperRoleCode {
			id.IsSuper = true
		}
		// 数据范围取最宽松者
		if r.DataScope < minScope {
			minScope = r.DataScope
		}
	}
	id.RoleIDs = roleIDs
	id.DataScope = minScope

	// 汇总角色关联的权限点
	var perms []string
	if err := db.Model(&model.Menu{}).
		Joins("JOIN sys_role_menus rm ON rm.menu_id = sys_menus.id").
		Where("rm.role_id IN ?", roleIDs).
		Where("sys_menus.status = ? AND sys_menus.permission <> ''", model.StatusEnabled).
		Where("sys_menus.deleted_at IS NULL").
		Distinct().
		Pluck("sys_menus.permission", &perms).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, p := range perms {
		id.Permissions[p] = struct{}{}
	}
	return id, nil
}

// ---------------- 可授予角色（防垂直越权） ----------------
//
// 背景：角色的「分配」长期只校验角色是否存在，不校验调用者是否有权授予。
// 于是只拥有 system:user:create 的系统管理员可以带 roleIds:[1]（超管）建号，
// 或者把自己改成超管 —— 一次请求即可完成提权。
//
// 判定规则（超管不受限）：
//  1. 超管角色（super_admin）是「绕过全部权限校验」的特权角色，仅超管可授予；
//  2. 其余角色要求其权限码集合 ⊆ 调用者自身权限集合 —— 即「不能授予自己没有的权限」。
//
// 注意这是基于「角色权限」的收敛，并不能防住「先造一个高权限角色、再授予」的路径，
// 后者需要按菜单维度限制 role:create/assign，属于另一层治理。

// rolePermissions 返回给定角色携带的权限码，按 role_id 分组。
// 过滤条件必须与 build() 汇总权限时的口径一致（仅启用、未删除、permission 非空），
// 否则会把「对身份生效的权限」和「用于校验的权限」算成两套。
func rolePermissions(roleIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(roleIDs))
	if len(roleIDs) == 0 {
		return out, nil
	}
	type row struct {
		RoleID     int64
		Permission string
	}
	var rows []row
	if err := database.DB().Model(&model.Menu{}).
		Select("rm.role_id AS role_id, sys_menus.permission AS permission").
		Joins("JOIN sys_role_menus rm ON rm.menu_id = sys_menus.id").
		Where("rm.role_id IN ?", roleIDs).
		Where("sys_menus.status = ? AND sys_menus.deleted_at IS NULL AND sys_menus.permission <> ''",
			model.StatusEnabled).
		Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, r := range rows {
		out[r.RoleID] = append(out[r.RoleID], r.Permission)
	}
	return out, nil
}

// scopeText 返回数据范围的中文描述，用于拒绝原因。
func scopeText(scope int16) string {
	switch scope {
	case 1:
		return "全部数据"
	case 2:
		return "本部门"
	case 3:
		return "本部门及以下"
	case 4:
		return "仅本人"
	default:
		return "未知"
	}
}

// bannedReason 返回「该角色不可授予」的原因；可授予时返回空串。
//
// 判定有两条独立的轴，缺一条就会留下提权通道：
//  1. 权限码：目标角色携带的每个权限码，调用者都必须自己拥有；
//  2. 数据范围：目标角色的范围不得比调用者更宽。
//
// 只判权限码是不够的：一个「不挂任何菜单 + dataScope=1」的角色，
// 权限码集合为空 → 第一条轴全部通过，于是任何部门管理员都能把它授出去，
// 受赠者直接获得全库数据可见性（1 全部 2 本部门 3 本部门及以下 4 仅本人，数值越小越宽）。
func bannedReason(actor *Identity, r model.Role, perms []string) string {
	if r.Code == SuperRoleCode {
		return "只有超级管理员可以授予「超级管理员」角色"
	}
	// 数值越小范围越宽，因此「比调用者更宽」等价于「小于调用者」
	if actor.DataScope > 0 && r.DataScope > 0 && r.DataScope < actor.DataScope {
		return fmt.Sprintf("无权授予角色「%s」：它的数据范围（%s）比你的（%s）更宽",
			r.Name, scopeText(r.DataScope), scopeText(actor.DataScope))
	}
	for _, p := range perms {
		if _, ok := actor.Permissions[p]; !ok {
			return fmt.Sprintf("无权授予角色「%s」：它包含你自身不具备的权限（%s）", r.Name, p)
		}
	}
	return ""
}

// loadRolesForGrant 按 ID 读取授予判定所需的字段。
//
// 为什么不直接用调用方传进来的角色对象：判定依赖 code 与 data_scope，
// 而调用方常常只 Select("id","name","code")——GORM 不会把未选中的列填成「真值」，
// data_scope 会是零值，于是「数据范围不得更宽」这条判定会被静默跳过。
// 授���判定的字段必须由本包自己从库里取，不能指望调用方选全。
func loadRolesForGrant(ids []int64) ([]model.Role, error) {
	roles := make([]model.Role, 0, len(ids))
	if len(ids) == 0 {
		return roles, nil
	}
	if err := database.DB().Select("id", "name", "code", "data_scope").
		Where("id IN ?", ids).Find(&roles).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return roles, nil
}

// EnsureGrantable 校验 actor 能否授予这些角色（roles 至少要带 ID）。
//
// 传入的角色对象只用于取 ID，实际判定一律以库中数据为准，理由见 loadRolesForGrant。
func EnsureGrantable(actor *Identity, roles []model.Role) error {
	if actor == nil || actor.IsSuper || len(roles) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(roles))
	for _, r := range roles {
		ids = append(ids, r.ID)
	}
	full, err := loadRolesForGrant(ids)
	if err != nil {
		return err
	}
	permsOf, err := rolePermissions(ids)
	if err != nil {
		return err
	}
	for _, r := range full {
		if reason := bannedReason(actor, r, permsOf[r.ID]); reason != "" {
			return errcode.New(errcode.ErrForbidden, reason)
		}
	}
	return nil
}

// EnsureRoleMenusGrantable 校验 actor 是否有权把这些菜单的权限授予一个角色。
//
// 为什么与 EnsureGrantable 并列存在：EnsureGrantable 管的是「把角色给人」，
// 而修改角色的菜单相当于「先改角色、再批量授予」，是绕过前者的另一条路。
// 两处判定口径必须一致：都只认「启用、未删除、permission 非空」的菜单。
// EnsurePermissionGrantable 校验 actor 是否能把菜单的 permission 设为给定权限码。
//
// 菜单上的 permission 就是授权单元：把它改成某个码，所有挂着这个菜单的角色
// 的持有者在 authz 缓存失效后立刻获得该权限——所以「改 permission」等价于一次
// 批量授权，必须与角色授予同一条红线：非超管只能设置自己持有的权限码（子集校验）。
// 与 EnsureRoleMenusGrantable 同口径：permission 是整串单码，不做逗号拆分。
func EnsurePermissionGrantable(actor *Identity, permission string) error {
	if actor == nil || actor.IsSuper || permission == "" {
		return nil
	}
	if _, ok := actor.Permissions[permission]; !ok {
		return errcode.Newf(errcode.ErrForbidden,
			"不能设置自己不持有的权限码「%s」（修改权限码等于批量授权）", permission)
	}
	return nil
}

func EnsureRoleMenusGrantable(actor *Identity, menuIDs []int64) error {
	if actor == nil || actor.IsSuper || len(menuIDs) == 0 {
		return nil
	}
	var perms []string
	if err := database.DB().Model(&model.Menu{}).
		Where("id IN ? AND status = ? AND deleted_at IS NULL AND permission <> ''", menuIDs, model.StatusEnabled).
		Pluck("permission", &perms).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, p := range perms {
		if _, ok := actor.Permissions[p]; !ok {
			return errcode.Newf(errcode.ErrForbidden, "无权把权限「%s」授予角色", p)
		}
	}
	return nil
}

// GrantableRoleIDs 返回 actor 可授予的角色 ID 集合。
// unrestricted 为 true 表示不受限（超管或未认证身份），调用方不必再过滤。
func GrantableRoleIDs(actor *Identity) (map[int64]bool, bool, error) {
	if actor == nil || actor.IsSuper {
		return nil, true, nil
	}
	var roles []model.Role
	// data_scope 必须一起取出来：bannedReason 的第二条轴要比较它，
	// 只 Select(id,name,code) 会让所有角色的范围都取到零值，判定形同虚设。
	if err := database.DB().Select("id", "name", "code", "data_scope").Find(&roles).Error; err != nil {
		return nil, false, errcode.Wrap(errcode.ErrDatabase, err)
	}
	ids := make([]int64, 0, len(roles))
	for _, r := range roles {
		ids = append(ids, r.ID)
	}
	permsOf, err := rolePermissions(ids)
	if err != nil {
		return nil, false, err
	}
	out := make(map[int64]bool, len(roles))
	for _, r := range roles {
		if bannedReason(actor, r, permsOf[r.ID]) == "" {
			out[r.ID] = true
		}
	}
	return out, false, nil
}
