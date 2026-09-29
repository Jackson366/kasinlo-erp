package model

// Role 角色表。
// 注意：code 的唯一性由「部分唯一索引」(WHERE deleted_at IS NULL) 保证，
// 见 database.EnsureIndexes。
type Role struct {
	BaseModel
	Name string `gorm:"size:64;not null" json:"name"`
	Code string `gorm:"size:64;not null" json:"code"`
	Sort int    `gorm:"default:0" json:"sort"`
	// 不加 default 的理由见 model.User.Status。
	Status    int16  `gorm:"not null" json:"status"`
	DataScope int16  `gorm:"default:1" json:"dataScope"` // 1 全部 2 本部门 3 本部门及以下 4 仅本人
	IsBuiltin bool   `gorm:"default:false" json:"isBuiltin"`
	Remark    string `gorm:"size:255" json:"remark"`

	Menus []Menu `gorm:"many2many:sys_role_menus;" json:"menus,omitempty"`
}

// TableName 指定表名。
func (Role) TableName() string { return "sys_roles" }

// RoleMenu 角色-菜单关联表。
type RoleMenu struct {
	RoleID int64 `gorm:"primaryKey" json:"roleId"`
	MenuID int64 `gorm:"primaryKey" json:"menuId"`
}

// TableName 指定表名。
func (RoleMenu) TableName() string { return "sys_role_menus" }

// 数据范围常量
const (
	DataScopeAll        int16 = 1 // 全部数据
	DataScopeDept       int16 = 2 // 本部门
	DataScopeDeptAndSub int16 = 3 // 本部门及以下
	DataScopeSelf       int16 = 4 // 仅本人
)
