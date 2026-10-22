/**
 * 叙事标记 AST → React（《02-技术方案》16.2，《04-交互与界面设计》8.4）。
 */
import { Fragment, memo, useContext, useMemo, type CSSProperties, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { WorkIcon } from '@/components/common/WorkIcon'

import { MarkupContext, type MarkupContextValue } from './context'
import { MarkupParser, type Block, type Inline } from './parse'
import { workTagClasses } from './workStyles'

/**
 * 渲染一段叙事标记源码。drafting：流式中，最后一段带光标。
 *
 * 流式中 source 每帧变长：增量解析器只重新解析尾部，已完成的块保持同一个对象，
 * BlockView 据此跳过重渲染——一万字的长回合每帧也只更新最后一段。
 */
export function Markup({ source, drafting = false }: { source: string; drafting?: boolean }) {
  const ctx = useContext(MarkupContext)
  const parser = useMemo(() => new MarkupParser(ctx.tags), [ctx.tags])
  const blocks = useMemo(() => parser.parse(source, drafting), [parser, source, drafting])
  return <Blocks blocks={blocks} drafting={drafting} />
}

function Blocks({ blocks, drafting }: { blocks: Block[]; drafting: boolean }) {
  return blocks.map((b, i) => <BlockView key={i} block={b} isDraftTail={drafting && i === blocks.length - 1} />)
}

const BlockView = memo(function BlockView({ block, isDraftTail }: { block: Block; isDraftTail: boolean }) {
  const ctx = useContext(MarkupContext)
  return renderBlock(block, ctx, isDraftTail)
})

function renderBlock(b: Block, ctx: MarkupContextValue, isDraftTail: boolean): ReactNode {
  const tail = isDraftTail ? 'is-drafting' : undefined
  switch (b.type) {
    case 'divider':
      return (
        <div className="scene-divider" role="separator" aria-hidden>
          ◇
        </div>
      )
    case 'cg': {
      const a = ctx.assets.get(b.attrs.src ?? '')
      if (!a) return null
      // TODO(M5-2)：点击全屏查看
      return (
        <figure className="cg-card">
          <img src={a.url} alt={b.attrs.caption ?? a.description ?? ''} loading="lazy" />
          {b.attrs.caption && <figcaption>{b.attrs.caption}</figcaption>}
        </figure>
      )
    }
    case 'scene':
      // 背景切换由舞台层负责（features/reader/stage），这里只显示标题卡。TODO(M5-2)：转场（fade / cut / blur）。
      return b.attrs.title ? <h3 className="scene-title">{b.attrs.title}</h3> : null
    case 'quote':
      return <blockquote className={tail}>{renderInline(b.children, ctx)}</blockquote>
    case 'aside':
      return <p className={`mk-aside mk-aside-block ${tail ?? ''}`}>{renderInline(b.children, ctx)}</p>
    case 'p':
      return <p className={tail}>{renderInline(b.children, ctx)}</p>
    case 'ooc':
      return (
        <OOCBlock>
          <Blocks blocks={b.children} drafting={isDraftTail} />
        </OOCBlock>
      )
  }
}

/** 执笔者的场外回应（D-30）：与玩家的场外发言同一视觉语汇，表明它不属于故事。 */
function OOCBlock({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <aside className="mk-ooc-block" aria-label={t('reader.oocReply')}>
      <span className="mk-ooc-label">{t('reader.oocReply')}</span>
      <div className="min-w-0">{children}</div>
    </aside>
  )
}

function renderInline(nodes: Inline[], ctx: MarkupContextValue): ReactNode {
  return nodes.map((n, i) => {
    switch (n.type) {
      case 'text':
        return <Fragment key={i}>{n.text}</Fragment>
      case 'raw':
        return (
          <span key={i} className="mk-raw">
            {n.text}
          </span>
        )
      case 'em':
        return <em key={i}>{renderInline(n.children, ctx)}</em>
      case 'strong':
        return <strong key={i}>{renderInline(n.children, ctx)}</strong>
      case 'tag':
        return <Fragment key={i}>{renderTag(n, ctx)}</Fragment>
    }
  })
}

function renderTag(n: Extract<Inline, { type: 'tag' }>, ctx: MarkupContextValue): ReactNode {
  const children = renderInline(n.children, ctx)
  switch (n.name) {
    case 'say': {
      const who = n.attrs.who ?? ''
      const c = ctx.characters.get(who)
      return (
        <span className="mk-say" style={c?.color ? ({ '--speaker-color': c.color } as CSSProperties) : undefined}>
          {who && <span className="mk-speaker">{c?.name ?? who}</span>}
          {children}
        </span>
      )
    }
    case 'aside':
      return <span className="mk-aside">{children}</span>
    case 'ooc': // 段落中间的场外回应（行首的已提升为场外块）
      return <span className="mk-ooc">{children}</span>
    case 'fx':
      return <span className={`fx-${(n.attrs.type ?? '').replace(/[^a-z-]/g, '')}`}>{children}</span>
    case 'pause': // 打字机停顿：TODO(M5-1)
    case 'bgm': // 音频：TODO(M5-2)
    case 'sfx':
      return null
    case 'cg': // 段落中间的插图按说明文字显示（段首的插图已提升为块级卡片）
    case 'scene':
      return n.attrs.caption || n.attrs.title ? <span className="mk-aside">［{n.attrs.caption ?? n.attrs.title}］</span> : null
    default: {
      const def = ctx.workTags.get(n.name)
      if (!def) return children
      return (
        <span className={workTagClasses(def)} title={def.tooltip?.replace('{{ text }}', '')}>
          {def.icon && <WorkIcon name={def.icon} className="mk-icon" />}
          {children}
        </span>
      )
    }
  }
}
