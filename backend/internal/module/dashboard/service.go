package dashboard

import (
	"time"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
)

// Service 仪表盘服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建仪表盘服务。
func NewService() *Service { return &Service{db: database.DB()} }

// Overview 概览统计数据。
type Overview struct {
	UserCount      int64 `json:"userCount"`
	RoleCount      int64 `json:"roleCount"`
	DeptCount      int64 `json:"deptCount"`
	MenuCount      int64 `json:"menuCount"`
	TodayLogins    int64 `json:"todayLogins"`
	TodayOps       int64 `json:"todayOps"`
	TodayFailures  int64 `json:"todayFailures"`
	ActiveUserWeek int64 `json:"activeUserWeek"`
}

// TrendPoint 单日趋势点。
type TrendPoint struct {
	Date      string `json:"date"`
	Logins    int64  `json:"logins"`
	Opers     int64  `json:"opers"`
	FailCount int64  `json:"failCount"`
}

// ModuleStat 模块操作统计。
type ModuleStat struct {
	Module string `json:"module"`
	Total  int64  `json:"total"`
}

// Overview 返回概览数据。actor 的数据范围会作用在「人」的统计上：
// userCount、今日登录/操作/失败、近一周活跃用户都只算范围内的人；
// roleCount / menuCount 是配置元数据（有多少种角色、多少个菜单），
// 不含业务数据，全量返回反而能让使用者判断系统规模。
func (s *Service) Overview(actor *authz.Identity) (*Overview, error) {
	var out Overview
	start, end := todayRange()
	scope := authz.ResolveScope(actor)

	// scopeUser / scopeLog 把范围条件分别叠在 sys_users 与带 user_id 的日志表上
	scopeUser := func(tx *gorm.DB) *gorm.DB { return scope.ApplyUser(tx) }
	scopeLog := func(tx *gorm.DB) *gorm.DB { return scope.ApplyLog(tx) }

	queries := []struct {
		dest  *int64
		model any
		cond  func(*gorm.DB) *gorm.DB
	}{
		{&out.UserCount, &model.User{}, scopeUser},
		{&out.RoleCount, &model.Role{}, nil},
		{&out.DeptCount, &model.Dept{}, nil},
		{&out.MenuCount, &model.Menu{}, nil},
		{&out.TodayLogins, &model.LoginLog{}, func(tx *gorm.DB) *gorm.DB {
			return scopeLog(tx.Where("success = ? AND created_at >= ? AND created_at < ?", true, start, end))
		}},
		{&out.TodayFailures, &model.LoginLog{}, func(tx *gorm.DB) *gorm.DB {
			return scopeLog(tx.Where("success = ? AND created_at >= ? AND created_at < ?", false, start, end))
		}},
		{&out.TodayOps, &model.OperationLog{}, func(tx *gorm.DB) *gorm.DB {
			return scopeLog(tx.Where("created_at >= ? AND created_at < ?", start, end))
		}},
	}

	for _, q := range queries {
		tx := s.db.Model(q.model)
		if q.cond != nil {
			tx = q.cond(tx)
		}
		if err := tx.Count(q.dest).Error; err != nil {
			return nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
	}

	// 近 7 天活跃用户（有操作记录的去重用户数）
	weekAgo := time.Now().AddDate(0, 0, -7)
	if err := scope.ApplyLog(s.db.Model(&model.OperationLog{}).
		Where("created_at >= ? AND user_id > 0", weekAgo)).
		Distinct("user_id").Count(&out.ActiveUserWeek).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return &out, nil
}

// Trend 返回最近 days 天的登录与操作趋势。
func (s *Service) Trend(days int, actor *authz.Identity) ([]TrendPoint, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	// 日期键口径与 SQL 的 DATE(created_at) 一致：都用数据库会话时区。
	zone := database.Location()
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -(days - 1))
	scope := authz.ResolveScope(actor)

	type dayCount struct {
		Day   time.Time
		Total int64
	}
	groupByDay := func(m any, extra func(*gorm.DB) *gorm.DB) (map[string]int64, error) {
		tx := s.db.Model(m).
			Select("DATE(created_at) AS day, COUNT(*) AS total").
			Where("created_at >= ?", startDate).
			Group("DATE(created_at)")
		if extra != nil {
			tx = extra(tx)
		}
		var rows []dayCount
		if err := tx.Scan(&rows).Error; err != nil {
			return nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
		out := make(map[string]int64, len(rows))
		for _, r := range rows {
			out[r.Day.Format("2006-01-02")] = r.Total
		}
		return out, nil
	}

	logins, err := groupByDay(&model.LoginLog{}, func(tx *gorm.DB) *gorm.DB {
		return scope.ApplyLog(tx.Where("success = ?", true))
	})
	if err != nil {
		return nil, err
	}
	fails, err := groupByDay(&model.LoginLog{}, func(tx *gorm.DB) *gorm.DB {
		return scope.ApplyLog(tx.Where("success = ?", false))
	})
	if err != nil {
		return nil, err
	}
	opers, err := groupByDay(&model.OperationLog{}, func(tx *gorm.DB) *gorm.DB {
		return scope.ApplyLog(tx)
	})
	if err != nil {
		return nil, err
	}

	out := make([]TrendPoint, 0, days)
	for i := 0; i < days; i++ {
		d := startDate.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, TrendPoint{
			Date:      d,
			Logins:    logins[d],
			Opers:     opers[d],
			FailCount: fails[d],
		})
	}
	return out, nil
}

// ModuleStats 返回最近 days 天各模块操作量排行。
func (s *Service) ModuleStats(days, limit int, actor *authz.Identity) ([]ModuleStat, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	since := time.Now().AddDate(0, 0, -days)
	var out []ModuleStat
	tx := authz.ResolveScope(actor).ApplyLog(s.db.Model(&model.OperationLog{})).
		Select("module, COUNT(*) AS total").
		Where("created_at >= ? AND module <> ''", since)
	if err := tx.Group("module").Order("total DESC").Limit(limit).Scan(&out).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return out, nil
}

// RecentLogins 返回最近登录记录。
//
// 这里比概览更敏感：登录日志带用户名、IP、UserAgent，跨部门可见等于
// 把「谁在什么时候、从哪登录的」全量交出去，所以必须按数据范围收窄。
func (s *Service) RecentLogins(limit int, actor *authz.Identity) ([]model.LoginLog, error) {
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	var rows []model.LoginLog
	tx := authz.ResolveScope(actor).ApplyLog(s.db).Where("1 = 1")
	if err := tx.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, nil
}

func todayRange() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, database.Location())
	return start, start.AddDate(0, 0, 1)
}
