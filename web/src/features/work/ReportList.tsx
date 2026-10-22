import { useTranslation } from 'react-i18next'

import type { Issue } from '@/api/types'
import { cn } from '@/lib/utils'

/** 校验报告：每条问题带级别、文件路径与行号，以及修复建议。 */
export function ReportList({ issues, className }: { issues: Issue[]; className?: string }) {
  const { t } = useTranslation()
  if (issues.length === 0) return <p className="text-ink-muted">{t('work.reportClean')}</p>
  return (
    <ul className={cn('divide-y divide-rule overflow-hidden rounded-xl border border-rule bg-paper-raised', className)}>
      {issues.map((i, idx) => (
        <li key={idx} className="flex gap-3 px-4 py-3">
          <span
            aria-hidden
            className={cn(
              'mt-2 size-1.5 shrink-0 rounded-full',
              i.level === 'error' ? 'bg-danger' : i.level === 'warning' ? 'bg-warn' : 'bg-info',
            )}
          />
          <div className="min-w-0">
            <p className="text-ink">{i.message}</p>
            <p className="mt-1 flex flex-wrap gap-x-3 text-xs leading-5 text-ink-faint">
              <span className={cn(i.level === 'error' ? 'text-danger' : i.level === 'warning' ? 'text-warn' : 'text-info')}>
                {t(`work.levels.${i.level}`)}
              </span>
              <span className="font-mono">{i.code}</span>
              {i.path && (
                <span className="font-mono">
                  {i.path}
                  {i.line ? `:${i.line}` : ''}
                </span>
              )}
              {i.hint && <span className="text-ink-muted">{i.hint}</span>}
            </p>
          </div>
        </li>
      ))}
    </ul>
  )
}
