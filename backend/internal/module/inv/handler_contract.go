package inv

import (
	"strconv"
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
	if v := strings.TrimSpace(c.Query("partyId")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 0 {
			return f, errcode.Newf(errcode.ErrBadRequest, "查询参数 partyId 非法：%s", v)
		}
		f.PartyID = id
	}
	// stage 是派生阶段的筛选值，允许值写死在这里：
	// 传错就 400，而不是静默返回全量（静默丢条件最难查）
	f.Stage = strings.TrimSpace(c.Query("stage"))
	switch f.Stage {
	case "", "pending", "accepted", "warranty", "warranty_due":
	default:
		return f, errcode.Newf(errcode.ErrBadRequest,
			"查询参数 stage 非法：%s（仅支持 pending / accepted / warranty / warranty_due）", f.Stage)
	}
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

// ContractAccept 合同验收
// @Summary      合同验收
// @Description  登记验收日期与结论，并按质保月数计算质保到期日。
// @Tags         进销存-合同
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int                true  "合同 ID"
// @Param        body  body  ContractAcceptReq  true  "验收信息"
// @Success      200   {object}  response.Body
// @Router       /inv/contracts/{id}/accept [post]
func (h *Handler) ContractAccept(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req ContractAcceptReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "合同验收")
	if err := h.svc.ContractAccept(id, req, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "验收已登记")
}

// ContractPlanSettle 合同期次收付款
// @Summary      合同期次收付款
// @Description  给一期付款计划登记收款（销售合同）或付款（采购合同），
// @Description  金额不得超过该期未收付余额；全部期次结清且质保了结后合同自动完成。
// @Tags         进销存-合同
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  PlanSettleReq  true  "收付款信息"
// @Success      200   {object}  response.Body
// @Router       /inv/contracts/plans/settle [post]
func (h *Handler) ContractPlanSettle(c *gin.Context) {
	var req PlanSettleReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "合同收付款")
	if err := h.svc.PlanSettle(req, middleware.CurrentIdentity(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "收付款已登记")
}

// ContractPlanSettles 期次收付款流水
// @Summary      期次收付款流水
// @Tags         进销存-合同
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  int  true  "期次 ID"
// @Success      200  {object}  response.Body{data=[]SettleRecord}
// @Router       /inv/contracts/plans/{id}/settles [get]
func (h *Handler) ContractPlanSettles(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, err := h.svc.PlanSettleHistory(id, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}
