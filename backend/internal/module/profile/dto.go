package profile

// UpdateReq 修改个人资料请求。
type UpdateReq struct {
	Nickname string `json:"nickname" binding:"required,max=64" label:"昵称"`
	Email    string `json:"email" binding:"omitempty,email,max=128" label:"邮箱"`
	Phone    string `json:"phone" binding:"omitempty,max=32" label:"手机号"`
	Avatar   string `json:"avatar" binding:"omitempty,max=255" label:"头像"`
}

// ChangePasswordReq 修改密码请求。
type ChangePasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required,min=6,max=64" label:"原密码"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=64" label:"新密码"`
}

// Info 个人信息视图。
type Info struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	Nickname    string   `json:"nickname"`
	Email       string   `json:"email"`
	Phone       string   `json:"phone"`
	Avatar      string   `json:"avatar"`
	DeptID      *int64   `json:"deptId"`
	DeptName    string   `json:"deptName"`
	Status      int16    `json:"status"`
	Roles       []string `json:"roles"`
	RoleCodes   []string `json:"roleCodes"`
	IsSuper     bool     `json:"isSuper"`
	Permissions []string `json:"permissions"`
}
