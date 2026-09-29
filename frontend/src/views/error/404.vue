<script setup lang="ts">
import { useRouter } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'

defineOptions({ name: 'NotFound' })

const router = useRouter()
</script>

<template>
  <div class="not-found">
    <div class="not-found__code">404</div>
    <h2 class="not-found__title">页面不存在</h2>
    <p class="not-found__desc text-secondary">您访问的页面已被移除，或地址输入有误。</p>
    <el-button type="primary" @click="router.replace('/')">
      <AppIcon name="HomeFilled" />
      <span>返回首页</span>
    </el-button>
  </div>
</template>

<style scoped lang="scss">
.not-found {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 420px;
  gap: 10px;

  &__code {
    font-size: 84px;
    font-weight: 800;
    line-height: 1;
    letter-spacing: 4px;
    color: transparent;
    background: var(--app-gradient-text);
    background-clip: text;
    -webkit-background-clip: text;
  }

  &__title {
    margin: 12px 0 0;
    font-size: 18px;
    color: var(--app-text);
  }

  &__desc {
    margin: 0 0 12px;
    font-size: 13px;
  }
}
</style>
