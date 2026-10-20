/**
 * 阅读设置 → 正文排版的 CSS 变量（作用于 .work-root，见 styles/reading.css）。
 * 优先级：平台默认 < 作品主题（TODO(M2-9)：theme.yaml）< 玩家阅读设置。
 */
import { useMemo, type CSSProperties } from 'react'

import type { ReadingSettings } from '@/api/types'

export function useReadingStyle(r?: ReadingSettings): CSSProperties {
  return useMemo(() => {
    if (!r) return {}
    const font = r.font_family === 'sans' ? 'var(--font-ui)' : 'var(--font-book)'
    return {
      '--reading-font': font,
      '--reading-size': `${r.font_size}px`,
      '--reading-leading': String(r.line_height),
      '--reading-gap': `${r.paragraph_spacing}em`,
      '--reading-width': `${r.max_width}em`,
      '--reading-indent': r.indent === 'off' ? '0' : '2em',
    } as CSSProperties
  }, [r])
}
