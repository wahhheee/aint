import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'

import { errorMessage } from '@/api/errors'
import { qk, useAccessActions, useAccessStatus } from '@/api/queries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/** 登录前只呈现密码表单；会话失效时卸载所有业务页与事件订阅。 */
export function AccessGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const status = useAccessStatus()
  const { login } = useAccessActions()
  const [password, setPassword] = useState('')
  const qc = useQueryClient()
  useEffect(() => {
    const expired = () => {
      void qc.invalidateQueries({ queryKey: qk.access })
    }
    window.addEventListener('aint:unauthorized', expired)
    return () => window.removeEventListener('aint:unauthorized', expired)
  }, [qc])
  if (status.data?.authenticated)
    return (
      <>
        {status.data.exposed && (
          <div role="alert" className="border-b border-warn/30 bg-paper-sunken px-4 py-2 text-center text-sm text-warn">
            {t('access.exposed')}{' '}
            <a className="underline" href="/settings/access">
              {t('access.configure')}
            </a>
          </div>
        )}
        {children}
      </>
    )
  return (
    <main className="grid min-h-dvh place-items-center bg-paper p-6">
      <form
        className="w-full max-w-sm space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          login.mutate(password, { onSuccess: () => setPassword('') })
        }}
      >
        <h1 className="title-book text-2xl">{t('access.title')}</h1>
        {status.isError ? (
          <>
            <p role="alert" className="text-sm text-danger">
              {errorMessage(status.error)}
            </p>
            <Button type="button" onClick={() => void status.refetch()}>
              {t('common.retry')}
            </Button>
          </>
        ) : status.isPending ? (
          <p>{t('common.loading')}</p>
        ) : (
          <>
            <Label className="grid gap-2">
              {t('access.password')}
              <Input
                type="password"
                autoComplete="current-password"
                autoFocus
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Label>
            {login.error && (
              <p role="alert" className="text-sm text-danger">
                {errorMessage(login.error)}
              </p>
            )}
            <Button className="w-full" disabled={login.isPending}>
              {t('access.login')}
            </Button>
          </>
        )}
      </form>
    </main>
  )
}
