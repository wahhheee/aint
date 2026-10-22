import '@/i18n'

import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AssetRef } from '@/api/types'
import { MarkupProvider, type MarkupContextValue } from '@/markup/context'

import { PanelHTML } from './PanelHTML'
import { sanitizeStyle } from './sanitize'

const lin: AssetRef = { id: 'lin', path: 'assets/characters/lin.webp', kind: 'image', role: 'reference', url: '/api/v1/assets/abc' }
const ctx: MarkupContextValue = {
  tags: new Set(),
  workTags: new Map(),
  characters: new Map(),
  assets: new Map([
    ['lin', lin],
    [lin.path, lin],
  ]),
}

const view = (html: string) =>
  render(
    <MarkupProvider value={ctx}>
      <PanelHTML html={html} />
    </MarkupProvider>,
  ).container

// 作品不执行（不变量）：面板 HTML 来自作者模板，净化后不能留下脚本、事件、外部资源。
describe('PanelHTML 净化', () => {
  it('去掉脚本、事件属性、iframe、表单与外链', () => {
    const c = view(
      `<p onclick="alert(1)">文字</p><script>alert(1)</script><iframe src="https://x"></iframe>` +
        `<form><input></form><a href="https://evil">链接</a><img src="x" onerror="alert(1)">`,
    )
    expect(c.innerHTML).not.toMatch(/script|onclick|onerror|iframe|form|input|href|evil/)
    expect(c.textContent).toContain('文字')
    expect(c.textContent).toContain('链接')
    expect(c.querySelector('img')).toBeNull()
  })

  it('图片只接受 asset: 方案，映射为素材地址', () => {
    const c = view(
      `<img src="asset:lin" alt="林舟"><img src="asset:assets/characters/lin.webp"><img src="https://x/y.png"><img src="/api/v1/x"><img src="asset:nobody">`,
    )
    const imgs = [...c.querySelectorAll('img')]
    expect(imgs.map((i) => i.getAttribute('src'))).toEqual(['/api/v1/assets/abc', '/api/v1/assets/abc'])
    expect(imgs[0].getAttribute('alt')).toBe('林舟')
  })

  it('style 只保留排版属性', () => {
    const c = view(`<span style="color: #b8860b; position: fixed; background: url(https://x); font-weight: bold">金</span>`)
    const style = c.querySelector('span')?.getAttribute('style') ?? ''
    expect(style).toContain('color')
    expect(style).toContain('font-weight')
    expect(style).not.toMatch(/position|url|background/)
  })

  it('ui-image 按素材引用显示图片，找不到时显示说明文字', () => {
    const c = view(`<ui-image src="lin" alt="林舟" size="sm"></ui-image><ui-image src="nobody" alt="不存在的图"></ui-image>`)
    expect(c.querySelector('img')?.getAttribute('src')).toBe('/api/v1/assets/abc')
    expect(c.textContent).toContain('不存在的图')
  })
})

describe('sanitizeStyle', () => {
  it.each([
    ['color: red; margin-top: 4px', 'color: red; margin-top: 4px'],
    ['position:absolute;top:0', ''],
    ['background-color: expression(alert(1))', ''],
    [String.raw`color: \72 ed`, ''],
    ['display: contents; display: flex', 'display: flex'],
    ['width: 50%; z-index: 9', 'width: 50%'],
  ])('%s → %s', (input, want) => {
    expect(sanitizeStyle(input)).toBe(want)
  })
})
