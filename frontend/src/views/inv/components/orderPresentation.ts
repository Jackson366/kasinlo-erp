export const statusMeta: Record<
  number,
  { text: string; type: 'warning' | 'success' | 'info' }
> = {
  1: { text: '待处理', type: 'warning' },
  2: { text: '已审核', type: 'success' },
  3: { text: '已取消', type: 'info' },
}
export const settleMeta: Record<
  string,
  { text: string; type: 'danger' | 'warning' | 'success' }
> = {
  unpaid: { text: '未结清', type: 'danger' },
  partial: { text: '部分结算', type: 'warning' },
  paid: { text: '已结清', type: 'success' },
}
