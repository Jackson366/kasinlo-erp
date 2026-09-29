package model

// 菜单类型
const (
	MenuTypeDir    = "dir"    // 目录
	MenuTypeMenu   = "menu"   // 菜单
	MenuTypeButton = "button" // 按钮（仅权限点）
)

// Menu 菜单/权限表，同时驱动前端动态路由。
// 注意：name 的唯一性由「部分唯一索引」(WHERE deleted_at IS NULL) 保证，
// 见 database.EnsureIndexes。
type Menu struct {
	BaseModel
	ParentID   int64  `gorm:"index;default:0" json:"parentId"`
	Name       string `gorm:"size:64;not null" json:"name"`  // 路由 name，需全局唯一
	Path       string `gorm:"size:255" json:"path"`          // 路由 path
	Component  string `gorm:"size:255" json:"component"`     // 组件路径，如 system/user/index
	Redirect   string `gorm:"size:255" json:"redirect"`      // 重定向
	Title      string `gorm:"size:64;not null" json:"title"` // 菜单标题
	Icon       string `gorm:"size:64" json:"icon"`           // 图标名
	Type       string `gorm:"size:16;default:'menu'" json:"type"`
	Permission string `gorm:"size:128;index" json:"permission"` // 权限标识，如 system:user:create
	Sort       int    `gorm:"default:0" json:"sort"`
	Hidden     bool   `gorm:"default:false" json:"hidden"`     // 是否在菜单中隐藏
	KeepAlive  bool   `gorm:"default:false" json:"keepAlive"`  // 是否缓存页面
	IsExternal bool   `gorm:"default:false" json:"isExternal"` // 是否外链
	// 不加 default 的理由见 model.User.Status。
	Status int16 `gorm:"not null" json:"status"`

	Children []Menu `gorm:"-" json:"children,omitempty"`
}

// TableName 指定表名。
func (Menu) TableName() string { return "sys_menus" }
