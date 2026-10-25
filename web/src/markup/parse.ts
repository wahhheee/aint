/**
 * 叙事标记解析（《02-技术方案》16.1、16.3）。
 *
 * 语法与容错规则写在 Go 端 internal/markup/markup.go 的包注释里，这里是同一套规则的实现：两端运行同一份
 * 测试向量 testdata/markup/*.json（parse.test.ts / vectors_test.go），产出相同的 AST 与纯文本。
 * 改语法时三处一起改：本文件、internal/markup、测试向量。
 *
 * 要点：每个非空行是一个段落（D-26）；--- 场景分隔、> 引用、*强调*、**加重**；白名单标签
 * <name attr="值">…</name> 与 <name attr="值"/>，白名单外与未启用的标签按原文显示（D-21）；
 * 行首的 <ooc> 开始可以跨行的场外块（D-30）；反斜杠转义 \\ \* \< \> \-。
 *
 * 增量解析（MarkupParser）：流式正文每帧变长。解析按"单元"推进——以换行结束的一行，或从行首 <ooc> 到
 * </ooc> 的场外块；已完成的单元不会因为后面追加文字而改变，其结果（连同对象引用）缓存下来，每次只重新
 * 解析最后一个未完成的单元（草稿态只影响它）；未闭合的场外块内部同样按行增量。新的源码不是已完成部分的
 * 延续时从头解析。
 */

export type Inline =
  | { type: 'text'; text: string }
  | { type: 'raw'; text: string }
  | { type: 'em'; children: Inline[] }
  | { type: 'strong'; children: Inline[] }
  | { type: 'tag'; name: string; attrs: Attrs; children: Inline[] }

export type Block =
  | { type: 'p'; children: Inline[] }
  | { type: 'aside'; children: Inline[] }
  | { type: 'quote'; children: Inline[] }
  | { type: 'divider' }
  | { type: 'scene'; attrs: Attrs }
  | { type: 'cg'; attrs: Attrs }
  /** 执笔者的场外回应（D-30）：内部按正文规则解析，不含嵌套的场外块 */
  | { type: 'ooc'; children: Block[] }

/** 标签属性（无原型的对象：任何属性名都只是普通的键） */
export type Attrs = Record<string, string>

export interface ParseOptions {
  /** 可渲染的标签白名单（已启用的内置标签 + 作品样式标签） */
  tags: ReadonlySet<string>
  /** 流式草稿态：隐藏最后一行末尾尚未写完的标记 */
  drafting?: boolean
}

/** 一次性解析。 */
export function parseMarkup(src: string, opts: ParseOptions): Block[] {
  return new MarkupParser(opts.tags).parse(src, opts.drafting)
}

/** 增量解析器：同一段正文流式变长时反复调用 parse，已完成的块复用上一次的对象。 */
export class MarkupParser {
  private readonly tags: ReadonlySet<string>
  private readonly inOOC: boolean
  /** 已完成单元的源码（总是本次源码的前缀）与解析结果 */
  private done = ''
  private doneBlocks: Block[] = []
  /** 紧接已完成部分、尚未闭合的场外块：内容的起点与内容的增量解析器 */
  private openOOC: { start: number; parser: MarkupParser } | null = null
  private last: { src: string; drafting: boolean; blocks: Block[] } | null = null

  constructor(tags: ReadonlySet<string>, inOOC = false) {
    this.tags = tags
    this.inOOC = inOOC
  }

  parse(src: string, drafting = false): Block[] {
    const last = this.last
    if (last && last.src === src && last.drafting === drafting) return last.blocks
    if (!src.startsWith(this.done)) this.reset()

    const tail: Block[] = []
    let pos = this.done.length
    for (;;) {
      const nl = src.indexOf('\n', pos)
      const lineEnd = nl < 0 ? src.length : nl
      const bodyStart = this.has('ooc') ? oocOpener(src, pos, lineEnd) : -1
      if (bodyStart >= 0) {
        if (this.openOOC?.start !== bodyStart) this.openOOC = { start: bodyStart, parser: new MarkupParser(this.tags, true) }
        const inner = this.openOOC.parser
        const close = findOOCClose(src, bodyStart)
        if (close) {
          const children = inner.parse(src.slice(bodyStart, close.start), false)
          this.commit(src, close.end, children.length > 0 ? [{ type: 'ooc', children }] : [])
          pos = close.end
          continue
        }
        // 尚未写到 </ooc>：到文末都属于场外块（流式中也先显示出来）
        const children = inner.parse(src.slice(bodyStart), drafting)
        if (children.length > 0) tail.push({ type: 'ooc', children })
        break
      }
      if (nl < 0) {
        tail.push(...lineBlocks(src.slice(pos), this, drafting))
        break
      }
      this.commit(src, nl + 1, lineBlocks(src.slice(pos, lineEnd), this, false))
      pos = nl + 1
    }

    const blocks = [...this.doneBlocks, ...tail]
    this.last = { src, drafting, blocks }
    return blocks
  }

  /** 标签是否在白名单中（场外块内部不再识别 <ooc>）。 */
  has(name: string): boolean {
    return !(this.inOOC && name === 'ooc') && (name === 'br' || this.tags.has(name))
  }

  private commit(src: string, end: number, blocks: Block[]) {
    this.done = src.slice(0, end)
    this.doneBlocks.push(...blocks)
    this.openOOC = null
  }

  private reset() {
    this.done = ''
    this.doneBlocks = []
    this.openOOC = null
  }
}

/** 取纯文本，与 Go 端 markup.Plain 一致：每个块一行；场景分隔为"◇"；段首场景、插图取标题、说明文字；
 *  <say who="林舟">台词</say> 为"林舟：台词"；场外内容不属于故事，整段去掉（D-30）；按原文显示的标签保留原文。 */
export function plainText(blocks: Block[]): string {
  const lines: string[] = []
  for (const b of blocks) {
    let s: string
    switch (b.type) {
      case 'divider':
        s = '◇'
        break
      case 'scene':
        s = b.attrs.title ?? ''
        break
      case 'cg':
        s = b.attrs.caption ?? ''
        break
      case 'ooc':
        continue
      default:
        s = inlineText(b.children)
    }
    if (s) lines.push(s)
  }
  return lines.join('\n')
}

function inlineText(nodes: Inline[]): string {
  let s = ''
  for (const n of nodes) {
    if (n.type === 'text' || n.type === 'raw') {
      s += n.text
    } else if (n.type === 'tag') {
      if (n.name === 'br') s += '\n'
      if (n.name === 'ooc') continue
      if (n.name === 'say' && n.attrs.who) s += n.attrs.who + '：'
      s += inlineText(n.children)
    } else {
      s += inlineText(n.children)
    }
  }
  return s
}

// ---------------------------------------------------------------------------
// 块

/** 行首（允许前导空白）是 <ooc> 开始标签时返回其后的位置，否则返回 -1。 */
function oocOpener(src: string, pos: number, lineEnd: number): number {
  let k = pos
  while (k < lineEnd && isSpace(src.charCodeAt(k))) k++
  if (k >= lineEnd || src[k] !== '<') return -1
  const t = lexTag(src, k, lineEnd)
  if (t === null || t === PARTIAL || t.close || t.selfClose || t.name !== 'ooc') return -1
  return t.end
}

/** 从 from 起查找第一个未被转义的 </ooc>。转义按连续反斜杠的奇偶判断，与行内解析一致。 */
function findOOCClose(src: string, from: number): { start: number; end: number } | null {
  for (let i = from; ; i++) {
    i = src.indexOf('</ooc', i)
    if (i < 0) return null
    let bs = 0
    for (let j = i - 1; j >= from && src[j] === '\\'; j--) bs++
    if (bs % 2 === 0) {
      const nl = src.indexOf('\n', i)
      const t = lexTag(src, i, nl < 0 ? src.length : nl)
      if (t !== null && t !== PARTIAL && t.close && t.name === 'ooc') return { start: i, end: t.end }
    }
  }
}

/** 把一行解析为块：分隔、引用、段落（段首 <scene/>、<cg/> 提升为块级，整段旁白为 aside）。 */
function lineBlocks(raw: string, p: MarkupParser, tail: boolean): Block[] {
  const line = trim(raw)
  if (!line) return []
  if (isDivider(line)) return [{ type: 'divider' }]
  if (line[0] === '>') {
    const children = parseInline(trim(line.slice(1)), p, tail)
    return children.length > 0 ? [{ type: 'quote', children }] : []
  }

  const children = parseInline(line, p, tail)
  const out: Block[] = []
  let k = 0
  for (let c = children[0]; c?.type === 'tag' && (c.name === 'scene' || c.name === 'cg'); c = children[k]) {
    out.push({ type: c.name, attrs: c.attrs })
    k++
    const next = children[k]
    if (next?.type === 'text') {
      const t = trimStart(next.text)
      if (t) next.text = t
      else k++
    }
  }
  const rest = k > 0 ? children.slice(k) : children
  if (rest.length === 0) return out
  const only = rest[0]
  if (rest.length === 1 && only.type === 'tag' && only.name === 'aside') out.push({ type: 'aside', children: only.children })
  else out.push({ type: 'p', children: rest })
  return out
}

/** 整行只有 3 个以上的同一种 "-" 或 "*"，中间可有空白。 */
function isDivider(line: string): boolean {
  const c = line[0]
  if (c !== '-' && c !== '*') return false
  let n = 0
  for (let i = 0; i < line.length; i++) {
    if (line[i] === c) n++
    else if (!isSpace(line.charCodeAt(i))) return false
  }
  return n >= 3
}

// ---------------------------------------------------------------------------
// 行内

const MAX_DEPTH = 4 // 行内元素最多嵌套 4 层
/** 空元素：没有内容，不写 "/" 也按自闭合处理，结束标签忽略 */
const VOID_TAGS: ReadonlySet<string> = new Set(['scene', 'cg', 'bgm', 'sfx', 'pause', 'br'])
const SPECIAL = /[\\<*]/g

type Container = Extract<Inline, { children: Inline[] }>
interface Frame {
  node: Container | null // null 为根
  children: Inline[]
}

function parseInline(line: string, p: MarkupParser, tail: boolean): Inline[] {
  return new InlineParser(line, p, tail).run()
}

class InlineParser {
  private readonly line: string
  private readonly p: MarkupParser
  /** 草稿态的最后一行：隐藏段尾未写完的标记 */
  private readonly tail: boolean
  private readonly stack: Frame[] = [{ node: null, children: [] }]
  private buf = ''
  private stop = false
  /** [单星, 双星]：已确认从该位置起没有结束符（避免逐个开始符重复扫描到行尾） */
  private readonly noCloser = [-1, -1]

  constructor(line: string, p: MarkupParser, tail: boolean) {
    this.line = line
    this.p = p
    this.tail = tail
  }

  run(): Inline[] {
    const line = this.line
    const n = line.length
    for (let i = 0; i < n && !this.stop;) {
      SPECIAL.lastIndex = i
      const m = SPECIAL.exec(line)
      if (!m) {
        this.buf += line.slice(i)
        break
      }
      this.buf += line.slice(i, m.index)
      i = m.index
      const c = line[i]
      if (c === '\\') {
        if (i + 1 < n && isEscapable(line[i + 1])) {
          this.buf += line[i + 1]
          i += 2
          continue
        }
        if (i + 1 === n && this.tail) {
          this.stop = true // 段尾的反斜杠：可能是未写完的转义
          continue
        }
      } else if (c === '<') {
        const t = lexTag(line, i, n)
        if (t !== null && t !== PARTIAL) {
          this.tag(t, line.slice(i, t.end))
          i = t.end
          continue
        }
        if (t === PARTIAL && this.tail) {
          this.stop = true // 段尾未写完的标签：先不显示
          continue
        }
      } else {
        i = this.star(i)
        continue
      }
      this.buf += c
      i++
    }
    this.closeTo(1)
    return this.stack[0].children
  }

  private top(): Frame {
    return this.stack[this.stack.length - 1]
  }

  private flush() {
    if (this.buf) {
      this.top().children.push({ type: 'text', text: this.buf })
      this.buf = ''
    }
  }

  private emit(n: Inline) {
    this.flush()
    this.top().children.push(n)
  }

  private open(node: Container) {
    this.emit(node)
    this.stack.push({ node, children: node.children })
  }

  /** 闭合栈中下标 ≥ idx 的元素。 */
  private closeTo(idx: number) {
    this.flush()
    if (this.stack.length > idx) this.stack.length = idx
  }

  private depthOK(): boolean {
    return this.stack.length - 1 < MAX_DEPTH
  }

  private tag(t: TagToken, raw: string) {
    if (!this.p.has(t.name)) {
      this.emit({ type: 'raw', text: raw })
      return
    }
    if (t.close) {
      if (VOID_TAGS.has(t.name)) return
      for (let i = this.stack.length - 1; i > 0; i--) {
        const node = this.stack[i].node
        if (node?.type === 'tag' && node.name === t.name) {
          this.closeTo(i)
          return
        }
      }
      return // 没有对应的开始标签：忽略
    }
    const node: Container = { type: 'tag', name: t.name, attrs: parseAttrs(t.attrs), children: [] }
    if (t.selfClose || VOID_TAGS.has(t.name)) this.emit(node)
    else if (this.depthOK()) this.open(node)
    // 超过嵌套上限：忽略这个开始标签，内容照常显示（结束标签随之被忽略）
  }

  /** 处理从 i 开始的一串 "*"，返回处理后的位置。 */
  private star(i: number): number {
    const line = this.line
    let run = 0
    while (i + run < line.length && line[i + run] === '*') run++
    if (this.tail && i + run === line.length) {
      this.stop = true // 段尾的星号：可能是未写完的强调符
      return i + run
    }
    const canClose = i > 0 && !isSpace(line.charCodeAt(i - 1))
    const top = this.top().node?.type
    if (canClose && top === 'strong' && run >= 2) {
      this.closeTo(this.stack.length - 1)
      return i + 2
    }
    // 栈顶是强调：用一个星号闭合它；除非这是一个能成立的加重开始符（*斜体**加粗**斜体*）
    if (canClose && top === 'em' && (run < 2 || !this.canOpen(i + 2, '**'))) {
      this.closeTo(this.stack.length - 1)
      return i + 1
    }
    const delim = run >= 2 ? '**' : '*'
    const j = i + delim.length
    if (this.canOpen(j, delim)) this.open({ type: run >= 2 ? 'strong' : 'em', children: [] })
    else this.buf += delim
    return j
  }

  /** j 是开始符之后的位置。嵌套未超限、紧跟非空白字符、本行后面有结束符（草稿态最后一行不要求）。 */
  private canOpen(j: number, delim: string): boolean {
    if (!this.depthOK() || j >= this.line.length) return false
    if (isSpace(this.line.charCodeAt(j))) return false
    return this.tail || this.hasCloser(j, delim)
  }

  /** from 之后（不含 from）是否有前面为非空白字符的结束符。转义的星号不算。 */
  private hasCloser(from: number, delim: string): boolean {
    const slot = delim.length - 1
    const c = this.noCloser[slot]
    if (c >= 0 && from >= c) return false
    const line = this.line
    for (let j = from; j < line.length; j++) {
      const ch = line[j]
      if (ch === '\\') {
        if (j + 1 < line.length && isEscapable(line[j + 1])) j++
      } else if (ch === '*' && j > from && line.startsWith(delim, j) && !isSpace(line.charCodeAt(j - 1))) {
        return true
      }
    }
    this.noCloser[slot] = from
    return false
  }
}

// ---------------------------------------------------------------------------
// 词法

interface TagToken {
  name: string
  close: boolean // </name>
  selfClose: boolean // <name …/>
  attrs: string // 名称之后、">" 之前的原文
  end: number // ">" 之后的位置
}

/** 到 lim 为止仍可能是未写完的标签 */
const PARTIAL = 'partial'

/** 识别 s[i] === '<' 处的标签（只看到 lim 为止）。 */
function lexTag(s: string, i: number, lim: number): TagToken | typeof PARTIAL | null {
  let j = i + 1
  let close = false
  if (j < lim && s[j] === '/') {
    close = true
    j++
  }
  if (j >= lim) return PARTIAL
  if (!isLower(s.charCodeAt(j))) return null
  const start = j
  while (j < lim && isNameChar(s.charCodeAt(j))) j++
  const name = s.slice(start, j)
  if (j >= lim) return PARTIAL
  const c = s[j]
  if (c !== '>' && c !== '/' && !isSpace(s.charCodeAt(j))) return null
  const attrStart = j
  let inQuote = false
  for (; j < lim; j++) {
    const ch = s[j]
    if (ch === '"') {
      inQuote = !inQuote
    } else if (ch === '>' && !inQuote) {
      const attrs = s.slice(attrStart, j)
      return { name, close, selfClose: !close && trimEnd(attrs).endsWith('/'), attrs, end: j + 1 }
    }
  }
  return PARTIAL
}

/** 取出 name="值" 形式的属性，规则与 Go 端 parseAttrs 相同：名称为字母或下划线开头的 [A-Za-z0-9_-]，
 *  其余内容忽略；同名属性取第一个；值原样保留。 */
function parseAttrs(s: string): Attrs {
  const attrs: Attrs = Object.create(null) as Attrs
  const n = s.length
  for (let k = 0; k < n;) {
    const c = s.charCodeAt(k)
    if (c === 0x22) {
      // 游离的引号串：整体跳过
      const q = s.indexOf('"', k + 1)
      if (q < 0) return attrs
      k = q + 1
    } else if (isAttrChar(c)) {
      const st = k
      while (k < n && isAttrChar(s.charCodeAt(k))) k++
      const name = s.slice(st, k)
      let m = skipSpace(s, k)
      if (m >= n || s[m] !== '=') continue
      m = skipSpace(s, m + 1)
      if (m >= n || s[m] !== '"') continue
      const q = s.indexOf('"', m + 1)
      if (q < 0) return attrs
      if (!Object.hasOwn(attrs, name) && isAttrStart(name.charCodeAt(0))) attrs[name] = s.slice(m + 1, q)
      k = q + 1
    } else {
      k++
    }
  }
  return attrs
}

/** 空白字符集，与 Go 端 isSpace 相同：Unicode White_Space（含全角空格、不换行空格、NEL）与 BOM。
 *  全部在 BMP 内，按 UTF-16 码元判断与 Go 按码点判断等价。 */
function isSpace(c: number): boolean {
  return (
    c === 0x20 ||
    (c >= 0x09 && c <= 0x0d) ||
    c === 0x85 ||
    c === 0xa0 ||
    c === 0x1680 ||
    (c >= 0x2000 && c <= 0x200a) ||
    c === 0x2028 ||
    c === 0x2029 ||
    c === 0x202f ||
    c === 0x205f ||
    c === 0x3000 ||
    c === 0xfeff
  )
}

function trimStart(s: string): string {
  let i = 0
  while (i < s.length && isSpace(s.charCodeAt(i))) i++
  return i > 0 ? s.slice(i) : s
}

function trimEnd(s: string): string {
  let j = s.length
  while (j > 0 && isSpace(s.charCodeAt(j - 1))) j--
  return j < s.length ? s.slice(0, j) : s
}

function trim(s: string): string {
  return trimEnd(trimStart(s))
}

function skipSpace(s: string, k: number): number {
  while (k < s.length && isSpace(s.charCodeAt(k))) k++
  return k
}

function isEscapable(c: string): boolean {
  return c === '\\' || c === '*' || c === '<' || c === '>' || c === '-'
}

function isLower(c: number): boolean {
  return c >= 0x61 && c <= 0x7a
}

function isNameChar(c: number): boolean {
  return isLower(c) || (c >= 0x30 && c <= 0x39) || c === 0x2d
}

function isAttrStart(c: number): boolean {
  return isLower(c) || (c >= 0x41 && c <= 0x5a) || c === 0x5f
}

function isAttrChar(c: number): boolean {
  return isAttrStart(c) || (c >= 0x30 && c <= 0x39) || c === 0x2d
}
