package log

import (
	"time"

	"gorm.io/gorm"

	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/page"
)

// Service 日志服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建日志服务。
func NewService() *Service { return &Service{db: database.DB()} }

var (
	opOrderWhitelist = map[string]string{
		"createdAt": "created_at",
		"latencyMs": "latency_ms",
		"status":    "status",
	}
	loginOrderWhitelist = map[string]string{
		"createdAt": "created_at",
	}
)

// OperationQuery 操作日志查询条件。时间为 RFC3339 或 "2006-01-02 15:04:05" 格式字符串。
type OperationQuery struct {
	Username string `form:"username" label:"用户名"`
	Module   string `form:"module" label:"模块"`
	Method   string `form:"method" label:"请求方法"`
	Status   *int   `form:"status" label:"状态码"`
	StartAt  string `form:"startAt" label:"开始时间"`
	EndAt    string `form:"endAt" label:"结束时间"`
}

// LoginQuery 登录日志查询条件。
type LoginQuery struct {
	Username string `form:"username" label:"用户名"`
	Success  *bool  `form:"success" label:"是否成功"`
	StartAt  string `form:"startAt" label:"开始时间"`
	EndAt    string `form:"endAt" label:"结束时间"`
}

// parseTime 兼容多种时间字符串格式。
func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return &t
		}
	}
	return nil
}

// Operations 分页查询操作日志。
func (s *Service) Operations(q page.Query, f OperationQuery) ([]model.OperationLog, int64, error) {
	filter := buildOperationFilter(q, f)

	var total int64
	if err := filter(s.db.Model(&model.OperationLog{})).Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var rows []model.OperationLog
	if err := filter(s.db.Model(&model.OperationLog{})).
		Order(q.SafeOrderBy(opOrderWhitelist, "created_at DESC")).
		Offset(q.Offset()).Limit(q.Limit()).Find(&rows).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, total, nil
}

// buildOperationFilter 构造操作日志的 where 子句。
// 分页查询与 Excel 导出共用，保证导出的范围与页面筛选结果一致。
func buildOperationFilter(q page.Query, f OperationQuery) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		if q.Keyword != "" {
			kw := "%" + q.Keyword + "%"
			tx = tx.Where("username ILIKE ? OR action ILIKE ? OR path ILIKE ?", kw, kw, kw)
		}
		if f.Username != "" {
			tx = tx.Where("username ILIKE ?", "%"+f.Username+"%")
		}
		if f.Module != "" {
			tx = tx.Where("module = ?", f.Module)
		}
		if f.Method != "" {
			tx = tx.Where("method = ?", f.Method)
		}
		if f.Status != nil {
			tx = tx.Where("status = ?", *f.Status)
		}
		if start := parseTime(f.StartAt); start != nil {
			tx = tx.Where("created_at >= ?", *start)
		}
		if end := parseTime(f.EndAt); end != nil {
			tx = tx.Where("created_at <= ?", *end)
		}
		return tx
	}
}

// Logins 分页查询登录日志。
func (s *Service) Logins(q page.Query, f LoginQuery) ([]model.LoginLog, int64, error) {
	filter := buildLoginFilter(q, f)

	var total int64
	if err := filter(s.db.Model(&model.LoginLog{})).Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var rows []model.LoginLog
	if err := filter(s.db.Model(&model.LoginLog{})).
		Order(q.SafeOrderBy(loginOrderWhitelist, "created_at DESC")).
		Offset(q.Offset()).Limit(q.Limit()).Find(&rows).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, total, nil
}

// buildLoginFilter 构造登录日志的 where 子句，分页与导出共用。
func buildLoginFilter(q page.Query, f LoginQuery) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		if q.Keyword != "" {
			kw := "%" + q.Keyword + "%"
			tx = tx.Where("username ILIKE ? OR ip ILIKE ? OR message ILIKE ?", kw, kw, kw)
		}
		if f.Username != "" {
			tx = tx.Where("username ILIKE ?", "%"+f.Username+"%")
		}
		if f.Success != nil {
			tx = tx.Where("success = ?", *f.Success)
		}
		if start := parseTime(f.StartAt); start != nil {
			tx = tx.Where("created_at >= ?", *start)
		}
		if end := parseTime(f.EndAt); end != nil {
			tx = tx.Where("created_at <= ?", *end)
		}
		return tx
	}
}

// Modules 返回出现过的模块名，供筛选下拉使用。
func (s *Service) Modules() ([]string, error) {
	var out []string
	if err := s.db.Model(&model.OperationLog{}).
		Distinct().Order("module ASC").Pluck("module", &out).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return out, nil
}

// ClearOperations 清空指定时间之前的操作日志；before 为 nil 时全部清空。
func (s *Service) ClearOperations(before *time.Time) (int64, error) {
	tx := s.db.Where("1 = 1")
	if before != nil {
		tx = tx.Where("created_at < ?", *before)
	}
	res := tx.Delete(&model.OperationLog{})
	if res.Error != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	return res.RowsAffected, nil
}

// ClearLogins 清空指定时间之前的登录日志；before 为 nil 时全部清空。
func (s *Service) ClearLogins(before *time.Time) (int64, error) {
	tx := s.db.Where("1 = 1")
	if before != nil {
		tx = tx.Where("created_at < ?", *before)
	}
	res := tx.Delete(&model.LoginLog{})
	if res.Error != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	return res.RowsAffected, nil
}
