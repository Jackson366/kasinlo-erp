// Package textutil 提供「按数据库列长度收敛文本」的工具。
//
// 为什么必须有这一层：PostgreSQL 的 varchar(n) 按**字符**计数，而 Go 的切片按**字节**。
// 两者混用会造出两个都会让**整行写入失败**的后果：
//
//  1. 超长 -> SQLSTATE 22001（value too long for type character varying(n)）
//  2. 按字节截断把多字节字符切成两半 -> 非法 UTF-8 -> SQLSTATE 22021
//
// 对本项目来说「整行写不进去」远比「内容被裁剪」严重：
// 审计日志写不进去等于没有审计；登录日志写不进去等于连续失败锁定失效。
// 所以所有来自外部的文本都要在写入边界上过一次 Fit。
package textutil

import (
	"strings"
	"time"
)

// FormatTime 把时间格式化为导出/展示通用的 "2006-01-02 15:04:05"；
// 零值返回空串 —— 导出场景里零值时间应显示为空而不是 "0001-01-01 00:00:00"。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// Fit 把 s 收敛到 limit 个字符以内，并保证结果是合法 UTF-8。
// limit <= 0 表示不限长度，但仍会剔除非法 UTF-8 字节。
func Fit(s string, limit int) string {
	s = strings.ToValidUTF8(s, "")
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
