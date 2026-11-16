package inv

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

type platformPartyRow struct {
	ID     int64  `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status int16  `json:"status"`
}

// platformParties 查询客户或供应商档案。
//
// @Summary 查询客户或供应商档案
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "往来单位编号或名称"
// @Param status query int false "启用状态（0 或 1）"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Body{data=response.PageData{list=[]inv.platformPartyRow}}
// @Router /internal/open-platform/customers [get]
// @Router /internal/open-platform/suppliers [get]
func (h *Handler) platformParties(bizType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		q := page.FromContext(c)
		status, err := bind.StatusQuery(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		rows, total, err := h.svc.platformPartyList(bizType, q, status)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OKPage(c, rows, total, q.Page, q.PageSize)
	}
}

// platformParty 按 ID 查询客户或供应商档案。
//
// @Summary 查询客户或供应商详情
// @Tags 开放平台内部接口
// @Produce json
// @Security BearerAuth
// @Param id path int true "往来单位 ID"
// @Success 200 {object} response.Body{data=inv.platformPartyRow}
// @Router /internal/open-platform/customers/{id} [get]
// @Router /internal/open-platform/suppliers/{id} [get]
func (h *Handler) platformParty(bizType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		id, err := bind.PathID(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		row, err := h.svc.platformPartyByID(bizType, id)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, row)
	}
}

func (s *Service) platformPartyList(bizType string, q page.Query, status *int16) ([]platformPartyRow, int64, error) {
	tx := s.partyRecords(bizType)
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		tx = tx.Where(likeExpr("code")+" OR "+likeExpr("name"), kw, kw)
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	rows := make([]platformPartyRow, 0)
	if err := q.Paginate(tx.Select("id", "code", "name", "status").Order("id DESC")).Find(&rows).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return rows, total, nil
}

func (s *Service) platformPartyByID(bizType string, id int64) (platformPartyRow, error) {
	var row platformPartyRow
	err := s.partyRecords(bizType).Select("id", "code", "name", "status").Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, errcode.New(errcode.ErrNotFound, "往来单位不存在")
	}
	if err != nil {
		return row, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return row, nil
}
