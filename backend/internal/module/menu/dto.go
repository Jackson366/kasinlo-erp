package menu

// CreateReq 新增菜单请求。
type CreateReq struct {
	ParentID   int64  `json:"parentId" label:"上级菜单"`
	Name       string `json:"name" binding:"required,max=64" label:"路由名称"`
	Path       string `json:"path" binding:"omitempty,max=255" label:"路由路径"`
	Component  string `json:"component" binding:"omitempty,max=255" label:"组件路径"`
	Redirect   string `json:"redirect" binding:"omitempty,max=255" label:"重定向"`
	Title      string `json:"title" binding:"required,max=64" label:"菜单标题"`
	Icon       string `json:"icon" binding:"omitempty,max=64" label:"图标"`
	Type       string `json:"type" binding:"required,oneof=dir menu button" label:"类型"`
	Permission string `json:"permission" binding:"omitempty,max=128" label:"权限标识"`
	Sort       int    `json:"sort" label:"排序"`
	Hidden     bool   `json:"hidden" label:"是否隐藏"`
	KeepAlive  bool   `json:"keepAlive" label:"是否缓存"`
	IsExternal bool   `json:"isExternal" label:"是否外链"`
	Status     *int16 `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
}

// UpdateReq 修改菜单请求。
type UpdateReq = CreateReq

// Node 菜单树节点。
type Node struct {
	ID         int64  `json:"id"`
	ParentID   int64  `json:"parentId"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Component  string `json:"component"`
	Redirect   string `json:"redirect"`
	Title      string `json:"title"`
	Icon       string `json:"icon"`
	Type       string `json:"type"`
	Permission string `json:"permission"`
	Sort       int    `json:"sort"`
	Hidden     bool   `json:"hidden"`
	KeepAlive  bool   `json:"keepAlive"`
	IsExternal bool   `json:"isExternal"`
	Status     int16  `json:"status"`

	Children []*Node `json:"children,omitempty"`
}

// RouteMeta 前端路由元信息。
type RouteMeta struct {
	Title      string `json:"title"`
	Icon       string `json:"icon"`
	Hidden     bool   `json:"hidden"`
	KeepAlive  bool   `json:"keepAlive"`
	IsExternal bool   `json:"isExternal"`
	Permission string `json:"permission,omitempty"`
}

// Route 前端动态路由节点。
type Route struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Component string    `json:"component"`
	Redirect  string    `json:"redirect,omitempty"`
	Meta      RouteMeta `json:"meta"`
	Children  []*Route  `json:"children,omitempty"`
}
