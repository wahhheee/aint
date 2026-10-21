import { useTranslation } from 'react-i18next'

import { ApiError, errorMessage } from '@/api/errors'
import { Button } from '@/components/ui/button'

/** 数据加载失败：说明原因、给出重试；有请求编号时一并显示（便于在日志中定位）。 */
export function LoadError({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation()
  return (
    <div role="alert" className="flex gap-3 rounded-xl border border-rule bg-paper-raised px-5 py-4">
      <span aria-hidden className="mt-2 size-1.5 shrink-0 rounded-full bg-danger" />
      <div className="min-w-0">
        <p className="text-ink">{errorMessage(error)}</p>
        {error instanceof ApiError && error.requestId && (
          <p className="mt-1 text-xs text-ink-faint">
            {t('common.requestId')}：<span className="tabular select-all">{error.requestId}</span>
          </p>
        )}
        {onRetry && (
          <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
            {t('common.retry')}
          </Button>
        )}
      </div>
    </div>
  )
}
