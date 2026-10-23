import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { formatTokens } from '@/lib/format'
import { cn } from '@/lib/utils'

/**
 * 上下文用量环（《04-交互与界面设计》8.10）：当前上下文 ÷ 整理阈值；超过 90% 变为提醒色。
 * 点开是一个小浮层：用量、已整理次数与"立即整理"。整理进行中时环旁显示"正在整理记忆"。
 * TODO(M5-7)：浮层中显示本回合各类 token、费用与累计费用。
 */
export function ContextRing({
  used,
  threshold,
  compacting,
  compactions,
  onCompact,
  busy,
}: {
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
      <PopoverContent align="end" className="w-72 text-[13px]">
        {used !== undefined && threshold ? (
          <p className="text-ink tabular">{t('reader.contextUsage', { used: formatTokens(used), threshold: formatTokens(threshold) })}</p>
        ) : null}
        <p className="mt-1 text-xs leading-5 text-ink-muted">{t('reader.contextHint')}</p>
        <div className="mt-3 flex items-center justify-between gap-3">
          <span className="text-xs text-ink-faint">{t('reader.compactions', { count: compactions })}</span>
          <Button size="xs" variant="outline" disabled={compacting || busy} onClick={onCompact}>
            {compacting ? t('reader.compacting') : t('reader.compactNow')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
