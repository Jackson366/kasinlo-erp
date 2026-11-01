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
import { computed, ref, watch, nextTick } from 'vue'
import { printSheet } from './printSheet'
import { invApi } from '@/api'
import type {
  InvStockOrderDetail,
  InvStockOrderRow,
  InvPrintTemplateRow,
} from '@/types/api'
import AppIcon from '@/components/AppIcon.vue'
import { parseContent, type PrintContentConfig } from './printConfig'
import PrintSheet from './PrintSheet.vue'

defineOptions({ name: 'StockPrintDialog' })

const emit = defineEmits<{ printed: [] }>()

const visible = ref(false)
const loading = ref(false)
const printing = ref(false)
const showStamp = ref(true)
const printError = ref('')
const previewRef = ref<HTMLElement>()
const detail = ref<InvStockOrderDetail | null>(null)
const templates = ref<InvPrintTemplateRow[]>([])
const templateId = ref<number | null>(null)

const currentTemplate = computed(() =>
  templates.value.find((t) => t.id === templateId.value),
)
const paper = computed(() =>
  currentTemplate.value?.paper === 'A5' ? 'A5' : 'A4',
)
const cfg = computed<PrintContentConfig>(() =>
  parseContent(currentTemplate.value?.content),
)
watch(templateId, () => {
  showStamp.value = true
  printError.value = ''
})
const missingStamp = computed(
  () => cfg.value.stamp.enabled && showStamp.value && !cfg.value.stamp.image,
)

/**
 * 纸面标题优先级：模板里填的标题 > 模板的单据类型（如「销售出库单」）
 * > 方向默认（入库单 / 出库单）。
 */
const title = computed(() => {
  if (cfg.value.title) return cfg.value.title
  const dt = currentTemplate.value?.docType || ''
  if (/入库|出库|领料|调拨|盘点/.test(dt)) return dt
  return detail.value?.direction === 'in' ? '入库单' : '出库单'
})

/** 打开并取数。模板默认选与方向匹配的单据类型（入库单 / 出库单），都没有则取第一个 */
async function open(row: InvStockOrderRow): Promise<void> {
  showStamp.value = true
  printError.value = ''
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
    // 先按来源（采购 / 销售）+ 方向精确命中，再退到方向，最后退第一个
    const dirKey = d.direction === 'in' ? '入库' : '出库'
    const srcKey =
      d.sourceType === 'sales'
        ? '销售'
        : d.sourceType === 'purchase'
          ? '采购'
          : ''
    const matched =
      (srcKey
        ? t.list.find(
            (x) =>
              (x.docType || '').startsWith(srcKey) &&
              (x.docType || '').includes(dirKey),
          )
        : undefined) ??
      t.list.find((x) => (x.docType || '').includes(dirKey)) ??
      t.list[0]
    templateId.value = matched?.id ?? null
  } finally {
    loading.value = false
  }
}

async function doPrint(): Promise<void> {
  if (
    !detail.value ||
    !previewRef.value ||
    printing.value ||
    missingStamp.value
  )
    return
  printing.value = true
  printError.value = ''
  try {
    await nextTick()
    if (!visible.value) return
    await printSheet(previewRef.value)
    emit('printed')
  } catch {
    printError.value = '印章或 Logo 加载失败，请重新打开预览后重试'
  } finally {
    printing.value = false
  }
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
    :close-on-press-escape="!printing"
    :show-close="!printing"
  >
    <div v-loading="loading" class="print-body">
      <div class="print-body__toolbar">
        <span class="print-body__label">打印模板</span>
        <el-select
          v-model="templateId"
          :disabled="printing"
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
        <el-checkbox
          v-if="cfg.stamp.enabled"
          v-model="showStamp"
          :disabled="printing"
          >显示印章</el-checkbox
        >
        <el-button
          type="primary"
          :disabled="
            loading || !detail || templates.length === 0 || missingStamp
          "
          :loading="printing"
          @click="doPrint"
        >
          <AppIcon name="Printer" />
          <span>打印</span>
        </el-button>
        <span v-if="templates.length === 0" class="print-body__hint">
          暂无启用中的模板，可到「打印模板」页新增
        </span>
        <span class="print-body__note">
          出现日期 /
          网址页眉页脚时，在打印对话框「更多设置」里把「边距」选为「无」即可去掉
        </span>
      </div>

      <p v-if="missingStamp" role="alert" class="print-body__hint">
        模板未配置印章图片，请取消“显示印章”后打印，或到模板设计器补充图片。
      </p>
      <p v-if="printError" role="alert" class="print-body__hint">
        {{ printError }}
      </p>
      <div v-if="detail" ref="previewRef" class="print-body__preview">
        <PrintSheet
          :show-stamp="showStamp"
          mode="stock"
          :detail="detail"
          :cfg="cfg"
          :title="title"
          :paper="paper"
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
    flex-wrap: wrap;
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
