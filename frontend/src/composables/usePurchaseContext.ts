import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

/** 只接受采购页作为返回目标，不执行查询字符串中的外部地址。 */
export function usePurchaseContext() {
  const route = useRoute()
  const router = useRouter()
  const purchaseId = computed(() => {
    const n = Number(route.query.purchaseId)
    return Number.isSafeInteger(n) && n > 0 ? n : undefined
  })
  const purchaseNo = computed(() =>
    typeof route.query.purchaseNo === 'string' ? route.query.purchaseNo : '',
  )
  const returnTarget = computed(() => {
    const raw = route.query.returnTo
    if (typeof raw === 'string' && raw.startsWith('/inv/purchase?')) {
      const resolved = router.resolve(raw)
      if (resolved.path === '/inv/purchase')
        return {
          path: resolved.path,
          query: { ...resolved.query, review: String(purchaseId.value) },
        }
    }
    return {
      path: '/inv/purchase',
      query: { review: String(purchaseId.value), view: 'pending' },
    }
  })
  return { purchaseId, purchaseNo, returnTarget }
}
