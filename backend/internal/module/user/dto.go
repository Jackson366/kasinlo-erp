package user

import "time"

// CreateReq 新增用户请求。
type CreateReq struct {
	Username string  `json:"username" binding:"required,min=3,max=64" label:"用户名"`
	Password string  `json:"password" binding:"required,min=6,max=64" label:"密码"`
	Nickname string  `json:"nickname" binding:"required,max=64" label:"昵称"`
	Email    string  `json:"email" binding:"omitempty,email,max=128" label:"邮箱"`
	Phone    string  `json:"phone" binding:"omitempty,max=32" label:"手机号"`
	DeptID   *int64  `json:"deptId" label:"部门"`
	Status   *int16  `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	RoleIDs  []int64 `json:"roleIds" label:"角色"`
	Remark   string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// UpdateReq 修改用户请求。
type UpdateReq struct {
	Nickname string  `json:"nickname" binding:"omitempty,max=64" label:"昵称"`
	Email    string  `json:"email" binding:"omitempty,email,max=128" label:"邮箱"`
	Phone    string  `json:"phone" binding:"omitempty,max=32" label:"手机号"`
	DeptID   *int64  `json:"deptId" label:"部门"`
	Status   *int16  `json:"status" binding:"omitempty,oneof=0 1" label:"状态"`
	RoleIDs  []int64 `json:"roleIds" label:"角色"`
	Remark   string  `json:"remark" binding:"omitempty,max=255" label:"备注"`
}

// StatusReq 启用/禁用请求。
type StatusReq struct {
	Status int16 `json:"status" binding:"oneof=0 1" label:"状态"`
}

// ResetPasswordReq 管理员重置密码请求。
type ResetPasswordReq struct {
	Password string `json:"password" binding:"required,min=6,max=64" label:"新密码"`
}

// ListQuery 用户列表查询条件。
type ListQuery struct {
	DeptID *int64 `form:"deptId" label:"部门"`
	Status *int16 `form:"status" label:"状态"`
	RoleID *int64 `form:"roleId" label:"角色"`
}

// ExportQuery 导出查询条件：在列表筛选条件之上附加「按选中行导出」。
type ExportQuery struct {
	ListQuery
	// IDs 指定要导出的用户 ID（逗号分隔）。为空时按筛选条件导出全部。
	IDs string `form:"ids" label:"选中项"`
}

// ImportRow 单行导入校验结果。
type ImportRow struct {
	// Row 是 Excel 中的实际行号（含表头，从 1 开始），便于用户直接定位
	Row      int      `json:"row"`
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Errors   []string `json:"errors"`
}

// ImportResult 导入（或导入预览）结果。
// Rows 返回全部数据行的校验明细（上限与单次导入行数一致），
// 前端据此逐行提示，无需再发一次请求。
type ImportResult struct {
	Total   int         `json:"total"`   // 数据行总数（不含表头）
	Valid   int         `json:"valid"`   // 校验通过的行数
	Invalid int         `json:"invalid"` // 校验失败的行数
	Created int         `json:"created"` // 实际写入的行数，预览阶段恒为 0
	Rows    []ImportRow `json:"rows"`    // 逐行明细
}

// VO 用户视图对象。
type VO struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	Avatar      string     `json:"avatar"`
	DeptID      *int64     `json:"deptId"`
	DeptName    string     `json:"deptName"`
	Status      int16      `json:"status"`
	Remark      string     `json:"remark"`
	RoleIDs     []int64    `json:"roleIds"`
	RoleNames   []string   `json:"roleNames"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	LastLoginIP string     `json:"lastLoginIp"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// Option 用于下拉选择。
type Option struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}
