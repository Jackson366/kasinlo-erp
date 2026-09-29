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
	Status int16 `gorm:"not null" json:"status"`
	// TokenEpoch 是令牌版本号。修改密码 / 重置密码 / 禁用账号时 +1，
	// 已签发令牌因携带的版本号不再匹配而立即失效（JWT 无状态，这是低成本的吊销手段）。
	//
	// 这里加 default:0 是**安全**的，与上面 Status 的情况不同：Status 的零值(0=禁用)
	// 与数据库默认值(1=启用)语义相反，所以不能给默认值；而令牌版本号的零值与默认值
	// 都是 0，两者一致。default:0 同时也是必需的 —— 否则给已有数据的表新增
	// NOT NULL 列时，PostgreSQL 会因存量行取到 NULL 而直接拒绝迁移（SQLSTATE 23502）。
	TokenEpoch  int64      `gorm:"not null;default:0" json:"-"`
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
