<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { InvOrderDetail } from '@/types/api'
import { formatDateTime, formatMoney } from '@/utils/format'
import Pagination from '@/components/Pagination.vue'
const props = defineProps<{ order: InvOrderDetail }>()
const page = ref(1)
const pageSize = ref(5)
const expanded = ref<string[]>([])
watch(
  () => props.order.id,
  () => {
    page.value = 1
    expanded.value = []
  },
)
const history = computed(() => [...(props.order.auditHistory || [])].reverse())
const visibleHistory = computed(() =>
  history.value.slice(
    (page.value - 1) * pageSize.value,
    page.value * pageSize.value,
  ),
)
</script>

<template>
  <section class="form-section order-downstream" aria-label="入库与应付">
    <div class="form-section__head">
      <div class="form-section__title">入库与应付</div>
    </div>
    <div class="order-downstream__row">
      <span>入库单</span>
      <div v-if="order.downstream?.visibility === 'restricted'">
        当前权限无法查看入库单，请联系仓储负责人核实。
      </div>
      <div
        v-else-if="order.downstream?.stockOrders.length"
        class="order-downstream__stocks"
      >
        <div v-for="stock in order.downstream.stockOrders" :key="stock.id">
          <strong>{{ stock.orderNo }}</strong>
          <el-tag
            size="small"
            :type="stock.status === 1 ? 'success' : 'info'"
            >{{ stock.status === 1 ? '已过账' : '已作废' }}</el-tag
          >
          <span>{{
            stock.status === 1 ? '本单入库已增加库存' : '本次入库已撤销'
          }}</span>
        </div>
      </div>
      <div v-else>
        {{
          order.downstream
            ? '当前可见范围内无入库单'
            : '入库关联信息未提供，请刷新或联系管理员'
        }}
      </div>
    </div>
    <div class="order-downstream__row">
      <span>应付</span>
      <div v-if="order.status === 2">
        应付 {{ formatMoney(order.totalAmount) }} · 已付款
        {{ formatMoney(order.settledAmount) }} · 未付款
        {{ formatMoney(order.totalAmount - order.settledAmount) }}
      </div>
      <div v-else>尚未形成应付</div>
    </div>
    <el-collapse v-model="expanded" class="order-audit-history">
      <el-collapse-item
        name="history"
        :title="`审核历史（${history.length} 条）`"
      >
        <p v-if="!history.length">
          {{
            order.status === 2 || order.reversedAt
              ? '历史数据未记录'
              : '暂无审核记录；历史数据如有缺失，不作补录推断。'
          }}
        </p>
        <el-table v-else :data="visibleHistory" size="small">
          <el-table-column label="操作" width="100"
            ><template #default="{ row }">{{
              row.action === 'audit' ? '审核' : '反审核'
            }}</template></el-table-column
          >
          <el-table-column label="操作人" min-width="120"
            ><template #default="{ row }"
              >{{ row.actorName || `账号 #${row.actorId}`
              }}<el-tag
                v-if="row.action === 'audit' && row.actorId === order.createdBy"
                size="small"
                type="info"
                >自审</el-tag
              ></template
            ></el-table-column
          >
          <el-table-column label="时间（本地）" min-width="190"
            ><template #default="{ row }">{{
              formatDateTime(row.occurredAt)
            }}</template></el-table-column
          >
        </el-table>
        <Pagination
          v-if="history.length > 5"
          v-model:page="page"
          v-model:page-size="pageSize"
          :total="history.length"
        />
      </el-collapse-item>
    </el-collapse>
  </section>
</template>

<style scoped lang="scss">
.order-downstream {
  &__row {
    display: grid;
    grid-template-columns: 64px minmax(0, 1fr);
    gap: 12px;
    margin-bottom: 12px;
    font-size: 13px;
    line-height: 1.8;
    > span {
      color: var(--app-text-secondary);
    }
  }
  &__stocks > div {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
    strong {
      font-weight: 500;
      overflow-wrap: anywhere;
    }
    span:last-child {
      color: var(--app-text-secondary);
      font-size: 12px;
    }
  }
}
.order-audit-history {
  margin-top: 16px;
  p {
    color: var(--app-text-secondary);
  }
}
</style>
