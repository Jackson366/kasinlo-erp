package log

import (
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// Service 日志服务。
type Service struct {
	db  *gorm.DB
	sec config.SecurityConfig
}

// NewService 创建日志服务。
func NewService(sec config.SecurityConfig) *Service {
	return &Service{db: database.DB(), sec: sec}
}

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

// parseTimeStrict 与 parseTime 的区别：解析失败返回错误。
// 清空接口必须用它——「before 传错值」若静默返回 nil，语义会从
// 「按时间清空」放大成「清空范围内全部」，仅剩锁定窗口兜底。
func parseTimeStrict(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, errcode.Newf(errcode.ErrBadRequest, "时间格式无法解析：%s", s)
}

// Operations 分页查询操作日志。
//
// actor 决定数据范围：日志含 IP / UA 等敏感信息，只持有 system:log:list
// 不等于可以看全库——部门管理员只能看到自己范围内的用户产生的日志
// （口径与 dashboard 一致，见 authz.Scope.ApplyLog）。
func (s *Service) Operations(q page.Query, f OperationQuery, actor *authz.Identity) ([]model.OperationLog, int64, error) {
	filter := buildOperationFilter(q, f, authz.ResolveScope(actor))

	var total int64
	if err := filter(s.db.Model(&model.OperationLog{})).Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 预分配空切片：无记录时返回 [] 而不是 null，前端表格不必各自判空
	rows := make([]model.OperationLog, 0, q.Limit())
	if err := q.Paginate(filter(s.db.Model(&model.OperationLog{}))).
		Order(q.SafeOrderBy(opOrderWhitelist, "created_at DESC")).
		Find(&rows).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, total, nil
}

// buildOperationFilter 构造操作日志的 where 子句。
// 分页查询与 Excel 导出共用，保证导出的范围与页面筛选结果一致；
// 数据范围也在这里统一叠加，导出不可能比列表看到更多。
func buildOperationFilter(q page.Query, f OperationQuery, scope *authz.Scope) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		tx = scope.ApplyLog(tx)
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

// Logins 分页查询登录日志（数据范围同 Operations）。
func (s *Service) Logins(q page.Query, f LoginQuery, actor *authz.Identity) ([]model.LoginLog, int64, error) {
	filter := buildLoginFilter(q, f, authz.ResolveScope(actor))

	var total int64
	if err := filter(s.db.Model(&model.LoginLog{})).Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	rows := make([]model.LoginLog, 0, q.Limit())
	if err := q.Paginate(filter(s.db.Model(&model.LoginLog{}))).
		Order(q.SafeOrderBy(loginOrderWhitelist, "created_at DESC")).
		Find(&rows).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, total, nil
}

// buildLoginFilter 构造登录日志的 where 子句，分页与导出共用。
func buildLoginFilter(q page.Query, f LoginQuery, scope *authz.Scope) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		tx = scope.ApplyLog(tx)
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

// Modules 返回范围内出现过的模块名，供筛选下拉使用。
// 模块名本身不敏感，但全库模块列表会告诉部门管理员「系统里还有哪些业务」，
// 顺带按范围过滤也保证下拉选项与列表内容一致。
func (s *Service) Modules(actor *authz.Identity) ([]string, error) {
	scope := authz.ResolveScope(actor)
	out := make([]string, 0, 16)
	if err := scope.ApplyLog(s.db.Model(&model.OperationLog{})).
		Distinct().Order("module ASC").Pluck("module", &out).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return out, nil
}

// ClearOperations 清空指定时间之前的操作日志；before 为 nil 时全部清空。
//
// 清空是写操作，按「读与写一起收窄」原则同样受数据范围约束：
// 非超管只能清掉范围内用户产生的日志（含 user_id=0 的匿名记录删不到，
// 与 ApplyLog 的读口径完全一致）。「清空全部」按钮对范围管理员而言
// 语义上是「清空我范围内能看到的全部」，前端提示无需区分。
func (s *Service) ClearOperations(before *time.Time, actor *authz.Identity) (int64, error) {
	// s.db 是共享句柄，GORM 链式调用即克隆（Open 返回 clone=1），
	// 不需要 Where("1 = 1") 占位——那样反而让 WHERE 恒真一眼看不出真假。
	tx := authz.ResolveScope(actor).ApplyLog(s.db)
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
//
// 保护窗口：登录失败日志不只是审计证据，还是 auth.isLocked 判定账号锁定的计数来源。
// 若允许把这段时间的记录一并删掉，「清空登录日志」就等于「解除所有账号的锁定」——
// 攻击者只要能触发锁定，就能靠这个接口（或有权限的管理员误点一次）立刻恢复无限次尝试。
// 因此 earliest 是硬下界：无论 before 传什么，都不会删掉仍在锁定期内的记录。
func (s *Service) ClearLogins(before *time.Time, actor *authz.Identity) (int64, error) {
	lockSeconds := s.sec.LoginLockSeconds
	if lockSeconds <= 0 {
		lockSeconds = 300
	}
	// 锁定期内的日志一律保留
	earliest := time.Now().Add(-time.Duration(lockSeconds) * time.Second)
	// cutoff = min(before, earliest)：无论 before 传什么，硬下界不变。
	// 先算好再单次 ResolveScope——「本部门及以下」范围每次 ResolveScope 都会
	// 全表扫一遍 sys_depts 展开部门树，调两次纯属翻倍开销。
	cutoff := earliest
	if before != nil && before.Before(earliest) {
		cutoff = *before
	}
	// 范围约束与 ClearOperations 同口径；self-only 的范围管理员
	// 只能清掉自己的登录日志，锁定计数由认证中间件继续积累不受影响。
	tx := authz.ResolveScope(actor).ApplyLog(s.db.Where("created_at < ?", cutoff))
	res := tx.Delete(&model.LoginLog{})
	if res.Error != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	return res.RowsAffected, nil
}
