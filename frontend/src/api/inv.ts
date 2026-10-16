import { http } from '@/utils/request'
import { downloadFile } from '@/utils/download'
import type {
  InvCategoryOption,
  InvCategoryPayload,
  InvCategoryRow,
  InvFinancePage,
  InvInventoryRow,
  InvOrderDetail,
  InvOrderPayload,
  InvOrderRow,
  InvOrderStatus,
  InvOverview,
  InvPartyPayload,
  InvPartyRow,
  InvPrintTemplatePayload,
  InvPrintTemplateRow,
  InvProductDupRow,
  InvProductPayload,
  InvProductPickResult,
  InvProductRow,
  InvReportCategoryRow,
  InvReportInventory,
  InvReportRankRow,
  InvReportSummary,
  InvReportTrendPoint,
  InvSettlePayload,
  InvStockOrderDetail,
  InvStockOrderPayload,
  InvStockOrderRow,
  PageResult,
} from '@/types/api'

/** 查询参数统一用宽松类型：调用方传的是内联字面量，无需逐个声明形状 */
type Params = Record<string, unknown>

/**
 * 进销存接口。采购与销售共用 /inv/orders 一组接口，凭 bizType 区分。
 */
export const invApi = {
  // ---------------- 概览与报表 ----------------
  overview() {
    return http.get<InvOverview>('/inv/overview')
  },
  reportSummary(params?: Params) {
    return http.get<InvReportSummary>('/inv/report/summary', params)
  },
  reportTrend(params?: Params) {
    return http.get<InvReportTrendPoint[]>('/inv/report/trend', params)
  },
  reportRanking(params?: Params) {
    return http.get<InvReportRankRow[]>('/inv/report/ranking', params)
  },
  reportCategory(params?: Params) {
    return http.get<InvReportCategoryRow[]>('/inv/report/category', params)
  },
  reportInventory() {
    return http.get<InvReportInventory>('/inv/report/inventory')
  },
  reportExport(params?: Params, fallbackName = '报表.xlsx') {
    return downloadFile('/inv/report/export', params as Record<string, unknown>, fallbackName)
  },

  // ---------------- 客户 ----------------
  customers(params?: Params) {
    return http.get<PageResult<InvPartyRow>>('/inv/customers', params)
  },
  createCustomer(data: InvPartyPayload) {
    return http.post<{ id: number }>('/inv/customers', data)
  },
  updateCustomer(id: number, data: InvPartyPayload) {
    return http.put<string>(`/inv/customers/${id}`, data)
  },
  deleteCustomer(id: number) {
    return http.delete<string>(`/inv/customers/${id}`)
  },

  // ---------------- 供应商 ----------------
  suppliers(params?: Params) {
    return http.get<PageResult<InvPartyRow>>('/inv/suppliers', params)
  },
  createSupplier(data: InvPartyPayload) {
    return http.post<{ id: number }>('/inv/suppliers', data)
  },
  updateSupplier(id: number, data: InvPartyPayload) {
    return http.put<string>(`/inv/suppliers/${id}`, data)
  },
  deleteSupplier(id: number) {
    return http.delete<string>(`/inv/suppliers/${id}`)
  },

  // ---------------- 商品分类 ----------------
  categories(params?: Params) {
    return http.get<PageResult<InvCategoryRow>>('/inv/categories', params)
  },
  /** 启用中的分类下拉项 */
  categoryOptions() {
    return http.get<InvCategoryOption[]>('/inv/categories/options')
  },
  createCategory(data: InvCategoryPayload) {
    return http.post<{ id: number }>('/inv/categories', data)
  },
  updateCategory(id: number, data: InvCategoryPayload) {
    return http.put<string>(`/inv/categories/${id}`, data)
  },
  deleteCategory(id: number) {
    return http.delete<string>(`/inv/categories/${id}`)
  },

  // ---------------- 商品 / 库存查询 ----------------
  // 商品列表既供商品管理页（带分页与筛选），也供单据明细的商品下拉
  products(params?: Params) {
    return http.get<PageResult<InvProductRow>>('/inv/products', params)
  },
  /** 商品 → 规格 → 单位 级联选项，供单据明细三级下拉使用 */
  productPick(params?: Params) {
    return http.get<InvProductPickResult>('/inv/products/pick', params)
  },
  /** 录入商品时的同名提示：只提示不拦截，同名不同规格是合法场景 */
  productDuplicates(params: Params) {
    return http.get<InvProductDupRow[]>('/inv/products/duplicates', params)
  },
  createProduct(data: InvProductPayload) {
    return http.post<{ id: number }>('/inv/products', data)
  },
  productDetail(id: number) {
    return http.get<InvProductRow>(`/inv/products/${id}`)
  },
  updateProduct(id: number, data: InvProductPayload) {
    return http.put<string>(`/inv/products/${id}`, data)
  },
  deleteProduct(id: number) {
    return http.delete<string>(`/inv/products/${id}`)
  },
  inventory(params?: Params) {
    return http.get<PageResult<InvInventoryRow>>('/inv/inventory', params)
  },

  // ---------------- 采购 / 销售单据 ----------------
  orders(params: Params) {
    return http.get<PageResult<InvOrderRow>>('/inv/orders', params)
  },
  orderDetail(id: number) {
    return http.get<InvOrderDetail>(`/inv/orders/${id}`)
  },
  createOrder(data: InvOrderPayload) {
    return http.post<{ id: number }>('/inv/orders', data)
  },
  updateOrder(id: number, data: InvOrderPayload) {
    return http.put<string>(`/inv/orders/${id}`, data)
  },
  deleteOrder(id: number) {
    return http.delete<string>(`/inv/orders/${id}`)
  },
  changeOrderStatus(id: number, status: InvOrderStatus) {
    return http.put<string>(`/inv/orders/${id}/status`, { status })
  },
  reverseOrder(id: number) {
    return http.put<string>(`/inv/orders/${id}/reverse`)
  },

  // ---------------- 出入库 ----------------
  stockOrders(params?: Params) {
    return http.get<PageResult<InvStockOrderRow>>('/inv/stock-orders', params)
  },
  createStockOrder(data: InvStockOrderPayload) {
    return http.post<{ id: number }>('/inv/stock-orders', data)
  },
  voidStockOrder(id: number) {
    return http.delete<string>(`/inv/stock-orders/${id}`)
  },
  stockOrderDetail(id: number) {
    return http.get<InvStockOrderDetail>(`/inv/stock-orders/${id}`)
  },

  // ---------------- 应收应付 ----------------
  receivables(params?: Params) {
    return http.get<InvFinancePage>('/inv/finance/receivables', params)
  },
  payables(params?: Params) {
    return http.get<InvFinancePage>('/inv/finance/payables', params)
  },
  settle(data: InvSettlePayload) {
    return http.post<string>('/inv/finance/settlements', data)
  },

  // ---------------- 打印模板 ----------------
  printTemplates(params?: Params) {
    return http.get<PageResult<InvPrintTemplateRow>>('/inv/print-templates', params)
  },
  createPrintTemplate(data: InvPrintTemplatePayload) {
    return http.post<{ id: number }>('/inv/print-templates', data)
  },
  updatePrintTemplate(id: number, data: InvPrintTemplatePayload) {
    return http.put<string>(`/inv/print-templates/${id}`, data)
  },
  deletePrintTemplate(id: number) {
    return http.delete<string>(`/inv/print-templates/${id}`)
  },
}
