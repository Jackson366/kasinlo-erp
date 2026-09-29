package auth

import (
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/config"
	"nova-admin/internal/database"
	"nova-admin/internal/logger"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/hash"
	"nova-admin/internal/pkg/jwt"
	"nova-admin/internal/pkg/textutil"
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

	now := time.Now()
	s.db.Model(&user).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	})
	authz.Invalidate(user.ID)
	s.writeLoginLog(user.ID, user.Username, ip, ua, true, "登录成功")

	// 先加载身份再签发令牌：令牌里要带上当前的令牌版本号，
	// 这样改密码 / 重置密码后旧的令牌会在下一次请求时立刻失效。
	identity, err := authz.Load(user.ID)
	if err != nil {
		return nil, err
	}

	pair, err := jwt.M().Generate(user.ID, user.Username, identity.TokenEpoch)
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

	// 刷新令牌同样要校验版本号：否则改密码后，攻击者手里的旧刷新令牌
	// 还能换出一对全新的有效令牌，吊销就白做了。
	if claims.Epoch != id.TokenEpoch {
		return nil, errcode.New(errcode.ErrTokenInvalid, "")
	}

	pair, err := jwt.M().Generate(id.UserID, id.Username, id.TokenEpoch)
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

// 登录日志各列的长度上限，必须与 model.LoginLog 的 gorm size 标签严格一致。
// 这些列里有 client 可控的 UserAgent，是「整行写入失败」的高发区。
const (
	colUsername  = 64
	colIP        = 64
	colUserAgent = 512
	colMessage   = 255
)

// writeLoginLog 写入一条登录日志。
//
// 这里的错误**不能**静默丢弃：登录日志同时是 isLocked 判定「连续失败」的计数来源，
// 一旦写不进去，失败次数永远累计不到阈值，锁定就形同虚设。
// 曾经的实现是 `ua = ua[:512]` —— 按字节截断会把多字节字符切成两半，
// PostgreSQL 以 22021 拒绝整行，而错误被 `_ =` 吞掉，于是：
// 攻击者只要带一个超过 512 字节、且截断点落在多字节字符中间的 User-Agent，
// 就能让自己的每一次失败尝试都不留痕迹，从而无限次暴力破解。
func (s *Service) writeLoginLog(userID int64, username, ip, ua string, ok bool, msg string) {
	err := s.db.Create(&model.LoginLog{
		UserID:    userID,
		Username:  textutil.Fit(username, colUsername),
		IP:        textutil.Fit(ip, colIP),
		UserAgent: textutil.Fit(ua, colUserAgent),
		Success:   ok,
		Message:   textutil.Fit(msg, colMessage),
	}).Error
	if err != nil {
		logger.Warn("登录日志写入失败",
			zap.Error(err),
			zap.String("username", username),
		)
	}
}
