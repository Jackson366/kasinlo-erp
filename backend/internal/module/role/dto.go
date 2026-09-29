package role

import "time"

// CreateReq 新增角色请求。
type CreateReq struct {
	Name      string  `json:"name" binding:"required,max=64" label:"角色名称"`
	Code      string  `json:"code" binding:"required,min=2,max=64" label:"角色标识"`
	Sort      int     `json:"sort" label:"排序"`
	Status    *int16  `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	DataScope int16   `json:"dataScope" binding:"omitempty,oneof=1 2 3 4" label:"数据范围"`
	Remark    string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
	MenuIDs   []int64 `json:"menuIds" label:"菜单权限"`
}

// UpdateReq 修改角色请求。
type UpdateReq struct {
	Name      string  `json:"name" binding:"required,max=64" label:"角色名称"`
	Sort      int     `json:"sort" label:"排序"`
	Status    *int16  `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	DataScope int16   `json:"dataScope" binding:"omitempty,oneof=1 2 3 4" label:"数据范围"`
	Remark    string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
	MenuIDs   []int64 `json:"menuIds" label:"菜单权限"`
}

// AssignMenusReq 分配菜单权限请求。
type AssignMenusReq struct {
	MenuIDs []int64 `json:"menuIds" label:"菜单权限"`
}

// VO 角色视图对象。
type VO struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	Sort      int       `json:"sort"`
	Status    int16     `json:"status"`
	DataScope int16     `json:"dataScope"`
	IsBuiltin bool      `json:"isBuiltin"`
	Remark    string    `json:"remark"`
	UserCount int64     `json:"userCount"`
	MenuIDs   []int64   `json:"menuIds"`
	CreatedAt time.Time `json:"createdAt"`
}

// Option 角色下拉项。
type Option struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}
