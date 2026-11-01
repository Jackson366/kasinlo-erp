import { computed } from 'vue'

type FilterField<T extends object> = {
  [K in keyof T]-?: {
    key: K
    label: string
    emptyValue: T[K]
    valueLabel: () => string
  }
}[keyof T]

export interface ActiveFilter {
  key: string
  label: string
}

/** 摘要只描述已选条件；是否立即查询由原页面决定，不另建查询状态。 */
export function useAdvancedFilters<T extends object>(
  query: T,
  fields: () => FilterField<T>[],
  search: () => unknown,
) {
  const activeFilters = computed<ActiveFilter[]>(() =>
    fields()
      .filter(({ key }) => {
        const value = query[key]
        return Array.isArray(value)
          ? value.length > 0
          : value !== undefined && value !== null && value !== ''
      })
      .map((field) => ({
        key: String(field.key),
        label: `${field.label}：${field.valueLabel()}`,
      })),
  )

  function clearFilter(key: string): void {
    const field = fields().find((item) => String(item.key) === key)
    if (!field) return
    query[field.key] = field.emptyValue
    search()
  }

  return { activeFilters, clearFilter }
}
