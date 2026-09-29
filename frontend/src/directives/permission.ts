import type { App, Directive, DirectiveBinding } from 'vue'
import { useUserStore } from '@/store'

/**
 * 权限指令：元素级权限控制。
 *
 *   <el-button v-perm="'system:user:create'">新增</el-button>
 *   <el-button v-perm="['system:user:update', 'system:user:delete']">更多</el-button>
 *
 * 无权限时直接从 DOM 中移除该元素。仅用于「显示与否」，
 * 真正的安全边界在后端接口权限校验。
 */
const permission: Directive = {
  mounted(el: HTMLElement, binding: DirectiveBinding<string | string[]>) {
    const userStore = useUserStore()
    const value = binding.value
    if (!value) return

    const codes = Array.isArray(value) ? value : [value]
    const allowed = userStore.hasAnyPerm(codes)
    if (!allowed) {
      el.parentNode?.removeChild(el)
    }
  },
}

export function setupPermissionDirective(app: App): void {
  app.directive('perm', permission)
}

export default permission
