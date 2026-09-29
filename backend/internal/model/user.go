package model

import "time"

// User 用户表。
// 注意：username 的唯一性由「部分唯一索引」(WHERE deleted_at IS NULL) 保证，
// 见 database.EnsureIndexes —— 这样软删除后的用户名可以被重新使用。
type User struct {
	BaseModel
	Username string `gorm:"size:64;not null" json:"username"`
	Password string `gorm:"size:128;not null" json:"-"`
	Nickname string `gorm:"size:64" json:"nickname"`
	Email    string `gorm:"size:128" json:"email"`
	Phone    string `gorm:"size:32" json:"phone"`
	Avatar   string `gorm:"size:255" json:"avatar"`
	DeptID   *int64 `gorm:"index" json:"deptId"`
	// Status 刻意不加 gorm:"default:1"：
	// GORM 对「带 default 标签且值为零值」的字段会直接从 INSERT 中省略，
	// 而 StatusDisabled == 0 正是零值，会导致「禁用」被静默写成数据库默认值 1（启用）。
	// 状态由应用层显式写入，数据库不使用默认值。详见 AGENTS.md「已知坑」。
	Status      int16      `gorm:"not null" json:"status"`
	Remark      string     `gorm:"size:255" json:"remark"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	LastLoginIP string     `gorm:"size:64" json:"lastLoginIp"`

	Dept  *Dept  `gorm:"foreignKey:DeptID" json:"dept,omitempty"`
	Roles []Role `gorm:"many2many:sys_user_roles;" json:"roles,omitempty"`
}

// TableName 指定表名。
func (User) TableName() string { return "sys_users" }

// UserRole 用户-角色关联表。
type UserRole struct {
	UserID int64 `gorm:"primaryKey" json:"userId"`
	RoleID int64 `gorm:"primaryKey" json:"roleId"`
}

// TableName 指定表名。
func (UserRole) TableName() string { return "sys_user_roles" }
