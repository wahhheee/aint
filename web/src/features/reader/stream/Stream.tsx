/**
 * 正文流：已提交的回合（时间线）+ 进行中的回合（草稿）。
 *
 * 时间线中还有编辑节点（玩家修改了状态或记忆，显示一行注记）与整理节点（一条分隔线）。
 * 重新生成、修改输入时，新回合的父节点较早：等待与生成期间，父节点之后的已提交回合先隐去。
 * head 停在较早的回合（回到此处）时，末尾提示继续游玩会开启新分支。
 *
 * 滚动（《04-交互与界面设计》8.5）：只有在底部附近（< 80px）时才自动跟随；
 * 玩家向上翻阅后停止跟随，底部出现"回到最新"。
 * 更早回合按页加载，加载时保留滚动锚点；目录支持回合号与正文片段检索。
 */
import { useReducedMotion } from 'motion/react'
import { ArrowDown, List } from 'lucide-react'
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import type { DisplayItem, Timeline, TimelineNode, TurnInput } from '@/api/types'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useReader, type DraftTurn } from '@/stores/reader'

import { useReaderContext } from '../ReaderContext'
import { useTypewriter } from '../useTypewriter'
import { displayedInput } from './displayedInput'
import { Notice, QuietLine, Waiting } from './Notice'
import { TurnToolbar } from './TurnToolbar'
import { TurnView } from './TurnView'

const FOLLOW_THRESHOLD = 80

interface Props {
  opening?: ReactNode
  loadEarlier: () => void
  loadingEarlier: boolean
  timeline?: Timeline
  onRetry: () => void
  onContinue: () => void
}

export function Stream({ timeline, onRetry, onContinue, loadEarlier, loadingEarlier, opening }: Props) {
  const { t } = useTranslation()
  const draft = useReader((s) => s.draft)
  const pending = useReader((s) => s.pending)
  const pendingParent = useReader((s) => s.pendingParent)
  const compacting = useReader((s) => s.compacting)
  const helper = useReader((s) => s.helper)
  const failure = useReader((s) => s.failure)
  const clearDraft = useReader((s) => s.clearDraft)
  const { reading, actions } = useReaderContext()
  const systemReduce = useReducedMotion()
  const reduce = reading?.reduce_motion === 'on' || (reading?.reduce_motion !== 'off' && systemReduce)

  const [indexOpen, setIndexOpen] = useState(false)
  const [search, setSearch] = useState('')
  const anchor = useRef<{ height: number; top: number } | null>(null)
  const nodes = timeline?.nodes ?? []
  // 草稿已提交但打字机还没显示完时，先不显示时间线里的同一个节点（避免内容跳变）
  let visible = draft?.nodeId ? nodes.filter((n) => n.id !== draft.nodeId) : nodes
  visible = cutAfter(visible, draft?.parentId ?? pendingParent)
  const latestTurn = visible.findLast((n) => n.kind === 'turn')
  const busy = pending !== null || draft !== null
  const draftCommittedInTimeline = draft?.nodeId !== undefined && nodes.some((n) => n.id === draft.nodeId)

  // ---- 滚动跟随
  const scrollRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const following = useRef(true)
  const [showJump, setShowJump] = useState(false)

  const onScroll = () => {
    const el = scrollRef.current!
    following.current = el.scrollHeight - el.scrollTop - el.clientHeight < FOLLOW_THRESHOLD
    if (following.current) setShowJump(false)
  }
  const jumpToLatest = () => {
    const el = scrollRef.current!
    el.scrollTo({ top: el.scrollHeight, behavior: reduce ? 'instant' : 'smooth' })
    following.current = true
    setShowJump(false)
  }
  useLayoutEffect(() => {
    const el = scrollRef.current
    const content = contentRef.current
    if (!el || !content) return
    const ro = new ResizeObserver(() => {
      if (following.current) el.scrollTop = el.scrollHeight
      else setShowJump(true)
    })
    ro.observe(content)
    return () => ro.disconnect()
  }, [])
  // 打开存档时定位到最新
  useEffect(() => {
    if (timeline && scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight
  }, [timeline?.save_id]) // eslint-disable-line react-hooks/exhaustive-deps

  const earlier = () => {
    const el = scrollRef.current
    if (el) anchor.current = { height: el.scrollHeight, top: el.scrollTop }
    following.current = false
    loadEarlier()
  }
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (anchor.current && el && !loadingEarlier) {
      el.scrollTop = anchor.current.top + el.scrollHeight - anchor.current.height
      anchor.current = null
    }
  }, [timeline?.nodes, loadingEarlier])
  const jump = (id: string) => {
    following.current = false
    contentRef.current?.querySelector<HTMLElement>(`[data-node="${id}"]`)?.scrollIntoView({ block: 'start' })
    setShowJump(true)
    setIndexOpen(false)
  }
  const entries = visible
    .filter((n) => n.kind === 'turn')
    .map((n) => ({
      id: n.id,
      no: n.turn_no,
      text: n.display
        .filter((d) => d.t === 'text')
        .map((d) => d.markup)
        .join(' ')
        .replace(/<[^>]*>/g, '')
        .slice(0, 120),
    }))
    .filter((n) => `${n.no} ${n.text}`.includes(search))
  return (
    <div className="relative flex min-h-0 flex-1">
      <Button
        variant="ghost"
        size="icon-sm"
        className="absolute top-1 left-2 z-20 xl:hidden"
        aria-label={t('reader.turnIndex')}
        onClick={() => setIndexOpen(!indexOpen)}
      >
        <List />
      </Button>
      <nav
        aria-label={t('reader.turnIndex')}
        className={cn(
          'absolute inset-y-0 left-0 z-10 w-52 shrink-0 overflow-y-auto border-r border-rule bg-paper/95 p-3 pt-10 backdrop-blur xl:relative xl:block xl:bg-transparent xl:pt-4',
          !indexOpen && 'hidden',
        )}
      >
        <p className="mb-3 font-serif text-sm text-ink-muted">{t('reader.turnIndex')}</p>
        <Input
          aria-label={t('reader.findTurn')}
          placeholder={t('reader.findTurn')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="mb-3 h-8 text-xs"
        />
        {timeline?.has_more && (
          <Button variant="ghost" size="sm" disabled={loadingEarlier} onClick={earlier}>
            {t('reader.loadEarlier')}
          </Button>
        )}
        <ol className="space-y-1">
          {entries.map((n) => (
            <li key={n.id}>
              <button
                className="w-full rounded-lg p-2 text-left text-xs leading-5 text-ink-muted hover:bg-hover focus-visible:bg-hover"
                onClick={() => jump(n.id)}
              >
                <span className="block text-gold-text">{t('reader.turnNumber', { no: n.no })}</span>
                <span className="line-clamp-2">{n.text}</span>
              </button>
            </li>
          ))}
        </ol>
      </nav>
      <div className="relative min-w-0 flex-1">
        {/* 上下两端渐隐：正文滚到顶栏与输入栏下面时柔和地淡出，而不是被一刀切断 */}
        <div
          ref={scrollRef}
          onScroll={onScroll}
          className="scroll-stable absolute inset-0 overscroll-contain [mask-image:linear-gradient(to_bottom,transparent,#000_28px,#000_calc(100%-36px),transparent)]"
        >
          <div
            ref={contentRef}
            className="mx-auto w-full max-w-[calc(var(--reading-width)+3rem)] px-6 pt-10 pb-14"
            style={{ fontSize: 'var(--reading-size)' }}
          >
            {opening}
            {timeline?.has_more && (
              <div className="mb-6 text-center">
                <Button variant="ghost" size="sm" disabled={loadingEarlier} onClick={earlier}>
                  {t('reader.loadEarlier')}
                </Button>
              </div>
            )}
            {visible.map((n, i) =>
              n.kind === 'turn' ? (
                <TurnFrame key={n.id} node={n} latest={n === latestTurn} disabled={busy} spaced={i > 0}>
                  <TurnView
                    items={n.display}
                    // 已选择、等待新回合开始期间：选项仍显示，所选项高亮、其余变淡
                    interactive={n === latestTurn && (!busy || (!!pending?.choice_id && !draft))}
                    selected={n === latestTurn ? pending?.choice_id : undefined}
                    noticeActions={
                      n === latestTurn && !busy && n.status === 'interrupted' ? (
                        <>
                          <Continue onClick={onContinue} />
                          <SmallButton onClick={() => void actions.regenerate(n.id, n.parent_id, undefined, displayedInput(n.display))}>
                            {t('reader.regenerate')}
                          </SmallButton>
                          <SmallButton onClick={() => void actions.discard(n.id)}>{t('reader.discard')}</SmallButton>
                        </>
                      ) : undefined
                    }
                  />
                </TurnFrame>
              ) : (
                <MetaNode key={n.id} node={n} />
              ),
            )}

            {/* 回到了更早的回合：继续游玩会开启新分支 */}
            {timeline?.has_later && !busy && latestTurn && (
              <Notice
                actions={
                  <SmallButton onClick={() => void actions.switchTo(timeline.head_id)}>{t('reader.backToLatestProgress')}</SmallButton>
                }
              >
                {t('reader.branchHint', { turn: latestTurn.turn_no })}
              </Notice>
            )}

            {/* 已提交输入、等待回合开始：先显示玩家的输入与等待态（整理期间排队） */}
            {pending && !draft && (
              <section className="mt-[2.4em]">
                <TurnView items={inputItems(pending, latestTurn?.display)} />
                <Waiting label={compacting ? t('reader.queued') : t('reader.waiting')} />
              </section>
            )}

            {draft && (
              <section className={visible.length > 0 ? 'mt-[2.4em]' : undefined}>
                <DraftTurnView
                  draft={draft}
                  typewriter={reduce ? 'instant' : (reading?.typewriter ?? 'normal')}
                  onRevealed={() => draftCommittedInTimeline && clearDraft()}
                />
              </section>
            )}

            {helper && <QuietLine>{t(helper === 'curate' ? 'reader.helperCurate' : 'reader.helperResearch')}</QuietLine>}
            {failure && !busy && (
              <Notice level="error" actions={failure.recoverable ? <RetryButton onClick={onRetry} /> : undefined}>
                {failure.message}
              </Notice>
            )}
          </div>
        </div>

        {showJump && (
          <Button
            size="sm"
            variant="outline"
            onClick={jumpToLatest}
            className="absolute bottom-3 left-1/2 z-10 -translate-x-1/2 rounded-full bg-paper-overlay/90 shadow-overlay backdrop-blur"
          >
            <ArrowDown />
            {t('reader.backToLatest')}
          </Button>
        )}
      </div>
    </div>
  )
}

/** 进行中的回合：打字机逐字显示；正文全部显示后才出现选项等后续内容。 */
function DraftTurnView({
  draft,
  typewriter,
  onRevealed,
}: {
  draft: DraftTurn
  typewriter: 'instant' | 'normal' | 'slow'
  onRevealed: () => void
}) {
  const { t } = useTranslation()
  const total = draft.items.reduce((n, it) => n + (it.t === 'text' ? it.markup.length : 0), 0)
  const { ui } = useReaderContext()
  const source = ui.builtin_tags.includes('pause')
    ? draft.items
        .filter((it) => it.t === 'text')
        .map((it) => it.markup)
        .join('')
    : ''
  const { shown, done, revealAll } = useTypewriter(total, typewriter, draft.turnId, source)

  useEffect(() => {
    if (done && draft.status !== 'streaming') onRevealed()
  }, [done, draft.status, onRevealed])

  // 按已显示的字符数截取显示项
  const items: DisplayItem[] = []
  let remaining = shown
  for (const it of draft.items) {
    if (it.t === 'text') {
      if (remaining <= 0 && items.length > 0) break
      const take = Math.min(remaining, it.markup.length)
      items.push({ t: 'text', markup: it.markup.slice(0, take) })
      remaining -= take
      if (take < it.markup.length) break
    } else if (remaining >= 0 && (done || draft.status === 'streaming')) {
      if (it.t === 'choices' && !done) break
      items.push(it)
    }
  }
  const streaming = draft.status === 'streaming'

  return (
    <div onClick={revealAll}>
      <TurnView items={[...inputItems(draft.input), ...items]} drafting={streaming || !done} interactive={!streaming && done} />
      {streaming && total === 0 && <Waiting label={draft.toolsRunning > 0 ? t('reader.toolsWorking') : t('reader.waiting')} />}
      {streaming && total > 0 && draft.toolsRunning > 0 && <QuietLine>{t('reader.toolsWorking')}……</QuietLine>}
    </div>
  )
}

/** 草稿与等待态中的玩家输入行（开场没有输入行）。 */
function inputItems(input: TurnInput, prevDisplay?: DisplayItem[]): DisplayItem[] {
  if (input.kind === 'opening') return []
  let text = input.text
  if (input.kind === 'choice') {
    text = input.choice_label
    if (!text) {
      const choices = prevDisplay?.findLast((d) => d.t === 'choices')
      text = choices?.t === 'choices' ? choices.items.find((c) => c.id === input.choice_id)?.label : input.choice_id
    }
  } else if (input.kind === 'command' && input.command) {
    text = ['/' + input.command.name, ...Object.values(input.command.args ?? {}), input.text].filter(Boolean).join(' ')
  } else if (input.kind === 'continue') {
    text = text || '继续'
  }
  return [{ t: 'input', kind: input.kind, mode: input.mode, text }]
}

function Continue({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation()
  return <SmallButton onClick={onClick}>{t('reader.continue')}</SmallButton>
}

function SmallButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <Button size="xs" variant="outline" onClick={onClick}>
      {children}
    </Button>
  )
}

/** 编辑节点（玩家修改了状态或记忆）与整理节点在正文流中的样子：一行注记、一条分隔线。 */
function MetaNode({ node }: { node: TimelineNode }) {
  const { t } = useTranslation()
  if (node.kind === 'edit') {
    return (
      <div data-node={node.id}>
        {(node.notes ?? []).map((note, i) => (
          <QuietLine key={i}>{note}</QuietLine>
        ))}
      </div>
    )
  }
  if (node.kind !== 'compaction') return null
  const detail =
    node.summary_from && node.summary_to
      ? t('reader.compactionDetail', { from: node.summary_from, to: node.summary_to, keep: node.keep_from_turn })
      : undefined
  return (
    <div data-node={node.id} className="my-8 flex items-center gap-3 font-sans text-xs text-ink-faint" title={detail}>
      <span aria-hidden className="h-px flex-1 bg-rule" />
      <span>◇ {t('reader.compactionDivider')}</span>
      <span aria-hidden className="h-px flex-1 bg-rule" />
    </div>
  )
}

/**
 * 只显示到 parentId 为止：重新生成、修改输入时新回合的父节点较早，父节点之后的回合先隐去。
 * 父节点是 root（重新生成第一个回合）时它不在时间线中，以"父节点为它的回合"为界。
 */
function cutAfter(nodes: TimelineNode[], parentId: string | null | undefined): TimelineNode[] {
  if (!parentId) return nodes
  const i = nodes.findIndex((n) => n.id === parentId)
  if (i >= 0) return nodes.slice(0, i + 1)
  const j = nodes.findIndex((n) => n.parent_id === parentId)
  return j >= 0 ? nodes.slice(0, j) : nodes
}

function RetryButton({ onClick }: { onClick: () => void }): ReactNode {
  const { t } = useTranslation()
  return (
    <Button size="xs" variant="outline" onClick={onClick}>
      {t('common.retry')}
    </Button>
  )
}

/** 触屏长按打开回合菜单，移动或触及交互控件时取消。 */
function TurnFrame({
  node,
  latest,
  disabled,
  spaced,
  children,
}: {
  node: TimelineNode
  latest: boolean
  disabled: boolean
  spaced: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const pointer = useRef<{ x: number; y: number; timer: number } | null>(null)
  const clear = () => {
    if (pointer.current) window.clearTimeout(pointer.current.timer)
    pointer.current = null
  }
  useEffect(
    () => () => {
      if (pointer.current) window.clearTimeout(pointer.current.timer)
    },
    [],
  )
  return (
    <section
      className={cn('group/turn', spaced && 'mt-[2.4em]')}
      data-node={node.id}
      data-turn={node.turn_no}
      onPointerDown={(e) => {
        if (e.pointerType !== 'touch' || (e.target as HTMLElement).closest('button,a,input,textarea')) return
        clear()
        pointer.current = {
          x: e.clientX,
          y: e.clientY,
          timer: window.setTimeout(() => {
            setOpen(true)
            pointer.current = null
          }, 550),
        }
      }}
      onPointerMove={(e) => {
        const p = pointer.current
        if (p && Math.hypot(e.clientX - p.x, e.clientY - p.y) > 10) clear()
      }}
      onPointerUp={clear}
      onPointerCancel={clear}
    >
      <TurnToolbar node={node} latest={latest} disabled={disabled} open={open} onOpenChange={setOpen} />
      {children}
    </section>
  )
}
