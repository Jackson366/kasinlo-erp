import {
  computed,
  inject,
  onBeforeUnmount,
  onMounted,
  ref,
  type InjectionKey,
} from 'vue'
import {
  onBeforeRouteLeave,
  onBeforeRouteUpdate,
  type RouteLocationNormalized,
} from 'vue-router'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'

export const pageLeaveGuardsKey: InjectionKey<Set<() => Promise<boolean>>> =
  Symbol('page-leave-guards')

/** Shared protection for long forms. Baselines are captured after defaults/detail hydration. */
export function useUnsavedChanges(options: {
  active: () => boolean
  value: () => unknown
  busy: () => boolean
  label: string
  onNavigate: () => void
  guardUpdate?: (
    to: RouteLocationNormalized,
    from: RouteLocationNormalized,
  ) => boolean
}) {
  const baseline = ref('')
  const dirty = computed(
    () =>
      options.active() && JSON.stringify(options.value()) !== baseline.value,
  )
  let pending: Promise<boolean> | undefined

  function markClean(): void {
    baseline.value = JSON.stringify(options.value())
  }

  async function canLeave(): Promise<boolean> {
    if (!options.active()) return true
    if (options.busy()) return false
    if (!dirty.value) return true
    if (pending) return pending
    pending = ElMessageBox.confirm(
      `${options.label}尚未保存，离开后本次修改将丢失。`,
      '放弃本次修改？',
      {
        confirmButtonText: '放弃修改',
        cancelButtonText: '继续编辑',
        type: 'warning',
        autofocus: false,
        closeOnClickModal: false,
        closeOnHashChange: false,
      },
    ).then(
      () => true,
      () => false,
    )
    try {
      return await pending
    } finally {
      pending = undefined
    }
  }

  async function beforeClose(done: () => void): Promise<void> {
    if (await canLeave()) done()
  }

  async function navigate(): Promise<boolean> {
    const allowed = await canLeave()
    if (allowed) options.onNavigate()
    return allowed
  }
  const refreshGuards = inject(pageLeaveGuardsKey, undefined)
  refreshGuards?.add(canLeave)
  onBeforeUnmount(() => refreshGuards?.delete(canLeave))
  onBeforeRouteLeave(navigate)
  onBeforeRouteUpdate((to, from) =>
    options.guardUpdate?.(to, from) ? navigate() : true,
  )

  function beforeUnload(event: BeforeUnloadEvent): void {
    if (!dirty.value && !(options.active() && options.busy())) return
    event.preventDefault()
    event.returnValue = ''
  }
  onMounted(() => window.addEventListener('beforeunload', beforeUnload))
  onBeforeUnmount(() =>
    window.removeEventListener('beforeunload', beforeUnload),
  )
  return { dirty, markClean, canLeave, beforeClose }
}
