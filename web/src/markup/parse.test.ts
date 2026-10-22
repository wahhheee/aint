import { describe, expect, it } from 'vitest'

import { MarkupParser, parseMarkup, plainText, type Attrs, type Block, type Inline } from './parse'

// 共享测试向量（格式见 testdata/README.md）：Go 端 internal/markup/vectors_test.go 运行同一份。
interface VectorCase {
  name: string
  input: string | string[]
  tags?: string[]
  drafting?: boolean
  ast: unknown[]
  plain?: string | string[]
}
interface VectorFile {
  description: string
  tags?: string[]
  drafting?: boolean
  cases: VectorCase[]
}

const vectorFiles = import.meta.glob<VectorFile>('../../../testdata/markup/*.json', { eager: true, import: 'default' })

const joinLines = (v: string | string[]) => (Array.isArray(v) ? v.join('\n') : v)

interface Case {
  title: string
  src: string
  tags: ReadonlySet<string>
  drafting: boolean
  ast: unknown[]
  plain?: string
}

const cases: Case[] = Object.entries(vectorFiles).flatMap(([path, f]) =>
  f.cases.map((c) => ({
    title: `${path.split('/').pop()}/${c.name}`,
    src: joinLines(c.input),
    tags: new Set(c.tags ?? f.tags ?? []),
    drafting: c.drafting ?? f.drafting ?? false,
    ast: c.ast,
    plain: c.plain === undefined ? undefined : joinLines(c.plain),
  })),
)

// AST → 向量中的紧凑写法（与 vectors_test.go 的 compactBlocks 相同）
function compactBlocks(blocks: Block[]): unknown[] {
  return blocks.map((b) => {
    switch (b.type) {
      case 'divider':
        return [b.type]
      case 'scene':
      case 'cg':
        return withAttrs([b.type], b.attrs)
      case 'ooc':
        return [b.type, ...compactBlocks(b.children)]
      default:
        return [b.type, ...compactInline(b.children)]
    }
  })
}

function compactInline(nodes: Inline[]): unknown[] {
  return nodes.map((n) => {
    switch (n.type) {
      case 'text':
        return n.text
      case 'raw':
        return ['raw', n.text]
      case 'em':
        return ['*', ...compactInline(n.children)]
      case 'strong':
        return ['**', ...compactInline(n.children)]
      case 'tag':
        return [...withAttrs([`<${n.name}>`], n.attrs), ...compactInline(n.children)]
    }
  })
}

function withAttrs(e: unknown[], attrs: Attrs): unknown[] {
  const entries = Object.entries(attrs)
  return entries.length > 0 ? [...e, Object.fromEntries(entries)] : e
}

/** 可复现的伪随机数（mulberry32） */
function rng(seed: number) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** 随机切分点（含末尾）：模拟流式增量 */
function cutPoints(len: number, random: () => number): number[] {
  const cuts: number[] = []
  for (let i = 0; i < len;) {
    i = Math.min(len, i + 1 + Math.floor(random() * 6))
    cuts.push(i)
  }
  return cuts.length > 0 ? cuts : [0]
}

describe('共享测试向量', () => {
  it('找到了向量文件，且只含已知字段', () => {
    expect(cases.length).toBeGreaterThan(100)
    const fileKeys = new Set(['description', 'tags', 'drafting', 'cases'])
    const caseKeys = new Set(['name', 'input', 'tags', 'drafting', 'ast', 'plain'])
    for (const [path, f] of Object.entries(vectorFiles)) {
      expect(
        Object.keys(f).filter((k) => !fileKeys.has(k)),
        path,
      ).toEqual([])
      for (const c of f.cases)
        expect(
          Object.keys(c).filter((k) => !caseKeys.has(k)),
          `${path}/${c.name}`,
        ).toEqual([])
    }
  })

  it.each(cases)('$title', (c) => {
    const blocks = parseMarkup(c.src, { tags: c.tags, drafting: c.drafting })
    expect(compactBlocks(blocks)).toEqual(c.ast)
    if (c.plain !== undefined) expect(plainText(blocks)).toBe(c.plain)
  })
})

describe('增量解析与一次性解析结果相同', () => {
  it.each(cases)('$title', (c) => {
    for (let seed = 1; seed <= 8; seed++) {
      const parser = new MarkupParser(c.tags)
      const cuts = seed === 1 ? Array.from({ length: c.src.length }, (_, i) => i + 1) : cutPoints(c.src.length, rng(seed * 7919))
      for (const end of cuts) {
        const prefix = c.src.slice(0, end)
        // 流式中逐段喂入（草稿态），每一步都与同一前缀的一次性解析相同
        expect(compactBlocks(parser.parse(prefix, true)), `seed ${seed}，前 ${end} 个字符`).toEqual(
          compactBlocks(parseMarkup(prefix, { tags: c.tags, drafting: true })),
        )
      }
      // 流结束：按向量自身的草稿态再解析一次，与期望相同
      expect(compactBlocks(parser.parse(c.src, c.drafting))).toEqual(c.ast)
    }
  })
})

describe('MarkupParser', () => {
  const tags = new Set(['say', 'gold', 'ooc', 'scene'])

  it('已完成的段落复用上一次的对象，只有最后一段重新解析', () => {
    const parser = new MarkupParser(tags)
    const a = parser.parse('<scene bg="a"/>第一段。\n<gold>第二段</gold>\n第三', true)
    const b = parser.parse('<scene bg="a"/>第一段。\n<gold>第二段</gold>\n第三段，', true)
    expect(b).toHaveLength(4)
    for (let i = 0; i < 3; i++) expect(b[i]).toBe(a[i])
    expect(b[3]).not.toBe(a[3])
  })

  it('同样的输入返回同一个数组', () => {
    const parser = new MarkupParser(tags)
    const a = parser.parse('甲\n乙', true)
    expect(parser.parse('甲\n乙', true)).toBe(a)
    expect(parser.parse('甲\n乙', false)).not.toBe(a)
  })

  it('未闭合的场外块内部按行复用，闭合后仍复用', () => {
    const parser = new MarkupParser(tags)
    const a = parser.parse('前文\n<ooc>第一行\n第二', true)
    const b = parser.parse('前文\n<ooc>第一行\n第二行</oo', true)
    expect(b[0]).toBe(a[0])
    const [oocA, oocB] = [a[1], b[1]]
    if (oocA.type !== 'ooc' || oocB.type !== 'ooc') throw new Error('应为场外块')
    expect(oocB.children[0]).toBe(oocA.children[0])
    const c = parser.parse('前文\n<ooc>第一行\n第二行</ooc>\n后文', false)
    const oocC = c[1]
    if (oocC.type !== 'ooc') throw new Error('应为场外块')
    expect(oocC.children[0]).toBe(oocA.children[0])
    expect(compactBlocks(c)).toEqual([
      ['p', '前文'],
      ['ooc', ['p', '第一行'], ['p', '第二行']],
      ['p', '后文'],
    ])
  })

  it('源码不是上次的延续时从头解析', () => {
    const parser = new MarkupParser(tags)
    parser.parse('甲\n乙\n丙', false)
    const b = parser.parse('甲\n丁\n丙', false)
    expect(compactBlocks(b)).toEqual([
      ['p', '甲'],
      ['p', '丁'],
      ['p', '丙'],
    ])
    expect(compactBlocks(parser.parse('', false))).toEqual([])
  })

  it('一万字的长回合逐字流入：已完成的段落不再重新解析', () => {
    const para = (i: number) => `<say who="林舟">「第${i}段。」</say>雨从傍晚开始下，**海雾**漫过了码头，灯塔的光一圈一圈扫过水面。`
    const src = Array.from({ length: 200 }, (_, i) => para(i)).join('\n')
    expect(src.length).toBeGreaterThan(10000)
    const parser = new MarkupParser(tags)
    const seen = new Map<number, Block>()
    for (let end = 1; end <= src.length; end += 3) {
      const blocks = parser.parse(src.slice(0, end), true)
      // 除最后一段外，每个块一旦出现就保持同一个对象
      for (let i = 0; i < blocks.length - 1; i++) {
        const prev = seen.get(i)
        if (prev) expect(blocks[i]).toBe(prev)
        else seen.set(i, blocks[i])
      }
    }
    expect(compactBlocks(parser.parse(src, false))).toEqual(compactBlocks(parseMarkup(src, { tags })))
  })
})
