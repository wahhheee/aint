/**
 * 作品中按名称引用的图标（markup.yaml 的 icon、命令的 icon、面板 ui-stat 的 icon）。
 *
 * 规范约定作品使用 lucide 图标集（名称如 cloud-fog）。常用的一部分随主包提供；其余名字第一次出现时，
 * 按需加载完整图标集——它是一个单独的构建产物（整个 icons/index 一次加载），不会让每个图标各成一个文件。
 * 加载完成前、或名字不在图标集中时，显示一个小圆点。
 */
import {
  Anchor,
  Backpack,
  Book,
  Calendar,
  Castle,
  Circle,
  CloudFog,
  Coins,
  Compass,
  Crown,
  Dices,
  Eye,
  Feather,
  Flame,
  Footprints,
  Gem,
  Ghost,
  Heart,
  Hourglass,
  Info,
  Key,
  Lightbulb,
  Lock,
  type LucideIcon,
  Map,
  MessageCircle,
  Moon,
  Music,
  Quote,
  Scroll,
  Search,
  Shield,
  Ship,
  Skull,
  Sparkles,
  Stamp,
  Star,
  Sun,
  Sword,
  TriangleAlert,
  User,
  Users,
  Zap,
} from 'lucide-react'
import { createElement, useEffect, useState } from 'react'

const ICONS: Record<string, LucideIcon> = {
  anchor: Anchor,
  backpack: Backpack,
  book: Book,
  calendar: Calendar,
  castle: Castle,
  'cloud-fog': CloudFog,
  coins: Coins,
  compass: Compass,
  crown: Crown,
  dices: Dices,
  eye: Eye,
  feather: Feather,
  flame: Flame,
  footprints: Footprints,
  gem: Gem,
  ghost: Ghost,
  heart: Heart,
  hourglass: Hourglass,
  info: Info,
  key: Key,
  lightbulb: Lightbulb,
  lock: Lock,
  map: Map,
  'message-circle': MessageCircle,
  moon: Moon,
  music: Music,
  quote: Quote,
  scroll: Scroll,
  search: Search,
  shield: Shield,
  ship: Ship,
  skull: Skull,
  sparkles: Sparkles,
  stamp: Stamp,
  star: Star,
  sun: Sun,
  sword: Sword,
  'triangle-alert': TriangleAlert,
  user: User,
  users: Users,
  zap: Zap,
}

/** 完整图标集（按需加载一次）：PascalCase 名称 → 图标组件。 */
let fullSet: Record<string, LucideIcon> | undefined
let loading: Promise<void> | undefined

function loadFullSet(): Promise<void> {
  loading ??= import('lucide-react/dist/esm/icons/index.mjs').then((m) => {
    fullSet = m as Record<string, LucideIcon>
  })
  return loading
}

/** cloud-fog → CloudFog（lucide 的导出名） */
function pascal(name: string): string {
  return name.replace(/(^|-)([a-z0-9])/g, (_, __: string, c: string) => c.toUpperCase())
}

function lookup(name: string): LucideIcon | undefined {
  if (ICONS[name]) return ICONS[name]
  if (!fullSet || !/^[a-z][a-z0-9-]*$/.test(name)) return undefined
  const Icon = fullSet[pascal(name)]
  return typeof Icon === 'object' || typeof Icon === 'function' ? Icon : undefined
}

export function WorkIcon({ name, className }: { name?: string; className?: string }) {
  const [, setLoaded] = useState(!!fullSet)
  const Icon = name ? lookup(name) : undefined
  useEffect(() => {
    if (name && !Icon && !fullSet) void loadFullSet().then(() => setLoaded(true))
  }, [name, Icon])
  if (!name) return null
  // 图标组件来自查表（模块级常量或已加载的图标集），不是渲染中新建的组件
  return createElement(Icon ?? Circle, { className, strokeWidth: 1.75, 'aria-hidden': true })
}
