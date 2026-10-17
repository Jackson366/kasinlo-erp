package dept

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/middleware"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/response"
	"nova-erp/internal/pkg/sheet"
	"nova-erp/internal/pkg/textutil"
)

// Export 导出部门树（按筛选条件，扁平化后带上层级与在职人数）。
//
// 与列表接口同一套筛选与数据范围，保证「页面上看不到的部门不会出现在导出文件里」。
//
// @Summary      导出部门
// @Description  返回 xlsx 二进制流，筛选参数同列表接口。
// @Tags         部门管理
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        keyword  query     string  false  "按部门名称模糊搜索"
// @Param        status   query     int     false  "状态：1 启用，0 禁用"
// @Success      200      {file}    binary  "xlsx 文件流"
// @Failure      400      {object}  response.Body  "无可导出数据"
// @Router       /system/depts/export [get]
func (h *Handler) Export(c *gin.Context) {
	c.Set("opAction", "导出部门")

	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	tree, err := h.svc.List(c.Query("keyword"), status, middleware.CurrentIdentity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	rows := flattenDept(tree, 0)
	if len(rows) == 0 {
		response.Fail(c, errcode.New(errcode.ErrBadRequest, "当前条件下没有可导出的数据"))
		return
	}

	tbl := sheet.NewTable[[7]string]("部门").
		Col("部门名称", 0, func(v [7]string) string { return v[0] }).
		Col("层级", 0, func(v [7]string) string { return v[1] }).
		Col("负责人", 0, func(v [7]string) string { return v[2] }).
		Col("电话", 0, func(v [7]string) string { return v[3] }).
		Col("邮箱", 0, func(v [7]string) string { return v[4] }).
		Col("人数", 0, func(v [7]string) string { return v[5] }).
		Col("状态", 0, func(v [7]string) string { return v[6] }).
		Rows(rows)
	data, err := tbl.Build()
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.ErrInternal, err))
		return
	}
	sheet.Download(c, "部门_"+textutil.Timestamp()+".xlsx", data)
}

// flattenDept 深度优先摊平。名称按层级缩进 —— 部门树离开缩进就只剩一堆平级名字。
func flattenDept(nodes []*Node, depth int) [][7]string {
	out := make([][7]string, 0, len(nodes))
	for _, n := range nodes {
		status := "启用"
		if n.Status != 1 {
			status = "停用"
		}
		out = append(out, [7]string{
			strings.Repeat("　", depth) + n.Name,
			strconv.Itoa(depth + 1),
			n.Leader,
			n.Phone,
			n.Email,
			strconv.FormatInt(n.UserCount, 10),
			status,
		})
		if len(n.Children) > 0 {
			out = append(out, flattenDept(n.Children, depth+1)...)
		}
	}
	return out
}
