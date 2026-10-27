import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { useUserStore } from './user'

type GuideMode = 'offered' | 'active' | 'closed'

/** Only teaching preferences are persisted; never order contents or business state. */
export const usePurchaseGuideStore = defineStore('purchaseGuide', () => {
  const user = useUserStore()
  const modes = ref<Record<string, GuideMode>>({})
  const account = computed(() => String(user.profile?.id ?? user.identity?.userId ?? 'guest'))
  const key = () => `nova_purchase_guide_v1:${account.value}`
  const mode = computed<GuideMode>(() => {
    if (modes.value[account.value]) return modes.value[account.value]
    try {
      const saved = localStorage.getItem(key())
      if (saved === 'closed' || saved === 'active') return saved
    } catch { /* Storage may be unavailable; teaching must still be optional. */ }
    return 'offered'
  })
  function setMode(value: GuideMode): boolean {
    modes.value[account.value] = value
    try {
      localStorage.setItem(key(), value)
      return true
    } catch {
      return false
    }
  }
  return { mode, setMode }
})
