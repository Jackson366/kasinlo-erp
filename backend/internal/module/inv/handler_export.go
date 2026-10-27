package inv

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
	"nova-erp/internal/pkg/sheet"
	"nova-erp/internal/pkg/textutil"
)

// Export 进销存列表导出（一个入口覆盖商品 / 客户 / 供应商 / 单据 / 出入库 / 库存）。
//
// 为什么不是每个列表一个接口：导出的参数与列表完全一致，做成 N 条路由
// 只是把同一段「解析筛选 → 查全量 → 建表 → 下发」抄 N 遍。
// 实体白名单在 service 的 switch 里，非法值直接 40000。
//
// 权限沿用列表的 list 权限（不另设 export 权限点）：导出是「查看」的延伸，
// 能看列表就能把同一批数据存成文件；再开一个权限点只会让管理员多配一遍，
// 而**看不到的数据本来就查不出来**（服务层仍按数据范围收窄）。
//
// @Summary      导出进销存列表
// @Description  返回 xlsx 二进制流，筛选参数与对应列表接口一致。
// @Tags         进销存-通用
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        entity      query     string  true   "product / customer / supplier / order / stock / inventory"
// @Param        bizType      query     string  false  "entity=order 时必填：purchase / sales"
// @Param        keyword      query     string  false  "关键字"
// @Param        status       query     int     false  "状态"
// @Param        categoryId   query     int     false  "商品分类"
// @Param        includeVoided query bool false "包含已作废出入库单据"
// @Param        direction    query     string  false  "出入库方向：in / out"
// @Param        settleStatus query     string  false  "结算状态：unpaid / partial / paid"
// @Param        startDate    query     string  false  "开始日期 yyyy-MM-dd"
// @Param        endDate      query     string  false  "结束日期 yyyy-MM-dd"
// @Success      200          {file}    binary  "xlsx 文件流"
// @Failure      400          {object}  response.Body  "参数非法或无可导出数据"
// @Router       /inv/export [get]
func (h *Handler) Export(c *gin.Context) {
	c.Set("opAction", "导出进销存列表")

	entity := strings.TrimSpace(c.Query("entity"))
	if err := ensureExportPerm(c, entity); err != nil {
		response.Fail(c, err)
		return
	}

	q := page.FromContext(c)
	f, err := exportQuery(c, entity, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	data, filename, err := h.svc.ExportInv(f, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	base := strings.TrimSuffix(filename, ".xlsx")
	sheet.Download(c, base+"_"+textutil.Timestamp()+".xlsx", data)
}

// exportQuery 解析导出参数。与列表接口共用同一批解析函数，
// 保证「页面上筛什么、导出就是什么」。
func exportQuery(c *gin.Context, entity string, q page.Query) (ExportQuery, error) {
	f := ExportQuery{
		Entity:     entity,
		Keyword:    q.Keyword,
		CategoryID: int64(atoiOrZero(c.Query("categoryId"))),
		Direction:  strings.TrimSpace(c.Query("direction")),
	}
	if entity == exportStock {
		includeVoided, err := stockHistoryQuery(c)
		if err != nil {
			return f, err
		}
		f.IncludeVoided = includeVoided
	}
	ids, err := parseIDList(c.Query("ids"))
	if err != nil {
		return f, err
	}
	f.IDs = ids
	status, err := orderStatusQuery(c)
	if err != nil {
		return f, err
	}
	// 商品 / 往来单位的状态是 0/1，单据状态是 1/2/3 —— 两者的取值范围不同，
	// 不能共用同一个解析器，否则「停用商品」的 status=0 会被判成非法。
	switch entity {
	case exportProduct, exportCustomer, exportSupplier:
		enabled, err := enabledStatusQuery(c)
		if err != nil {
			return f, err
		}
		f.Status = enabled
	case exportOrder:
		f.Status = status
		settle, err := settleQuery(c)
		if err != nil {
			return f, err
		}
		bizType, err := bizTypeOf(c)
		if err != nil {
			return f, err
		}
		start, end, err := orderRangeQuery(c)
		if err != nil {
			return f, err
		}
		f.Order = OrderFilter{BizType: bizType, Status: status, Settle: settle, Start: start, End: end, Handoff: c.Query("handoff")}
	case exportFinance:
		// 应收应付的筛选比单据多一个「已逾期」，用单独的解析器
		settle, err := financeSettleQuery(c)
		if err != nil {
			return f, err
		}
		bizType, err := bizTypeOf(c)
		if err != nil {
			return f, err
		}
		// 来源筛选：空 / order / plan。传错就 400（静默忽略会让导出的范围
		// 比页面看到的大，而导出的文件是拿去对账的）。
		source := strings.TrimSpace(c.Query("source"))
		switch source {
		case "", model.InvSettleSourceOrder, model.InvSettleSourcePlan:
		default:
			return f, errcode.Newf(errcode.ErrBadRequest,
				"查询参数 source 非法：%s（仅支持 order / plan）", source)
		}
		f.Order = OrderFilter{BizType: bizType, Settle: settle, Source: source}
	case exportContract:
		// 合同允许「全部类型」，所以不用 bizTypeOf（那个要求必填）
		biz := strings.TrimSpace(c.Query("bizType"))
		if biz != "" && biz != model.InvBizPurchase && biz != model.InvBizSales {
			return f, errcode.Newf(errcode.ErrBadRequest, "查询参数 bizType 非法：%s", biz)
		}
		start, end, err := orderRangeQuery(c)
		if err != nil {
			return f, err
		}
		f.Contract = ContractFilter{BizType: biz, Status: status, Start: start, End: end}
	}
	if entity == exportStock && f.Direction != "" &&
		f.Direction != model.InvStockIn && f.Direction != model.InvStockOut {
		return f, errcode.Newf(errcode.ErrBadRequest, "查询参数 direction 非法：%s", f.Direction)
	}
	return f, nil
}

// ensureExportPerm 按实体校验读权限。
//
// 单据/出入库还要看业务线：只有 inv:sales:list 的账号不能借 entity=order
// 把采购单导出去 —— 越权不需要绕过中间件，换个参数就够了。
func ensureExportPerm(c *gin.Context, entity string) error {
	switch entity {
	case exportOrder, exportFinance:
		bizType, err := bizTypeOf(c)
		if err != nil {
			return err
		}
		return ensureBizPerm(c, bizType)
	case exportStock, exportInventory:
		return requirePerm(c, "inv:stock:list")
	case exportProduct:
		// 与 /inv/products 的权限集合保持一致：能看商品列表的四条业务线都能导出，
		// 否则会出现「列表打得开、导出按钮却 403」的割裂
		return requireAnyPerm(c, "inv:product:list", "inv:purchase:list", "inv:sales:list", "inv:stock:list")
	case exportCustomer:
		return requireAnyPerm(c, "inv:customer:list", "inv:sales:list")
	case exportSupplier:
		return requireAnyPerm(c, "inv:supplier:list", "inv:purchase:list")
	case exportContract:
		return requirePerm(c, "inv:contract:list")
	default:
		return errcode.Newf(errcode.ErrBadRequest, "查询参数 entity 非法：%s", entity)
	}
}

// requirePerm 单一权限点校验（超管直接放行）。
func requirePerm(c *gin.Context, perm string) error {
	id := middleware.CurrentIdentity(c)
	if id == nil {
		return errcode.New(errcode.ErrUnauthorized, "")
	}
	if id.IsSuper {
		return nil
	}
	if _, ok := id.Permissions[perm]; ok {
		return nil
	}
	return errcode.Newf(errcode.ErrForbidden, "缺少权限：%s", perm)
}

// requireAnyPerm 任一条权限命中即放行（与路由上的 RequirePerm 多参语义一致）。
func requireAnyPerm(c *gin.Context, perms ...string) error {
	id := middleware.CurrentIdentity(c)
	if id == nil {
		return errcode.New(errcode.ErrUnauthorized, "")
	}
	if id.IsSuper {
		return nil
	}
	for _, p := range perms {
		if _, ok := id.Permissions[p]; ok {
			return nil
		}
	}
	return errcode.Newf(errcode.ErrForbidden, "缺少权限：%s", strings.Join(perms, " 或 "))
}

// enabledStatusQuery 解析「启用 / 停用」这类 0/1 状态参数。
func enabledStatusQuery(c *gin.Context) (*int16, error) {
	v := strings.TrimSpace(c.Query("status"))
	if v == "" {
		return nil, nil
	}
	switch v {
	case "0":
		s := int16(0)
		return &s, nil
	case "1":
		s := int16(1)
		return &s, nil
	default:
		return nil, errcode.Newf(errcode.ErrBadRequest, "查询参数 status 非法：%s（仅支持 0 / 1）", v)
	}
}

func atoiOrZero(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// parseIDList 解析逗号分隔的 id 列表（勾选导出用）。
//
// 非法片段直接报错而不是跳过：跳过会让用户以为「导出成功了」，
// 实际上少了几行 —— 表格里少了哪一行，事后根本对不出来。
func parseIDList(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 {
			return nil, errcode.Newf(errcode.ErrBadRequest, "查询参数 ids 非法：%s", raw)
		}
		out = append(out, n)
	}
	if len(out) > sheet.MaxExportRows {
		return nil, errcode.Newf(errcode.ErrBadRequest, "一次最多导出 %d 条", sheet.MaxExportRows)
	}
	return out, nil
}

// financeSettleQuery 应收应付的结算筛选：比单据多一个 overdue（已逾期）。
func financeSettleQuery(c *gin.Context) (string, error) {
	v := strings.TrimSpace(c.Query("settleStatus"))
	switch v {
	case "", model.InvSettleUnpaid, model.InvSettlePartial, model.InvSettlePaid, "overdue":
		return v, nil
	default:
		return "", errcode.Newf(errcode.ErrBadRequest,
			"查询参数 settleStatus 非法：%s（仅支持 unpaid / partial / paid / overdue）", v)
	}
}
