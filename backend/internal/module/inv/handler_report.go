package inv

import (
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// ReportFilter 报表的查询条件：时间范围 + 品类。
//
// 时间用 range 预设或 from/to 自定义（后者优先）；品类按 categoryId 过滤。
// 品类筛选**包含其子分类**：报表看的是「钢材卖了多少钱」，
// 螺纹钢归在钢材下面，只算钢材自己那一行等于把子类的销售额丢了。
type ReportFilter struct {
	Range      string
	From       string
	To         string
	CategoryID int64
}

func reportFilterFrom(c *gin.Context) (ReportFilter, error) {
	categoryID, err := bind.IntQuery(c, "categoryId")
	if err != nil {
		return ReportFilter{}, err
	}
	return ReportFilter{
		Range:      c.Query("range"),
		From:       c.Query("from"),
		To:         c.Query("to"),
		CategoryID: categoryID,
	}, nil
}

// reportStart 解析筛选条件的起始时间。from 非空时以它为准，否则走 range 预设。
func (f ReportFilter) start() (time.Time, string, error) {
	if f.From != "" {
		t, err := parseDate(f.From)
		if err != nil {
			return time.Time{}, "", err
		}
		return t, "自定义", nil
	}
	s, label := rangeStart(f.Range)
	return s, label, nil
}

// reportEnd 解析筛选条件的结束时间（含当天，所以补到 23:59:59）。
func (f ReportFilter) end() (*time.Time, error) {
	if f.To == "" {
		return nil, nil
	}
	t, err := parseDate(f.To)
	if err != nil {
		return nil, err
	}
	e := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, database.Location())
	return &e, nil
}

// categoryScope 品类筛选的 SQL 片段：命中自身 + 全部子孙分类。
// 返回空串表示不筛选。用 IN 子查询而不是在 Go 里先查一遍 ID ——
// 少一次往返，也不会出现「分类刚删了但内存里还有」的竞态。
func (s *Service) categoryScope(categoryID int64, col string) (string, []any, error) {
	if categoryID <= 0 {
		return "", nil, nil
	}
	var ids []int64
	if err := s.db.Model(&model.InvCategory{}).
		Where("id = ?", categoryID).Pluck("id", &ids).Error; err != nil {
		return "", nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 沿子孙下钻：分类表很小，逐层展开比写递归 CTE 直白得多。
	// 上限用 categoryMaxDepth（与 categoryCycleFree 的防环口径一致），
	// 而不是各写一个魔法数 —— 两处不一致的话，其中一处会在异常深的
	// 合法分类上静默截断，且没有任何报错。
	for round := 0; round < categoryMaxDepth; round++ {
		var next []int64
		if err := s.db.Model(&model.InvCategory{}).
			Where("parent_id IN ?", ids).Pluck("id", &next).Error; err != nil {
			return "", nil, errcode.Wrap(errcode.ErrDatabase, err)
		}
		if len(next) == 0 {
			break
		}
		ids = append(ids, next...)
	}
	return col + " IN ?", []any{ids}, nil
}

// Overview 数据概览
// @Summary      数据概览
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=inv.OverviewVO}
// @Router       /inv/overview [get]
func (h *Handler) Overview(c *gin.Context) {
	data, err := h.svc.Overview()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportSummary 报表汇总
// @Summary      报表汇总（营业收入 / 成本 / 毛利）
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        range       query  string  false  "today | week | month | quarter | year"
// @Param        from        query  string  false  "自定义起始日 yyyy-MM-dd（优先于 range）"
// @Param        to          query  string  false  "自定义结束日 yyyy-MM-dd"
// @Param        categoryId  query  int     false  "品类筛选（含子分类），0 = 全部"
// @Success      200  {object}  response.Body{data=inv.ReportSummary}
// @Router       /inv/report/summary [get]
func (h *Handler) ReportSummary(c *gin.Context) {
	f, err := reportFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	data, err := h.svc.ReportSummary(f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportTrend 销售 / 采购趋势
// @Summary      销售与采购趋势
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        days         query  int     false  "天数，默认 30"
// @Param        from         query  string  false  "自定义起始日 yyyy-MM-dd"
// @Param        to           query  string  false  "自定义结束日 yyyy-MM-dd"
// @Param        categoryId   query  int     false  "品类筛选（含子分类），0 = 全部"
// @Success      200  {object}  response.Body{data=[]inv.ReportTrendPoint}
// @Router       /inv/report/trend [get]
func (h *Handler) ReportTrend(c *gin.Context) {
	f, err := reportFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	days := page.AtoiOr(c.Query("days"), 30)
	data, err := h.svc.ReportTrend(days, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportRanking 销售排行
// @Summary      销售排行（按客户）
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        range       query  string  false  "时间范围"
// @Param        from        query  string  false  "自定义起始日 yyyy-MM-dd（优先于 range）"
// @Param        to          query  string  false  "自定义结束日 yyyy-MM-dd"
// @Param        categoryId  query  int     false  "品类筛选（含子分类），0 = 全部"
// @Param        limit       query  int     false  "条数，默认 5"
// @Success      200  {object}  response.Body{data=[]inv.ReportRankRow}
// @Router       /inv/report/ranking [get]
func (h *Handler) ReportRanking(c *gin.Context) {
	f, err := reportFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	limit := page.AtoiOr(c.Query("limit"), 5)
	data, err := h.svc.ReportRanking(f, limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReportCategory 品类销售占比
// @Summary      品类销售占比
// @Tags         进销存-报表
// @Produce      json
// @Security     BearerAuth
// @Param        range       query  string  false  "时间范围"
// @Param        from        query  string  false  "自定义起始日 yyyy-MM-dd（优先于 range）"
// @Param        to          query  string  false  "自定义结束日 yyyy-MM-dd"
// @Param        categoryId  query  int     false  "品类筛选（含子分类），0 = 全部"
// @Success      200  {object}  response.Body{data=[]inv.ReportCategoryRow}
// @Router       /inv/report/category [get]
func (h *Handler) ReportCategory(c *gin.Context) {
	f, err := reportFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	data, err := h.svc.ReportCategory(f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
