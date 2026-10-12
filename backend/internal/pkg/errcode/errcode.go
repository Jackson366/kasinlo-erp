package errcode

import "fmt"

// Code 是业务错误码。约定：
//
//	0        成功
//	4xxxx    客户端错误（参数、权限、业务校验）
//	5xxxx    服务端错误
type Code int

const (
	// 通用客户端错误 40xxx
	ErrBadRequest   Code = 40000 // 请求参数错误
	ErrUnauthorized Code = 40100 // 未登录或令牌失效
	ErrForbidden    Code = 40300 // 无权限
	ErrNotFound     Code = 40400 // 资源不存在
	ErrConflict     Code = 40900 // 资源冲突
	ErrTooManyReq   Code = 42900 // 请求过于频繁

	// 认证相关 401xx
	ErrInvalidCredential Code = 40101 // 用户名或密码错误
	ErrAccountDisabled   Code = 40102 // 账号已被禁用
	ErrAccountLocked     Code = 40103 // 账号已被锁定
	ErrTokenExpired      Code = 40104 // 令牌已过期
	ErrTokenInvalid      Code = 40105 // 令牌无效
	ErrOldPasswordWrong  Code = 40106 // 原密码错误

	// 通用服务端错误 50xxx
	ErrInternal Code = 50000 // 服务器内部错误
	ErrDatabase Code = 50100 // 数据库操作失败
)

var messages = map[Code]string{
	ErrBadRequest:        "请求参数错误",
	ErrUnauthorized:      "登录状态已失效，请重新登录",
	ErrForbidden:         "没有操作权限",
	ErrNotFound:          "资源不存在",
	ErrConflict:          "资源已存在或存在冲突",
	ErrTooManyReq:        "请求过于频繁，请稍后再试",
	ErrInvalidCredential: "用户名或密码错误",
	ErrAccountDisabled:   "账号已被禁用，请联系管理员",
	ErrAccountLocked:     "账号已被锁定，请稍后再试",
	ErrTokenExpired:      "登录已过期，请重新登录",
	ErrTokenInvalid:      "身份凭证无效",
	ErrOldPasswordWrong:  "原密码错误",
	ErrInternal:          "服务器内部错误",
	ErrDatabase:          "数据库操作失败",
}

// Message 返回错误码对应的默认提示。
func (c Code) Message() string {
	if msg, ok := messages[c]; ok {
		return msg
	}
	return "未知错误"
}

// HTTPStatus 返回错误码对应的 HTTP 状态码。
// 约定：业务码前 3 位直接对应 HTTP 状态码，便于前后端一致理解。
//
//	40000-40099 -> 400   参数/业务校验错误
//	40100-40199 -> 401   未认证（含凭据错误、令牌失效、账号禁用/锁定）
//	40300-40399 -> 403   已认证但无权限
//	40400-40499 -> 404   资源不存在
//	40900-40999 -> 409   资源冲突
//	42900-42999 -> 429   请求过于频繁
//	50000-59999 -> 500   服务端错误
func (c Code) HTTPStatus() int {
	switch {
	case c >= 40000 && c < 40100:
		return 400
	case c >= 40100 && c < 40200:
		return 401
	case c >= 40300 && c < 40400:
		return 403
	case c >= 40400 && c < 40500:
		return 404
	case c >= 40900 && c < 41000:
		return 409
	case c >= 42900 && c < 43000:
		return 429
	case c >= 40000 && c < 50000:
		return 400
	default:
		return 500
	}
}

// Error 是带业务码的错误类型。
type Error struct {
	Code Code
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Msg, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// New 创建一个业务错误。
func New(code Code, msg string) *Error {
	if msg == "" {
		msg = code.Message()
	}
	return &Error{Code: code, Msg: msg}
}

// Newf 创建带格式化消息的业务错误。
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap 包装底层错误。
func Wrap(code Code, err error) *Error {
	return &Error{Code: code, Msg: code.Message(), Err: err}
}

// Wrapf 包装底层错误并自定义消息。
func Wrapf(code Code, err error, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Err: err}
}
