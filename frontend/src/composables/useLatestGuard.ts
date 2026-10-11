/**
 * 请求竞态守卫。
 *
 * 列表页的 load 序列没有序号守卫时，快速连续查询（连续回车、快速翻页、
 * 快速切 tab）会让「先发的慢响应」覆盖「后发的快响应」——表格显示的是
 * 旧查询条件的数据，而 total/选中集已按新条件更新，产生数据错位。
 *
 * 用法：
 *   const guard = useLatestGuard()
 *   async function loadList(): Promise<void> {
 *     const ticket = guard.start()
 *     loading.value = true
 *     try {
 *       const data = await api.list(query)
 *       if (!guard.isCurrent(ticket)) return // 已有更新的查询发出，丢弃过期响应
 *       rows.value = data.list
 *     } finally {
 *       if (guard.isCurrent(ticket)) loading.value = false
 *     }
 *   }
 *
 * finally 里同样要判 isCurrent：过期请求若提前返回会把 loading 提前关掉，
 * 让正在飞行的新请求看起来已经结束。
 */
export function useLatestGuard() {
  let seq = 0
  return {
    /** 每次发请求前调用，返回本次请求的序号票据 */
    start: (): number => ++seq,
    /** 响应回来时判断：false 表示已有更新的请求发出，本次响应应丢弃 */
    isCurrent: (ticket: number): boolean => ticket === seq,
  }
}
