import { useTranslation } from 'react-i18next'
import { Link, useRouteError } from 'react-router'

import { ApiError } from '@/api/errors'
import { Button } from '@/components/ui/button'

/** 路由级错误边界：渲染出错或数据加载失败时显示，并给出排查线索（请求编号、错误信息）。 */
export function RouteError() {
  const err = useRouteError()
  const { t } = useTranslation()
  console.error('[route] 页面出错', err)
  const message = err instanceof Error ? err.message : String(err)
  return (
    <div className="mx-auto max-w-lg px-6 py-[18vh]">
      <h1 className="title-book text-2xl">{t('errors.loadFailed')}</h1>
      <p className="mt-4 text-ink-muted">{message}</p>
      {err instanceof ApiError && err.requestId && (
        <p className="mt-2 text-xs text-ink-faint">
          {t('common.requestId')}：<span className="tabular select-all">{err.requestId}</span>
        </p>
      )}
      <Button asChild className="mt-8">
        <Link to="/">{t('errors.backHome')}</Link>
      </Button>
    </div>
  )
}

export function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="py-[16vh] text-center">
      <p className="title-book text-xl">{t('errors.notFound')}</p>
      <Button asChild variant="outline" className="mt-6">
        <Link to="/">{t('errors.backHome')}</Link>
      </Button>
    </div>
  )
}
