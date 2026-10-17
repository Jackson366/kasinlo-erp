package menu

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/response"
	"nova-erp/internal/pkg/sheet"
	"nova-erp/internal/pkg/textutil"
)

// Export 导出菜单树（按筛选条件，扁平化后带上层级）。
//
// 菜单是配置数据，导出的用途是「换个环境照着配一遍」或「留个配置快照」，
// 所以层级必须体现在纸面上 —— 只给一列同级名字，收到表格的人不知道
// 「用户管理」挂在「系统管理」下面。
//
// @Summary      导出菜单
// @Description  返回 xlsx 二进制流，筛选参数同列表接口。
// @Tags         菜单管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按标题 / 路由名 / 权限标识模糊搜索"
// @Param        status   query     int     false  "状态：1 启用，0 禁用"
// @Success      200      {file}    binary  "xlsx 文件流"
// @Failure      400      {object}  response.Body  "无可导出数据"
// @Router       /system/menus/export [get]
func (h *Handler) Export(c *gin.Context) {
	c.Set("opAction", "导出菜单")

	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	tree, err := h.svc.List(c.Query("keyword"), status)
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := flattenMenu(tree, 0, c.Query("type"))
	if len(rows) == 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "当前条件下没有可导出的数据"))
		return
	}

	tbl := sheet.NewTable[[7]string]("菜单配置").
		Col("名称", 0, func(v [7]string) string { return v[0] }).
		Col("层级", 0, func(v [7]string) string { return v[1] }).
		Col("类型", 0, func(v [7]string) string { return v[2] }).
		Col("路由路径", 0, func(v [7]string) string { return v[3] }).
		Col("权限标识", 0, func(v [7]string) string { return v[4] }).
		Col("状态", 0, func(v [7]string) string { return v[5] }).
		Col("排序", 0, func(v [7]string) string { return v[6] }).
		Rows(rows)
	data, err := tbl.Build()
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.ErrInternal, err))
		return
	}
	sheet.Download(c, "菜单配置_"+textutil.Timestamp()+".xlsx", data)
}

// flattenMenu 深度优先摊平，名称前用全角空格缩进 —— Excel 里看层级最直观的方式
// 仍是缩进，加一列纯数字的 level 反而要来回对照。
func flattenMenu(nodes []*Node, depth int, onlyType string) [][7]string {
	out := make([][7]string, 0, len(nodes))
	for _, n := range nodes {
		// 类型筛选在摊平后做：父目录即使被过滤掉，子节点仍按自己的类型决定去留，
		// 与页面上的「只看按钮」视图口径一致
		if onlyType != "" && n.Type != onlyType {
			if len(n.Children) > 0 {
				out = append(out, flattenMenu(n.Children, depth+1, onlyType)...)
			}
			continue
		}
		name := strings.Repeat("　", depth) + n.Title
		if name == "" {
			name = strings.Repeat("　", depth) + n.Name
		}
		status := "启用"
		if n.Status != 1 {
			status = "停用"
		}
		out = append(out, [7]string{
			name,
			strconv.Itoa(depth + 1),
			menuTypeText(n.Type),
			n.Path,
			n.Permission,
			status,
			strconv.Itoa(n.Sort),
		})
		if len(n.Children) > 0 {
			out = append(out, flattenMenu(n.Children, depth+1, onlyType)...)
		}
	}
	return out
}

func menuTypeText(t string) string {
	switch t {
	case "dir", "M":
		return "目录"
	case "menu", "C":
		return "菜单"
	case "button", "F":
		return "按钮"
	default:
		return t
	}
}
