/**
 * 叙事标记解析（《02-技术方案》16.1）。
 *
 * 语法：Markdown 子集（段落、*强调*、**加重**、--- 场景分隔、> 引用）+ 白名单标签
 *   <name attr="值">内容</name>   <name attr="值"/>
 *
 * 容错（模型输出不可控，任何输入都不能让渲染出错）：
 *   - 白名单外的标签按原文显示（raw）；
 *   - 行内标签在段落结束时自动闭合；嵌套最多 4 层；属性非法时忽略该属性；
 *   - 流式草稿态（drafting）下，段尾尚未写完的 "<sce…" 先不显示，避免一闪而过的原文。
 *
 * 分段：每个非空行是一个段落（D-26）——空行分隔与单换行分段两种写法都能正确显示。
 * 例外：<ooc>…</ooc> 可以跨行，整体作为一个场外块（D-30）。
 * TODO(M0-5)：与 Go 端 internal/markup 共用 testdata/markup 测试向量。
 *
 * TODO(M0-5)：改为真正的增量解析（只解析新增文本），一万字长回合时避免每帧全量重解析。
 */

export type Inline =
  | { type: 'text'; text: string }
  | { type: 'raw'; text: string }
  | { type: 'em'; children: Inline[] }
  | { type: 'strong'; children: Inline[] }
  | { type: 'tag'; name: string; attrs: Record<string, string>; children: Inline[] }

export type Block =
  | { type: 'p'; children: Inline[] }
  | { type: 'aside'; children: Inline[] }
  | { type: 'quote'; children: Inline[] }
  | { type: 'divider' }
  | { type: 'scene'; attrs: Record<string, string> }
  | { type: 'cg'; attrs: Record<string, string> }
  /** 执笔者的场外回应（D-30）：内部按正文规则解析，不含嵌套的 <ooc> */
  | { type: 'ooc'; children: Block[] }

export interface ParseOptions {
  /** 可渲染的标签白名单（已启用的内置标签 + 作品样式标签） */
  tags: ReadonlySet<string>
  /** 流式草稿态：隐藏段尾未写完的标签 */
  drafting?: boolean
}

const MAX_DEPTH = 4
const BLOCK_TAGS = new Set(['scene', 'cg'])
const TAG_RE = /^<(\/?)([a-z][a-z0-9-]*)((?:\s+[a-zA-Z_-]+="[^"]*")*)\s*(\/?)>/
const ATTR_RE = /([a-zA-Z_-]+)="([^"]*)"/g
const OOC_OPEN_RE = /^[ \t]*<ooc>/m
const OOC_CLOSE = '</ooc>'

export function parseMarkup(src: string, opts: ParseOptions): Block[] {
  src = src.replace(/\r\n/g, '\n')
  if (!opts.tags.has('ooc')) return parseLines(src, opts)

  // 行首的 <ooc> 开始一个场外块，直到 </ooc>（流式中尚未写到 </ooc> 时到文末为止）
  const inner: ParseOptions = { ...opts, tags: new Set([...opts.tags].filter((t) => t !== 'ooc')) }
  const blocks: Block[] = []
  let rest = src
  for (let m = OOC_OPEN_RE.exec(rest); m; m = OOC_OPEN_RE.exec(rest)) {
    blocks.push(...parseLines(rest.slice(0, m.index), { ...opts, drafting: false }))
    const start = m.index + m[0].length
    const end = rest.indexOf(OOC_CLOSE, start)
    const body = end < 0 ? rest.slice(start) : rest.slice(start, end)
    const children = parseLines(body, { ...inner, drafting: opts.drafting && end < 0 })
    if (children.length > 0 || end < 0) blocks.push({ type: 'ooc', children })
    if (end < 0) return blocks
    rest = rest.slice(end + OOC_CLOSE.length)
  }
  blocks.push(...parseLines(rest, opts))
  return blocks
}

function parseLines(src: string, opts: ParseOptions): Block[] {
  const lines = src.split('\n')
  const blocks: Block[] = []
  lines.forEach((raw, i) => {
    const line = raw.trim()
    if (!line) return
    const isLast = i === lines.length - 1
    if (/^(-{3,}|\*{3,})$/.test(line)) {
      blocks.push({ type: 'divider' })
      return
    }
    if (line.startsWith('>')) {
      blocks.push({ type: 'quote', children: parseInline(line.replace(/^>\s?/, ''), opts, isLast) })
      return
    }
    const children = parseInline(line, opts, isLast)
    // 段首的 <scene/>、<cg/> 提升为块级（舞台切换与标题卡、插图卡片）
    for (let first = children[0]; first?.type === 'tag' && BLOCK_TAGS.has(first.name); first = children[0]) {
      blocks.push({ type: first.name as 'scene' | 'cg', attrs: first.attrs })
      children.shift()
      const next = children[0]
      if (next?.type === 'text' && !next.text.trim()) children.shift()
    }
    if (children.length === 0) return
    // 整段只有一个 <aside>：按旁白段落排版
    if (children.length === 1 && children[0].type === 'tag' && children[0].name === 'aside') {
      blocks.push({ type: 'aside', children: children[0].children })
      return
    }
    blocks.push({ type: 'p', children })
  })
  return blocks
}

type Frame = { node: Extract<Inline, { children: Inline[] }> | null; children: Inline[]; key: string }

function parseInline(line: string, opts: ParseOptions, isLastLine: boolean): Inline[] {
  const root: Frame = { node: null, children: [], key: '' }
  const stack: Frame[] = [root]
  const top = () => stack[stack.length - 1]
  let buf = ''
  const flush = () => {
    if (buf) top().children.push({ type: 'text', text: buf })
    buf = ''
  }
  const open = (node: Frame['node'] & object, key: string) => {
    flush()
    top().children.push(node)
    stack.push({ node, children: node.children, key })
  }
  const close = (key: string): boolean => {
    const idx = stack.map((f) => f.key).lastIndexOf(key)
    if (idx <= 0) return false
    flush()
    stack.length = idx
    return true
  }

  let i = 0
  while (i < line.length) {
    const ch = line[i]
    if (ch === '<') {
      const m = TAG_RE.exec(line.slice(i))
      if (!m) {
        // 草稿态下，段尾看起来像未写完的标签：先不显示
        if (opts.drafting && isLastLine && /^<\/?[a-z][^>]*$/.test(line.slice(i))) break
        buf += ch
        i++
        continue
      }
      const [whole, slash, name, attrText, selfClose] = m
      i += whole.length
      if (!opts.tags.has(name)) {
        flush()
        top().children.push({ type: 'raw', text: whole })
        continue
      }
      if (slash) {
        if (!close('tag:' + name)) {
          flush()
          top().children.push({ type: 'raw', text: whole })
        }
        continue
      }
      const attrs: Record<string, string> = {}
      for (const am of attrText.matchAll(ATTR_RE)) attrs[am[1]] = am[2]
      if (selfClose) {
        flush()
        top().children.push({ type: 'tag', name, attrs, children: [] })
      } else if (stack.length > MAX_DEPTH) {
        flush()
        top().children.push({ type: 'raw', text: whole })
      } else {
        open({ type: 'tag', name, attrs, children: [] }, 'tag:' + name)
      }
      continue
    }
    if (ch === '*') {
      const strong = line.startsWith('**', i)
      const key = strong ? 'strong' : 'em'
      i += strong ? 2 : 1
      if (top().key === key) {
        close(key)
      } else if (stack.length <= MAX_DEPTH && line.indexOf(strong ? '**' : '*', i) > i) {
        open({ type: key, children: [] } as Extract<Inline, { type: 'em' | 'strong' }>, key)
      } else {
        buf += strong ? '**' : '*'
      }
      continue
    }
    buf += ch
    i++
  }
  flush()
  return root.children // 未闭合的节点在段末自然结束（自动闭合）
}

/** 取纯文本（复制正文、无障碍播报）。 */
export function plainText(blocks: Block[]): string {
  const inline = (nodes: Inline[]): string =>
    nodes.map((n) => (n.type === 'text' || n.type === 'raw' ? n.text : inline(n.children))).join('')
  return blocks
    .map((b) => {
      if (b.type === 'divider') return '◇'
      if (b.type === 'scene') return b.attrs.title ?? ''
      if (b.type === 'cg') return b.attrs.caption ?? ''
      if (b.type === 'ooc') return '' // 场外回应不属于故事原文（与 Go 端 markup.PlainText 一致）
      return inline(b.children)
    })
    .filter(Boolean)
    .join('\n')
}
