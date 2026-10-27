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

/** 提醒阈值只用于视觉提示，不代表审批 SLA。 */
export const pendingWarningHours = 24
export function pendingAge(
  since?: string | null,
  now = Date.now(),
): { text: string; warning: boolean } {
  const at = since ? Date.parse(since) : NaN
  if (!Number.isFinite(at)) return { text: '历史数据未记录', warning: false }
  const hours = Math.max(0, (now - at) / 3600000)
  return {
    text:
      hours < 1
        ? '等待不足 1 小时'
        : hours < 24
          ? `已等待 ${Math.floor(hours)} 小时`
          : `已等待 ${Math.floor(hours / 24)} 天`,
    warning: hours >= pendingWarningHours,
  }
}
