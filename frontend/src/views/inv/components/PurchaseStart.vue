<script setup lang="ts">
import { computed, onActivated, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { invApi } from '@/api'
import { useUserStore } from '@/store'
import PurchaseGuide from './PurchaseGuide.vue'

const router = useRouter()
const user = useUserStore()
const allowed = computed(
  () =>
    user.hasAnyPerm(['inv:purchase:list']) && router.hasRoute('InvPurchase'),
)
const loading = ref(false)
const failed = ref(false)
const pending = ref<number | null>(null)
async function load(): Promise<void> {
  if (!allowed.value || loading.value) return
  loading.value = true
  failed.value = false
  try {
    const result = await invApi.orders({
      bizType: 'purchase',
      status: 1,
      page: 1,
      pageSize: 1,
    })
    pending.value = result.total
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}
onMounted(load)
onActivated(load)
</script>

<template>
  <section
    v-if="allowed"
    class="page-card purchase-start"
    aria-labelledby="purchase-start-title"
    :aria-busy="loading"
  >
    <div class="purchase-start__task">
      <div>
        <div class="purchase-start__heading">
          <h2 id="purchase-start-title">
            {{ pending && !failed ? '继续处理采购' : '从一张采购单开始' }}
          </h2>
          <PurchaseGuide />
        </div>
        <p v-if="loading" role="status">正在查看待处理采购单…</p>
        <p v-else-if="failed" role="alert">
          待处理数量加载失败，仍可进入采购工作台。
        </p>
        <p v-else-if="pending">
          有 {{ pending }} 张采购单待核对。取得收货确认依据后，再审核并入库。
        </p>
        <p v-else>先记录采购，再核对到货；付款可单独登记。</p>
      </div>
      <div class="purchase-start__actions">
        <el-button v-if="failed" @click="load">重新加载</el-button>
        <el-button
          v-if="pending || failed"
          type="primary"
          @click="router.push('/inv/purchase')"
          >进入采购工作台</el-button
        >
        <el-button
          v-if="user.hasAnyPerm(['inv:purchase:create'])"
          :type="pending || failed ? 'default' : 'primary'"
          @click="
            router.push({ path: '/inv/purchase', query: { action: 'create' } })
          "
          >新增采购单</el-button
        >
      </div>
    </div>
  </section>
</template>

<style scoped lang="scss">
.purchase-start {
  &__heading {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px 12px;
  }
  &__task {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
  }
  h2 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--app-text);
  }
  p {
    margin: 8px 0 0;
    color: var(--app-text-secondary);
    font-size: 13px;
    line-height: 1.6;
  }
  &__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    flex-shrink: 0;
  }
  &__actions :deep(.el-button + .el-button) {
    margin-left: 0;
  }
}
@media (max-width: 800px) {
  .purchase-start__task {
    align-items: flex-start;
    flex-direction: column;
    gap: 16px;
  }
}
</style>
