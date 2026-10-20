<script setup lang="ts">
/**
 * 采购 / 销售单打印弹窗（自包含）。
 *
 * 与 StockPrintDialog 同一套闭环：父级只管 `printRef.value.open(row)`，
 * 取数（单据详情 + 启用中的模板）、模板选择、版式渲染与调起系统打印都在这里。
 * 模板按 docType 匹配业务线（采购单 / 销售单），都不匹配则取第一个。
 */
import { computed, ref } from 'vue'
import { invApi } from '@/api'
import type { InvOrderDetail, InvOrderRow, InvPrintTemplateRow } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { parseContent, type PrintContentConfig } from './printConfig'
import PrintSheet from './PrintSheet.vue'

defineOptions({ name: 'OrderPrintDialog' })

const props = defineProps<{
  bizType: 'purchase' | 'sales'
  /** 往来单位标签：供应商 / 客户 */
  partyLabel: string
}>()

const emit = defineEmits<{ printed: [] }>()

const visible = ref(false)
const loading = ref(false)
const detail = ref<InvOrderDetail | null>(null)
const templates = ref<InvPrintTemplateRow[]>([])
const templateId = ref<number | null>(null)

const currentTemplate = computed(() => templates.value.find((t) => t.id === templateId.value))
const paper = computed(() => (currentTemplate.value?.paper === 'A5' ? 'A5' : 'A4'))
const cfg = computed<PrintContentConfig>(() => parseContent(currentTemplate.value?.content))

/** 配置标题留空时按业务线取默认——「采购单 / 销售单」比模板名更贴纸面 */
const title = computed(() => cfg.value.title || (props.bizType === 'purchase' ? '采购单' : '销售单'))

/** 打开并取数。模板默认选与业务线匹配的类型，都没有则取第一个 */
async function open(row: InvOrderRow): Promise<void> {
  visible.value = true
  loading.value = true
  detail.value = null
  try {
    const [d, t] = await Promise.all([
      invApi.orderDetail(row.id),
      invApi.printTemplates({ page: 1, pageSize: 100, status: 1 }),
    ])
    detail.value = d
    templates.value = t.list
    const key = props.bizType === 'purchase' ? '采购单' : '销售单'
    const matched = t.list.find((x) => (x.docType || '').includes(key))
    templateId.value = (matched ?? t.list[0])?.id ?? null
  } finally {
    loading.value = false
  }
}

function doPrint(): void {
  if (!detail.value) return
  document.body.classList.add('printing')
  const done = () => document.body.classList.remove('printing')
  window.addEventListener('afterprint', done, { once: true })
  window.print()
  // 部分浏览器（尤其 Firefox 旧版）不触发 afterprint，兜底摘掉类
  setTimeout(done, 2000)
  emit('printed')
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`打印${bizType === 'purchase' ? '采购单' : '销售单'}`"
    width="1280px"
    top="4vh"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div v-loading="loading" class="print-body">
      <div class="print-body__toolbar">
        <span class="print-body__label">打印模板</span>
        <el-select
          v-model="templateId"
          placeholder="选择模板（未配置时按默认版式打印）"
          style="width: 320px"
        >
          <el-option
            v-for="t in templates"
            :key="t.id"
            :value="t.id"
            :label="`${t.name}（${t.paper || 'A4'}）`"
          />
        </el-select>
        <el-button
          type="primary"
          :disabled="!detail || templates.length === 0"
          @click="doPrint"
        >
          <AppIcon name="Printer" />
          <span>打印</span>
        </el-button>
        <span v-if="templates.length === 0" class="print-body__hint">
          暂无启用中的模板，可到「打印模板」页新增
        </span>
        <span class="print-body__note">
          出现日期 / 网址页眉页脚时，在打印对话框「更多设置」里把「边距」选为「无」即可去掉
        </span>
      </div>

      <div v-if="detail" class="print-body__preview">
        <PrintSheet
          mode="order"
          :detail="detail"
          :cfg="cfg"
          :title="title"
          :paper="paper"
          :party-label="partyLabel"
        />
      </div>
    </div>
  </el-dialog>
</template>

<style scoped lang="scss">
.print-body {
  min-height: 320px;

  &__toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
  }

  &__label {
    color: var(--app-text-muted);
  }

  &__hint {
    color: var(--tone-red);
    font-size: 12px;
  }

  &__note {
    color: var(--app-text-muted);
    font-size: 12px;
    margin-left: auto;
  }

  &__preview {
    max-height: 72vh;
    overflow: auto;
    padding: 4px;
  }
}
</style>

<style lang="scss">
/* 打印态：纸面以外的整页内容全部隐掉（与出入库单打印共用同一套全局规则） */
@media print {
  body.printing * {
    visibility: hidden !important;
  }

  body.printing .print-body__preview {
    max-height: none;
    overflow: visible;
    padding: 0;
  }

  body.printing .sheet {
    position: absolute;
    left: 0;
    top: 0;
    width: auto !important;
    /* 页边距已由模板注入 @page（毫米），纸面自身留白归零，避免双重边距 */
    padding: 0 !important;
    box-shadow: none;
  }

  body.printing .sheet,
  body.printing .sheet * {
    visibility: visible !important;
  }
}
</style>
