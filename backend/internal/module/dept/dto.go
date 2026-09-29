package dept

import "time"

// CreateReq 新增部门请求。
type CreateReq struct {
	ParentID int64  `json:"parentId" label:"上级部门"`
	Name     string `json:"name" binding:"required,max=64" label:"部门名称"`
	Leader   string `json:"leader" binding:"omitempty,max=64" label:"负责人"`
	Phone    string `json:"phone" binding:"omitempty,max=32" label:"联系电话"`
	Email    string `json:"email" binding:"omitempty,email,max=128" label:"邮箱"`
	Sort     int    `json:"sort" label:"排序"`
	Status   *int16 `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
}

// UpdateReq 修改部门请求。
type UpdateReq = CreateReq

// Node 部门树节点。
type Node struct {
	ID        int64     `json:"id"`
	ParentID  int64     `json:"parentId"`
	Name      string    `json:"name"`
	Leader    string    `json:"leader"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email"`
	Sort      int       `json:"sort"`
	Status    int16     `json:"status"`
	UserCount int64     `json:"userCount"`
	CreatedAt time.Time `json:"createdAt"`

	Children []*Node `json:"children,omitempty"`
}
