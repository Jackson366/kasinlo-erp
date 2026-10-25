import { onBeforeUnmount } from 'vue'

/**
 * 关键字输入即查询的防抖封装。
 *
 * 列表页的「查询」按钮与回车仍然有效，但录关键字时不必再按一下：
 * 输入停顿 delay 毫秒后自动跑一次 search。三个约束：
 *
 *  - 防抖而不是节流：连续敲字只跑最后一次，不会每敲一个字母打一次接口；
 *  - 只绑 @input，不动分页 / 筛选下拉：那些控件的 change 有自己的语义，
 *    混进来会让「翻页」又把页码重置回 1；
 *  - 组件卸载前清定时器：慢响应回来时页面已经切走，不该再触发请求。
 */
export function useDebouncedSearch(fn: () => void, delay = 320) {
  let timer: ReturnType<typeof setTimeout> | undefined

  function cancel(): void {
    if (timer) clearTimeout(timer)
    timer = undefined
  }

  function searchNow(): void {
    cancel()
    fn()
  }

  function searchSoon(): void {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = undefined
      fn()
    }, delay)
  }

  onBeforeUnmount(cancel)

  return { searchSoon, searchNow, cancel }
}
