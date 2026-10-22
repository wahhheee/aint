import { describe, expect, it } from 'vitest'

import { tooltipText, workTagCSS } from './workStyles'

describe('workStyles', () => {
  it('悬停提示中的 {{ text }} 换成标签内的文字', () => {
    const children = [
      { type: 'text' as const, text: '老陈' },
      { type: 'em' as const, children: [{ type: 'text' as const, text: '从未' }] },
    ]
    expect(tooltipText('封印：{{ text }}（{{text}}）', children)).toBe('封印：老陈从未（老陈从未）')
  })

  it('深色颜色优先用 dark_color', () => {
    const css = workTagCSS([{ name: 'gold', description: '', style: { color: '#a8770e', dark_color: '#f2c14e' } }])
    expect(css).toContain('.dark .work-root .mk-gold{color:#f2c14e}')
  })
})
