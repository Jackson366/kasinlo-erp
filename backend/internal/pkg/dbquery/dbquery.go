// Package dbquery 收敛「带上限取数」这一重复模式。
//
// 四个导出接口（用户 / 角色 / 操作日志 / 登录日志）曾经各写一遍：
// Limit(cap+1) 多查一条、判断是否超限、超限报各自的文案。
// 抄四遍的真正风险不是重复本身，而是某一处哪天改了查询忘了保留 +1，
// 超限判断会静默失效，全表被读进内存 —— 而这种退化没有任何测试会红。
package dbquery

import (
	"gorm.io/gorm"

	"nova-erp/internal/pkg/errcode"
)

// FetchWithCap 按 cap 取数，内部多查一行用于判断是否超限。
//
// 返回的 rows 长度不超过 cap；exceeded 为 true 表示「实际还有更多行」，
// 由调用方决定报什么文案（导出场景各自不同：缩小筛选范围 / 缩小时间范围 / 分批导出）。
// 数据库错误统一归一为 errcode.ErrDatabase，调用方不必再包一层。
func FetchWithCap[T any](tx *gorm.DB, cap int) (rows []T, exceeded bool, err error) {
	out := make([]T, 0, cap+1)
	if err := tx.Limit(cap + 1).Find(&out).Error; err != nil {
		return nil, false, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if len(out) > cap {
		return out[:cap], true, nil
	}
	return out, false, nil
}
