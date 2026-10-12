package role

import (
	"strconv"

	"nova-admin/internal/model"
	"nova-admin/internal/pkg/dbquery"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/sheet"
	"nova-admin/internal/pkg/textutil"
)

// Export 查询待导出的角色。
func (s *Service) Export(keyword string) ([]VO, error) {
	tx := s.db.Model(&model.Role{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where("name ILIKE ? OR code ILIKE ?", kw, kw)
	}

	// 多查一条用于判断是否超限（dbquery 内部处理，调用方只关心 exceeded）
	roles, exceeded, err := dbquery.FetchWithCap[model.Role](tx.Order("sort ASC, id ASC"), sheet.MaxExportRows)
	if err != nil {
		return nil, err
	}
	if exceeded {
		return nil, errcode.Newf(errcode.ErrBadRequest,
			"单次导出上限 %d 条，请缩小筛选范围后重试", sheet.MaxExportRows)
	}

	items, err := s.toVOs(roles)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// BuildExportFile 把角色列表渲染成 xlsx 字节流。
func (s *Service) BuildExportFile(items []VO) ([]byte, error) {
	tbl := sheet.NewTable[VO]("角色数据").
		Col("ID", 8, func(v VO) string { return strconv.FormatInt(v.ID, 10) }).
		Col("角色名称", 18, func(v VO) string { return v.Name }).
		Col("角色标识", 18, func(v VO) string { return v.Code }).
		Col("排序", 8, func(v VO) string { return strconv.Itoa(v.Sort) }).
		Col("状态", 9, func(v VO) string { return roleStatusText(v.Status) }).
		Col("数据范围", 16, func(v VO) string { return dataScopeText(v.DataScope) }).
		Col("类型", 10, func(v VO) string {
			if v.IsBuiltin {
				return "内置"
			}
			return "自定义"
		}).
		Col("用户数", 9, func(v VO) string { return strconv.FormatInt(v.UserCount, 10) }).
		Col("备注", 30, func(v VO) string { return v.Remark }).
		Col("创建时间", 20, func(v VO) string { return textutil.FormatTime(v.CreatedAt) }).
		Rows(items)
	return tbl.Build()
}

func roleStatusText(status int16) string {
	if status == model.StatusEnabled {
		return "启用"
	}
	return "禁用"
}

// dataScopeText 返回数据范围的中文文案，与前端展示保持一致。
func dataScopeText(scope int16) string {
	switch scope {
	case model.DataScopeAll:
		return "全部数据"
	case model.DataScopeDept:
		return "本部门"
	case model.DataScopeDeptAndSub:
		return "本部门及以下"
	case model.DataScopeSelf:
		return "仅本人"
	default:
		return "未知"
	}
}
