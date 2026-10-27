import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

/** Only restore a local purchase page; never trust arbitrary return URLs. */
export function usePurchaseHandoff() {
  const route = useRoute()
  const router = useRouter()
  const returnTo = computed(() => {
    const value = route.query.returnTo
    if (typeof value !== 'string' || !value.startsWith('/inv/purchase?'))
      return undefined
    return router.resolve(value).path === '/inv/purchase' ? value : undefined
  })

  function follow(path: '/inv/stock' | '/inv/finance', apply: () => void) {
    watch(
      () => (route.path === path ? route.fullPath : ''),
      (value) => {
        if (value && returnTo.value) apply()
      },
      { immediate: true },
    )
  }

  return { returnTo, follow }
}
