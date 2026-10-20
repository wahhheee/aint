/**
 * 叙事标记 AST → React（《02-技术方案》16.2，《04-交互与界面设计》8.4）。
 */
import { Fragment, useContext, useMemo, type CSSProperties, type ReactNode } from 'react'

import { WorkIcon } from '@/components/common/WorkIcon'

import { MarkupContext, type MarkupContextValue } from './context'
import { parseMarkup, type Block, type Inline } from './parse'
import { workTagClasses } from './workStyles'

/** 渲染一段叙事标记源码。drafting：流式中，最后一段带光标。 */
export function Markup({ source, drafting = false }: { source: string; drafting?: boolean }) {
  const ctx = useContext(MarkupContext)
  const blocks = useMemo(() => parseMarkup(source, { tags: ctx.tags, drafting }), [source, ctx.tags, drafting])
  return <>{blocks.map((b, i) => renderBlock(b, i, ctx, drafting && i === blocks.length - 1))}</>
}

function renderBlock(b: Block, key: number, ctx: MarkupContextValue, isDraftTail: boolean): ReactNode {
  const tail = isDraftTail ? 'is-drafting' : undefined
  switch (b.type) {
    case 'divider':
      return (
        <div key={key} className="scene-divider" role="separator" aria-hidden>
          ◇
        </div>
      )
    case 'cg': {
      const a = ctx.assets.get(b.attrs.src ?? '')
      if (!a) return null
      // TODO(M5-2)：点击全屏查看
      return (
        <figure key={key} className="cg-card">
          <img src={a.url} alt={b.attrs.caption ?? a.description ?? ''} loading="lazy" />
          {b.attrs.caption && <figcaption>{b.attrs.caption}</figcaption>}
        </figure>
      )
    }
    case 'scene':
      // 背景切换由舞台层负责（features/reader/stage），这里只显示标题卡。TODO(M5-2)：转场（fade / cut / blur）。
      return b.attrs.title ? (
        <h3 key={key} className="scene-title">
          {b.attrs.title}
        </h3>
      ) : null
    case 'quote':
      return (
        <blockquote key={key} className={tail}>
          {renderInline(b.children, ctx)}
        </blockquote>
      )
    case 'aside':
      return (
        <p key={key} className={`mk-aside mk-aside-block ${tail ?? ''}`}>
          {renderInline(b.children, ctx)}
        </p>
      )
    case 'p':
      return (
        <p key={key} className={tail}>
          {renderInline(b.children, ctx)}
        </p>
      )
  }
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
