package profile

import (
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
func (s *Service) Update(userID int64, req UpdateReq) error {
	err := s.db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"nickname": req.Nickname,
		"email":    req.Email,
		"phone":    req.Phone,
		"avatar":   req.Avatar,
	}).Error
	if err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.Invalidate(userID)
	return nil
}

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
