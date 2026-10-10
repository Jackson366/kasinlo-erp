import type { App, Directive, DirectiveBinding } from 'vue'
import { useUserStore } from '@/store'

/**
 * 权限指令：元素级权限控制。
 *
 *   <el-button v-perm="'system:user:create'">新增</el-button>
 *   <el-button v-perm="['system:user:update', 'system:user:delete']">更多</el-button>
 *
 * 仅用于「显示与否」，真正的安全边界在后端接口权限校验。
 *
 * 实现上刻意把「移除」换成「用注释节点占位」：
 * 直接把元素从 DOM 里摘掉是不可逆的 —— 一旦权限在运行期变成有权限
 * （例如切换账号、管理员改了角色后重新拉取权限），元素已经不在文档里，
 * 任何后续钩子都拿不到它，按钮就再也回不来了。
 * 用注释节点占位后，有权限时把它换回来即可。
 */

type PlaceholderCarrier = HTMLElement & { __permPlaceholder?: Comment }

function normalize(value: unknown): string[] {
  if (!value) return []
  return Array.isArray(value) ? (value as string[]) : [value as string]
}

function apply(el: PlaceholderCarrier, binding: DirectiveBinding<string | string[]>): void {
  const codes = normalize(binding.value)
  if (codes.length === 0) return

  const allowed = useUserStore().hasAnyPerm(codes)
  const placeholder = el.__permPlaceholder

  if (allowed) {
    // 之前被占位隐藏，现在有权限了 —— 换回来
    if (placeholder?.parentNode) {
      placeholder.parentNode.replaceChild(el, placeholder)
      delete el.__permPlaceholder
    }
    return
  }

  // 无权限：用注释节点占住位置，保留「换回来」的可能
  const parent = el.parentNode
  if (!parent || placeholder) return
  const comment = document.createComment(' v-perm ')
  parent.replaceChild(comment, el)
  el.__permPlaceholder = comment
}

const permission: Directive = {
  mounted(el: HTMLElement, binding: DirectiveBinding<string | string[]>) {
    apply(el as PlaceholderCarrier, binding)
  },
  // 组件每次更新都重新判定：权限可能在运行期变化（切换账号 / 重新拉取权限）
  updated(el: HTMLElement, binding: DirectiveBinding<string | string[]>) {
    apply(el as PlaceholderCarrier, binding)
  },
}

export function setupPermissionDirective(app: App): void {
  app.directive('perm', permission)
}

export default permission
