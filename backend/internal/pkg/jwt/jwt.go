package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"nova-admin/internal/config"
	"nova-admin/internal/pkg/errcode"
)

// TokenType 区分访问令牌与刷新令牌。
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Claims 是自定义载荷。
type Claims struct {
	UserID   int64     `json:"uid"`
	Username string    `json:"uname"`
	Type     TokenType `json:"typ"`
	// Epoch 是签发时用户的令牌版本号，对应 sys_users.token_epoch。
	// 修改密码 / 重置密码 / 禁用账号时会把它 +1，于是所有旧令牌都因版本号
	// 不匹配而立即失效 —— 这是无状态 JWT 唯一低成本的吊销手段。
	Epoch int64 `json:"epo"`
	jwt.RegisteredClaims
}

// Pair 是一次签发的令牌对。
type Pair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"` // 访问令牌剩余有效秒数
	TokenType    string `json:"tokenType"` // 固定为 Bearer
}

var (
	manager *Manager
	// ErrTokenExpired 令牌过期
	ErrTokenExpired = errors.New("token expired")
	// ErrTokenInvalid 令牌无效
	ErrTokenInvalid = errors.New("token invalid")
)

// Manager 负责令牌签发与解析。
type Manager struct {
	secret        []byte
	issuer        string
	accessExpire  time.Duration
	refreshExpire time.Duration
}

// Init 初始化全局令牌管理器。
func Init(cfg config.JWTConfig) {
	manager = New(cfg)
}

// New 按配置构造一个独立的 Manager，不触碰全局单例。
//
// 生产代码一律用 M() 取单例；这个导出构造器是给单元测试用的
// —— 测试若通过 Init 改全局状态，会污染同进程内的其他用例
// （令牌密钥、issuer 都会变，而令牌可能被其他测试持有）。
func New(cfg config.JWTConfig) *Manager {
	return &Manager{
		secret:        []byte(cfg.Secret),
		issuer:        cfg.Issuer,
		accessExpire:  time.Duration(cfg.AccessExpire) * time.Second,
		refreshExpire: time.Duration(cfg.RefreshExpire) * time.Second,
	}
}

// M 返回全局令牌管理器。
func M() *Manager {
	if manager == nil {
		panic("jwt manager 未初始化，请先调用 jwt.Init")
	}
	return manager
}

// Generate 为用户签发令牌对。epoch 为该用户当前的令牌版本号。
func (m *Manager) Generate(userID int64, username string, epoch int64) (*Pair, error) {
	access, err := m.sign(userID, username, epoch, AccessToken, m.accessExpire)
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	refresh, err := m.sign(userID, username, epoch, RefreshToken, m.refreshExpire)
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	return &Pair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(m.accessExpire.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (m *Manager) sign(userID int64, username string, epoch int64, typ TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Type:     typ,
		Epoch:    epoch,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse 解析并校验令牌。
//
// 三道校验，缺一不可：
//  1. **算法白名单**：只接受 HS256。golang-jwt 的 keyfunc 拿到的是「令牌自称的算法」，
//     不校验就等于把「用哪个算法验签」的决定权交给签发方。这里显式比对 Alg()，
//     而不是只断言「是 HMAC 类」—— HS512 同样属于 HMAC，一样不该被接受。
//  2. **issuer 匹配**：签发时写进 claims，这里必须比对。不校验 issuer 意味着
//     「同一份密钥签发的其它系统/其它环境的令牌」也能在本服务通过验签
//     （多环境共用 secret 是很常见的配置）。
//  3. 有效期、nbf 由 RegisteredClaims 的默认校验负责。
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	}
	if m.issuer != "" {
		opts = append(opts, jwt.WithIssuer(m.issuer))
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method == nil || t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, ErrTokenInvalid
		}
		return m.secret, nil
	}, opts...)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}
	// claims.Issuer 为空的令牌（老版本签发的）只有在配置了 issuer 时才会被上面拒绝，
	// 这里再兜一次，保证「issuer 非空却不匹配」不可能通过。
	if m.issuer != "" && claims.Issuer != m.issuer {
		return nil, ErrTokenInvalid
	}
	// UserID <= 0 说明载荷被截断或伪造，直接判无效 —— 各处都用它做数据范围判定，
	// 一个 0 值会命中「查不到用户」的分支而不是报错。
	if claims.UserID <= 0 {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// ParseAccess 解析访问令牌，类型不符返回无效。
func (m *Manager) ParseAccess(tokenStr string) (*Claims, error) {
	claims, err := m.Parse(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Type != AccessToken {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// ParseRefresh 解析刷新令牌，类型不符返回无效。
func (m *Manager) ParseRefresh(tokenStr string) (*Claims, error) {
	claims, err := m.Parse(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Type != RefreshToken {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
