package page

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultPage     = 1
	defaultPageSize = 10
	maxPageSize     = 200
	// maxOffset 是 OFFSET 的上界。
	//
	// PageSize 已经封顶，但**页码没有**：Offset = (Page-1) * PageSize，
	// 传 `?page=99999999` 会算出接近 2×10⁹ 的 OFFSET。PostgreSQL 执行
	// `LIMIT n OFFSET 2000000000` 时必须先扫描并丢弃这 2 亿行 —— 单个请求
	// 就能把连接池里的连接占满几分钟，且 CPU/IO 曲线和真实业务量无关，
	// 是很容易触发的拒绝服务。超过上界直接收敛到末页附近，
	// 返回空结果比让数据库扫 2 亿行正确。
	maxOffset = 1000000
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
	// 先按 OFFSET 上界反推页码上界，顺带避免 (Page-1)*PageSize 的整型溢出
	// （page 来自 strconv.Atoi，能到 9×10¹⁸，乘 200 直接溢出成负数）。
	maxPage := maxOffset/q.PageSize + 1
	if q.Page > maxPage {
		q.Page = maxPage
	}
	if q.Order == "" {
		q.Order = "desc"
	}
}

// Offset 返回 SQL OFFSET，已保证不超过 maxOffset。
func (q Query) Offset() int {
	off := (q.Page - 1) * q.PageSize
	if off > maxOffset {
		return maxOffset
	}
	if off < 0 {
		return 0
	}
	return off
}

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
