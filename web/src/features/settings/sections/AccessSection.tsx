import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { errorMessage } from '@/api/errors'
import { useAccessActions, useAccessStatus, useGitHubToken, useSetGitHubToken, useSettings, useUpdateSettings } from '@/api/queries'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

export function AccessSection() {
  const { t } = useTranslation()
  const status = useAccessStatus()
  const actions = useAccessActions()
  const settings = useSettings()
  const update = useUpdateSettings()
  const github = useGitHubToken()
  const setGitHub = useSetGitHubToken()
  const [current, setCurrent] = useState('')
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [listen, setListen] = useState<string | null>(null)
  const [token, setToken] = useState('')
  const failure = (e: unknown) => toast.error(errorMessage(e))
  const changePassword = (next: string) =>
    actions.password.mutate(
      { current, password: next },
      {
        onError: failure,
        onSuccess: () => {
          setCurrent('')
          setPassword('')
          setRepeat('')
        },
      },
    )
  return (
    <section>
      <SectionTitle description={t('access.hint')}>{t('settings.sections.access')}</SectionTitle>
      <SettingGroup>
        <form
          className="space-y-4 py-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (password !== repeat) {
              toast.error(t('access.mismatch'))
              return
            }
            changePassword(password)
          }}
        >
          {status.data?.enabled && (
            <Label className="grid gap-2">
              {t('access.current')}
              <Input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
            </Label>
          )}
          <Label className="grid gap-2">
            {t('access.newPassword')}
            <Input
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={8}
              required
            />
          </Label>
          <Label className="grid gap-2">
            {t('access.repeat')}
            <Input type="password" autoComplete="new-password" value={repeat} onChange={(e) => setRepeat(e.target.value)} required />
          </Label>
          <div className="flex flex-wrap gap-2">
            <Button disabled={actions.password.isPending}>{t('common.save')}</Button>
            {status.data?.enabled && (
              <Button
                type="button"
                variant="outline"
                disabled={actions.password.isPending || !current}
                onClick={() =>
                  void (async () => {
                    if (await confirmAction(t('access.clearConfirm'))) changePassword('')
                  })()
                }
              >
                {t('access.clear')}
              </Button>
            )}
          </div>
        </form>
        {status.data?.enabled && (
          <SettingRow label={t('access.devices')}>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => actions.logout.mutate(undefined, { onError: failure })}>
                {t('access.logout')}
              </Button>
              <Button
                variant="outline"
                onClick={() =>
                  void (async () => {
                    if (await confirmAction(t('access.revokeConfirm'))) actions.revoke.mutate(undefined, { onError: failure })
                  })()
                }
              >
                {t('access.revoke')}
              </Button>
            </div>
          </SettingRow>
        )}
      </SettingGroup>
      <SettingGroup>
        <SettingRow label={t('access.listen')} hint={t('access.listenHint')}>
          <div className="flex gap-2">
            <Input
              aria-label={t('access.listen')}
              value={listen ?? settings.data?.listen ?? '127.0.0.1'}
              onChange={(e) => setListen(e.target.value)}
            />
            <Button
              variant="outline"
              disabled={update.isPending || listen === null}
              onClick={() => update.mutate({ listen: listen! }, { onError: failure, onSuccess: () => toast(t('access.restart')) })}
            >
              {t('common.save')}
            </Button>
          </div>
        </SettingRow>
      </SettingGroup>
      <SettingGroup>
        <SettingRow label={t('access.github')} hint={t('access.githubHint')}>
          <form
            className="flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              setGitHub.mutate(token, {
                onSuccess: () => {
                  setToken('')
                  toast(t('access.tokenSaved'))
                },
                onError: failure,
              })
            }}
          >
            <Input
              type="password"
              aria-label={t('access.github')}
              autoComplete="off"
              placeholder={github.data?.configured ? t('access.configured') : ''}
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            <Button variant="outline" disabled={setGitHub.isPending || !token}>
              {t('common.save')}
            </Button>
            {github.data?.configured && (
              <Button type="button" variant="ghost" onClick={() => setGitHub.mutate('', { onError: failure })}>
                {t('common.delete')}
              </Button>
            )}
          </form>
        </SettingRow>
      </SettingGroup>
    </section>
  )
}
