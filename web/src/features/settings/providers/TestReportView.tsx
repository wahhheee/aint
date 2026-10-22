/**
 * 连接测试结果：逐项显示通过 / 失败 / 跳过，失败项可展开原始错误。
 */
import { CircleCheck, CircleMinus, CircleX, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { TestReport } from '@/api/types'
import { cn } from '@/lib/utils'

export function TestReportView({ report, running, model }: { report?: TestReport; running: boolean; model?: string }) {
  const { t } = useTranslation()
  if (running) {
    return (
      <p className="flex items-center gap-2 py-6 text-sm text-ink-muted">
        <LoaderCircle className="size-4 animate-spin" />
        {t('settings.providers.testing.running', { model: model ?? '' })}
      </p>
    )
  }
  if (!report) return null
  return (
    <div className="space-y-4">
      <p className="text-[13px] text-ink-faint">{t('settings.providers.testing.model', { model: report.model })}</p>
      <ul className="divide-y divide-rule rounded-xl border border-rule">
        {report.steps.map((st) => (
          <li key={st.name} className="flex gap-3 px-4 py-3">
            {st.skipped ? (
              <CircleMinus className="mt-0.5 size-4 shrink-0 text-ink-faint" />
            ) : st.ok ? (
              <CircleCheck className="mt-0.5 size-4 shrink-0 text-ok" />
            ) : (
              <CircleX className="mt-0.5 size-4 shrink-0 text-danger" />
            )}
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline justify-between gap-3 text-sm">
                <span className="text-ink">{t(`settings.providers.testing.steps.${st.name}`)}</span>
                {st.latency_ms !== undefined && st.latency_ms > 0 && (
                  <span className="tabular text-xs text-ink-faint">{(st.latency_ms / 1000).toFixed(1)}s</span>
                )}
              </div>
              <p className={cn('mt-0.5 text-xs leading-5', st.ok || st.skipped ? 'text-ink-muted' : 'text-danger')}>{st.message}</p>
              {st.detail && !st.ok && !st.skipped && (
                <pre className="mt-1.5 max-h-28 overflow-auto rounded-md bg-paper-sunken px-2 py-1.5 font-mono text-[11px] leading-4 break-all whitespace-pre-wrap text-ink-faint">
                  {st.detail}
                </pre>
              )}
            </div>
          </li>
        ))}
      </ul>
      <p className={cn('text-[13px]', report.ok ? 'text-ok' : 'text-danger')}>
        {report.ok ? t('settings.providers.testing.passed') : t('settings.providers.testing.failed')}
      </p>
    </div>
  )
}
