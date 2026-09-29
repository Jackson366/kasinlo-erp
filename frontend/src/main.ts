import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
// Element Plus 官方暗色变量（MIT），由 <html class="dark"> 激活。
// 必须紧跟亮色样式之后、项目样式之前引入，方便我们的覆写在同特异度下胜出。
import 'element-plus/theme-chalk/dark/css-vars.css'

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
app.use(ElementPlus, { locale: zhCn, size: 'default' })

// 路由守卫依赖 Pinia，必须在 createPinia 之后安装
setupRouterGuard(router)
app.use(router)

setupPermissionDirective(app)

app.mount('#app')
