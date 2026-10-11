import type { App, Directive, DirectiveBinding } from 'vue'
import { effectScope, watch } from 'vue'
import { useUserStore } from '@/store'

/**
 * 权限指令：元素级权限控制。
 *
 *   <el-button v-perm="'system:user:create'">新增</el-button>
 *   <el-button v-perm="['system:user:update', 'system:user:delete']">更多</el-button>
 *
 * 仅用于「显示与否」，真正的安全边界在后端接口权限校验。
 *
 * 实现方式：**打标记属性 + CSS 隐藏，绝不碰 DOM 结构**。
 *
 * 为什么不能把元素从 DOM 里摘掉（无论是直接 remove，还是上一版那种「注释节点占位」）：
 *
 *  1. 占位方案要能「换回来」，前提是能感知权限变化。但 Vue 调用指令钩子时
 *     组件的依赖收集是暂停的（pauseTracking），在钩子里读 Pinia 不会建立依赖，
 *     于是 `updated` 根本不会被触发——上一版的补救在机制上就是无效的。
 *  2. 摘除/换回会破坏 Vue 自己的 vnode 锚点。el-table 单元格、el-dropdown
 *     内部都维护锚点，手动替换子节点会让后续 patch 写进已被丢弃的 vnode，
 *     表现为「行操作按钮点了没反应」「下拉菜单错位」，且极难复现定位。
 *  3. `hostRemove` 在 `parentNode === null` 时是 no-op，占位注释会永久泄漏在 DOM 里。
 *
 * 正确做法是**在 mounted 里为每个元素建一个 watch**：watch 的回调运行在
 * 独立的响应式上下文中，依赖收集正常，权限一变就会重新判定。
 * 这样连组件重渲染都不需要——直接切 class 即可。
 */

const PERM_ATTR = 'data-perm'
const DENIED_CLASS = 'perm-denied'

function normalize(value: unknown): string[] {
  if (!value) return []
  return Array.isArray(value) ? (value as string[]) : [value as string]
}

/** 每个元素挂一个停止函数，unmounted 时调用，避免 watcher 泄漏 */
type PermEl = HTMLElement & { __stopPermWatch?: () => void; __lastPermKey?: string }

/** binding.value 的比较键：updated 钩子高频触发，绝大多数时候 value 根本没变 */
function permValueKey(value: string | string[] | undefined): string {
  return Array.isArray(value) ? value.join('\u0000') : String(value ?? '')
}

/** 同步单个元素的可见性（打标记 + CSS 隐藏，不动 DOM 结构） */
function apply(el: HTMLElement, binding: DirectiveBinding<string | string[]>): void {
  const codes = normalize(binding.value)
  if (codes.length === 0) return

  const allowed = useUserStore().hasAnyPerm(codes)
  if (allowed) {
    el.removeAttribute(PERM_ATTR)
    el.classList.remove(DENIED_CLASS)
  } else {
    // 用属性记录判定结果，排查「为什么按钮没显示」时可以直接在 DOM 里看
    el.setAttribute(PERM_ATTR, 'denied')
    el.classList.add(DENIED_CLASS)
  }
}

function startWatch(el: PermEl, binding: DirectiveBinding<string | string[]>): void {
  // effectScope 让这个 watcher 随元素一起销毁，不泄漏到组件之外
  const scope = effectScope()
  scope.run(() => {
    // 依赖 store 的权限集合：权限一变就重新判定。
    // 这条 watch 是权限变化的唯一驱动，不要改成在 updated 钩子里判定——
    // 钩子里读 store 建立不了依赖，权限变化时根本不会被调用。
    watch(
      () => useUserStore().permissions.join(','),
      () => apply(el, binding),
    )
  })
  el.__stopPermWatch = () => scope.stop()
}

const permission: Directive = {
  mounted(el: HTMLElement, binding: DirectiveBinding<string | string[]>) {
    const target = el as PermEl
    target.__lastPermKey = permValueKey(binding.value)
    apply(el, binding)
    startWatch(target, binding)
  },
  updated(el: HTMLElement, binding: DirectiveBinding<string | string[]>) {
    const target = el as PermEl
    const key = permValueKey(binding.value)
    if (target.__lastPermKey === key) return
    // binding.value 变了（比如从固定权限码换成表达式），重新判定并换掉 watcher。
    // value 未变时必须跳过：表格任意状态变化都会触发行内所有 v-perm 走一遍
    // 销毁+重建 effectScope，批量表格下是可观的重复开销，而权限的运行期变化
    // 由 startWatch 里的 watch 驱动，与 updated 无关。
    target.__lastPermKey = key
    apply(el, binding)
    if (target.__stopPermWatch) {
      target.__stopPermWatch()
      target.__stopPermWatch = undefined
    }
    startWatch(target, binding)
  },
  unmounted(el: HTMLElement) {
    const target = el as PermEl
    target.__stopPermWatch?.()
    target.__stopPermWatch = undefined
  },
}

export function setupPermissionDirective(app: App): void {
  app.directive('perm', permission)
}

export { PERM_ATTR, DENIED_CLASS }
export default permission