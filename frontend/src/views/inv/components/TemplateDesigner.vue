<script setup lang="ts">
/**
 * 打印模板设计器（自包含弹窗）。
 *
 * 左侧编辑版式配置（标题 / 列显隐 / 字号 / 页脚 / 纸张），右侧用示例数据实时预览；
 * 保存时把配置序列化成 JSON 写进模板的 content 字段。
 * 「调整设计」的粒度是字段级显隐与文案——版式骨架（表头 / 明细表 / 签字栏）固定，
 * 这是刻意收窄：自由拖拽式设计器成本高，而单据纸面的骨架本来就该长一个样。
 * 模板的单据类型指向采购 / 销售单时，预览自动切换到带单价金额的对应版式。
 */
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { invApi } from '@/api'
import type { InvOrderDetail, InvPrintTemplateRow, InvStockOrderDetail } from '@/types/api'
import { parseContent } from './printConfig'
import StockSheet from './StockSheet.vue'
import OrderSheet from './OrderSheet.vue'

defineOptions({ name: 'TemplateDesigner' })

const emit = defineEmits<{ saved: [] }>()

const visible = ref(false)
const saving = ref(false)
const row = ref<InvPrintTemplateRow | null>(null)
const paper = ref<'A4' | 'A5'>('A4')

const cfg = reactive({
  title: '',
  showCode: true,
  showSpec: true,
  showUnit: true,
  showPrice: true,
  showAmount: true,
  showSource: true,
  showOperator: true,
  showPrintTime: true,
  fontSize: 12,
  footer: '',
})

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
    width="1080px"
    top="4vh"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div class="designer">
      <div class="designer__panel">
        <el-form label-width="82px" label-position="left">
          <el-form-item label="单据标题">
            <el-input v-model="cfg.title" placeholder="留空则按单据类型显示默认标题" />
          </el-form-item>
          <el-form-item label="纸张规格">
            <el-radio-group v-model="paper">
              <el-radio-button value="A4">A4</el-radio-button>
              <el-radio-button value="A5">A5</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="明细列">
            <div class="designer__checks">
              <el-checkbox v-model="cfg.showCode">商品编码</el-checkbox>
              <el-checkbox v-model="cfg.showSpec">规格</el-checkbox>
              <el-checkbox v-model="cfg.showUnit">单位</el-checkbox>
              <el-checkbox v-if="isOrderDoc" v-model="cfg.showPrice">单价</el-checkbox>
              <el-checkbox v-if="isOrderDoc" v-model="cfg.showAmount">金额</el-checkbox>
            </div>
          </el-form-item>
          <el-form-item label="表头信息">
            <div class="designer__checks">
              <el-checkbox v-if="!isOrderDoc" v-model="cfg.showSource">来源单号</el-checkbox>
              <el-checkbox v-model="cfg.showOperator">制单人</el-checkbox>
              <el-checkbox v-model="cfg.showPrintTime">打印时间</el-checkbox>
            </div>
          </el-form-item>
          <el-form-item label="正文字号">
            <el-select v-model="cfg.fontSize" style="width: 120px">
              <el-option v-for="n in [10, 11, 12, 13, 14]" :key="n" :value="n" :label="`${n} pt`" />
            </el-select>
          </el-form-item>
          <el-form-item label="页脚文字">
            <el-input v-model="cfg.footer" placeholder="如：货物品以当面验收为准" maxlength="60" />
          </el-form-item>
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
    width: 300px;
    flex: none;
  }

  &__checks {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  &__preview {
    flex: 1;
    max-height: 72vh;
    overflow: auto;
    padding: 4px;
    background: var(--app-bg-muted, transparent);
    border-radius: var(--radius-sm);
  }
}
</style>
