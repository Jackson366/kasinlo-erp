package inv

import (
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"nova-erp/internal/authz"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/textutil"
)

// reportSheet 是导出工作簿里的一张表。
type reportSheet struct {
	Name    string
	Headers []string
	Widths  []float64
	Rows    [][]string
}

// ReportExport 生成报表 xlsx 字节流与建议文件名。
// kind 对应报表中心的四个面板：overview（默认）/ ranking / purchase / inventory。
func (s *Service) ReportExport(f ReportFilter, kind, dimension, biz string, actor *authz.Identity) ([]byte, string, error) {
	switch kind {
	case "ranking":
		return s.exportRanking(f, dimension, biz, actor)
	case "purchase":
		return s.exportPurchase(f, actor)
	case "inventory":
		return s.exportInventory()
	default:
		return s.exportOverview(f, actor)
	}
}

// exportOverview 经营总览：汇总 + 客户排行 + 品类占比。
func (s *Service) exportOverview(f ReportFilter, actor *authz.Identity) ([]byte, string, error) {
	sm, err := s.ReportSummary(f, actor)
	if err != nil {
		return nil, "", err
	}
	rank, err := s.ReportRanking(f, "customer", model.InvBizSales, reportExportRows, actor)
	if err != nil {
		return nil, "", err
	}
	cats, err := s.ReportCategory(f, actor)
	if err != nil {
		return nil, "", err
	}

	summaryRows := [][]string{
		{"营业收入", money(sm.SalesAmount)},
		{"销售成本", money(sm.CostAmount)},
		{"毛利", money(sm.GrossProfit)},
		{"毛利率", pct(sm.GrossRate)},
		{"销售单数", strconv.FormatInt(sm.SalesOrders, 10)},
		{"采购单数", strconv.FormatInt(sm.PurchaseOrders, 10)},
		{"采购额", money(sm.Purchases)},
	}
	rankRows := make([][]string, 0, len(rank))
	for _, r := range rank {
		rankRows = append(rankRows, []string{
			strconv.Itoa(r.Rank), r.Name, money(r.Amount),
			strconv.FormatInt(r.OrderCount, 10), pct(r.Ratio),
		})
	}
	catRows := make([][]string, 0, len(cats))
	for _, c := range cats {
		catRows = append(catRows, []string{c.Category, money(c.Amount), pct(c.Ratio)})
	}

	data, err := buildReportWorkbook([]reportSheet{
		{Name: "经营汇总（" + sm.Range + "）", Headers: []string{"指标", "数值"}, Widths: []float64{20, 22}, Rows: summaryRows},
		{Name: "客户销售排行", Headers: []string{"排名", "客户", "销售额", "订单数", "占比"}, Widths: []float64{8, 28, 16, 12, 10}, Rows: rankRows},
		{Name: "品类销售占比", Headers: []string{"品类", "销售额", "占比"}, Widths: []float64{22, 16, 10}, Rows: catRows},
	})
	if err != nil {
		return nil, "", err
	}
	return data, "经营总览_" + textutil.Timestamp() + ".xlsx", nil
}

// exportRanking 销售排行，列随维度（客户 / 产品）切换。
func (s *Service) exportRanking(f ReportFilter, dimension, biz string, actor *authz.Identity) ([]byte, string, error) {
	items, err := s.ReportRanking(f, dimension, biz, reportExportRows, actor)
	if err != nil {
		return nil, "", err
	}
	headers := []string{"排名", "客户", "销售额", "订单数", "占比"}
	widths := []float64{8, 28, 16, 12, 10}
	if dimension == "product" {
		headers = []string{"排名", "商品", "销售额", "销量", "订单数", "占比"}
		widths = []float64{8, 28, 16, 12, 12, 10}
	}
	rows := make([][]string, 0, len(items))
	for _, r := range items {
		base := []string{strconv.Itoa(r.Rank), r.Name, money(r.Amount)}
		if dimension == "product" {
			base = append(base, qty(r.Quantity))
		}
		base = append(base, strconv.FormatInt(r.OrderCount, 10), pct(r.Ratio))
		rows = append(rows, base)
	}
	data, err := buildReportWorkbook([]reportSheet{
		{Name: "销售排行", Headers: headers, Widths: widths, Rows: rows},
	})
	if err != nil {
		return nil, "", err
	}
	return data, "销售排行_" + textutil.Timestamp() + ".xlsx", nil
}

// exportPurchase 采购分析：采购汇总 + 供应商排行。
func (s *Service) exportPurchase(f ReportFilter, actor *authz.Identity) ([]byte, string, error) {
	sm, err := s.ReportSummary(f, actor)
	if err != nil {
		return nil, "", err
	}
	rank, err := s.ReportRanking(f, "customer", model.InvBizPurchase, reportExportRows, actor)
	if err != nil {
		return nil, "", err
	}
	summaryRows := [][]string{
		{"采购总额", money(sm.Purchases)},
		{"采购单数", strconv.FormatInt(sm.PurchaseOrders, 10)},
	}
	rankRows := make([][]string, 0, len(rank))
	for _, r := range rank {
		rankRows = append(rankRows, []string{
			strconv.Itoa(r.Rank), r.Name, money(r.Amount),
			strconv.FormatInt(r.OrderCount, 10), pct(r.Ratio),
		})
	}
	data, err := buildReportWorkbook([]reportSheet{
		{Name: "采购汇总（" + sm.Range + "）", Headers: []string{"指标", "数值"}, Widths: []float64{20, 22}, Rows: summaryRows},
		{Name: "供应商采购排行", Headers: []string{"排名", "供应商", "采购额", "订单数", "占比"}, Widths: []float64{8, 28, 16, 12, 10}, Rows: rankRows},
	})
	if err != nil {
		return nil, "", err
	}
	return data, "采购分析_" + textutil.Timestamp() + ".xlsx", nil
}

// exportInventory 库存分析：品类库存金额分布 + 低库存预警清单。
func (s *Service) exportInventory() ([]byte, string, error) {
	inv, err := s.ReportInventory()
	if err != nil {
		return nil, "", err
	}
	catRows := make([][]string, 0, len(inv.CategoryValue))
	for _, c := range inv.CategoryValue {
		catRows = append(catRows, []string{c.Category, money(c.Value), pct(c.Ratio)})
	}
	warnRows := make([][]string, 0, len(inv.WarnList))
	for _, w := range inv.WarnList {
		warnRows = append(warnRows, []string{
			w.ProductName, w.ProductCode, w.Spec, w.Unit, qty(w.Stock), qty(w.WarnStock),
		})
	}
	data, err := buildReportWorkbook([]reportSheet{
		{Name: "库存概况", Headers: []string{"指标", "数值"}, Widths: []float64{20, 22}, Rows: [][]string{
			{"库存总金额", money(inv.TotalValue)},
			{"库存规格数", strconv.FormatInt(inv.SpecCount, 10)},
			{"低库存预警数", strconv.FormatInt(inv.WarnCount, 10)},
		}},
		{Name: "品类库存金额分布", Headers: []string{"品类", "库存金额", "占比"}, Widths: []float64{22, 16, 10}, Rows: catRows},
		{Name: "低库存预警清单", Headers: []string{"商品", "编码", "规格", "单位", "当前库存", "预警下限"}, Widths: []float64{24, 16, 16, 10, 12, 12}, Rows: warnRows},
	})
	if err != nil {
		return nil, "", err
	}
	return data, "库存分析_" + textutil.Timestamp() + ".xlsx", nil
}

// reportExportRows 导出时排行取满上限（列表页只显示前 N，导出应是完整一份）。
const reportExportRows = 50

func money(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func qty(v float64) string { return strconv.FormatFloat(round2(v), 'f', -1, 64) }

func pct(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) + "%" }

// buildReportWorkbook 把多张表渲染成一个 xlsx 工作簿。
//
// 不复用 pkg/sheet：那个包一次只产出一张表（它的 Notes 另开「填写说明」表），
// 而一个报表面板有 2~3 张表（汇总 / 排行 / 品类），拆成多个文件下载很难用；
// 塞进一张表又会让「金额」和「张数」挤在同一列、没法筛选。这里补最小的工作簿
// 拼装能力，样式与 pkg/sheet 保持一致（加粗表头 / 文本格式 / 冻结首行）。
func buildReportWorkbook(sheets []reportSheet) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F2937", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EEF2F7"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    reportBorder(),
	})
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	// 文本格式：编号、长数字不会被转成科学计数法
	bodyStyle, err := f.NewStyle(&excelize.Style{
		NumFmt:    49,
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    reportBorder(),
	})
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}

	for i, sh := range sheets {
		name := reportSheetName(sh.Name)
		if i == 0 {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
		} else if _, err := f.NewSheet(name); err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}

		for c, h := range sh.Headers {
			cell, err := excelize.CoordinatesToCellName(c+1, 1)
			if err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
			if err := f.SetCellValue(name, cell, h); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
		}
		for r, row := range sh.Rows {
			for c, v := range row {
				cell, err := excelize.CoordinatesToCellName(c+1, r+2)
				if err != nil {
					return nil, errcode.Wrap(errcode.ErrInternal, err)
				}
				if err := f.SetCellValue(name, cell, v); err != nil {
					return nil, errcode.Wrap(errcode.ErrInternal, err)
				}
			}
		}

		if len(sh.Headers) > 0 {
			lastCol, err := excelize.ColumnNumberToName(len(sh.Headers))
			if err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
			if err := f.SetCellStyle(name, "A1", lastCol+"1", headerStyle); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
			if len(sh.Rows) > 0 {
				lastCell, err := excelize.CoordinatesToCellName(len(sh.Headers), len(sh.Rows)+1)
				if err != nil {
					return nil, errcode.Wrap(errcode.ErrInternal, err)
				}
				if err := f.SetCellStyle(name, "A2", lastCell, bodyStyle); err != nil {
					return nil, errcode.Wrap(errcode.ErrInternal, err)
				}
			}
			// 冻结首行，长表滚动时表头始终可见
			if err := f.SetPanes(name, &excelize.Panes{
				Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
			}); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
		}
		for c, w := range sh.Widths {
			col, err := excelize.ColumnNumberToName(c + 1)
			if err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
			if err := f.SetColWidth(name, col, col, w); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	return buf.Bytes(), nil
}

// reportSheetName 清洗工作表名：去掉 xlsx 不允许的字符并截断到 31 字符。
func reportSheetName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '-'
		}
		return r
	}, name)
	runes := []rune(cleaned)
	if len(runes) > 31 {
		cleaned = string(runes[:31])
	}
	if cleaned == "" {
		return "Sheet"
	}
	return cleaned
}

// reportBorder 四边细边框（与 pkg/sheet 的样式一致）。
func reportBorder() []excelize.Border {
	const color = "D1D5DB"
	return []excelize.Border{
		{Type: "left", Color: color, Style: 1},
		{Type: "right", Color: color, Style: 1},
		{Type: "top", Color: color, Style: 1},
		{Type: "bottom", Color: color, Style: 1},
	}
}
