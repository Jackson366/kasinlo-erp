// Package bind 统一请求参数绑定与校验错误提示。
package bind

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"nova-admin/internal/pkg/errcode"
)

// JSON 绑定 JSON 请求体并校验，失败返回带友好提示的业务错误。
func JSON(c *gin.Context, obj any) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		return translate(obj, err)
	}
	return nil
}

// Query 绑定 URL 查询参数并校验。
func Query(c *gin.Context, obj any) error {
	if err := c.ShouldBindQuery(obj); err != nil {
		return translate(obj, err)
	}
	return nil
}

// PathID 解析路径参数 :id（int64，必须为正）。
//
// 四个模块（user/role/menu/dept）曾经各有一份私有的 pathID：
// 判错口径（<=0 算非法）、错误文案都要各自维护，迟早漂移。
// 路径参数解析没有 per-module 差异，收敛到这里。
func PathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errcode.New(errcode.ErrBadRequest, "路径参数 id 非法")
	}
	return id, nil
}

// StatusQuery 解析可选的 status 查询参数（0/1）。
//
// 返回 nil 表示「请求没带该参数，不过滤」。
// 非法值（?status=abc、?status=9）一律 400，**不**静默忽略：
// 静默忽略等于用户要「只看禁用」却拿到全量，筛选条件被悄悄丢掉了。
func StatusQuery(c *gin.Context) (*int16, error) {
	v := c.Query("status")
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 1 {
		return nil, errcode.Newf(errcode.ErrBadRequest, "查询参数 status 非法：%s（仅支持 0/1）", v)
	}
	s := int16(n)
	return &s, nil
}

func translate(obj any, err error) error {
	var ve validator.ValidationErrors
	if !asValidationErrors(err, &ve) {
		return errcode.Newf(errcode.ErrBadRequest, "请求参数格式错误: %s", err.Error())
	}
	labels := fieldLabels(obj)
	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		name := fe.Field()
		if l, ok := labels[fe.StructField()]; ok && l != "" {
			name = l
		}
		msgs = append(msgs, fmt.Sprintf("%s%s", name, ruleMessage(fe)))
	}
	return errcode.Newf(errcode.ErrBadRequest, "%s", strings.Join(msgs, "；"))
}

// fieldLabels 读取结构体 tag 中定义的 label（中文名）。
func fieldLabels(obj any) map[string]string {
	out := map[string]string{}
	t := reflect.TypeOf(obj)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if l := f.Tag.Get("label"); l != "" {
			out[f.Name] = l
		}
	}
	return out
}

func ruleMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "不能为空"
	case "min":
		return fmt.Sprintf("长度/值不能小于 %s", fe.Param())
	case "max":
		return fmt.Sprintf("长度/值不能大于 %s", fe.Param())
	case "email":
		return "格式不正确"
	case "len":
		return fmt.Sprintf("长度必须为 %s", fe.Param())
	case "oneof":
		return fmt.Sprintf("必须是以下值之一: %s", fe.Param())
	case "numeric":
		return "必须为数字"
	case "gt":
		return fmt.Sprintf("必须大于 %s", fe.Param())
	case "gte":
		return fmt.Sprintf("必须大于等于 %s", fe.Param())
	case "url":
		return "URL 格式不正确"
	default:
		return "不合法"
	}
}

// asValidationErrors 用 errors.As 判定校验错误。
//
// 不能写成 `err.(validator.ValidationErrors)` 的类型断言：gin 在部分版本里会把
// 校验错误包一层再往上抛，直接断言会失败，于是所有中文友好提示全部退化成
// 「请求参数格式错误: Key: 'LoginReq.username' Error:..." 这种英文原文。
func asValidationErrors(err error, target *validator.ValidationErrors) bool {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		*target = ve
		return true
	}
	return false
}
