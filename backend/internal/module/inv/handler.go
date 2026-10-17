package inv

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
)

// Handler 进销存接口处理器。
type Handler struct {
	svc *Service
}

// NewHandler 创建进销存处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// bizTypeOf 解析并校验单据类型参数。
//
// 采购与销售共用 /inv/orders，仅凭 URL 无法判断业务归属。若不在入口把它钉死成
// 二选一，调用方传什么都会被当成合法值往下走。
func bizTypeOf(c *gin.Context) (string, error) {
	v := strings.TrimSpace(c.Query("bizType"))
	if v != model.InvBizPurchase && v != model.InvBizSales {
		return "", errcode.Newf(errcode.ErrBadRequest, "参数 bizType 非法：%s（仅支持 purchase / sales）", v)
	}
	return v, nil
}

// ensureBizPerm 校验当前身份是否持有目标业务线的任一权限。
//
// 为什么必须在 handler 里再判一次：/inv/orders 的 RequirePerm 接受
// 「采购或销售任意一个」，若不按 bizType 精确校验，只有 inv:sales:list 的账号
// 传 bizType=purchase 就能读到采购单据 —— 越权并不需要绕过中间件，换个参数即可。
//
// 只用于**读**路径。写路径必须用 ensureBizPermAction 精确到动作，理由见那里。
func ensureBizPerm(c *gin.Context, bizType string) error {
	return ensureBizPermAction(c, bizType, "")
}

// ensureBizPermAction 校验「某业务线的某个动作」权限。action 为空表示
// 「该业务线任意权限即可」（读路径的宽松口径）。
//
// 为什么读可以宽松、写必须精确：路由上的 RequirePerm 是「采购或销售**任一**」，
// 等于把两条业务线的同名动作串在了一起。若写路径也只校验「业务线任意权限」，
// 那么一个持有「采购只读 + 销售审核」的账号就能审核**采购单** ——
// 它确实有 finish，但那个 finish 是**销售**的。
// 越权不需要绕过中间件，只要权限码拼得够巧就能穿过去。
func ensureBizPermAction(c *gin.Context, bizType, action string) error {
	id := middleware.CurrentIdentity(c)
	if id == nil {
		return errcode.New(errcode.ErrUnauthorized, "")
	}
	if id.IsSuper {
		return nil
	}
	prefix := "inv:" + bizType + ":"
	want := prefix + action
	for p := range id.Permissions {
		if action == "" {
			if strings.HasPrefix(p, prefix) {
				return nil
			}
			continue
		}
		if p == want {
			return nil
		}
	}
	if action == "" {
		return errcode.Newf(errcode.ErrForbidden, "缺少 %s 业务线权限", bizType)
	}
	return errcode.Newf(errcode.ErrForbidden, "缺少 %s 业务线的 %s 权限", bizType, action)
}

// orderStatusQuery 解析单据状态过滤参数（1 待处理 / 2 已审核 / 3 已取消）。
// 空串表示不过滤；非法值报 400 而不是静默忽略 —— 忽略会让「筛已审核」变成「全部」。
func orderStatusQuery(c *gin.Context) (*int16, error) {
	v := c.Query("status")
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 3 {
		return nil, errcode.Newf(errcode.ErrBadRequest, "查询参数 status 非法：%s（仅支持 1~3）", v)
	}
	s := int16(n)
	return &s, nil
}

// settleQuery 解析结算状态过滤参数（unpaid / partial / paid）。
//
// 同样不静默忽略：把 settleStatus=paied 这种笔误当成「全部」返回，
// 用户会以为系统里真没有已结清的单据。
func settleQuery(c *gin.Context) (string, error) {
	v := strings.TrimSpace(c.Query("settleStatus"))
	switch v {
	case "", model.InvSettleUnpaid, model.InvSettlePartial, model.InvSettlePaid:
		return v, nil
	default:
		return "", errcode.Newf(errcode.ErrBadRequest,
			"查询参数 settleStatus 非法：%s（仅支持 unpaid / partial / paid）", v)
	}
}

// orderRangeQuery 解析单据日期的起止（yyyy-MM-dd，闭区间）。
//
// 结束日按「当天 00:00」补齐：库里 order_date 是 date 类型，传时间戳也能比较，
// 但若直接拿用户传来的 23:59 之外的零值做 <=，会把当天整天漏掉。
func orderRangeQuery(c *gin.Context) (*time.Time, *time.Time, error) {
	parse := func(key string) (*time.Time, error) {
		v := strings.TrimSpace(c.Query(key))
		if v == "" {
			return nil, nil
		}
		t, err := parseDate(v)
		if err != nil {
			return nil, errcode.Newf(errcode.ErrBadRequest, "查询参数 %s 格式非法：%s（应为 yyyy-MM-dd）", key, v)
		}
		return &t, nil
	}
	start, err := parse("startDate")
	if err != nil {
		return nil, nil, err
	}
	end, err := parse("endDate")
	if err != nil {
		return nil, nil, err
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, nil, errcode.Newf(errcode.ErrBadRequest, "结束日期不能早于开始日期")
	}
	return start, end, nil
}
