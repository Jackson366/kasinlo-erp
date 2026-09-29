/**
 * 主题（亮色 / 暗色）的唯一数据源与副作用入口。
 *
 * 约定：**`<html>` 上是否带 `dark` 类 = 当前是否暗色主题**。
 * - 自定义样式：`styles/variables.scss` 里 `html.dark` 覆盖全部 `--app-*` 令牌
 * - Element Plus：官方 `dark/css-vars.css` 同样以 `html.dark` 激活
 *   （`element.scss` 再对齐一层取色，见该文件顶部说明）
 * - 首屏：`index.html` 内联脚本在样式生效前就写好这个类，避免闪白
 *
 * 因此切换主题**不需要**任何组件参与样式计算，只要改这个类即可。
 */
export type ThemeMode = 'light' | 'dark'

export const THEME_STORAGE_KEY = 'nova_theme'

/** 读取初始主题：优先本地已保存的选择，其次跟随系统偏好 */
export function resolveInitialTheme(): ThemeMode {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') return stored
  } catch {
    // localStorage 不可用（隐私模式等）时退化为跟随系统
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

/** 把主题作用到 `<html>`，幂等，可重复调用 */
export function applyTheme(mode: ThemeMode): void {
  const root = document.documentElement
  root.classList.toggle('dark', mode === 'dark')
  // 同时写一个属性，方便调试与将来支持「跟随系统」时的样式钩子
  root.dataset.theme = mode
}

/** 记住用户显式选择过的主题 */
export function persistTheme(mode: ThemeMode): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, mode)
  } catch {
    // 存不了就算了，不影响本次会话
  }
}

/**
 * 读取当前主题下的 CSS 变量。
 * 用于 ECharts / Canvas 这类**无法解析 `var()`** 的消费方：
 * 它们需要在 JS 里拿到具体色值，所以在主题切换后重新读取一次。
 */
export function readToken(name: string, fallback = ''): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name)
  return value.trim() || fallback
}

/** 图表调色板：把主题相关的色值一次读全，页面放进 computed 即可随主题重绘 */
export interface ChartPalette {
  /** 次级文字（轴刻度） */
  text: string
  /** 主文字（图例、类目名） */
  textStrong: string
  /** 轴线 */
  axis: string
  /** 网格分割线 */
  split: string
  /** 品牌主色（折线、主柱） */
  primary: string
  /** 语义绿（对比折线） */
  success: string
  /** 面积渐变起点 / 终点 */
  areaFrom: string
  areaTo: string
  /** 横向柱状渐变起点 */
  barFrom: string
  /** tooltip 底色与描边 */
  overlay: string
  border: string
}

export function chartPalette(): ChartPalette {
  return {
    text: readToken('--app-text-muted', '#9ca3af'),
    textStrong: readToken('--app-text-secondary', '#6b7280'),
    axis: readToken('--app-border', '#e5e7eb'),
    split: readToken('--app-border-light', '#f1f5f9'),
    primary: readToken('--brand-primary', '#2563eb'),
    success: readToken('--app-success', '#16a34a'),
    areaFrom: readToken('--app-chart-area-from', 'rgba(37, 99, 235, 0.22)'),
    areaTo: readToken('--app-chart-area-to', 'rgba(37, 99, 235, 0.01)'),
    barFrom: readToken('--app-chart-bar-from', '#93b4f8'),
    overlay: readToken('--app-surface-overlay', '#ffffff'),
    border: readToken('--app-border-light', '#eef1f6'),
  }
}
