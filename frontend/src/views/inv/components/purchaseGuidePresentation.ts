import type { InvOrderRow } from '@/types/api'
import { formatMoney } from '@/utils/format'

export type GuideOrder = Pick<InvOrderRow,
  'orderNo' | 'status' | 'settleStatus' | 'totalAmount' | 'settledAmount'>

export interface PurchaseTourStep {
  anchor: string
  title: string
  description: string
  note?: string
}

export function purchaseTourSteps(order?: GuideOrder | null, entering = false, editing = false, overview = false): PurchaseTourStep[] {
  const context = purchaseGuidePresentation(order, entering, editing)
  if (overview) return [{
    anchor: '.purchase-start__actions', title: '从采购工作台开始',
    description: '进入工作台处理已有采购单，或新增一张采购单。',
    note: '保存只记录单据；确认收到货物后审核才会入库并形成应付，付款可单独登记。',
  }]
  if (entering || editing) return [
    { anchor: '[data-purchase-tour="party"]', title: '选择供应商', description: '先确认这张采购单属于哪家供应商，再核对单据日期。' },
    { anchor: '.purchase-entry-help__tools', title: '缺少资料时从这里补齐', description: '维护资料后回到原单据，刷新可选资料。刷新失败可以重试，当前输入会保留。' },
    { anchor: '.item-editor', title: '核对商品与采购数量', description: '选择商品、规格与单位，核对数量和单价；留意换算后的基本数量。' },
    { anchor: '[data-purchase-tour="save"]', title: editing ? '保存本次修改' : '保存采购单', description: '退出引导后再保存。保存只记录单据，不会增加库存或登记付款。', note: '尚未收货也可以先保存，待实际到货后再核对入库。' },
  ]
  if (!order) return [
    { anchor: '[data-purchase-tour="create"]', title: '录入实际采购', description: '从这里新增采购单。先记录供应商、商品与金额，保存后再根据实际到货情况决定是否入库。' },
    { anchor: '.purchase-tabs', title: '找到需要继续的单据', description: '待处理用于日常核对，全部单据用于查找已审核、已取消或已付款记录。' },
  ]
  return [
    { anchor: '.purchase-queue', title: '选择要核对的采购单', description: '从待办中选择单据，右侧显示对应明细；切换单据不会自动审核。' },
    { anchor: '.order-detail-content .detail-head', title: context.title, description: context.instruction, note: context.consequence },
    { anchor: '.order-detail-content .el-table', title: '逐项核对采购事实', description: '检查商品规格、数量、单位与金额。发现错误时先退出引导，再编辑单据。' },
    ...(order.status === 1 ? [{ anchor: '[data-purchase-tour="approve"]', title: '确认实际收货后再审核', description: '审核会增加库存并形成应付。尚未收货时保留待处理，不必为了走完引导而审核。', note: '引导不会执行审核。请退出后按实际业务情况办理。' }] : []),
  ]
}

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
    instruction: `剩余未付 ${formatMoney(Math.max(0, order.totalAmount - order.settledAmount))}。请打开「应收应付 → 应付明细」，核查并登记实际付款。`,
    consequence: '可以稍后付款；审核入库不等于已付款，登记付款后才会减少未付金额。',
  }
}
