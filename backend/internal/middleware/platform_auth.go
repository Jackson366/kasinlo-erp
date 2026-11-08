package middleware

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"nova-erp/internal/authz"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/response"
)

const maxPlatformTokenLifetime = time.Minute

type platformClaims struct {
	jwt.RegisteredClaims
	Scope   string `json:"scope"`
	GrantID string `json:"grant_id"`
	Epoch   *int64 `json:"epoch"`
}

// PlatformAuth verifies a short-lived credential issued for this ERP instance.
func PlatformAuth(key []byte, issuer, audience, scope string) gin.HandlerFunc {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)

	return func(c *gin.Context) {
		raw, err := extractToken(c)
		if err != nil {
			response.FailWith(c, errcode.ErrUnauthorized, "")
			return
		}
		claims, userID, err := parsePlatformToken(parser, key, raw)
		if err != nil {
			response.FailWith(c, errcode.ErrTokenInvalid, "")
			return
		}
		if claims.Scope != scope {
			response.FailWith(c, errcode.ErrForbidden, "")
			return
		}
		identity, err := authz.Load(userID)
		if err != nil {
			response.Fail(c, err)
			return
		}
		if identity.TokenEpoch != *claims.Epoch {
			response.FailWith(c, errcode.ErrTokenInvalid, "")
			return
		}

		c.Set(CtxUserID, identity.UserID)
		c.Set(CtxUsername, identity.Username)
		c.Set(CtxIdentity, identity)
		c.Next()
	}
}

func parsePlatformToken(parser *jwt.Parser, key []byte, raw string) (*platformClaims, int64, error) {
	claims := &platformClaims{}
	token, err := parser.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return key, nil
	})
	if err != nil || !token.Valid {
		return nil, 0, errors.New("invalid platform signature or registered claims")
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 || claims.GrantID == "" || claims.Epoch == nil || claims.IssuedAt == nil {
		return nil, 0, errors.New("missing platform identity claims")
	}
	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > maxPlatformTokenLifetime {
		return nil, 0, errors.New("invalid platform token lifetime")
	}
	return claims, userID, nil
}
