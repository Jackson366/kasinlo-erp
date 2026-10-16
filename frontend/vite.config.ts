import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'

// 无独立 barrel 的子组件 → 父组件目录映射（父 barrel 均导出子组件）
const EP_PARENT: Record<string, string> = {
  'table-column': 'table',
  'form-item': 'form',
  option: 'select',
  'option-group': 'select',
  'menu-item': 'menu',
  'menu-item-group': 'menu',
  'sub-menu': 'menu',
  'tab-pane': 'tabs',
  'radio-group': 'radio',
  'radio-button': 'radio',
  'checkbox-group': 'checkbox',
  'checkbox-button': 'checkbox',
  'breadcrumb-item': 'breadcrumb',
  'dropdown-menu': 'dropdown',
  'dropdown-item': 'dropdown',
  // descriptions-item 没有独立目录（node_modules 里只有 descriptions），
  // 漏了这条会让 `el-descriptions-item` 解析到一个不存在的模块、构建直接失败。
  // 上游没踩到是因为它的仪表盘不用 descriptions，而进销存的单据详情用了。
  'descriptions-item': 'descriptions',
  aside: 'container',
  header: 'container',
  main: 'container',
  footer: 'container',
  'carousel-item': 'carousel',
  'collapse-item': 'collapse',
  'timeline-item': 'timeline',
  step: 'steps',
}

function epKebab(s: string): string {
  return s.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase()
}

// element-plus 按组件路径导入解析器。
// 官方 ElementPlusResolver 对 2.x 生成 `import { ElXxx } from 'element-plus/es'`，
// 而根入口顶层执行 makeInstaller([...全部组件])——这个副作用调用让 rollup
// 无法摇树，整个组件库（约 950KB）全部进包。按子路径导入彻底绕开根入口。
function ElementPlusDirectResolver() {
  return {
    type: 'component' as const,
    resolve: (rawName: string) => {
      if (!/^El[A-Z]/.test(rawName)) return undefined
      const partial = epKebab(rawName.slice(2))
      const barrel = EP_PARENT[partial] ?? partial
      return {
        name: rawName,
        from: `element-plus/es/components/${barrel}/index`,
        sideEffects: `element-plus/es/components/${barrel}/style/css`,
      }
    },
  }
}

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
    plugins: [
      vue(),
      // element-plus 按需引入：模板里的 el-* 组件及其样式按实际使用注入。
      // 不走模板的三样（ElMessage / ElMessageBox / v-loading 指令）由
      // main.ts 手工注册并引样式；tsconfig 的 element-plus/global 继续负责模板类型。
      Components({
        resolvers: [ElementPlusDirectResolver()],
        dts: 'src/components.d.ts',
      }),
    ],
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
          // 拆包：把体积大且变动少的库单独分出来，提升缓存命中率。
          // 必须用函数式而不是对象式——对象式按包名把入口整个拉进模块图，
          // 会禁用 tree-shaking，按需引入就白做了。
          manualChunks(id: string): string | undefined {
            if (!id.includes('node_modules')) return undefined
            if (id.includes('element-plus') || id.includes('@element-plus')) return 'element'
            if (id.includes('echarts') || id.includes('zrender')) return 'echarts'
            if (id.includes('@vue') || id.includes('vue-router') || id.includes('/pinia/') || /\/vue\//.test(id)) {
              return 'vue'
            }
            return undefined
          },
        },
      },
    },
  }
})
