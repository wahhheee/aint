import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useNodeUsage, useSaveUsage } from '@/api/queries'
import type { Usage } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { formatCosts, formatTokens } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useReader } from '@/stores/reader'

/**
 * 上下文用量环（《04-交互与界面设计》8.10）：当前上下文 ÷ 整理阈值；超过 90% 变为提醒色。
 * 点开是一个小浮层：上下文用量；本回合各类 token 与费用；本存档的累计费用、其中整理的花费与整理次数；"立即整理"。
 * 整理进行中时环旁显示"正在整理记忆"。
 * TODO(M5-9)：浮层中"查看详情"打开检查器。
 */
export function ContextRing({
  saveId,
  lastTurnId,
  used,
  threshold,
  compacting,
  compactions,
  onCompact,
  busy,
}: {
  saveId: string
  /** 进度线上最近的回合（时间线最后一个回合节点） */
  lastTurnId?: string
  used?: number
  threshold?: number
  compacting: boolean
  compactions: number
  onCompact: () => void
  /** 回合进行中：不能手动整理 */
  busy: boolean
}) {
  const { t } = useTranslation()
  const ratio = used !== undefined && threshold ? Math.min(1, used / threshold) : 0
  const r = 7
  const c = 2 * Math.PI * r
  const warn = ratio > 0.9
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="inline-flex h-8 items-center gap-1.5 rounded-lg px-2 text-xs text-ink-muted tabular transition-colors hover:bg-hover"
          aria-label={t('reader.contextHint')}
        >
          <svg viewBox="0 0 18 18" className={cn('size-[18px] -rotate-90', compacting && 'animate-pulse')} aria-hidden>
            <circle cx="9" cy="9" r={r} fill="none" stroke="var(--rule-strong)" strokeWidth="2" />
            <circle
              cx="9"
              cy="9"
              r={r}
              fill="none"
              stroke={warn ? 'var(--warn)' : 'var(--ink-muted)'}
              strokeWidth="2"
              strokeLinecap="round"
              strokeDasharray={`${ratio * c} ${c}`}
              className="transition-[stroke-dasharray] duration-300"
            />
          </svg>
          {compacting ? (
            <span className="text-gold-text">{t('reader.compacting')}</span>
          ) : (
            <span className={cn('w-8 text-left', warn && 'text-warn')}>
              {used !== undefined && threshold ? `${Math.round(ratio * 100)}%` : '—'}
            </span>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[min(24rem,calc(100vw-1rem))] text-[13px]">
        {/* 浮层内容只在打开时挂载：累计用量在打开时才请求 */}
        <UsageDetails saveId={saveId} lastTurnId={lastTurnId} used={used} threshold={threshold} compactions={compactions} />
        <p className="text-xs leading-5 text-ink-muted">{t('reader.contextHint')}</p>
        <div className="flex justify-end">
          <Button size="xs" variant="outline" disabled={compacting || busy} onClick={onCompact}>
            {compacting ? t('reader.compacting') : t('reader.compactNow')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

/**
 * 浮层中的用量明细。本回合：进行中与本页面刚完成的回合来自 turn.usage；打开存档或回到较早的回合后，
 * 按进度线上最近的回合向用量接口查询（静态开场等没有模型调用的回合不显示这一行）。
 * 本存档：用量接口的累计（含重新生成、已删除分支上花费的调用），其中整理的花费单独列出。
 */
function UsageDetails({
  saveId,
  lastTurnId,
  used,
  threshold,
  compactions,
}: {
  saveId: string
  lastTurnId?: string
  used?: number
  threshold?: number
  compactions: number
}) {
  const { t } = useTranslation()
  const live = useReader((s) => s.turnUsage)
  const draftId = useReader((s) => s.draft?.turnId)
  const streaming = useReader((s) => s.draft?.status === 'streaming')
  const turnId = draftId ?? lastTurnId
  const liveTurn = live && live.turnId === turnId ? live : null
  const node = useNodeUsage(saveId, turnId, !liveTurn && !streaming)
  const turn = liveTurn ?? (node.data?.total.calls ? node.data.total : null)
  const saved = useSaveUsage(saveId)
  const total = saved.data?.total
  const compact = saved.data?.by_kind.compact

  // 费用：按货币分别列出；全部调用都没有价格时显示"费用：未配置价格"
  const cost = ({ costs, unpriced }: { costs: Record<string, number>; unpriced: number }) => {
    const amount = formatCosts(costs)
    if (!amount) return t('reader.usage.unpriced')
    return t(unpriced > 0 ? 'reader.usage.costPartial' : 'reader.usage.cost', { amount })
  }
  const tokens = (u: Usage) => ({
    total: formatTokens(u.input_uncached + u.cache_read + u.cache_write),
    read: formatTokens(u.cache_read),
    write: formatTokens(u.cache_write),
    uncached: formatTokens(u.input_uncached),
    output: formatTokens(u.output),
    reasoning: formatTokens(u.reasoning),
  })

  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 leading-5 tabular">
      {used !== undefined && threshold ? (
        <UsageRow label={t('reader.usage.context')}>
          <p>{t('reader.usage.contextValue', { used: formatTokens(used), threshold: formatTokens(threshold) })}</p>
        </UsageRow>
      ) : null}
      {turn ? (
        <UsageRow label={t('reader.usage.turn')}>
          <p>
            {t('reader.usage.input', tokens(turn.usage))}
            {/* 放不下时缓存明细整体换行 */}
            <span className="inline-block">{t('reader.usage.inputDetail', tokens(turn.usage))}</span>
          </p>
          <p>
            {t('reader.usage.output', tokens(turn.usage))} · {cost(turn)}
          </p>
        </UsageRow>
      ) : (
        streaming && (
          <UsageRow label={t('reader.usage.turn')}>
            <p className="text-ink-muted">{t('reader.usage.generating')}</p>
          </UsageRow>
        )
      )}
      <UsageRow label={t('reader.usage.save')}>
        <p>{total ? (total.calls ? t('reader.usage.total', { cost: cost(total) }) : t('reader.usage.noCalls')) : '—'}</p>
        <p className="text-xs text-ink-muted">
          {compact && `${t('reader.usage.compactCost', { cost: cost(compact) })} · `}
          {t('reader.compactions', { count: compactions })}
        </p>
      </UsageRow>
    </dl>
  )
}

function UsageRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-xs leading-5 text-ink-faint">{label}</dt>
      <dd className="min-w-0 text-ink">{children}</dd>
    </>
  )
}
