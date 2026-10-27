<script setup lang="ts">
import type { InvOrderDetail, InvOrderItem, InvProductPick } from '@/types/api'
import {
  formatDay,
  formatMoney,
  formatQty,
  formatDateTime,
} from '@/utils/format'
import OrderAuditTrail from './OrderAuditTrail.vue'
import { statusMeta, settleMeta } from './orderPresentation'

const props = defineProps<{
  order: InvOrderDetail
  items: InvOrderItem[]
  partyLabel: string
  isPurchase: boolean
  picks: InvProductPick[]
}>()

function baseUnitNameOf(productId: number, specId: number): string {
  const spec = props.picks
    .find((p) => p.id === productId)
    ?.specs.find((s) => s.id === specId)
  return spec?.units.find((u) => u.isBase)?.unit ?? ''
}
function detailSummary(param: { columns: unknown[] }): string[] {
  return param.columns.map((_, i) =>
    i === 0
      ? '合计'
      : i === param.columns.length - 1
        ? formatMoney(props.order.totalAmount)
        : '',
  )
}
</script>

<template>
  <div class="order-detail-content">
    <!-- 概览条：单号与两个状态在左，金额与结算情况在右 ——
             这三项是看单据时最先要确认的东西，不该埋在描述表里 -->
    <div class="detail-head">
      <div class="detail-head__main">
        <span class="detail-head__no">{{ order.orderNo }}</span>
        <el-tag
          :type="statusMeta[order.status]?.type"
          size="small"
          effect="light"
        >
          {{ statusMeta[order.status]?.text }}
        </el-tag>
        <el-tag
          v-if="!isPurchase || order.status === 2"
          :type="settleMeta[order.settleStatus]?.type"
          size="small"
          effect="light"
        >
          {{ settleMeta[order.settleStatus]?.text }}
        </el-tag>
      </div>
      <div class="detail-head__amount">
        <span class="detail-head__label">单据金额</span>
        <strong>{{ formatMoney(order.totalAmount) }}</strong>
        <span v-if="!isPurchase || order.status === 2" class="detail-head__sub">
          已结算 {{ formatMoney(order.settledAmount) }} · 未结
          {{ formatMoney(order.totalAmount - order.settledAmount) }}
        </span>
      </div>
    </div>

    <p v-if="isPurchase" class="audit-summary">
      {{ order.status === 2 ? '审核' : '上次审核' }}：
      <template v-if="order.auditedAt"
        >{{ order.auditorName || `账号 #${order.auditorId}` }} ·
        {{ formatDateTime(order.auditedAt) }}（本地时间）</template
      >
      <template v-else>{{
        order.status === 2 || order.reversedAt ? '历史数据未记录' : '尚未审核'
      }}</template>
      <el-tag v-if="order.selfAudit" size="small" type="info">自审</el-tag>
      <span v-if="order.reversedAt"
        >上次反审核：{{ order.reversedByName || `账号 #${order.reversedBy}` }} ·
        {{ formatDateTime(order.reversedAt) }}（本地时间）</span
      >
    </p>
    <section class="form-section">
      <div class="form-section__head">
        <div class="form-section__title">基本信息</div>
      </div>
      <div class="detail-grid">
        <div class="detail-field">
          <span class="detail-field__label">{{ partyLabel }}</span>
          <div class="detail-field__value">{{ order.partyName }}</div>
        </div>
        <div class="detail-field">
          <span class="detail-field__label">单据日期</span>
          <div class="detail-field__value">
            {{ formatDay(order.orderDate) }}
          </div>
        </div>
        <div class="detail-field">
          <span class="detail-field__label">{{
            isPurchase ? '付款到期日' : '到期日'
          }}</span>
          <div class="detail-field__value">
            {{
              formatDay(order.dueDate) === '-' && isPurchase
                ? '未设付款期限'
                : formatDay(order.dueDate)
            }}
          </div>
        </div>
        <div class="detail-field">
          <span class="detail-field__label">业务员</span>
          <div class="detail-field__value">{{ order.salesman || '-' }}</div>
        </div>
        <div class="detail-field">
          <span class="detail-field__label">操作人员</span>
          <div class="detail-field__value">{{ order.operator || '-' }}</div>
        </div>
        <div class="detail-field">
          <span class="detail-field__label">备注</span>
          <div class="detail-field__value">{{ order.remark || '-' }}</div>
        </div>
      </div>
    </section>

    <section class="form-section">
      <div class="form-section__head">
        <div class="form-section__title">
          单据明细
          <small>共 {{ items.length }} 行</small>
        </div>
      </div>
      <!-- 合计做成表格的合计行而不是表格外面的一行文字：
               金额与「金额」列自然对齐，扫一眼就知道总数对应哪一列 -->
      <el-table
        :data="items"
        size="small"
        show-summary
        :summary-method="detailSummary"
        class="detail-items"
      >
        <el-table-column type="index" label="#" width="52" align="center" />
        <el-table-column
          prop="productName"
          label="商品"
          min-width="150"
          show-overflow-tooltip
        />
        <el-table-column
          prop="spec"
          label="规格"
          width="120"
          show-overflow-tooltip
        />
        <el-table-column label="数量" width="120" align="right">
          <template #default="{ row }"
            >{{ formatQty(row.qty) }} {{ row.unit }}</template
          >
        </el-table-column>
        <el-table-column label="折算基本数量" width="150" align="right">
          <template #default="{ row }">
            <span v-if="row.rate === 1" class="text-muted">—</span>
            <span v-else>
              ×{{ row.rate }} = {{ formatQty(row.qtyBase) }}
              {{ baseUnitNameOf(row.productId, row.specId) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="单价" width="110" align="right">
          <template #default="{ row }">{{ formatMoney(row.price) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="120" align="right">
          <template #default="{ row }">
            <strong>{{ formatMoney(row.amount) }}</strong>
          </template>
        </el-table-column>
      </el-table>
    </section>
    <OrderAuditTrail v-if="isPurchase" :order="order" />
  </div>
</template>

<style scoped lang="scss">
.audit-summary {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
  color: var(--app-text-secondary);
  font-size: 12px;
  line-height: 1.8;
  margin: 0 0 20px;
}
.detail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
  margin-bottom: 18px;
  background: var(--app-surface-subtle);
  border-radius: var(--radius-md);

  &__main {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  &__no {
    font-family: ui-monospace, monospace;
    font-size: 15px;
    font-weight: 600;
    color: var(--app-text);
  }

  &__amount {
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }

  &__label {
    font-size: 12px;
    color: var(--app-text-secondary);
  }

  &__amount strong {
    font-size: 20px;
    font-weight: 600;
    color: var(--app-text);
    font-variant-numeric: tabular-nums;
  }

  &__sub {
    font-size: 12px;
    color: var(--app-text-secondary);
  }
}

// 详情字段：三列等宽，标签在上、值在下 ——
// 值的长短差别很大（单号 / 备注），左对齐的标签在右会让右侧全空。
.detail-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px 20px;
}

.detail-field {
  min-width: 0;

  &__label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--app-text-secondary);
  }

  &__value {
    font-size: 13px;
    color: var(--app-text);
    word-break: break-word;
  }
}

@media (max-width: 1200px) {
  .detail-head {
    align-items: flex-start;
    flex-direction: column;
  }
  .detail-head__main {
    flex-wrap: wrap;
  }
  .detail-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 540px) {
  .detail-head__amount {
    flex-wrap: wrap;
  }
  .detail-head__sub {
    flex-basis: 100%;
  }
  .detail-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
