package model

// Dept 部门表（树形）。
type Dept struct {
	BaseModel
	ParentID int64  `gorm:"index;default:0" json:"parentId"`
	Name     string `gorm:"size:64;not null" json:"name"`
	Leader   string `gorm:"size:64" json:"leader"`
	Phone    string `gorm:"size:32" json:"phone"`
	Email    string `gorm:"size:128" json:"email"`
	Sort     int    `gorm:"default:0" json:"sort"`
	// 不加 default 的理由见 model.User.Status。
	Status int16 `gorm:"not null" json:"status"`

	Children []Dept `gorm:"-" json:"children,omitempty"`
}

// TableName 指定表名。
func (Dept) TableName() string { return "sys_depts" }
