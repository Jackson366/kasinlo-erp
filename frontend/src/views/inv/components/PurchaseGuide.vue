<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { usePurchaseGuideStore } from '@/store/purchaseGuide'

const guide = usePurchaseGuideStore()
const toggle = ref<{ $el: HTMLButtonElement }>()
async function change(active: boolean): Promise<void> {
  const persisted = guide.setMode(active ? 'active' : 'closed')
  if (!active)
    ElMessage.info('已退出引导，当前输入仍保留。可从「采购操作指南」重新开启。')
  if (!persisted)
    ElMessage.warning('本次选择已生效，但浏览器未能记住，下次进入可能再次显示。')
  await nextTick()
  toggle.value?.$el.focus()
}
</script>

<template>
  <section class="purchase-guide" aria-label="采购操作指南">
    <div class="purchase-guide__controls">
      <strong v-if="guide.mode !== 'closed'">采购操作指南</strong>
      <el-button
        ref="toggle"
        :aria-expanded="guide.mode === 'active'"
        @click="change(guide.mode !== 'active')"
      >
        {{ guide.mode === 'active' ? '退出引导' : guide.mode === 'offered' ? '开始引导' : '采购操作指南' }}
      </el-button>
      <el-button v-if="guide.mode === 'offered'" @click="change(false)">直接办理</el-button>
    </div>
    <p v-if="guide.mode === 'offered'">可先了解录单、入库与付款的区别，也可直接办理，稍后随时打开指南。</p>
    <div v-if="guide.mode === 'active'" class="purchase-guide__content">
      <p>选择供应商和商品，填写数量与单价，再根据实际到货与付款情况办理。</p>
      <ol aria-label="采购办理顺序">
        <li><strong>录入采购</strong><span>保存单据，尚未增加库存</span></li>
        <li><strong>核对并入库</strong><span>确认收货后审核，增加库存并形成应付</span></li>
        <li><strong>登记付款</strong><span>按实际付款情况结算，减少未付金额</span></li>
      </ol>
      <p>可随时退出，继续当前单据；重新开启只显示说明，不会重置录入内容或执行任何业务操作。</p>
    </div>
  </section>
</template>

<style scoped lang="scss">
.purchase-guide {
  margin-bottom: 16px;
  &__controls {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }
  &__controls > strong { margin-right: auto; }
  :deep(.el-button + .el-button) { margin-left: 0; }
  p {
    margin: 8px 0 0;
    color: var(--app-text-secondary);
    font-size: 13px;
    line-height: 1.6;
  }
  ol {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 24px;
    padding: 16px 0 0;
    margin: 16px 0 0;
    list-style-position: inside;
    border-top: 1px solid var(--app-border-light);
  }
  li {
    font-size: 13px;
    line-height: 1.6;
    color: var(--app-text-secondary);
  }
  strong { color: var(--app-text); font-weight: 600; }
  li span { display: block; margin-top: 4px; }
}
@media (max-width: 800px) {
  .purchase-guide ol { grid-template-columns: 1fr; gap: 12px; }
}
</style>
