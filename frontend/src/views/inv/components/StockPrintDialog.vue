<script setup lang="ts">
/**
 * 出入库单打印弹窗（自包含）。
 *
 * 父级只管 `printRef.value.open(row)`；取数（单据详情 + 启用中的模板）、
 * 模板选择、版式渲染与调起系统打印都在这里闭环。
 *
 * 打印原理：window.print() 会打整个页面，所以打印时给 body 挂 `printing` 类，
 * 配合底部非 scoped 的 @media print 规则把纸面以外的内容全部隐掉。
 */
import { computed, ref } from 'vue'
import { invApi } from '@/api'
import type { InvStockOrderDetail, InvStockOrderRow, InvPrintTemplateRow } from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { parseContent, type PrintContentConfig } from './printConfig'
import StockSheet from './StockSheet.vue'

defineOptions({ name: 'StockPrintDialog' })

const emit = defineEmits<{ printed: [] }>()

const visible = ref(false)
const loading = ref(false)
const detail = ref<InvStockOrderDetail | null>(null)
const templates = ref<InvPrintTemplateRow[]>([])
const templateId = ref<number | null>(null)

const currentTemplate = computed(() => templates.value.find((t) => t.id === templateId.value))
const paper = computed(() => (currentTemplate.value?.paper === 'A5' ? 'A5' : 'A4'))
const cfg = computed<PrintContentConfig>(() => parseContent(currentTemplate.value?.content))

/** 配置标题留空时按方向取默认——「入库单 / 出库单」比模板名更贴纸面 */
const title = computed(
  () => cfg.value.title || (detail.value?.direction === 'in' ? '入库单' : '出库单'),
)

/** 打开并取数。模板默认选与方向匹配的单据类型（入库单 / 出库单），都没有则取第一个 */
async function open(row: InvStockOrderRow): Promise<void> {
  visible.value = true
  loading.value = true
  detail.value = null
  try {
    const [d, t] = await Promise.all([
      invApi.stockOrderDetail(row.id),
      invApi.printTemplates({ page: 1, pageSize: 100, status: 1 }),
    ])
    detail.value = d
    templates.value = t.list
    const key = d.direction === 'in' ? '入库单' : '出库单'
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
    title="打印出入库单"
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
      </div>

      <div v-if="detail" class="print-body__preview">
        <StockSheet :detail="detail" :cfg="cfg" :title="title" :paper="paper" />
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

  &__preview {
    max-height: 72vh;
    overflow: auto;
    padding: 4px;
  }
}
</style>

<style lang="scss">
/* 打印态：纸面以外的整页内容全部隐掉（非 scoped，作用于全局） */
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
    box-shadow: none;
  }

  body.printing .sheet,
  body.printing .sheet * {
    visibility: visible !important;
  }
}

@media print {
  @page {
    margin: 12mm;
  }
}
</style>
