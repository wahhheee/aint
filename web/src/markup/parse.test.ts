import { describe, expect, it } from 'vitest'

import { parseMarkup } from './parse'

const tags = new Set(['say', 'aside', 'gold', 'scene', 'fx'])

describe('parseMarkup', () => {
  it('按行分段，识别场景分隔与段首场景标签', () => {
    const blocks = parseMarkup('<scene bg="x" title="序章"/>\n\n第一段\n---\n第二段', { tags })
    expect(blocks.map((b) => b.type)).toEqual(['scene', 'p', 'divider', 'p'])
  })

  it('白名单外的标签按原文显示', () => {
    const [p] = parseMarkup('他说<unknown a="1">什么</unknown>', { tags })
    expect(p).toEqual({
      type: 'p',
      children: [
        { type: 'text', text: '他说' },
        { type: 'raw', text: '<unknown a="1">' },
        { type: 'text', text: '什么' },
        { type: 'raw', text: '</unknown>' },
      ],
    })
  })

  it('未闭合的标签在段末自动闭合，不影响下一段', () => {
    const blocks = parseMarkup('<gold>老陈在午夜前\n下一段', { tags })
    expect(blocks).toHaveLength(2)
    expect(blocks[1]).toEqual({ type: 'p', children: [{ type: 'text', text: '下一段' }] })
  })

  it('草稿态隐藏段尾未写完的标签', () => {
    const [p] = parseMarkup('雾很浓。<go', { tags, drafting: true })
    expect(p).toEqual({ type: 'p', children: [{ type: 'text', text: '雾很浓。' }] })
  })

  it('整段旁白提升为旁白段落', () => {
    const [b] = parseMarkup('<aside>远处传来汽笛声。</aside>', { tags })
    expect(b.type).toBe('aside')
  })

  it('强调与加重', () => {
    const [p] = parseMarkup('这是**很重要**的*线索*', { tags })
    expect(p.type === 'p' && p.children.map((c) => c.type)).toEqual(['text', 'strong', 'text', 'em'])
  })

  describe('场外块 <ooc>（D-30）', () => {
    const withOOC = new Set([...tags, 'ooc'])

    it('可以跨行，内部按正文规则分段；块后的正文照常解析', () => {
      const blocks = parseMarkup('<ooc>好的，节奏放慢一些。\n\n<gold>线索</gold>会更隐蔽。</ooc>\n雾又浓了。', { tags: withOOC })
      expect(blocks.map((b) => b.type)).toEqual(['ooc', 'p'])
      const ooc = blocks[0]
      expect(ooc.type === 'ooc' && ooc.children.map((b) => b.type)).toEqual(['p', 'p'])
    })

    it('流式中尚未写到 </ooc> 时，到文末都算场外块', () => {
      const blocks = parseMarkup('前文。\n<ooc>我想想', { tags: withOOC, drafting: true })
      expect(blocks.map((b) => b.type)).toEqual(['p', 'ooc'])
    })

    it('未启用时按原文显示', () => {
      const [p] = parseMarkup('<ooc>场外</ooc>', { tags })
      expect(p.type === 'p' && p.children[0]).toEqual({ type: 'raw', text: '<ooc>' })
    })
  })
})
