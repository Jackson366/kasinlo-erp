<script setup lang="ts">
/**
 * 打印模板设计器（自包含弹窗）。
 *
 * 左侧编辑版式，右侧用示例数据实时预览；保存时把配置序列化成 JSON
 * 写进模板的 content 字段。设计器分四组，对应四类可调项：
 *
 *   1. 单据与纸张   标题、纸张规格、标题是否显示
 *   2. 公司信息     公司名 / Logo / 地址 / 电话 / 开户行（纸面表头区）
 *   3. 明细列       显隐、列标题可改、对齐方式、可上下移动调顺序
 *   4. 页面与底部   页边距 / 行高 / 正文字号 / 标题字号 / 表头底色；
 *                  签收栏 / 备注 / 合计大写 / 页脚与附加文字
 *
 * 明细列支持增删改排序：旧版只能勾选预设列，客户想加一列「批次号」
 * 或者把「规格」挪到「名称」前面都做不到。列的渲染统一收在
 * printConfig 的 ColumnKey 里，新增一列要在 OrderSheet / StockSheet
 * 的 cell() 里补一个 case —— 两处都改，纸面才会跟着变。
 *
 * 模板的单据类型指向采购 / 销售单时，预览自动切换到带单价金额的对应版式。
 */
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { invApi } from '@/api'
import type { InvOrderDetail, InvPrintTemplateRow, InvStockOrderDetail } from '@/types/api'
import {
  DEFAULT_COLUMNS,
  parseContent,
  type ColumnKey,
  type PrintColumn,
  type PrintContentConfig,
} from './printConfig'
import StockSheet from './StockSheet.vue'
import OrderSheet from './OrderSheet.vue'

defineOptions({ name: 'TemplateDesigner' })

const emit = defineEmits<{ saved: [] }>()

const visible = ref(false)
const saving = ref(false)
const row = ref<InvPrintTemplateRow | null>(null)
const paper = ref<'A4' | 'A5'>('A4')
/** 左侧分组默认展开哪一个（单据组，最常用） */
const activeGroup = ref('doc')

const cfg = reactive<PrintContentConfig>(parseContent(''))

/** 设计器预览用示例数据——字段结构与真实打印详情一致 */
const sample: InvStockOrderDetail = {
  id: 0,
  orderNo: 'CK202610130001',
  direction: 'in',
  sourceType: 'purchase',
  sourceOrderNo: 'CG202610010001',
  orderDate: '2026-10-13T00:00:00Z',
  remark: '示例备注：按采购单批量入库，已验收。',
  operator: '张三',
  totalQty: 148,
  items: [
    // 第 2 行刻意用非基本单位（1 箱 = 10 盒），让预览能看出折算行
    { productCode: 'SP001', productName: '高强度螺栓 M12', spec: 'M12×40', unit: '盒', rate: 1, qty: 120, qtyBase: 120, unitBase: '盒' },
    { productCode: 'SP002', productName: '角钢 50×50', spec: '50×50×5mm', unit: '箱', rate: 10, qty: 2, qtyBase: 20, unitBase: '根' },
    { productCode: 'SP003', productName: '密封胶条', spec: '10m/卷', unit: '卷', rate: 1, qty: 8, qtyBase: 8, unitBase: '卷' },
  ],
}

/** 采购 / 销售单预览用示例数据（带单价金额） */
const sampleOrder: InvOrderDetail = {
  id: 0,
  orderNo: 'CG202610130001',
  bizType: 'purchase',
  partyId: 0,
  partyName: '信阳市宏达钢材有限公司',
  orderDate: '2026-10-13T00:00:00Z',
  dueDate: null, // 到期日是内部结算字段，不进纸面；示例数据同样留空
  status: 1,
  totalAmount: 15440,
  settledAmount: 0,
  settleStatus: 'unpaid',
  itemCount: 3,
  remark: '示例备注：含运费，货到付款。',
  createdAt: '2026-10-13T08:00:00Z',
  operator: '张三',
  items: [
    { id: 1, productId: 1, specId: 1, unitId: 1, productCode: 'SP001', productName: '高强度螺栓 M12', spec: 'M12×40', unit: '盒', rate: 1, qty: 120, qtyBase: 120, price: 45, amount: 5400 },
    // 第 2 行用「箱」成交（1 箱 = 10 根），单价按箱计，纸面会多出一行换算说明
    { id: 2, productId: 2, specId: 2, unitId: 3, productCode: 'SP002', productName: '角钢 50×50', spec: '50×50×5mm', unit: '箱', rate: 10, qty: 8, qtyBase: 80, price: 180, amount: 1440 },
    { id: 3, productId: 3, specId: 3, unitId: 4, productCode: 'SP003', productName: '密封胶条', spec: '10m/卷', unit: '卷', rate: 1, qty: 80, qtyBase: 80, price: 107.5, amount: 8600 },
  ],
}

/** 模板的单据类型指向采购 / 销售单时，预览切换到对应的纸面版式 */
const isOrderDoc = computed(() => /采购|销售/.test(row.value?.docType || ''))
const orderPartyLabel = computed(() =>
  /销售/.test(row.value?.docType || '') ? '客户' : '供应商',
)

function open(r: InvPrintTemplateRow): void {
  row.value = r
  paper.value = r.paper === 'A5' ? 'A5' : 'A4'
  Object.assign(cfg, parseContent(r.content))
  visible.value = true
}

const previewTitle = computed(() => {
  if (cfg.title) return cfg.title
  if (isOrderDoc.value) return /采购/.test(row.value?.docType || '') ? '采购单' : '销售单'
  return sample.direction === 'in' ? '入库单' : '出库单'
})

// ---------------- 明细列编辑 ----------------
/**
 * 可添加的列池。出入库单没有单价 / 金额（成本不归仓管看），
 * 所以那两个键在库存单据下不进池子 —— 加进去纸面也没有数据可渲染。
 */
const columnPool = computed<PrintColumn[]>(() =>
  DEFAULT_COLUMNS.filter((c) => (isOrderDoc.value ? c.key !== 'price' && c.key !== 'amount' : true)),
)

function moveCol(index: number, dir: -1 | 1): void {
  const to = index + dir
  if (to < 0 || to >= cfg.columns.length) return
  const [it] = cfg.columns.splice(index, 1)
  cfg.columns.splice(to, 0, it)
}

function addColumn(key: ColumnKey): void {
  if (cfg.columns.some((c) => c.key === key)) {
    ElMessage.warning('这一列已经在表里了')
    return
  }
  const tpl = DEFAULT_COLUMNS.find((c) => c.key === key)
  if (tpl) cfg.columns.push({ ...tpl })
}

function removeCol(index: number): void {
  const c = cfg.columns[index]
  // 名称与数量是表格的主体，删了纸面就没有可读内容
  if (c.key === 'name' || c.key === 'qty') {
    ElMessage.warning(`「${c.label}」是明细表的主体列，不能删除（可以改为不显示）`)
    return
  }
  cfg.columns.splice(index, 1)
}

/** 恢复默认列集合：用户改乱了之后的一条退路 */
function resetColumns(): void {
  cfg.columns = DEFAULT_COLUMNS.filter((c) => (isOrderDoc.value ? c.key !== 'price' && c.key !== 'amount' : true)).map(
    (c) => ({ ...c }),
  )
}

async function save(): Promise<void> {
  if (!row.value) return
  saving.value = true
  try {
    await invApi.updatePrintTemplate(row.value.id, {
      code: row.value.code,
      name: row.value.name,
      docType: row.value.docType,
      paper: paper.value,
      content: JSON.stringify(cfg),
      status: row.value.status,
      remark: row.value.remark,
    })
    ElMessage.success('版式已保存')
    emit('saved')
    visible.value = false
  } finally {
    saving.value = false
  }
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`设计版式 — ${row?.name || ''}`"
    width="1180px"
    top="4vh"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div class="designer">
      <div class="designer__panel">
        <el-form label-width="82px" label-position="left" size="small">
          <el-collapse v-model="activeGroup">
            <el-collapse-item title="单据与纸张" name="doc">
              <el-form-item label="单据标题">
                <el-input v-model="cfg.title" placeholder="留空则按单据类型显示默认标题" />
              </el-form-item>
              <el-form-item label="显示标题">
                <el-switch v-model="cfg.header.showTitle" />
              </el-form-item>
              <el-form-item label="纸张规格">
                <el-radio-group v-model="paper">
                  <el-radio-button value="A4">A4</el-radio-button>
                  <el-radio-button value="A5">A5</el-radio-button>
                </el-radio-group>
              </el-form-item>
            </el-collapse-item>

            <el-collapse-item title="公司信息（表头区）" name="company">
              <el-form-item label="公司名称">
                <el-input v-model="cfg.header.companyName" placeholder="显示在标题下方" />
              </el-form-item>
              <el-form-item label="Logo 地址">
                <el-input v-model="cfg.header.logoUrl" placeholder="图片 URL，留空不显示" />
              </el-form-item>
              <el-form-item label="地址">
                <el-input v-model="cfg.header.address" />
              </el-form-item>
              <el-form-item label="电话">
                <el-input v-model="cfg.header.phone" />
              </el-form-item>
              <el-form-item label="开户行">
                <el-input v-model="cfg.header.bank" placeholder="选填，显示在地址下方" />
              </el-form-item>
            </el-collapse-item>

            <el-collapse-item title="明细列（可改名 / 排序）" name="columns">
              <div class="col-editor">
                <div class="col-editor__row col-editor__row--head">
                  <span class="col-editor__move" />
                  <span class="col-editor__label">列标题</span>
                  <span class="col-editor__align">对齐</span>
                  <span class="col-editor__show">显示</span>
                  <span class="col-editor__op" />
                </div>
                <div v-for="(c, i) in cfg.columns" :key="c.key" class="col-editor__row">
                  <span class="col-editor__move">
                    <el-button link size="small" :disabled="i === 0" @click="moveCol(i, -1)">↑</el-button>
                    <el-button
                      link
                      size="small"
                      :disabled="i === cfg.columns.length - 1"
                      @click="moveCol(i, 1)"
                    >
                      ↓
                    </el-button>
                  </span>
                  <span class="col-editor__label">
                    <el-input v-model="c.label" size="small" />
                  </span>
                  <span class="col-editor__align">
                    <el-select v-model="c.align" size="small">
                      <el-option label="左" value="left" />
                      <el-option label="中" value="center" />
                      <el-option label="右" value="right" />
                    </el-select>
                  </span>
                  <span class="col-editor__show">
                    <el-checkbox v-model="c.visible" />
                  </span>
                  <span class="col-editor__op">
                    <el-button link type="danger" size="small" @click="removeCol(i)">删</el-button>
                  </span>
                </div>
              </div>
              <div class="col-editor__foot">
                <span class="text-muted">可添加：</span>
                <el-button
                  v-for="p in columnPool"
                  :key="p.key"
                  link
                  type="primary"
                  size="small"
                  @click="addColumn(p.key)"
                >
                  {{ p.label }}
                </el-button>
                <el-button link size="small" @click="resetColumns">恢复默认列</el-button>
              </div>
            </el-collapse-item>

            <el-collapse-item title="页面设置" name="page">
              <el-form-item label="页边距">
                <el-input-number v-model="cfg.page.margin" :min="0" :max="60" size="small" controls-position="right" />
                <span class="unit">px</span>
              </el-form-item>
              <el-form-item label="明细行高">
                <el-input-number v-model="cfg.page.rowHeight" :min="20" :max="80" size="small" controls-position="right" />
                <span class="unit">px</span>
              </el-form-item>
              <el-form-item label="正文字号">
                <el-select v-model="cfg.page.fontSize" size="small" style="width: 110px">
                  <el-option v-for="n in [9, 10, 11, 12, 13, 14]" :key="n" :value="n" :label="`${n} pt`" />
                </el-select>
              </el-form-item>
              <el-form-item label="标题字号">
                <el-select v-model="cfg.page.titleFontSize" size="small" style="width: 110px">
                  <el-option v-for="n in [16, 18, 20, 22, 24, 26]" :key="n" :value="n" :label="`${n} pt`" />
                </el-select>
              </el-form-item>
              <el-form-item label="表头底色">
                <el-input v-model="cfg.page.headerBg" size="small" placeholder="如 #f2f2f2，留空为无" />
              </el-form-item>
            </el-collapse-item>

            <el-collapse-item title="底部区域" name="footer">
              <el-form-item label="签收栏">
                <el-switch v-model="cfg.footer.showSign" />
              </el-form-item>
              <el-form-item label="单据备注">
                <el-switch v-model="cfg.footer.showRemark" />
              </el-form-item>
              <el-form-item v-if="isOrderDoc" label="合计大写">
                <el-switch v-model="cfg.footer.showAmountCn" />
              </el-form-item>
              <el-form-item label="页脚文字">
                <el-input v-model="cfg.footer.text" placeholder="如：货物品以当面验收为准" maxlength="60" />
              </el-form-item>
              <el-form-item label="附加文字">
                <el-input
                  v-model="cfg.footer.customText"
                  type="textarea"
                  :rows="2"
                  placeholder="显示在备注之后，可写账期、验收标准等"
                />
              </el-form-item>
            </el-collapse-item>
          </el-collapse>
        </el-form>
      </div>

      <div class="designer__preview">
        <OrderSheet
          v-if="isOrderDoc"
          :detail="sampleOrder"
          :cfg="cfg"
          :title="previewTitle"
          :paper="paper"
          :party-label="orderPartyLabel"
        />
        <StockSheet v-else :detail="sample" :cfg="cfg" :title="previewTitle" :paper="paper" />
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存版式</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.designer {
  display: flex;
  gap: 20px;

  &__panel {
    width: 360px;
    flex: none;
    max-height: 74vh;
    overflow: auto;
  }

  &__preview {
    flex: 1;
    max-height: 74vh;
    overflow: auto;
    padding: 4px;
    background: var(--app-bg-muted, transparent);
    border-radius: var(--radius-sm);
  }
}

.unit {
  margin-left: 6px;
  color: var(--app-text-muted);
  font-size: 12px;
}

.col-editor {
  &__row {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 4px;

    &--head {
      font-size: 12px;
      color: var(--app-text-muted);
      margin-bottom: 6px;
    }
  }

  &__move {
    width: 56px;
    flex: none;
  }

  &__label {
    flex: 1;
  }

  &__align {
    width: 72px;
    flex: none;
  }

  &__show {
    width: 40px;
    flex: none;
    text-align: center;
  }

  &__op {
    width: 28px;
    flex: none;
  }

  &__foot {
    margin-top: 8px;
    font-size: 12px;
    line-height: 1.8;
  }
}
</style>
