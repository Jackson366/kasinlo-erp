package log

import (
	"strconv"

	"nova-admin/internal/authz"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/page"
	"nova-admin/internal/pkg/sheet"
	"nova-admin/internal/pkg/textutil"
)

// 导出时对超长文本做截断，避免单元格内容撑爆表格
const (
	maxExportErrorMsg  = 200
	maxExportUserAgent = 120
)

// ExportOperations 查询待导出的操作日志（数据范围与列表同口径）。
func (s *Service) ExportOperations(q page.Query, f OperationQuery, actor *authz.Identity) ([]model.OperationLog, error) {
	filter := buildOperationFilter(q, f, authz.ResolveScope(actor))

	var rows []model.OperationLog
	// 多查一条用于判断是否超限
	if err := filter(s.db.Model(&model.OperationLog{})).
		Order("created_at DESC").Limit(sheet.MaxExportRows + 1).Find(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if len(rows) > sheet.MaxExportRows {
		return nil, errcode.Newf(errcode.ErrBadRequest,
			"单次导出上限 %d 条，请缩小时间范围后重试", sheet.MaxExportRows)
	}
	return rows, nil
}

// ExportLogins 查询待导出的登录日志（数据范围与列表同口径）。
func (s *Service) ExportLogins(q page.Query, f LoginQuery, actor *authz.Identity) ([]model.LoginLog, error) {
	filter := buildLoginFilter(q, f, authz.ResolveScope(actor))

	var rows []model.LoginLog
	if err := filter(s.db.Model(&model.LoginLog{})).
		Order("created_at DESC").Limit(sheet.MaxExportRows + 1).Find(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	if len(rows) > sheet.MaxExportRows {
		return nil, errcode.Newf(errcode.ErrBadRequest,
			"单次导出上限 %d 条，请缩小时间范围后重试", sheet.MaxExportRows)
	}
	return rows, nil
}

// BuildOperationsFile 把操作日志渲染成 xlsx 字节流。
func (s *Service) BuildOperationsFile(items []model.OperationLog) ([]byte, error) {
	tbl := sheet.NewTable[model.OperationLog]("操作日志").
		Col("ID", 8, func(v model.OperationLog) string { return strconv.FormatInt(v.ID, 10) }).
		Col("操作人", 14, func(v model.OperationLog) string { return v.Username }).
		Col("模块", 14, func(v model.OperationLog) string { return v.Module }).
		Col("操作", 20, func(v model.OperationLog) string { return v.Action }).
		Col("请求方法", 10, func(v model.OperationLog) string { return v.Method }).
		Col("请求路径", 32, func(v model.OperationLog) string { return v.Path }).
		Col("IP", 16, func(v model.OperationLog) string { return v.IP }).
		Col("状态码", 9, func(v model.OperationLog) string { return strconv.Itoa(v.Status) }).
		Col("耗时(ms)", 10, func(v model.OperationLog) string { return strconv.FormatInt(v.Latency, 10) }).
		Col("错误信息", 30, func(v model.OperationLog) string { return truncate(v.ErrorMsg, maxExportErrorMsg) }).
		Col("用户代理", 30, func(v model.OperationLog) string { return truncate(v.UserAgent, maxExportUserAgent) }).
		Col("操作时间", 20, func(v model.OperationLog) string { return textutil.FormatTime(v.CreatedAt) }).
		Rows(items)
	return tbl.Build()
}

// BuildLoginsFile 把登录日志渲染成 xlsx 字节流。
func (s *Service) BuildLoginsFile(items []model.LoginLog) ([]byte, error) {
	tbl := sheet.NewTable[model.LoginLog]("登录日志").
		Col("ID", 8, func(v model.LoginLog) string { return strconv.FormatInt(v.ID, 10) }).
		Col("用户名", 16, func(v model.LoginLog) string { return v.Username }).
		Col("IP", 16, func(v model.LoginLog) string { return v.IP }).
		Col("登录结果", 10, func(v model.LoginLog) string {
			if v.Success {
				return "成功"
			}
			return "失败"
		}).
		Col("说明", 24, func(v model.LoginLog) string { return v.Message }).
		Col("用户代理", 30, func(v model.LoginLog) string { return truncate(v.UserAgent, maxExportUserAgent) }).
		Col("登录时间", 20, func(v model.LoginLog) string { return textutil.FormatTime(v.CreatedAt) }).
		Rows(items)
	return tbl.Build()
}

// truncate 按字符（而非字节）截断，避免把多字节中文切坏。
func truncate(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
