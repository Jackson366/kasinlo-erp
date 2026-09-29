import type { Component } from 'vue'
import {
  ArrowDown,
  Avatar,
  Bell,
  CircleCheck,
  CircleClose,
  Close,
  DataLine,
  Delete,
  Document,
  Download,
  Edit,
  Expand,
  Files,
  Fold,
  FullScreen,
  Grid,
  HomeFilled,
  InfoFilled,
  Key,
  List,
  Lock,
  Menu as MenuIcon,
  Monitor,
  More,
  Moon,
  OfficeBuilding,
  Odometer,
  PieChart,
  Plus,
  Postcard,
  Refresh,
  Search,
  Setting,
  Star,
  Sunny,
  SwitchButton,
  Timer,
  Tools,
  TrendCharts,
  Upload,
  User,
  UserFilled,
  Warning,
} from '@element-plus/icons-vue'

/**
 * 图标注册表：菜单里配置的 icon 名称需要在此登记。
 * 采用显式登记而不是全量导入，可显著减小打包体积。
 * 新增菜单图标时，在这里补一行 import 与映射即可。
 */
export const iconMap: Record<string, Component> = {
  // 业务图标（与后端种子菜单一一对应）
  Odometer,
  Setting,
  User,
  Key,
  Menu: MenuIcon,
  OfficeBuilding,
  Document,
  // 通用 UI 图标
  DataLine,
  TrendCharts,
  PieChart,
  Grid,
  List,
  Files,
  Postcard,
  Timer,
  Tools,
  Monitor,
  Star,
  Bell,
  Avatar,
  UserFilled,
  Lock,
  Search,
  Refresh,
  Plus,
  Edit,
  Delete,
  More,
  Folder: Files,
  Download,
  Upload,
  Fold,
  Expand,
  FullScreen,
  ArrowDown,
  Close,
  CircleCheck,
  CircleClose,
  InfoFilled,
  Warning,
  SwitchButton,
  HomeFilled,
  // 主题切换
  Sunny,
  Moon,
}

/** 菜单未配置图标或图标未登记时的兜底图标 */
export const fallbackIcon: Component = MenuIcon

/** 按名称取图标组件 */
export function getIcon(name?: string): Component | null {
  if (!name) return null
  return iconMap[name] || null
}
