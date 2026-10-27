import { describe, expect, it } from 'vitest'
import { purchaseGuidePresentation, purchaseTourSteps, type GuideOrder } from './purchaseGuidePresentation'

const order: GuideOrder = {
  orderNo: 'CG001', status: 1, settleStatus: 'unpaid',
  totalAmount: 1200, settledAmount: 0,
}

describe('current purchase milestones', () => {
  it('shows a route preview without fabricated progress when no order is selected', () => {
    expect(purchaseGuidePresentation().measurable).toBe(false)
  })
  it('keeps a new entry at zero even if another order was selected behind the dialog', () => {
    const view = purchaseGuidePresentation({ ...order, status: 2, settleStatus: 'paid' }, true)
    expect(view.completed).toBe(0)
    expect(view.consequence).toContain('不会增加库存或登记付款')
  })
  it('keeps a saved order at one and tells users to wait for actual arrival', () => {
    const view = purchaseGuidePresentation(order)
    expect(view.completed).toBe(1)
    expect(view.consequence).toContain('尚未到货时可保留待处理')
  })
  it('keeps an existing draft at one while editing and directs back to saving changes', () => {
    const view = purchaseGuidePresentation(order, false, true)
    expect(view.completed).toBe(1)
    expect(view.instruction).toContain('保存修改后再回到核对区')
  })
  it('leaves both unpaid and partially paid orders at two, using the actual balance', () => {
    expect(purchaseGuidePresentation({ ...order, status: 2 }).completed).toBe(2)
    const view = purchaseGuidePresentation({ ...order, status: 2, settleStatus: 'partial', settledAmount: 300 })
    expect(view.completed).toBe(2)
    expect(view.instruction).toContain('900.00')
    expect(view.label).toBe('部分付款')
  })
  it('completes only when an approved order is paid, and regresses after reversal', () => {
    expect(purchaseGuidePresentation({ ...order, status: 2, settleStatus: 'paid', settledAmount: 1200 }).completed).toBe(3)
    expect(purchaseGuidePresentation(order).completed).toBe(1)
  })
  it('never treats cancellation as completion', () => {
    const view = purchaseGuidePresentation({ ...order, status: 3 })
    expect(view.measurable).toBe(false)
    expect(view.label).toBe('已取消')
  })
  it('does not offer an approval target for already approved or cancelled records', () => {
    for (const status of [2, 3] as const) {
      expect(purchaseTourSteps({ ...order, status }).some(step => step.anchor.includes('approve'))).toBe(false)
    }
  })
})
