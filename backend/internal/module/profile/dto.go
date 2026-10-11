package profile

// UpdateReq 修改个人资料请求。
//
// Email / Phone / Avatar 是指针，语义是 PATCH：
//   - nil   → 请求里没带这个字段，不要写库；
//   - 非 nil → 按请求值写入，显式传空串即为清空。
//
// 为什么必须用指针：GORM 对 map 形式的 Updates **不做零值省略**，
// 若用普通 string，只提交昵称的请求会把邮箱、手机号、头像一并清空。
//
// 注意邮箱不能用 binding:"omitempty,email"：validator 的 omitempty
// 判断的是「指针是否为 nil」，显式传 "" 时指针非 nil 于是规则照常生效，
// 空串被 email 规则判为非法——清空邮箱这条路直接被堵死。
// 格式校验因此下移到服务层，只在非空时生效。
type UpdateReq struct {
	Nickname *string `json:"nickname" binding:"omitempty,max=64" label:"昵称"`
	Email    *string `json:"email" binding:"omitempty,max=128" label:"邮箱"`
	Phone    *string `json:"phone" binding:"omitempty,max=32" label:"手机号"`
	Avatar   *string `json:"avatar" binding:"omitempty,max=255" label:"头像"`
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
