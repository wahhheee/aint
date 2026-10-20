import { useTranslation } from 'react-i18next'

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatTokens } from '@/lib/format'
import { cn } from '@/lib/utils'

/**
 * 上下文用量环（《04-交互与界面设计》8.10）：当前上下文 ÷ 整理阈值；超过 90% 变为提醒色。
 * TODO(M5-7)：悬停浮层中显示本回合各类 token、费用、累计费用与"立即整理"。
 */
export function ContextRing({ used, threshold }: { used?: number; threshold?: number }) {
  const { t } = useTranslation()
  const ratio = used !== undefined && threshold ? Math.min(1, used / threshold) : 0
  const r = 7
  const c = 2 * Math.PI * r
  const warn = ratio > 0.9
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-xs text-ink-faint tabular" tabIndex={0}>
          <svg viewBox="0 0 18 18" className="size-[18px] -rotate-90" aria-hidden>
            <circle cx="9" cy="9" r={r} fill="none" stroke="var(--rule)" strokeWidth="2" />
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
          <span className={cn(warn && 'text-warn')}>{used !== undefined ? `${Math.round(ratio * 100)}%` : '—'}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-64">
        {used !== undefined && threshold ? (
          <p className="tabular">{t('reader.contextUsage', { used: formatTokens(used), threshold: formatTokens(threshold) })}</p>
        ) : null}
        <p className="mt-1 opacity-80">{t('reader.contextHint')}</p>
      </TooltipContent>
    </Tooltip>
  )
}
