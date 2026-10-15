package inv

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/model"
	"nova-erp/internal/pkg/bind"
	"nova-erp/internal/pkg/page"
	"nova-erp/internal/pkg/response"
)

// CustomerList 客户列表
// @Summary      客户列表
// @Tags         进销存-客户
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int     false  "页码"
// @Param        pageSize  query  int     false  "每页条数"
// @Param        keyword   query  string  false  "编号 / 名称 / 联系人"
// @Param        status    query  int     false  "状态 1 启用 0 禁用"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.PartyRow}}
// @Router       /inv/customers [get]
func (h *Handler) CustomerList(c *gin.Context) {
	h.partyList(c, model.InvBizSales)
}

// SupplierList 供应商列表
// @Summary      供应商列表
// @Tags         进销存-供应商
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.PartyRow}}
// @Router       /inv/suppliers [get]
func (h *Handler) SupplierList(c *gin.Context) {
	h.partyList(c, model.InvBizPurchase)
}

func (h *Handler) partyList(c *gin.Context, bizType string) {
	q := page.FromContext(c)
	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, total, err := h.svc.PartyList(bizType, q, status)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// CustomerCreate 新增客户
// @Summary      新增客户
// @Tags         进销存-客户
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  PartyReq  true  "客户信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/customers [post]
func (h *Handler) CustomerCreate(c *gin.Context) { h.partyCreate(c, model.InvBizSales, "新增客户") }

// SupplierCreate 新增供应商
// @Summary      新增供应商
// @Tags         进销存-供应商
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  PartyReq  true  "供应商信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/suppliers [post]
func (h *Handler) SupplierCreate(c *gin.Context) {
	h.partyCreate(c, model.InvBizPurchase, "新增供应商")
}

func (h *Handler) partyCreate(c *gin.Context, bizType, action string) {
	var req PartyReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", action)
	id, err := h.svc.PartyCreate(bizType, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// CustomerUpdate 修改客户
// @Summary      修改客户
// @Tags         进销存-客户
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int       true  "客户 ID"
// @Param        body  body  PartyReq  true  "客户信息"
// @Success      200   {object}  response.Body
// @Router       /inv/customers/{id} [put]
func (h *Handler) CustomerUpdate(c *gin.Context) { h.partyUpdate(c, model.InvBizSales, "修改客户") }

// SupplierUpdate 修改供应商
// @Summary      修改供应商
// @Tags         进销存-供应商
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int       true  "供应商 ID"
// @Param        body  body  PartyReq  true  "供应商信息"
// @Success      200   {object}  response.Body
// @Router       /inv/suppliers/{id} [put]
func (h *Handler) SupplierUpdate(c *gin.Context) {
	h.partyUpdate(c, model.InvBizPurchase, "修改供应商")
}

func (h *Handler) partyUpdate(c *gin.Context, bizType, action string) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req PartyReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", action)
	if err := h.svc.PartyUpdate(bizType, id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// CustomerDelete 删除客户
// @Summary      删除客户
// @Tags         进销存-客户
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "客户 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/customers/{id} [delete]
func (h *Handler) CustomerDelete(c *gin.Context) { h.partyDelete(c, model.InvBizSales, "删除客户") }

// SupplierDelete 删除供应商
// @Summary      删除供应商
// @Tags         进销存-供应商
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "供应商 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/suppliers/{id} [delete]
func (h *Handler) SupplierDelete(c *gin.Context) {
	h.partyDelete(c, model.InvBizPurchase, "删除供应商")
}

func (h *Handler) partyDelete(c *gin.Context, bizType, action string) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", action)
	if err := h.svc.PartyDelete(bizType, id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// ============================================================
// 商品分类
// ============================================================

// CategoryList 商品分类列表
// @Summary      商品分类列表
// @Tags         进销存-商品分类
// @Produce      json
// @Security     BearerAuth
// @Param        page      query  int     false  "页码"
// @Param        pageSize  query  int     false  "每页条数"
// @Param        keyword   query  string  false  "分类名称"
// @Param        status    query  int     false  "状态 1 启用 0 停用"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.CategoryRow}}
// @Router       /inv/categories [get]
func (h *Handler) CategoryList(c *gin.Context) {
	q := page.FromContext(c)
	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, total, err := h.svc.CategoryList(q, status)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// CategoryOptions 商品分类下拉项（树形，含 parentId）
// @Summary      商品分类下拉项（仅启用）
// @Tags         进销存-商品分类
// @Produce      json
// @Security     BearerAuth
// @Param        currentId  query  int  false  "当前编辑项的分类 ID（会一并返回，避免编辑停用分类时显示空白）"
// @Success      200  {object}  response.Body{data=[]inv.CategoryOption}
// @Router       /inv/categories/options [get]
func (h *Handler) CategoryOptions(c *gin.Context) {
	currentID, err := bind.IntQuery(c, "currentId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, err := h.svc.CategoryOptions(currentID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if list == nil {
		list = []CategoryOption{}
	}
	response.OK(c, list)
}

// CategoryCreate 新增商品分类
// @Summary      新增商品分类
// @Tags         进销存-商品分类
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  CategoryReq  true  "分类信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/categories [post]
func (h *Handler) CategoryCreate(c *gin.Context) {
	var req CategoryReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增商品分类")
	id, err := h.svc.CategoryCreate(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// CategoryUpdate 修改商品分类
// @Summary      修改商品分类
// @Tags         进销存-商品分类
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int          true  "分类 ID"
// @Param        body  body  CategoryReq  true  "分类信息"
// @Success      200   {object}  response.Body
// @Router       /inv/categories/{id} [put]
func (h *Handler) CategoryUpdate(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req CategoryReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改商品分类")
	if err := h.svc.CategoryUpdate(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// CategoryDelete 删除商品分类
// @Summary      删除商品分类
// @Tags         进销存-商品分类
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "分类 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/categories/{id} [delete]
func (h *Handler) CategoryDelete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除商品分类")
	if err := h.svc.CategoryDelete(id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}

// ============================================================
// 商品
// ============================================================

// ProductList 商品列表
// @Summary      商品列表（含规格与计量单位）
// @Tags         进销存-商品
// @Produce      json
// @Security     BearerAuth
// @Param        page        query  int     false  "页码"
// @Param        pageSize    query  int     false  "每页条数"
// @Param        keyword     query  string  false  "编码 / 名称 / 规格"
// @Param        status      query  int     false  "状态 1 启用 0 停用"
// @Param        categoryId  query  int     false  "商品分类 ID"
// @Success      200  {object}  response.Body{data=response.PageData{list=[]inv.ProductRow}}
// @Router       /inv/products [get]
func (h *Handler) ProductList(c *gin.Context) {
	q := page.FromContext(c)
	status, err := bind.StatusQuery(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	categoryID, err := bind.IntQuery(c, "categoryId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, total, err := h.svc.ProductList(q, status, categoryID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

// ProductPick 商品 → 规格 → 单位 级联选项
// @Summary      商品级联选项（供单据明细选择规格与单位）
// @Tags         进销存-商品
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query  string  false  "编码 / 名称 / 规格"
// @Success      200  {object}  response.Body{data=inv.ProductPickResult}
// @Router       /inv/products/pick [get]
func (h *Handler) ProductPick(c *gin.Context) {
	res, err := h.svc.ProductPick(c.Query("keyword"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// ProductDuplicates 同名商品提示
// @Summary      同名商品提示（录商品时防止录重）
// @Tags         进销存-商品
// @Produce      json
// @Security     BearerAuth
// @Param        name       query  string  true   "商品名称（支持部分匹配）"
// @Param        excludeId  query  int     false  "编辑时排除自己的 ID"
// @Success      200  {object}  response.Body{data=[]inv.ProductDupRow}
// @Router       /inv/products/duplicates [get]
func (h *Handler) ProductDuplicates(c *gin.Context) {
	excludeID, err := bind.IntQuery(c, "excludeId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, err := h.svc.ProductDuplicates(c.Query("name"), excludeID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if list == nil {
		list = []ProductDupRow{}
	}
	response.OK(c, list)
}

// ProductDetail 商品详情
// @Summary      商品详情（含规格与计量单位）
// @Tags         进销存-商品
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "商品 ID"
// @Success      200  {object}  response.Body{data=inv.ProductRow}
// @Router       /inv/products/{id} [get]
func (h *Handler) ProductDetail(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	row, err := h.svc.ProductDetail(id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, row)
}

// ProductCreate 新增商品
// @Summary      新增商品（含规格与计量单位）
// @Tags         进销存-商品
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  ProductReq  true  "商品信息"
// @Success      200  {object}  response.Body{data=response.IDData}
// @Router       /inv/products [post]
func (h *Handler) ProductCreate(c *gin.Context) {
	var req ProductReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "新增商品")
	id, err := h.svc.ProductCreate(req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.IDData{ID: id})
}

// ProductUpdate 修改商品
// @Summary      修改商品（含规格与计量单位）
// @Tags         进销存-商品
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int         true  "商品 ID"
// @Param        body  body  ProductReq  true  "商品信息"
// @Success      200   {object}  response.Body
// @Router       /inv/products/{id} [put]
func (h *Handler) ProductUpdate(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req ProductReq
	if err := bind.JSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "修改商品")
	if err := h.svc.ProductUpdate(id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "修改成功")
}

// ProductDelete 删除商品
// @Summary      删除商品
// @Tags         进销存-商品
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "商品 ID"
// @Success      200  {object}  response.Body
// @Router       /inv/products/{id} [delete]
func (h *Handler) ProductDelete(c *gin.Context) {
	id, err := bind.PathID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Set("opAction", "删除商品")
	if err := h.svc.ProductDelete(id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMsg(c, "删除成功")
}
