// Package sheet 提供基于 excelize 的通用表格导出与导入能力。
//
// 设计目标：让业务模块只描述「有哪些列、每列怎么取值」，不关心 xlsx 细节。
//
//	tbl := sheet.NewTable[VO]("用户数据").
//		Col("用户名", 18, func(v VO) string { return v.Username }).
//		Rows(items)
//	data, err := tbl.Build()
//
// 导出侧统一处理了表头样式、冻结首行、自动筛选、文本格式（避免手机号被转成科学计数法）
// 以及 Content-Disposition 的中文文件名编码；导入侧统一处理解析与空行裁剪。
package sheet

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/textutil"
)

const (
	// MaxExportRows 是单次导出的行数上限，避免大表把内存打满。
	// 各业务模块导出时统一引用该值，保证各处行为一致。
	MaxExportRows = 10000

	defaultColWidth = 14
	maxColWidth     = 60
	// autoWidthSample 是列宽估算时采样的最大行数
	autoWidthSample = 200
	// noteSheetName 是模板文件的「填写说明」工作表名
	noteSheetName = "填写说明"
	// textNumFmt 是 Excel 内置的文本格式编号（"@"），用于防止长数字被科学计数法处理
	textNumFmt = 49
)

// Column 描述一列：表头文案、列宽（0 表示按内容自动估算）与取值函数。
type Column[T any] struct {
	Header string
	Width  float64
	Value  func(T) string
}

// Table 是一张待导出的表。使用泛型保证 Value 的函数签名与行类型一致。
type Table[T any] struct {
	sheetName string
	columns   []Column[T]
	rows      []T
	notes     []string
}

// NewTable 创建一张表，sheetName 为工作簿中的工作表名。
func NewTable[T any](sheetName string) *Table[T] {
	return &Table[T]{sheetName: sheetName}
}

// Col 追加一列，返回自身以支持链式调用。
func (t *Table[T]) Col(header string, width float64, value func(T) string) *Table[T] {
	t.columns = append(t.columns, Column[T]{Header: header, Width: width, Value: value})
	return t
}

// Rows 设置数据行。
func (t *Table[T]) Rows(rows []T) *Table[T] {
	t.rows = rows
	return t
}

// Notes 追加填写说明。非空时会额外生成一个「填写说明」工作表，
// 避免说明文字混进数据区影响解析。
func (t *Table[T]) Notes(lines ...string) *Table[T] {
	t.notes = append(t.notes, lines...)
	return t
}

// Build 生成 xlsx 字节流。
func (t *Table[T]) Build() ([]byte, error) {
	if len(t.columns) == 0 {
		return nil, errcode.New(errcode.ErrInternal, "导出失败：未定义任何列")
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := sanitizeSheetName(t.sheetName)
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F2937", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EEF2F7"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder(),
	})
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	// 数据单元格统一使用文本格式：手机号、编号等长数字不会被转成科学计数法
	bodyStyle, err := f.NewStyle(&excelize.Style{
		NumFmt:    textNumFmt,
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    thinBorder(),
	})
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}

	// ---- 表头 ----
	for i, col := range t.columns {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
		if err := f.SetCellStr(sheet, cell, col.Header); err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
	}
	lastColName, err := excelize.ColumnNumberToName(len(t.columns))
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	headerRange := "A1:" + lastColName + "1"
	if err := f.SetCellStyle(sheet, "A1", lastColName+"1", headerStyle); err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	if err := f.SetRowHeight(sheet, 1, 24); err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}

	// ---- 数据 ----
	for r, row := range t.rows {
		for i, col := range t.columns {
			cell, err := excelize.CoordinatesToCellName(i+1, r+2)
			if err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
			if err := f.SetCellStr(sheet, cell, col.Value(row)); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
		}
	}
	if len(t.rows) > 0 {
		lastCell := lastColName + strconv.Itoa(len(t.rows)+1)
		if err := f.SetCellStyle(sheet, "A2", lastCell, bodyStyle); err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
	}

	// ---- 列宽 ----
	for i, col := range t.columns {
		letter, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
		width := col.Width
		if width <= 0 {
			width = t.autoWidth(i)
		}
		if err := f.SetColWidth(sheet, letter, letter, width); err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
	}

	// ---- 冻结首行 + 自动筛选 ----
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
		Selection: []excelize.Selection{
			{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"},
		},
	}); err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	if err := f.AutoFilter(sheet, headerRange, nil); err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}

	// ---- 填写说明（仅模板需要）----
	if len(t.notes) > 0 {
		if _, err := f.NewSheet(noteSheetName); err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
		for i, line := range t.notes {
			cell, err := excelize.CoordinatesToCellName(1, i+1)
			if err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
			if err := f.SetCellStr(noteSheetName, cell, line); err != nil {
				return nil, errcode.Wrap(errcode.ErrInternal, err)
			}
		}
		if err := f.SetColWidth(noteSheetName, "A", "A", 72); err != nil {
			return nil, errcode.Wrap(errcode.ErrInternal, err)
		}
	}

	// 显式指定默认打开的工作表为数据表
	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	f.SetActiveSheet(idx)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, errcode.Wrap(errcode.ErrInternal, err)
	}
	return buf.Bytes(), nil
}

// Download 把生成的 xlsx 写入 HTTP 响应。
//
// 注意：这里不能走 response.OK —— 导出返回的是二进制流，不是统一 JSON 响应体。
func Download(c *gin.Context, filename string, data []byte) {
	if filename == "" {
		filename = "export.xlsx"
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", contentDisposition(filename))
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// Parse 读取上传的 xlsx 第一个工作表，返回全部单元格文本。
// 首行视为表头，尾部空行会被裁掉。
func Parse(data []byte) ([][]string, error) {
	if len(data) == 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "上传的文件为空")
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		// 保留底层原因：多数「解析失败」其实是文件格式不对或内容损坏，
		// 丢掉了 cause 就只能靠猜。
		return nil, errcode.Wrapf(errcode.ErrBadRequest, err,
			"无法解析该文件，请使用模板生成的 .xlsx 文件")
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "文件中没有任何工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, errcode.Wrapf(errcode.ErrBadRequest, err, "读取工作表内容失败")
	}
	// 裁剪尾部空行：Excel 常有大量空白行
	for len(rows) > 0 && isBlankRow(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	return rows, nil
}

// IsBlankRow 判断一行是否全为空，供调用方跳过表格中间的空白行。
func IsBlankRow(row []string) bool { return isBlankRow(row) }

func isBlankRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// Cell 安全取单元格，越界返回空串。
func Cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// contentDisposition 生成兼容中文文件名的响应头。
// 老客户端读 filename（ASCII 回退名），现代浏览器读 filename*（RFC 5987 UTF-8）。
func contentDisposition(filename string) string {
	fallback := asciiFallback(filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fallback, rfc5987Escape(filename))
}

// asciiFallback 把非 ASCII 字符替换为下划线，保证 filename 参数是合法的 ASCII 值。
func asciiFallback(filename string) string {
	var b strings.Builder
	for _, r := range filename {
		if r > unicode.MaxASCII || r == '"' || r == '\\' {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if strings.Trim(out, "_") == "" {
		return "export.xlsx"
	}
	return out
}

// rfc5987Escape 按 RFC 5987 对文件名做百分号编码。
func rfc5987Escape(s string) string {
	// url.QueryEscape 会把空格编成 '+'，而 RFC 5987 要求 %20
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// sanitizeSheetName 收敛工作表名：excelize 对长度与非法字符有限制。
func sanitizeSheetName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Sheet1"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", "?", "_", "*", "_", "[", "_", "]", "_", ":", "_")
	name = replacer.Replace(name)
	// 31 是 Excel 工作表名的硬上限，按字符截断（中文一个字算一个字符）
	return textutil.Truncate(name, 31)
}

// autoWidth 按表头与数据内容估算列宽。
// 只采样前 autoWidthSample 行：大表全量扫描会让导出明显变慢，而列宽本身是近似值。
func (t *Table[T]) autoWidth(idx int) float64 {
	col := t.columns[idx]
	widest := displayWidth(col.Header)
	limit := len(t.rows)
	if limit > autoWidthSample {
		limit = autoWidthSample
	}
	for i := 0; i < limit; i++ {
		if w := displayWidth(col.Value(t.rows[i])); w > widest {
			widest = w
		}
	}

	width := float64(widest + 4)
	if width > maxColWidth {
		width = maxColWidth
	}
	if width < defaultColWidth {
		width = defaultColWidth
	}
	return width
}

// displayWidth 返回字符串的显示宽度，中日韩等全角字符按 2 列计算。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > unicode.MaxASCII {
			w += 2
			continue
		}
		w++
	}
	return w
}

func thinBorder() []excelize.Border {
	const color = "D8DEE9"
	return []excelize.Border{
		{Type: "left", Color: color, Style: 1},
		{Type: "right", Color: color, Style: 1},
		{Type: "top", Color: color, Style: 1},
		{Type: "bottom", Color: color, Style: 1},
	}
}
