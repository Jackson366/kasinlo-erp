package page

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultPage     = 1
	defaultPageSize = 10
	maxPageSize     = 200
)

// Query 是标准分页查询参数。
type Query struct {
	Page     int    `form:"page" json:"page"`         // 页码，从 1 开始
	PageSize int    `form:"pageSize" json:"pageSize"` // 每页条数
	Keyword  string `form:"keyword" json:"keyword"`   // 模糊搜索关键字
	OrderBy  string `form:"orderBy" json:"orderBy"`   // 排序字段，如 created_at
	Order    string `form:"order" json:"order"`       // asc | desc
}

// FromContext 从请求中解析分页参数并做边界收敛。
func FromContext(c *gin.Context) Query {
	q := Query{
		Page:     atoiOr(c.Query("page"), defaultPage),
		PageSize: atoiOr(c.Query("pageSize"), defaultPageSize),
		Keyword:  c.Query("keyword"),
		OrderBy:  c.Query("orderBy"),
		Order:    c.Query("order"),
	}
	q.Normalize()
	return q
}

// Normalize 收敛分页参数到合法区间。
func (q *Query) Normalize() {
	if q.Page < 1 {
		q.Page = defaultPage
	}
	if q.PageSize < 1 {
		q.PageSize = defaultPageSize
	}
	if q.PageSize > maxPageSize {
		q.PageSize = maxPageSize
	}
	if q.Order == "" {
		q.Order = "desc"
	}
}

// Offset 返回 SQL OFFSET。
func (q Query) Offset() int { return (q.Page - 1) * q.PageSize }

// Limit 返回 SQL LIMIT。
func (q Query) Limit() int { return q.PageSize }

// SafeOrderBy 返回受白名单约束的排序子句。
func (q Query) SafeOrderBy(allowed map[string]string, fallback string) string {
	dir := "DESC"
	if q.Order == "asc" || q.Order == "ASC" {
		dir = "ASC"
	}
	if col, ok := allowed[q.OrderBy]; ok {
		return col + " " + dir
	}
	return fallback
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
