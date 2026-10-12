import { reactive, ref } from 'vue'
import type { PageQuery, PageResult } from '@/types/api'
import { useLatestGuard } from './useLatestGuard'

/** 查询状态：筛选项 F 之上补上必有默认值的分页参数 */
export type TableQuery<F extends PageQuery> = F & { page: number; pageSize: number }

/**
 * 列表页查询状态机。
 *
 * user/role/log 三页（log 一页里有两个 tab，各一套）曾各自维护同一套代码：
 * query（page/pageSize + 筛选项）、loading/rows/total、竞态守卫、load/search/reset。
 * 差异只在「调哪个接口」和「重置哪些筛选项」。收敛成一份——尤其是守卫那段
 * 「finally 里也要判 isCurrent」的坑，不该在每个页面各写一遍还指望不忘。
 *
 * 用法：
 *   const {
 *     query, loading, rows, total,
 *     load: loadList, search: searchTable, reset: resetTable,
 *   } = useTableQuery<UserQuery, UserItem>(
 *     { keyword: '', status: undefined, deptId: undefined },
 *     (params) => userApi.list({ ...params, keyword: params.keyword || undefined }),
 *   )
 *
 * 注意解构：模板只展开 setup 顶层绑定，不会展开 table.query 这类嵌套 ref。
 * search() 供「改筛选条件」用（页码回 1）；reset() 把筛选项恢复成 initial 的值。
 * 页面若还有附加动作（如清勾选），包一层即可。
 */
export function useTableQuery<F extends PageQuery, R>(
  initial: F,
  fetch: (params: TableQuery<F>) => Promise<PageResult<R>>,
) {
  // reactive() 泛型实参会让 TS 得到 UnwrapNestedRefs<...>，无法证明它等于
  // TableQuery<F>，传给 fetch 就会报「F 可能是别的子类型」。这里在创建处
  // 一次性断言收口，之后读写都按 TableQuery<F> 走。
  // 之所以先推断再断言、而不是 reactive<TableQuery<F>>(...)：后者报的是另一个
  // 位置（对象字面量不满足 F 的未知成员），断言放在推断之后最稳。
  const query = reactive({ page: 1, pageSize: 10, ...initial }) as TableQuery<F>
  const loading = ref(false)
  const rows = ref<R[]>([])
  const total = ref(0)
  const guard = useLatestGuard()

  async function load(): Promise<void> {
    const ticket = guard.start()
    loading.value = true
    try {
      const data = await fetch(query)
      if (!guard.isCurrent(ticket)) return // 快速连续查询时丢弃过期响应
      rows.value = data.list
      total.value = data.total
    } finally {
      // 过期请求提前返回会把 loading 提前关掉，让正在飞行的新请求看起来已结束
      if (guard.isCurrent(ticket)) loading.value = false
    }
  }

  function search(): void {
    query.page = 1
    load()
  }

  function reset(): void {
    Object.assign(query, initial, { page: 1 })
    load()
  }

  return { query, loading, rows, total, load, search, reset }
}
