package inv

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nova-erp/internal/model"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

type platformCategoryRow struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parentId"`
	Name     string `json:"name"`
	Sort     int    `json:"sort"`
	Status   int16  `json:"status"`
}

type platformCategoryFilter struct {
	ParentID *int64 `form:"parentId" binding:"omitempty,gte=0" label:"上级分类"`
	Status   *int16 `form:"status" binding:"omitempty,oneof=0 1" label:"状态"`
}

// platformCategories 查询共享商品分类，保留父级 ID 供调用方逐级查询。
//
// @Summary 查询商品分类
// @Tags 开放平台内部接口
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "分类名称（字面匹配）"
// @Param status query int false "启用状态（0 或 1）"
// @Param parentId query int false "直接父分类 ID，0 为根分类，省略不过滤"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=response.PageData{list=[]inv.platformCategoryRow}}
// @Failure 400 {object} response.Body
// @Router /internal/open-platform/product-categories [get]
func (h *Handler) platformCategories(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var filter platformCategoryFilter
	if err := bind.Query(c, &filter); err != nil {
		response.Fail(c, err)
		return
	}
	q := page.FromContext(c)
	rows, total, err := h.svc.platformCategoryList(q, filter)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// platformCategory 查询分类详情。
//
// @Summary 查询商品分类详情
// @Tags 开放平台内部接口
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "分类 ID"
// @Success 200 {object} response.Body{data=inv.platformCategoryRow}
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Router /internal/open-platform/product-categories/{id} [get]
func (h *Handler) platformCategory(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	row, err := h.svc.platformCategoryByID(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, row)
}

// 主数据保持共享；显式投影避免把内部备注和后续新增字段带出开放接口。
func (s *Service) platformCategoryRecords() *gorm.DB {
	return s.db.Model(&model.InvCategory{}).Select("id", "parent_id", "name", "sort", "status")
}

func (s *Service) platformCategoryList(q page.Query, filter platformCategoryFilter) ([]platformCategoryRow, int64, error) {
	tx := s.platformCategoryRecords()
	if q.Keyword != "" {
		tx = tx.Where(likeExpr("name"), likeKW(q.Keyword))
	}
	if filter.Status != nil {
		tx = tx.Where("status = ?", *filter.Status)
	}
	if filter.ParentID != nil {
		tx = tx.Where("parent_id = ?", *filter.ParentID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	rows := make([]platformCategoryRow, 0)
	if err := q.Paginate(tx.Order("parent_id ASC, sort ASC, id ASC")).Find(&rows).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, total, nil
}

func (s *Service) platformCategoryByID(id int64) (platformCategoryRow, error) {
	var row platformCategoryRow
	err := s.platformCategoryRecords().Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, errcode.New(errcode.ErrNotFound, "分类不存在")
	}
	if err != nil {
		return row, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return row, nil
}
