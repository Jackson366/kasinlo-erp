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
	manager = &Manager{
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

// Generate 为用户签发令牌对。
func (m *Manager) Generate(userID int64, username string) (*Pair, error) {
	access, err := m.sign(userID, username, AccessToken, m.accessExpire)
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	refresh, err := m.sign(userID, username, RefreshToken, m.refreshExpire)
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

func (m *Manager) sign(userID int64, username string, typ TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Type:     typ,
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
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return m.secret, nil
	})
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
