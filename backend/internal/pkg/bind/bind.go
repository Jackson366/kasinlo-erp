// Package bind 统一请求参数绑定与校验错误提示。
package bind

import (
	"fmt"
	"reflect"
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

// URI 绑定路径参数并校验。
func URI(c *gin.Context, obj any) error {
	if err := c.ShouldBindUri(obj); err != nil {
		return translate(obj, err)
	}
	return nil
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

func asValidationErrors(err error, target *validator.ValidationErrors) bool {
	if ve, ok := err.(validator.ValidationErrors); ok {
		*target = ve
		return true
	}
	return false
}
