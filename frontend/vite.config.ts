import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

// Vite 配置：开发环境通过代理转发 /api 到后端，避免跨域问题
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_')
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080'
  // 开发与本地预览（npm run preview）共用同一份代理配置，
  // 方便直接用生产产物做一次端到端冒烟验证。
  // /swagger 一并转发：Swagger UI 挂在后端端口上，转发后 5173 也能直接打开在线文档。
  const proxy = {
    '/api': {
      target: proxyTarget,
      changeOrigin: true,
    },
    '/swagger': {
      target: proxyTarget,
      changeOrigin: true,
    },
  }

  return {
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: '0.0.0.0',
      port: Number(env.VITE_PORT || 5173),
      open: false,
      proxy,
    },
    preview: {
      host: '0.0.0.0',
      port: Number(env.VITE_PREVIEW_PORT || 4173),
      proxy,
    },
    build: {
      outDir: 'dist',
      sourcemap: false,
      chunkSizeWarningLimit: 1500,
      rollupOptions: {
        output: {
          // 拆包：把体积大且变动少的库单独分出来，提升缓存命中率
          manualChunks: {
            vue: ['vue', 'vue-router', 'pinia'],
            element: ['element-plus', '@element-plus/icons-vue'],
            echarts: ['echarts'],
          },
        },
      },
    },
  }
})
