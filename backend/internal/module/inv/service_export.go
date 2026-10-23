package inv

import (
	"strconv"
	"time"

	"nova-erp/internal/authz"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/sheet"
)

// 导出实体。前端只传这几个值，其余一律 40000 —— 用「白名单 switch」而不是
// 反射/map 拼装，是为了让「哪个列表能导出」在代码里是一份可读的清单。
const (
	exportProduct   = "product"
	exportCustomer  = "customer"
	exportSupplier  = "supplier"
	exportOrder     = "order"
	exportStock     = "stock"
	exportInventory = "inventory"
	exportFinance   = "finance"
	exportContract  = "contract"
)

// ExportQuery 导出参数。字段与列表页一一对应（导出 = 把当前筛选结果落成文件），
// 少一个参数就会出现「看到的 20 条」和「导出的 20 条」不是同一批。
type ExportQuery struct {
	Entity     string
	Keyword    string
	Status     *int16
	CategoryID int64
	Direction  string
	Order      OrderFilter
	Contract   ContractFilter
	// IDs 非空时只导出这些记录（列表勾选后「导出选中」）。
	// 用 id 过滤而不是新开一套查询：勾选导出与整表导出必须走同一条取数路径，
	// 否则两边的口径会慢慢漂开。
	IDs []int64
}

// ExportInv 按实体导出 xlsx，返回文件内容与建议文件名。
//
// 行数上限沿用 sheet.MaxExportRows：导出是同步接口，一把捞十万行会把内存和
// 响应时间一起拖垮；超出部分由前端提示「缩小筛选范围」。
func (s *Service) ExportInv(q ExportQuery, actor *authz.Identity) ([]byte, string, error) {
	// 分页参数只借用「页码 1 + 大页长」，其余仍走各实体的列表方法 ——
	// 导出与列表共用同一套查询，不会出现「列表查得到、导出查不到」的口径漂移。
	// 关键字随分页参数一起带下去：各列表方法的筛选都读 q.Keyword，
	// 这里丢掉它就会出现「页面筛了关键字，导出却是全表」
	pageQ := page.Query{Page: 1, PageSize: sheet.MaxExportRows, Keyword: q.Keyword}

	switch q.Entity {
	case exportProduct:
		rows, err := s.exportProductRows(pageQ, q)
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 0 {
			return nil, "", errNoExportData()
		}
		data, err := buildExport("商品档案", []string{"编码", "名称", "条码", "分类", "规格", "规格数", "单位数", "总库存", "状态", "备注"}, rows)
		return data, "商品档案.xlsx", err

	case exportCustomer, exportSupplier:
		bizType := model.InvBizSales
		label := "客户"
		if q.Entity == exportSupplier {
			bizType = model.InvBizPurchase
			label = "供应商"
		}
		rows, err := s.exportPartyRows(bizType, pageQ, q, actor)
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 0 {
			return nil, "", errNoExportData()
		}
		data, err := buildExport(label+"档案", []string{"编号", "名称", "联系人", "电话", "地址", "开票信息", "期初余额", "状态", "备注"}, rows)
		return data, label + "档案.xlsx", err

	case exportOrder:
		label := "采购单"
		if q.Order.BizType == model.InvBizSales {
			label = "销售单"
		}
		list, err := s.exportOrderList(pageQ, q, actor)
		if err != nil {
			return nil, "", err
		}
		if len(list) == 0 {
			return nil, "", errNoExportData()
		}
		// 单据导出**带明细**：只看列表的「金额」对不出账，收到文件的人还得
		// 回头一张张点详情。所以出两个工作表 —— 列表看汇总，明细看每一行。
		detail, err := s.exportOrderDetailRows(list)
		if err != nil {
			return nil, "", err
		}
		data, err := buildReportWorkbook([]reportSheet{
			{
				Name:    label + "列表",
				Headers: []string{"单号", "往来单位", "单据日期", "到期日", "状态", "明细行数", "金额", "已结算", "结算状态", "业务员", "制单人", "备注"},
				Widths:  []float64{20, 22, 12, 12, 10, 10, 14, 14, 12, 12, 12, 28},
				Rows:    orderRowCells(list),
			},
			{
				Name:    label + "明细",
				Headers: []string{"单号", "往来单位", "业务员", "商品编码", "商品名称", "规格", "单位", "数量", "换算率", "折算基本数量", "单价", "金额"},
				Widths:  []float64{20, 22, 12, 14, 24, 14, 10, 12, 10, 16, 12, 14},
				Rows:    detail,
			},
		})
		return data, label + "列表.xlsx", err

	case exportContract:
		rows, err := s.exportContractRows(pageQ, q.Contract, actor)
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 0 {
			return nil, "", errNoExportData()
		}
		data, err := buildExport("合同台账",
			[]string{"合同编号", "合同名称", "类型", "往来单位", "签订日期", "开始日期", "结束日期", "金额", "状态", "业务员", "备注"},
			rows)
		return data, "合同台账.xlsx", err

	case exportStock:
		rows, err := s.exportStockRows(pageQ, q, actor)
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 0 {
			return nil, "", errNoExportData()
		}
		data, err := buildExport("出入库单据", []string{"单号", "方向", "来源", "单据日期", "明细行数", "总数量", "业务员", "制单人", "备注"}, rows)
		return data, "出入库单据.xlsx", err

	case exportFinance:
		label := "应收明细"
		var list []FinanceRow
		var err error
		if q.Order.BizType == model.InvBizPurchase {
			label = "应付明细"
			list, _, _, err = s.Payables(pageQ, q.Order.Settle, q.Order.Source, actor)
		} else {
			list, _, _, err = s.Receivables(pageQ, q.Order.Settle, q.Order.Source, actor)
		}
		if err != nil {
			return nil, "", err
		}
		rows := make([][]string, 0, len(list))
		for _, it := range list {
			overdue := "否"
			if it.Overdue {
				overdue = "是"
			}
			// 单据与合同期次混在一张表里导出，必须让「这行是什么」一眼可见：
			// 单据行填单号，合同行填合同号 + 第几期。
			source := "单据"
			sourceNo := it.OrderNo
			if it.Source == model.InvSettleSourcePlan {
				source = "合同"
				sourceNo = it.ContractNo
			}
			period := ""
			if it.PlanSeq > 0 {
				period = strconv.Itoa(it.PlanSeq) + "期 " + it.PlanName
			}
			rows = append(rows, []string{
				source, sourceNo, period, it.PartyName,
				fmtDay(it.OrderDate), fmtDayPtr(it.DueDate),
				money(it.TotalAmount), money(it.SettledAmount), money(it.Outstanding),
				settleText(it.SettleStatus), overdue,
			})
		}
		if len(rows) == 0 {
			return nil, "", errNoExportData()
		}
		data, err := buildExport(label, []string{"来源", "单号/合同号", "期次", "往来单位", "单据日期", "到期日", "单据金额", "已结算", "未结余额", "结算状态", "逾期"}, rows)
		return data, label + ".xlsx", err

	case exportInventory:
		rows, err := s.exportInventoryRows(pageQ)
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 0 {
			return nil, "", errNoExportData()
		}
		data, err := buildExport("库存查询", []string{"商品编码", "商品", "规格", "基本单位", "分类", "库存", "预警下限", "库存成本", "状态"}, rows)
		return data, "库存查询.xlsx", err

	default:
		return nil, "", errcode.Newf(errcode.ErrBadRequest,
			"查询参数 entity 非法：%s（仅支持 product / customer / supplier / order / stock / inventory）", q.Entity)
	}
}

func errNoExportData() error {
	return errcode.New(errcode.ErrBadRequest, "当前条件下没有可导出的数据")
}

// buildExport 把「表头 + 二维字符串」拼成一张带样式的表。
//
// 各列表的行类型不同（ProductRow / OrderRow / …），逐个写泛型实例化会把
// 导出文件撑成几百行样板；这里先映射成 []string，列宽交给 sheet 自动估算。
func buildExport(sheetName string, headers []string, rows [][]string) ([]byte, error) {
	tbl := sheet.NewTable[[]string](sheetName)
	for i, h := range headers {
		idx := i
		tbl.Col(h, 0, func(v []string) string {
			if idx < len(v) {
				return v[idx]
			}
			return ""
		})
	}
	return tbl.Rows(rows).Build()
}

func (s *Service) exportProductRows(q page.Query, f ExportQuery) ([][]string, error) {
	list, _, err := s.ProductList(q, f.Status, f.CategoryID)
	if err != nil {
		return nil, err
	}
	keep := idSet(f.IDs)
	out := make([][]string, 0, len(list))
	for _, it := range list {
		if !keep(it.ID) {
			continue
		}
		out = append(out, []string{
			it.Code, it.Name, it.Barcode, it.CategoryName, specJoin(it.Specs),
			strconv.Itoa(it.SpecCount), strconv.Itoa(it.UnitCount),
			fmtQty(it.TotalStock), statusText(it.Status), it.Remark,
		})
	}
	return out, nil
}

func (s *Service) exportPartyRows(bizType string, q page.Query, f ExportQuery, actor *authz.Identity) ([][]string, error) {
	list, _, err := s.PartyList(bizType, q, f.Status, actor)
	if err != nil {
		return nil, err
	}
	keep := idSet(f.IDs)
	out := make([][]string, 0, len(list))
	for _, it := range list {
		if !keep(it.ID) {
			continue
		}
		out = append(out, []string{
			it.Code, it.Name, it.Contact, it.Phone, it.Address, it.TaxNo,
			money(it.Balance), statusText(it.Status), it.Remark,
		})
	}
	return out, nil
}

// exportOrderList 取筛选后的单据行（同时供列表页与明细页复用）。
//
// 返回结构体而不是直接返回单元格：明细工作表要靠这批 id 去捞 order_items，
// 先转成字符串数组就得再从里面抠 id 出来，多一道没意义的往返。
func (s *Service) exportOrderList(q page.Query, f ExportQuery, actor *authz.Identity) ([]OrderRow, error) {
	list, _, err := s.OrderList(f.Order, q, actor)
	if err != nil {
		return nil, err
	}
	keep := idSet(f.IDs)
	out := make([]OrderRow, 0, len(list))
	for _, it := range list {
		if keep(it.ID) {
			out = append(out, it)
		}
	}
	return out, nil
}

// orderRowCells 单据列表工作表的行。
func orderRowCells(list []OrderRow) [][]string {
	out := make([][]string, 0, len(list))
	for _, it := range list {
		out = append(out, []string{
			it.OrderNo, it.PartyName, fmtDay(it.OrderDate), fmtDayPtr(it.DueDate),
			orderStatusText(it.Status), strconv.FormatInt(it.ItemCount, 10),
			money(it.TotalAmount), money(it.SettledAmount), settleText(it.SettleStatus),
			it.Salesman, it.Operator, it.Remark,
		})
	}
	return out
}

// exportOrderDetailRows 明细工作表：一行一条单据明细。
//
// 一次 IN 查询把所有明细捞出来再按 order_id 归组，而不是对每张单各查一次 ——
// 导出上限一万张单，逐单查就是一万次往返。
func (s *Service) exportOrderDetailRows(list []OrderRow) ([][]string, error) {
	if len(list) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(list))
	byID := make(map[int64]OrderRow, len(list))
	for _, it := range list {
		ids = append(ids, it.ID)
		byID[it.ID] = it
	}

	var items []model.InvOrderItem
	if err := s.db.Where("order_id IN ?", ids).Order("order_id ASC, id ASC").Find(&items).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 明细快照里没有商品编码，从档案补齐；商品已删就留空，不因档案变动导不出文件
	productIDs := make([]int64, 0, len(items))
	for _, it := range items {
		productIDs = append(productIDs, it.ProductID)
	}
	products, err := s.productByIDs(productIDs)
	if err != nil {
		return nil, err
	}

	out := make([][]string, 0, len(items))
	for _, it := range items {
		head, ok := byID[it.OrderID]
		if !ok {
			continue
		}
		code := ""
		if p, hit := products[it.ProductID]; hit {
			code = p.Code
		}
		out = append(out, []string{
			head.OrderNo, head.PartyName, head.Salesman, code, it.ProductName,
			it.Spec, it.Unit, fmtQty(it.Qty), strconv.FormatFloat(it.Rate, 'f', -1, 64),
			fmtQty(it.QtyBase), money(it.Price), money(it.Amount),
		})
	}
	return out, nil
}

func (s *Service) exportStockRows(q page.Query, f ExportQuery, actor *authz.Identity) ([][]string, error) {
	list, _, err := s.StockOrderList(q, f.Direction, actor)
	if err != nil {
		return nil, err
	}
	keep := idSet(f.IDs)
	out := make([][]string, 0, len(list))
	for _, it := range list {
		if !keep(it.ID) {
			continue
		}
		out = append(out, []string{
			it.OrderNo, directionText(it.Direction), sourceText(it.SourceType),
			fmtDay(it.OrderDate), strconv.FormatInt(it.ItemCount, 10),
			fmtQty(it.TotalQty), it.Salesman, it.Operator, it.Remark,
		})
	}
	return out, nil
}

func (s *Service) exportInventoryRows(q page.Query) ([][]string, error) {
	list, _, err := s.Inventory(q)
	if err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(list))
	for _, it := range list {
		warn := "正常"
		if it.Warn {
			warn = "库存预警"
		}
		out = append(out, []string{
			it.ProductCode, it.ProductName, it.Spec, it.Unit, it.CategoryName,
			fmtQty(it.Stock), fmtQty(it.WarnStock), money(it.Stock * it.PurchasePrice), warn,
		})
	}
	return out, nil
}

// ---------- 展示文案 ----------
// 导出文件是给人看的，不能把 1 / 2 / 3 原样落进单元格 —— 收到表格的人不知道
// 「2」是「已审核」还是「已取消」。

func statusText(s int16) string {
	if s == model.StatusEnabled {
		return "启用"
	}
	return "停用"
}

func orderStatusText(s int16) string {
	switch s {
	case model.InvStatusPending:
		return "待处理"
	case model.InvStatusDone:
		return "已审核"
	case model.InvStatusCanceled:
		return "已取消"
	default:
		return strconv.Itoa(int(s))
	}
}

func settleText(s string) string {
	switch s {
	case model.InvSettleUnpaid:
		return "未结清"
	case model.InvSettlePartial:
		return "部分结算"
	case model.InvSettlePaid:
		return "已结清"
	default:
		return s
	}
}

func directionText(d string) string {
	if d == model.InvStockOut {
		return "出库"
	}
	return "入库"
}

func sourceText(s string) string {
	switch s {
	case model.InvSourcePurchase:
		return "采购"
	case model.InvSourceSales:
		return "销售"
	default:
		return "手工"
	}
}

// specJoin 把商品的规格摘要拼成一行（「φ20 · φ25」），与页面上的「规格」列一致。
func specJoin(specs []ProductSpecVO) string {
	if len(specs) == 0 {
		return "-"
	}
	out := ""
	for i, sp := range specs {
		if i > 0 {
			out += " · "
		}
		out += sp.Spec
	}
	return out
}

// fmtDay 单据日期固定 yyyy-MM-dd：导出的表格常见于二次汇总，
// 不能随着 Excel 的区域设置变成 2026/10/17 或 10-17-2026。
func fmtDay(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func fmtDayPtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return fmtDay(*t)
}

// idSet 返回一个「是否保留」判定。ids 为空表示全都要。
func idSet(ids []int64) func(int64) bool {
	if len(ids) == 0 {
		return func(int64) bool { return true }
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return func(id int64) bool {
		_, ok := set[id]
		return ok
	}
}
