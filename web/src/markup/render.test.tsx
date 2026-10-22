import '@/i18n'

import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MarkupProvider, type MarkupContextValue } from './context'
import { Markup } from './render'

const ctx: MarkupContextValue = {
  tags: new Set(['say', 'ooc']),
  workTags: new Map(),
  characters: new Map(),
  assets: new Map(),
}

const view = (source: string, drafting: boolean) => (
  <MarkupProvider value={ctx}>
    <Markup source={source} drafting={drafting} />
  </MarkupProvider>
)

describe('Markup', () => {
  it('流式中逐帧变长：已完成的段落保持不变，光标只在最后一段', () => {
    const { container, rerender } = render(view('雨从傍晚开始下。\n<say who="老陈">「坐。', true))
    const first = container.querySelector('p')
    rerender(view('雨从傍晚开始下。\n<say who="老陈">「坐。」</say>\n<ooc>好的', true))

    const paragraphs = container.querySelectorAll('p')
    expect(container.querySelector('p')).toBe(first)
    expect([...paragraphs].map((p) => p.textContent)).toEqual(['雨从傍晚开始下。', '老陈「坐。」', '好的'])
    expect([...container.querySelectorAll('.is-drafting')].map((e) => e.textContent)).toEqual(['好的'])
    expect(container.querySelector('.mk-ooc-block')).not.toBeNull()

    rerender(view('雨从傍晚开始下。\n<say who="老陈">「坐。」</say>\n<ooc>好的</ooc>', false))
    expect(container.querySelectorAll('.is-drafting')).toHaveLength(0)
  })
})
