package authz

import (
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
	UserID      int64               `json:"userId"`
	Username    string              `json:"username"`
	Nickname    string              `json:"nickname"`
	Avatar      string              `json:"avatar"`
	DeptID      *int64              `json:"deptId"`
	RoleIDs     []int64             `json:"roleIds"`
	RoleCodes   []string            `json:"roleCodes"`
	DataScope   int16               `json:"dataScope"`
	IsSuper     bool                `json:"isSuper"`
	Permissions map[string]struct{} `json:"-"`
}

// HasPerm 判断是否拥有指定权限。
func (i *Identity) HasPerm(code string) bool {
	if i == nil {
		return false
	}
	if i.IsSuper {
		return true
	}
	_, ok := i.Permissions[code]
	return ok
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
