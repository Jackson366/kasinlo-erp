package middleware

import (
	"go.uber.org/zap"
)

// 本文件集中封装少量 zap 字段构造，保持业务代码简洁。

func zapStr(k, v string) zap.Field              { return zap.String(k, v) }
func zapErr(err error) zap.Field                { return zap.Error(err) }
func zapStrings(k string, v []string) zap.Field { return zap.Strings(k, v) }
