/**
 * 展示用的格式化函数。
 */
import i18n from '@/i18n'
import type { Save } from '@/api/types'

const pad = (n: number) => String(n).padStart(2, '0')

/** "10月16日 21:30"；跨年时带年份（《02-技术方案》6.5）。 */
export function formatDateTime(ms: number): string {
  const d = new Date(ms)
  const now = new Date()
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const md = `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`
  return d.getFullYear() === now.getFullYear() ? md : `${d.getFullYear()}年${md}`
}

/** 相对时间："刚刚""5 分钟前""昨天 21:30"，更早则显示日期。 */
export function formatRelative(ms: number): string {
  const diff = Date.now() - ms
  const t = i18n.t.bind(i18n)
  if (diff < 60_000) return t('time.justNow')
  if (diff < 3_600_000) return t('time.minutesAgo', { n: Math.floor(diff / 60_000) })
  const d = new Date(ms)
  const today = new Date()
  if (diff < 86_400_000 && d.getDate() === today.getDate()) return t('time.hoursAgo', { n: Math.floor(diff / 3_600_000) })
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1)
  if (d >= yesterday && d.getDate() === yesterday.getDate()) {
    return t('time.yesterday', { time: `${pad(d.getHours())}:${pad(d.getMinutes())}` })
  }
  return formatDateTime(ms)
}

/** 存档标题：手动或生成的标题优先，否则"未命名存档 · 最近游玩时间"（该文案只在界面生成，不入库）。 */
export function saveTitle(s: Pick<Save, 'title' | 'last_played_at' | 'created_at'>): string {
  if (s.title) return s.title
  return i18n.t('work.untitledSave', { time: formatDateTime(s.last_played_at || s.created_at) })
}

/** 13.4k、128k 这样的 token 数。 */
export function formatTokens(n: number): string {
  if (n < 1000) return String(n)
  const k = n / 1000
  return `${k >= 100 ? Math.round(k) : k.toFixed(1).replace(/\.0$/, '')}k`
}

/** 文件大小："812 B""12.3 KB""1.2 MB"（按 1024 进位，与系统文件管理器一致）。 */
export function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let u = 0
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024
    u++
  }
  if (u === 0) return `${v} B`
  return `${v >= 100 || Number.isInteger(v) ? Math.round(v) : v.toFixed(1)} ${units[u]}`
}

/** 从字符串得到稳定的非负整数（用于按作品 ID 选择封面颜色等）。 */
export function hashString(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}
