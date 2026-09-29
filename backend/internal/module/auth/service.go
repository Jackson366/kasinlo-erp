package auth

import (
	"time"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/config"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/hash"
	"nova-admin/internal/pkg/jwt"
)

// Service 认证服务。
type Service struct {
	db  *gorm.DB
	sec config.SecurityConfig
}

// NewService 创建认证服务。
func NewService(sec config.SecurityConfig) *Service {
	return &Service{db: database.DB(), sec: sec}
}

// Login 校验凭据并签发令牌。
func (s *Service) Login(req LoginReq, ip, ua string) (*LoginResp, error) {
	var user model.User
	err := s.db.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.writeLoginLog(0, req.Username, ip, ua, false, "用户不存在")
			return nil, errcode.New(errcode.ErrInvalidCredential, "")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}

	// 账号状态校验
	if user.Status != model.StatusEnabled {
		s.writeLoginLog(user.ID, user.Username, ip, ua, false, "账号已禁用")
		return nil, errcode.New(errcode.ErrAccountDisabled, "")
	}

	// 连续失败锁定校验
	if s.isLocked(user.Username) {
		s.writeLoginLog(user.ID, user.Username, ip, ua, false, "账号锁定中")
		return nil, errcode.New(errcode.ErrAccountLocked, "")
	}

	if !hash.Verify(user.Password, req.Password) {
		s.writeLoginLog(user.ID, user.Username, ip, ua, false, "密码错误")
		return nil, errcode.New(errcode.ErrInvalidCredential, "")
	}

	pair, err := jwt.M().Generate(user.ID, user.Username)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	s.db.Model(&user).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	})
	authz.Invalidate(user.ID)
	s.writeLoginLog(user.ID, user.Username, ip, ua, true, "登录成功")

	identity, err := authz.Load(user.ID)
	if err != nil {
		return nil, err
	}

	return &LoginResp{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		TokenType:    pair.TokenType,
		User:         identity,
	}, nil
}

// Refresh 用刷新令牌换取新的令牌对。
func (s *Service) Refresh(refreshToken string) (*LoginResp, error) {
	claims, err := jwt.M().ParseRefresh(refreshToken)
	if err != nil {
		code := errcode.ErrTokenInvalid
		if err == jwt.ErrTokenExpired {
			code = errcode.ErrTokenExpired
		}
		return nil, errcode.New(code, "")
	}

	// 用户可能已被删除或禁用，需要重新校验
	id, err := authz.Load(claims.UserID)
	if err != nil {
		return nil, err
	}

	pair, err := jwt.M().Generate(id.UserID, id.Username)
	if err != nil {
		return nil, err
	}
	return &LoginResp{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		TokenType:    pair.TokenType,
		User:         id,
	}, nil
}

// Logout 退出登录。JWT 无状态，这里仅清理服务端缓存并记录日志。
func (s *Service) Logout(userID int64) error {
	authz.Invalidate(userID)
	return nil
}

// isLocked 判断账号是否处于锁定期：统计锁定时间窗内的失败次数。
func (s *Service) isLocked(username string) bool {
	windowStart := time.Now().Add(-time.Duration(s.sec.LoginLockSeconds) * time.Second)
	var fails int64
	s.db.Model(&model.LoginLog{}).
		Where("username = ? AND success = ? AND created_at >= ?", username, false, windowStart).
		Count(&fails)
	return fails >= int64(s.sec.MaxLoginFail)
}

func (s *Service) writeLoginLog(userID int64, username, ip, ua string, ok bool, msg string) {
	if ua != "" && len(ua) > 512 {
		ua = ua[:512]
	}
	_ = s.db.Create(&model.LoginLog{
		UserID:    userID,
		Username:  username,
		IP:        ip,
		UserAgent: ua,
		Success:   ok,
		Message:   msg,
	}).Error
}
