// Package dberr 提供 PostgreSQL 驱动层错误码的判定辅助函数。
//
// 存在的理由：GORM 只会把驱动错误原样交回来，「先查后插」的 TOCTOU 窗口
// （两个请求同时通过唯一性预检）要靠数据库兜底。驱动错误不映射成业务码，
// 前端看到的就是 50100「数据库操作失败」——而真相只是「重名了，换一个」。
// 这类映射必须在写入边界上统一做，否则每个模块各写一份，
// 口径漂移是迟早的事（user 模块曾经就有一份私copy）。
package dberr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PgErrorCodeUniqueViolation 是 PostgreSQL 唯一约束冲突的 SQLSTATE。
const PgErrorCodeUniqueViolation = "23505"

// IsUniqueViolation 判断错误是否为 PostgreSQL 唯一约束冲突（SQLSTATE 23505）。
//
// 覆盖 gorm 包裹多层后的场景（errors.As 会逐层 Unwrap）。
// 非 pgconn.PgError（连接断开、上下文取消等）一律返回 false，
// 由调用方按数据库错误处理，不冒充成 409。
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == PgErrorCodeUniqueViolation
	}
	return false
}
