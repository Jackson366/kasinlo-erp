import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'

/**
 * 独立于 vite.config.ts 的测试配置。
 *
 * 刻意不加载 vue / unplugin-vue-components 插件：后者在运行时会把
 * src/components.d.ts 重写一遍，跑测试不该让工作区变脏。当前用例只覆盖
 * 纯逻辑（工具函数、路由辅助函数），不需要这些插件。
 */
export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.spec.ts'],
  },
})
