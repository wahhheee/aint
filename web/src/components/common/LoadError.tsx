import { useTranslation } from 'react-i18next'

import { ApiError, errorMessage } from '@/api/errors'
import { Button } from '@/components/ui/button'

/** 数据加载失败：说明原因、给出重试；有请求编号时一并显示（便于在日志中定位）。 */
export function LoadError({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation()
  return (
    <div role="alert" className="rounded-xl border border-l-2 border-rule border-l-danger bg-paper-raised px-5 py-4">
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
  )
}
