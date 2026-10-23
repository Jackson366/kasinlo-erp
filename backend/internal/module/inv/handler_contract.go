package inv

import (
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// contractStatusQuery 解析合同状态（1 执行中 / 2 已完成 / 3 已终止）。
//
// 不用 bind.StatusQuery：那是通用的 0/1 启用停用，合同有三态。
// 非法值一律 400 而不是静默忽略 —— 静默忽略等于用户要「只看已终止」
// 却拿到全量，筛选条件被悄悄丢掉。
func contractStatusQuery(c *gin.Context) (*int16, error) {
	v := strings.TrimSpace(c.Query("status"))
	if v == "" {
		return nil, nil
	}
	switch v {
	case "1":
		s := model.InvContractActive
		return &s, nil
	case "2":
		s := model.InvContractFinished
		return &s, nil
	case "3":
		s := model.InvContractTerminated
		return &s, nil
	default:
		return nil, errcode.Newf(errcode.ErrBadRequest, "查询参数 status 非法：%s（仅支持 1/2/3）", v)
	}
}

// contractFilterFrom 解析合同列表的筛选条件。
func contractFilterFrom(c *gin.Context) (ContractFilter, error) {
	f := ContractFilter{BizType: strings.TrimSpace(c.Query("bizType"))}
	if f.BizType != "" && f.BizType != model.InvBizPurchase && f.BizType != model.InvBizSales {
		return f, errcode.Newf(errcode.ErrBadRequest, "查询参数 bizType 非法：%s", f.BizType)
	}
	status, err := contractStatusQuery(c)
	if err != nil {
		return f, err
	}
	f.Status = status
	// 复用单据的日期区间解析：合同按**签订日期**筛，参数名同样是 startDate / endDate
	start, end, err := orderRangeQuery(c)
	if err != nil {
		return f, err
	}
	f.Start, f.End = start, end
	return f, nil
}

// ContractList 合同列表
// @Summary      合同列表
// @Description  按类型 / 状态 / 关键字 / 签订日期筛选，受数据范围约束。
// @Tags         进销存-合同
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int     false  "页码"
// @Param        pageSize   query  int     false  "每页条数"
// @Param        keyword    query  string  false  "编号 / 名称 / 往来单位 / 业务员"
// @Param        bizType    query  string  false  "purchase / sales"
// @Param        status     query  int     false  "1 执行中 / 2 已完成 / 3 已终止"
// @Param        startDate  query  string  false  "签订日期起 yyyy-MM-dd"
// @Param        endDate    query  string  false  "签订日期止 yyyy-MM-dd"
// @Success      200        {object}  response.Body{data=[]ContractRow}
// @Router       /inv/contracts [get]
func (h *Handler) ContractList(c *gin.Context) {
	f, err := contractFilterFrom(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	q := page.FromContext(c)
	rows, total, err := h.svc.ContractList(f, q, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, rows, total, q.Page, q.PageSize)
}

// ContractDetail 合同详情
// @Summary      合同详情
// @Tags         进销存-合同
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  int  true  "合同 ID"
// @Success      200  {object}  response.Body{data=ContractRow}
// @Router       /inv/contracts/{id} [get]
func (h *Handler) ContractDetail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	row, err := h.svc.ContractDetail(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, row)
}

// ContractCreate 新增合同
// @Summary      新增合同
// @Description  合同编号留空自动生成 HT + 流水号，填了就用填的（重号拒绝）。
// @Tags         进销存-合同
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  ContractReq  true  "合同内容"
// @Success      200   {object}  response.Body{data=response.IDData}
// @Router       /inv/contracts [post]
func (h *Handler) ContractCreate(c *gin.Context) {
	var req ContractReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增合同")
	id, err := h.svc.ContractCreate(req, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// ContractUpdate 修改合同
// @Summary      修改合同
// @Description  合同编号留空表示沿用原编号。
// @Tags         进销存-合同
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int          true  "合同 ID"
// @Param        body  body  ContractReq  true  "合同内容"
// @Success      200   {object}  response.Body
// @Router       /inv/contracts/{id} [put]
func (h *Handler) ContractUpdate(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req ContractReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改合同")
	if err := h.svc.ContractUpdate(id, req, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// ContractDelete 删除合同
// @Summary      删除合同
// @Tags         进销存-合同
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  int  true  "合同 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/contracts/{id} [delete]
func (h *Handler) ContractDelete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除合同")
	if err := h.svc.ContractDelete(id, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}
