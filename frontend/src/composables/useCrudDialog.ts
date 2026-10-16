import { computed, ref, type Ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormInstance } from 'element-plus'

export type CrudMode = 'create' | 'edit'

/**
 * CRUD 弹窗公共状态机。
 *
 * user/role/menu/dept 四个系统页曾各自维护同一组变量（dialogVisible /
 * dialogMode / submitting / editingId）和同一段提交骨架（校验 → 按模式
 * 建/改 → 成功提示 → 关窗 → 刷新）。真正的差异只剩「灌表单」和「调接口」，
 * 收敛成一份：弹窗状态不再四份各改一处，提交骨架里的空值链坑
 * （formRef.value?.validate().catch）也只需在一个地方防。
 *
 * 用法：
 *   const dialog = useCrudDialog<MenuPayload>({
 *     formRef,
 *     create: (payload) => menuApi.create(payload),
 *     update: (id, payload) => menuApi.update(id, payload),
 *     messages: { create: '菜单创建成功', update: '菜单已更新' },
 *   })
 *   // 解构保持模板里的旧名字：模板只展开 setup 顶层绑定，
 *   // 不会自动展开 dialog.visible 这种嵌套 ref
 *   const { visible: dialogVisible, mode: dialogMode, submitting, editingId } = dialog
 *
 *   function openEdit(row: MenuNode): void {
 *     dialog.openEdit(row.id) // 先置模式与目标，再灌表单
 *     Object.assign(form, { ... })
 *   }
 *
 *   async function submitForm(): Promise<void> {
 *     await dialog.submit(loadTree, () => ({ ...form }))
 *   }
 */
export function useCrudDialog<P>(options: {
  /** 表单实例引用；未挂载时按校验不通过处理，不能让它把提交静默崩掉 */
  formRef: Ref<FormInstance | undefined>
  create: (payload: P) => Promise<unknown>
  update: (id: number, payload: P) => Promise<unknown>
  messages: { create: string; update: string }
}) {
  const visible = ref(false)
  const mode = ref<CrudMode>('create')
  const submitting = ref(false)
  const editingId = ref<number | null>(null)

  /** 进入「新增」：清空编辑目标；表单重置紧跟其后由调用方灌值 */
  function openCreate(): void {
    mode.value = 'create'
    editingId.value = null
    visible.value = true
  }

  /** 进入「编辑」：记下目标 ID；表单灌值紧跟其后由调用方完成 */
  function openEdit(id: number): void {
    mode.value = 'edit'
    editingId.value = id
    visible.value = true
  }

  /**
   * 统一提交：校验 → 按模式建/改 → 成功提示 → 关窗 → 刷新。
   * buildPayload 在接口调用前一刻构建载荷，页面可依模式裁剪字段
   * （如编辑时不带 username / code）。
   */
  async function submit(reload: () => void, buildPayload: (mode: CrudMode) => P): Promise<void> {
    // 不能写成 formRef.value?.validate().catch(...)：表单未挂载时 ?. 短路出
    // undefined，再 .catch 就是 TypeError，整个提交静默崩溃。
    // 表单没就绪按校验不通过处理，直接返回。
    const valid = options.formRef.value
      ? await options.formRef.value.validate().catch(() => false)
      : false
    if (!valid) return

    submitting.value = true
    try {
      const payload = buildPayload(mode.value)
      if (mode.value === 'create') {
        await options.create(payload)
        ElMessage.success(options.messages.create)
      } else if (editingId.value) {
        await options.update(editingId.value, payload)
        ElMessage.success(options.messages.update)
      }
      visible.value = false
      reload()
    } finally {
      submitting.value = false
    }
  }

  return {
    visible,
    mode,
    submitting,
    editingId,
    isEdit: computed(() => mode.value === 'edit'),
    openCreate,
    openEdit,
    submit,
  }
}
