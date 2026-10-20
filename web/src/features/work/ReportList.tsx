import { useTranslation } from 'react-i18next'

import type { Issue } from '@/api/types'
import { cn } from '@/lib/utils'

/** 校验报告：每条问题带文件路径与行号，以及修复建议。 */
export function ReportList({ issues }: { issues: Issue[] }) {
  const { t } = useTranslation()
  if (issues.length === 0) return <p className="text-ink-muted">{t('work.reportClean')}</p>
  return (
    <ul className="space-y-2">
      {issues.map((i, idx) => (
        <li
          key={idx}
          className={cn(
            'rounded-lg border-l-2 bg-paper-raised px-4 py-2.5',
            i.level === 'error' ? 'border-danger' : i.level === 'warning' ? 'border-warn' : 'border-info',
          )}
        >
          <p className="text-ink">{i.message}</p>
          <p className="mt-0.5 text-xs text-ink-faint">
            <span className="font-mono">{i.code}</span>
            {i.path && (
              <span className="ml-3">
                {i.path}
                {i.line ? `:${i.line}` : ''}
              </span>
            )}
            {i.hint && <span className="ml-3 text-ink-muted">{i.hint}</span>}
          </p>
        </li>
      ))}
    </ul>
  )
}
