/**
 * 阅读设置 + 作品主题 → 正文排版的 CSS 变量（作用于 .work-root，见 styles/reading.css）。
 *
 * 优先级：平台默认 < 作品主题（theme.yaml 的 fonts / reading）< 玩家阅读设置。
 * 阅读设置总有取值，没法区分"没动过"与"选了默认值"：字体与缩进有"跟随作品"选项；行距、段距、版心宽度
 * 与平台默认值相同时视为没动过，采用作品主题的值。玩家关闭"使用作品主题"时不传入 theme。
 */
import { useMemo, type CSSProperties } from 'react'

import type { ReadingSettings, ThemeReading } from '@/api/types'

/** 平台默认的阅读设置（与 internal/settings 的默认值一致）。 */
const DEFAULTS = { line_height: 1.9, paragraph_spacing: 0.9, max_width: 40 }

function fontVar(font: string | undefined, fallback: string): string {
  if (font === 'sans') return 'var(--font-ui)'
  if (font === 'serif' || !font) return fallback
  return `"${font}", ${fallback}`
}

export function useReadingStyle(r?: ReadingSettings, theme?: ThemeReading): CSSProperties {
  return useMemo(() => {
    const style: Record<string, string> = {}
    if (theme) {
      style['--reading-font'] = fontVar(theme.font, 'var(--font-book)')
      style['--reading-heading-font'] = fontVar(theme.heading_font, 'var(--reading-font)')
      if (theme.max_width) style['--reading-width'] = theme.max_width
      if (theme.line_height) style['--reading-leading'] = String(theme.line_height)
      if (theme.paragraph_spacing) style['--reading-gap'] = theme.paragraph_spacing
      if (theme.indent) style['--reading-indent'] = theme.indent
    }
    if (!r) return style as CSSProperties
    if (r.font_family !== 'work') style['--reading-font'] = r.font_family === 'sans' ? 'var(--font-ui)' : 'var(--font-book)'
    style['--reading-size'] = `${r.font_size}px`
    const custom = (k: keyof typeof DEFAULTS) => !theme || r[k] !== DEFAULTS[k]
    if (custom('line_height')) style['--reading-leading'] = String(r.line_height)
    if (custom('paragraph_spacing')) style['--reading-gap'] = `${r.paragraph_spacing}em`
    if (custom('max_width')) style['--reading-width'] = `${r.max_width}em`
    if (r.indent !== 'work' || !theme?.indent) style['--reading-indent'] = r.indent === 'off' ? '0' : '2em'
    return style as CSSProperties
  }, [r, theme])
}
