import { createApp } from 'vue'
import { createPinia } from 'pinia'
// 子路径导入：element-plus 根入口会引用全部组件，禁用 tree-shaking
import { ElLoading } from 'element-plus/es/components/loading/index'
// Element Plus 官方暗色变量（MIT），由 <html class="dark"> 激活。
// 必须紧跟亮色样式之后、项目样式之前引入，方便我们的覆写在同特异度下胜出。
import 'element-plus/theme-chalk/dark/css-vars.css'
// 组件样式由 unplugin-vue-components 按需注入。下面三样不走模板解析，
// 需要手工引样式，否则 ElMessage / v-loading 渲染成无样式 DOM：
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/loading/style/css'

import App from './App.vue'
import router from './router'
import { setupRouterGuard } from './router/guard'
import { setupPermissionDirective } from './directives/permission'
import { applyTheme, resolveInitialTheme } from './utils/theme'
import './styles/index.scss'

// 主题预处理（与 index.html 的内联脚本等价，此处为幂等兜底）
applyTheme(resolveInitialTheme())

const app = createApp(App)

app.use(createPinia())
// v-loading 指令：完整引入时由 app.use(ElementPlus) 顺带注册，按需引入后自己装
app.use(ElLoading)

// 路由守卫依赖 Pinia，必须在 createPinia 之后安装
setupRouterGuard(router)
app.use(router)

setupPermissionDirective(app)

app.mount('#app')
