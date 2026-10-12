package profile

import (
	"errors"
	"regexp"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/module/menu"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/hash"
)

// Service 个人中心服务。
type Service struct {
	db      *gorm.DB
	menuSvc *menu.Service
}

// NewService 创建个人中心服务。
func NewService() *Service {
	return &Service{db: database.DB(), menuSvc: menu.NewService()}
}

// Info 返回当前用户完整信息（含角色与权限）。
func (s *Service) Info(id *authz.Identity) (*Info, error) {
	var u model.User
	if err := s.db.Preload("Dept").Preload("Roles").First(&u, id.UserID).Error; err != nil {
		// 令牌有效但用户已不存在（删除账号后旧令牌仍在有效期内的窗口）。
		// 按数据库错误返回 500 会把问题描成「服务端故障」；
		// 按 401 返回前端才会走重新登录，与「账号已被删除」的事实对齐。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrUnauthorized, "账号不存在或已被删除，请重新登录")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := &Info{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Phone:       u.Phone,
		Avatar:      u.Avatar,
		DeptID:      u.DeptID,
		Status:      u.Status,
		IsSuper:     id.IsSuper,
		Roles:       make([]string, 0, len(u.Roles)),
		RoleCodes:   make([]string, 0, len(u.Roles)),
		Permissions: []string{},
	}
	if u.Dept != nil {
		out.DeptName = u.Dept.Name
	}
	for _, r := range u.Roles {
		out.Roles = append(out.Roles, r.Name)
		out.RoleCodes = append(out.RoleCodes, r.Code)
	}
	if !id.IsSuper {
		out.Permissions = id.PermList()
	}
	return out, nil
}

// Update 修改个人资料。
//
// 只写请求里真正带了的字段：见 UpdateReq 的说明，map 形式的 Updates
// 不会省略零值，把没传的字段一并写进去等于「清空」。
func (s *Service) Update(userID int64, req UpdateReq) error {
	updates := map[string]any{}
	if req.Nickname != nil {
		updates["nickname"] = *req.Nickname
	}
	if req.Email != nil {
		if err := validateEmail(*req.Email); err != nil {
			return err
		}
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}
	if len(updates) == 0 {
		// 一个字段都没带还发一次请求，多半是前端表单绑定出了问题；
		// 直接当成功返回会掩盖问题，这里明确拒绝。
		return errcode.New(errcode.ErrBadRequest, "没有需要更新的字段")
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.Invalidate(userID)
	return nil
}

// validateEmail 校验邮箱格式，空串视为「清空」直接放行。
//
// 格式校验不能留在 binding 标签里：Email 是 *string，validator 的 omitempty
// 判断的是指针是否为 nil，显式传 "" 清空时指针非 nil，email 规则会照常生效并报错。
func validateEmail(v string) error {
	if v == "" {
		return nil
	}
	if !emailRe.MatchString(v) || len(v) > 128 {
		return errcode.New(errcode.ErrBadRequest, "邮箱格式不正确")
	}
	return nil
}

// emailRe 与 go-playground/validator 的 email 规则等价的宽松版本：
// 只要求有唯一的 @、@ 前非空、@ 后是点分隔的域名。
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)

// ChangePassword 修改本人密码。
func (s *Service) ChangePassword(userID int64, oldPwd, newPwd string) error {
	if oldPwd == newPwd {
		return errcode.New(errcode.ErrBadRequest, "新密码不能与原密码相同")
	}
	var u model.User
	if err := s.db.First(&u, userID).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if !hash.Verify(u.Password, oldPwd) {
		return errcode.New(errcode.ErrOldPasswordWrong, "")
	}
	hashed, err := hash.Password(newPwd)
	if err != nil {
		return errcode.Wrap(errcode.ErrInternal, err)
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{
			"password":    hashed,
			"token_epoch": gorm.Expr("token_epoch + 1"),
		}).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 版本号已变，本人当前令牌立即失效，需要重新登录
	authz.Invalidate(userID)
	return nil
}

// Routes 返回当前用户可访问的前端路由树。
func (s *Service) Routes(id *authz.Identity) ([]*menu.Route, error) {
	return s.menuSvc.RoutesFor(id)
}
