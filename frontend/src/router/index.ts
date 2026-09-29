import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

/** 布局容器 */
const Layout = () => import('@/layout/index.vue')

/**
 * 静态路由：所有登录用户都可见的基础页面。
 * 业务页面由后端菜单动态下发，见 store/permission.ts。
 * 采用 hash 模式，部署时无需后端配置 history fallback。
 *
 * 注意：这里的路由**一律不要写 redirect**。
 * vue-router 会在路由守卫执行之前先解析 redirect，
 * 一旦根路径或兜底路由带了 redirect，导航会被提前改写，
 * 守卫里「注入动态路由」的逻辑就永远没有机会运行。
 * 落地页的决策统一交给 guard.ts。
 */
export const constantRoutes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/login/index.vue'),
    meta: { title: '登录', hidden: true },
  },
  {
    path: '/',
    name: 'Root',
    component: Layout,
    children: [
      {
        path: '/profile',
        name: 'Profile',
        component: () => import('@/views/profile/index.vue'),
        meta: { title: '个人中心', icon: 'User', hidden: true, keepAlive: false },
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/views/error/404.vue'),
    meta: { title: '页面不存在', hidden: true },
  },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes: constantRoutes,
  scrollBehavior: () => ({ top: 0 }),
})

export default router

/** 动态路由统一挂载到 Root 之下 */
export const ROOT_ROUTE_NAME = 'Root'

/** 无需登录即可访问的路径 */
export const WHITE_LIST = ['/login', '/404']
