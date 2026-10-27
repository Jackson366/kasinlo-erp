import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { usePurchaseGuideStore } from './purchaseGuide'

const account = reactive({ profile: { id: 1 }, identity: null })
vi.mock('./user', () => ({ useUserStore: () => account }))

describe('purchase guide preferences', () => {
  let saved: Map<string, string>
  beforeEach(() => {
    setActivePinia(createPinia())
    account.profile.id = 1
    saved = new Map()
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => saved.get(key) ?? null,
      setItem: (key: string, value: string) => saved.set(key, value),
    })
  })
  it('remembers explicit exit across store recreation and can reopen', () => {
    const guide = usePurchaseGuideStore()
    expect(guide.mode).toBe('offered')
    guide.setMode('closed')
    setActivePinia(createPinia())
    const restored = usePurchaseGuideStore()
    expect(restored.mode).toBe('closed')
    restored.setMode('active')
    expect(restored.mode).toBe('active')
  })
  it('keeps account choices separate in the same session', () => {
    const guide = usePurchaseGuideStore()
    guide.setMode('closed')
    account.profile.id = 2
    expect(guide.mode).toBe('offered')
    guide.setMode('active')
    account.profile.id = 1
    expect(guide.mode).toBe('closed')
  })
  it('allows exit even when persistence is blocked', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => { throw new Error('blocked') },
      setItem: () => { throw new Error('blocked') },
    })
    const guide = usePurchaseGuideStore()
    expect(guide.mode).toBe('offered')
    expect(guide.setMode('closed')).toBe(false)
    expect(guide.mode).toBe('closed')
    guide.setMode('active')
    expect(guide.mode).toBe('active')
  })
  it('ignores stale or malformed preference values', () => {
    saved.set('nova_purchase_guide_v1:1', 'completed-unknown')
    expect(usePurchaseGuideStore().mode).toBe('offered')
  })
})
