import type { InvOrderRow } from '@/types/api'
import { formatMoney } from '@/utils/format'

export type GuideOrder = Pick<InvOrderRow,
  'orderNo' | 'status' | 'settleStatus' | 'totalAmount' | 'settledAmount'>

/** Derive milestones from the current server snapshot, never from reading help. */
export function purchaseGuidePresentation(order?: GuideOrder | null, entering = false, editing = false) {
  if (entering) return {
    completed: 0, measurable: true, label: '正在录入', title: '先把实际采购情况记录完整',
    instruction: '在下方选择供应商、商品规格和单位，核对数量与单价后保存采购单。',
    consequence: '保存只记录单据，不会增加库存或登记付款。',
  }
  if (!order) return {
    completed: 0, measurable: false, label: '办理路线', title: '从当前要办的采购开始',
    instruction: '新增采购单，或从待办队列选择一张已有单据；指南会随当前单据状态更新。',
    consequence: '录单、审核入库和登记付款是三个独立环节，请根据实际业务情况办理。',
  }
  if (order.status === 3) return {
    completed: 0, measurable: false, label: '已取消', title: '这张采购单已取消',
    instruction: '当前单据不继续入库或付款。请选择其他待处理单据，或按实际采购情况新建单据。',
    consequence: '已取消不代表完成采购，指南不会把它计作已完成。',
  }
  if (order.status === 1) return {
    completed: 1, measurable: true, label: '待核对入库', title: '已保存，下一步核对实际到货',
    instruction: editing
      ? '在下方修正供应商、商品规格、数量、单位和金额，保存修改后再回到核对区。'
      : '在核对区检查供应商、商品规格、数量、单位和金额；填错时先编辑采购单。',
    consequence: '确认货物已实际到达后，再审核并入库；尚未到货时可保留待处理。',
  }
  if (order.settleStatus === 'paid') return {
    completed: 3, measurable: true, label: '已办理完成', title: '这张采购单已入库并结清付款',
    instruction: '可继续处理下一张采购单，或查看当前单据明细和相关记录。',
    consequence: '已有付款的单据不能直接反审核，发现录错时请先核查关联付款。',
  }
  return {
    completed: 2, measurable: true,
    label: order.settleStatus === 'partial' ? '部分付款' : '待登记付款',
    title: '已审核入库，按实际付款情况登记结算',
    instruction: `剩余未付 ${formatMoney(Math.max(0, order.totalAmount - order.settledAmount))}。请在应付管理中核查并登记实际付款。`,
    consequence: '可以稍后付款；审核入库不等于已付款，登记付款后才会减少未付金额。',
  }
}
