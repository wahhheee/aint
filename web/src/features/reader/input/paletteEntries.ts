/** 命令面板的条目：作品命令在前，平台内置命令在后，按输入的前缀过滤。 */
import i18n from '@/i18n'
import type { Presentation } from '@/api/types'

export interface PaletteEntry {
  name: string
  signature: string
  description: string
  icon?: string
  builtin: boolean
}

export function paletteEntries(ui: Presentation['ui'], query: string): PaletteEntry[] {
  const t = i18n.t.bind(i18n)
  const builtin: PaletteEntry[] = [
    ...(ui.features.ooc ? [{ name: 'ooc', signature: '/ooc <文字>', description: t('reader.builtin.ooc'), builtin: true }] : []),
    { name: 'continue', signature: '/continue', description: t('reader.builtin.continue'), builtin: true },
    { name: 'compact', signature: '/compact', description: t('reader.builtin.compact'), builtin: true },
    { name: 'help', signature: '/help', description: t('reader.builtin.help'), builtin: true },
  ]
  const work: PaletteEntry[] = ui.commands
    .filter((c) => c.present.includes('slash') || c.present.includes('button'))
    .map((c) => ({
      name: c.name,
      signature: `/${c.name}${(c.args ?? []).map((a) => (a.required ? ` <${a.hint ?? a.name}>` : ` [${a.hint ?? a.name}]`)).join('')}`,
      description: c.description,
      icon: c.icon,
      builtin: false,
    }))
  const q = query.toLowerCase()
  return [...work, ...builtin].filter((e) => e.name.toLowerCase().startsWith(q))
}
