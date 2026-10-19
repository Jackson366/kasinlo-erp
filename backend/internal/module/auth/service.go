package auth

import (
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/logger"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/hash"
	"nova-erp/internal/pkg/jwt"
	"nova-erp/internal/pkg/limiter"
	"nova-erp/internal/pkg/textutil"
	"nova-erp/internal/seed"
)

// Service 认证服务。
type Service struct {
	db  *gorm.DB
	sec config.SecurityConfig
	// ipFails 按 IP 统计登录尝试次数，拦截「同一来源撞库大量账号」。
	// 每次尝试在入口原子占用一格，成功登录即清零，因此不会牵连 NAT 后的正常用户。
	ipFails *limiter.FailLimiter
}

// NewService 创建认证服务。
func NewService(sec config.SecurityConfig) *Service {
	return &Service{
		db:      database.DB(),
		sec:     sec,
		ipFails: limiter.New(sec.IPFailLimit(), time.Duration(sec.LoginLockSeconds)*time.Second),
	}
}

// Login 校验凭据并签发令牌。
func (s *Service) Login(req LoginReq, ip, ua string) (*LoginResp, error) {
	// 单来源失败限流放在最前面，连「账号不存在」的探测也一并计入 ——
	// 否则撞库者可以用一个不存在的用户名无限试探而不触发任何限制。
	// 用原子的 AllowAndConsume 而不是 Allow + Fail 两步：两步之间有时间窗，
	// 并发爆破时 N 个请求能同时挤过检查、把阈值突破 N 倍。这里每次尝试占一格，
	// 登录成功再 Reset 清零，净语义与原来一致（见 limiter 包内同名方法的注释）。
	if !s.ipFails.AllowAndConsume(ip) {
		logger.Warn("登录请求被来源限流", zap.String("ip", ip))
		return nil, errcode.New(errcode.ErrTooManyReq, "")
	}

	var user model.User
	err := s.db.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// 额度已在入口 AllowAndConsume 占用，无需再 Fail
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
	if s.isLocked(user.Username, ip) {
		s.writeLoginLog(user.ID, user.Username, ip, ua, false, msgLockedReject)
		return nil, errcode.New(errcode.ErrAccountLocked, "")
	}

	if !hash.Verify(user.Password, req.Password) {
		s.writeLoginLog(user.ID, user.Username, ip, ua, false, "密码错误")
		return nil, errcode.New(errcode.ErrInvalidCredential, "")
	}

	// 登录成功即清空该来源的失败计数
	s.ipFails.Reset(ip)

	now := time.Now()
	// 更新失败不阻断登录，但要留痕：静默失败会让 last_login_ip
	// 这类审计字段长期失真而无人察觉。
	if err := s.db.Model(&user).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	}).Error; err != nil {
		logger.Warn("更新最后登录信息失败",
			zap.Error(err), zap.Int64("userID", user.ID))
	}
	authz.Invalidate(user.ID)
	s.writeLoginLog(user.ID, user.Username, ip, ua, true, "登录成功")

	// 默认口令告警：内置口令是公开的，部署者忘了改密就等于门户大开。
	// 每次用默认口令登录都留一条 WARN，让「还没改密」这件事在日志里显眼。
	if seed.IsDefaultPassword(user.Username, req.Password) {
		logger.Warn("账号仍在使用内置默认口令，请尽快修改密码",
			zap.String("username", user.Username))
	}

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
//
// 设计取舍：当前是**无状态刷新令牌** —— 同一个 refresh token 在过期前可被重复使用，
// 吊销依赖 token_epoch（登出 / 改密 / 禁用时 +1，旧令牌立即失效）。
// 没有做「单次使用 + 轮换」：那需要给 JWT 加 jti 并新增一张黑名单表；
// 对后台系统而言，epoch 已经覆盖了「令牌被复制后如何止血」的主路径。
// 若将来威胁模型要求单次使用，再引入 jti 黑名单即可（结构同 sys_idempotencies）。
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

// Logout 退出登录。
//
// 除了清身份缓存，还要把 token_epoch +1：无状态 JWT 没有服务端会话可销毁，
// 若只清缓存，已签发的令牌（尤其长寿命的刷新令牌）会一直有效到自然过期——
// 令牌一旦被复制走（公共电脑、抓包），「退出登录」就是个毫无约束力的假动作。
// 代价是登出同时作废该用户在其他设备上的会话；这与改密/禁用是同款机制，
// 对后台系统而言「登出即全部失效」是更安全的选择。
func (s *Service) Logout(userID int64) error {
	if err := s.db.Model(&model.User{}).Where("id = ?", userID).
		Update("token_epoch", gorm.Expr("token_epoch + 1")).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.Invalidate(userID)
	return nil
}

// isLocked 判断账号是否处于锁定期。
//
// 这里有两个关键设计，都是修复实际缺陷的：
//
//  1. 计数维度是「用户名 + IP」。旧实现只按用户名统计，于是任何人拿 5 次错密码
//     就能把 admin 锁整整 5 分钟 —— 锁号型 DoS 的成本低到可以忽略。按 IP 分组后，
//     攻击者最多只能锁住「自己这个 IP 对某个账号」的尝试，波及不到其他来源。
//  2. 以「该 用户名+IP 最近一次成功登录」为计数起点。旧实现只看时间窗内的失败次数、
//     登录成功并不清零，于是「失败 4 次 -> 成功 -> 失败 1 次」也会被莫名锁定。
//     这里刻意**不删除**失败日志（它们是审计证据），而是用时间起点把计数归零。
//
// 代价是：分布式（多 IP）撞库不再受此单机锁定约束，需要由网关/WAF 层的全局限流兜底。
// msgLockedReject 是「因锁定被拒」的登录日志标记。
//
// 必须与「密码错误」区分开：isLocked 统计的是**真正的凭据失败**，
// 若把「锁定中」这条也计入，锁定期内每一次重试都会刷新失败计数，
// 锁定就永远无法自然到期 —— 攻击者只要一直重试就能把账号永久锁死，
// 一个「防爆破」机制反而变成了「锁号 DoS」工具。
const msgLockedReject = "账号锁定中"

func (s *Service) isLocked(username, ip string) bool {
	windowStart := time.Now().Add(-time.Duration(s.sec.LoginLockSeconds) * time.Second)
	start := windowStart

	var last model.LoginLog
	if err := s.db.Select("id", "created_at").
		Where("username = ? AND ip = ? AND success = ?", username, ip, true).
		Order("id DESC").Limit(1).Find(&last).Error; err == nil && !last.CreatedAt.IsZero() {
		if last.CreatedAt.After(start) {
			start = last.CreatedAt
		}
	}

	var fails int64
	if err := s.db.Model(&model.LoginLog{}).
		Where("username = ? AND ip = ? AND success = ? AND created_at >= ? AND message <> ?",
			username, ip, false, start, msgLockedReject).
		Count(&fails).Error; err != nil {
		// 查询本身失败时按「未锁定」放行：后续密码校验与日志写入同样会失败，
		// 在这里返回 true 只会掩盖真正的数据库故障。
		return false
	}
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
