package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-admin/internal/authz"
	"nova-admin/internal/logger"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/jwt"
	"nova-admin/internal/pkg/response"
)

// Auth 校验访问令牌并把身份写入上下文。
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractToken(c)
		if err != nil {
			response.FailWith(c, errcode.ErrUnauthorized, err.Error())
			return
		}

		claims, err := jwt.M().ParseAccess(token)
		if err != nil {
			code := errcode.ErrTokenInvalid
			if errors.Is(err, jwt.ErrTokenExpired) {
				code = errcode.ErrTokenExpired
			}
			response.FailWith(c, code, "")
			return
		}

		identity, err := authz.Load(claims.UserID)
		if err != nil {
			var be *errcode.Error
			if errors.As(err, &be) {
				response.FailWith(c, be.Code, be.Msg)
				return
			}
			logger.Error("加载用户身份失败", zapErr(err))
			response.FailWith(c, errcode.ErrInternal, "")
			return
		}

		// 令牌版本号必须与用户当前版本一致。
		// 修改密码 / 重置密码 / 禁用账号都会把 sys_users.token_epoch 加一，
		// 于是此前签发的所有令牌（含刷新令牌）在这里被立即挡下 ——
		// 否则「改了密码，旧会话还能用」会一直持续到令牌自然过期。
		if claims.Epoch != identity.TokenEpoch {
			logger.Warn("令牌版本已失效",
				zapStr("username", identity.Username),
				zapStr("path", c.Request.URL.Path),
			)
			response.FailWith(c, errcode.ErrTokenInvalid, "")
			return
		}

		c.Set(CtxUserID, identity.UserID)
		c.Set(CtxUsername, identity.Username)
		c.Set(CtxIdentity, identity)
		c.Next()
	}
}

// RequirePerm 要求当前用户拥有任一指定权限（超级管理员放行）。
func RequirePerm(codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := CurrentIdentity(c)
		if identity == nil {
			response.FailWith(c, errcode.ErrUnauthorized, "")
			return
		}
		if !identity.HasAnyPerm(codes...) {
			logger.Warn("权限校验未通过",
				zapStr("username", identity.Username),
				zapStrings("require", codes),
				zapStr("path", c.Request.URL.Path),
			)
			response.FailWith(c, errcode.ErrForbidden, "缺少权限: "+strings.Join(codes, " / "))
			return
		}
		c.Next()
	}
}

func extractToken(c *gin.Context) (string, error) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return "", errors.New("缺少认证信息，请先登录")
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("认证格式错误，应为 Bearer <token>")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("认证令牌为空")
	}
	return token, nil
}
