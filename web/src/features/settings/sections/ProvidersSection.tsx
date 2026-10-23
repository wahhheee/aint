/**
 * 模型供应商与模型（《04-交互与界面设计》9.1–9.3）。
 * 添加向导、连接测试、模型设置见 ../providers/；模型角色目前只有主笔（轻量模型随 M5-8 / M5-13）。
 */
import { Ellipsis, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { errorMessage } from '@/api/errors'
import { useModels, useProviderMutations, useProviders, useSettings, useSystemInfo, useUpdateSettings } from '@/api/queries'
import type { Model, ProviderView } from '@/api/types'
import { LoadError } from '@/components/common/LoadError'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatTokens } from '@/lib/format'
import { cn } from '@/lib/utils'

import { ModelDialog } from '../providers/ModelDialog'
import { ProviderDialog, type ProviderStep } from '../providers/ProviderDialog'
import { PROTOCOL_LABEL, priceSummary } from '../providers/protocols'
import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

type Removing = { kind: 'provider'; item: ProviderView } | { kind: 'model'; item: Model }

export function ProvidersSection() {
  const { t } = useTranslation()
  const info = useSystemInfo()
  const providers = useProviders()
  const models = useModels()
  const settings = useSettings()
  const updateSettings = useUpdateSettings()
  const { removeProvider, removeModel } = useProviderMutations()

  const [dialog, setDialog] = useState<{ provider?: ProviderView; step: ProviderStep; modelId?: string } | null>(null)
  const [editing, setEditing] = useState<Model | undefined>()
  const [removing, setRemoving] = useState<Removing | null>(null)
  const fake = info.data?.fake_llm
  const narrator = settings.data?.models.narrator
  const light = settings.data?.models.light
  const providerOf = (m: Model) => providers.data?.find((p) => p.id === m.provider_id)

  const confirmRemove = () => {
    if (!removing) return
    const done = { onSuccess: () => toast(t('settings.providers.removed')), onError: (e: Error) => toast.error(errorMessage(e)) }
    if (removing.kind === 'provider') removeProvider.mutate(removing.item.id, done)
    else removeModel.mutate(removing.item.id, done)
    setRemoving(null)
  }

  const addButton = (size: 'xs' | 'sm') => (
    <Button variant={size === 'xs' ? 'ghost' : 'default'} size={size} onClick={() => setDialog({ step: 'form' })} disabled={fake}>
      <Plus />
      {t('settings.providers.add')}
    </Button>
  )

  return (
    <section>
      <SectionTitle description={t('settings.providers.intro')}>{t('settings.sections.providers')}</SectionTitle>

      {fake && (
        <p className="mb-8 flex gap-3 rounded-xl border border-rule bg-paper-raised px-4 py-3 text-[13px] leading-6 text-ink-muted">
          <span aria-hidden className="mt-2.5 size-1.5 shrink-0 rounded-full bg-gold" />
          {t('app.fakeHint')}
        </p>
      )}

      {providers.isError && <LoadError error={providers.error} />}
      {providers.data && (
        <SettingGroup title={t('settings.providers.services')} action={providers.data.length > 0 && addButton('xs')}>
          {providers.data.length === 0 ? (
            <div className="flex flex-col items-center px-6 py-10 text-center">
              <p className="text-ink-muted">{t('settings.providers.empty')}</p>
              <div className="mt-4">{addButton('sm')}</div>
            </div>
          ) : (
            providers.data.map((p) => (
              <SettingRow
                key={p.id}
                label={p.name}
                hint={
                  <span>
                    {PROTOCOL_LABEL[p.protocol]}
                    <span className="ml-2 font-mono">{p.key_masked}</span>
                    {p.price_multiplier !== 1 && (
                      <span className="ml-2">{t('settings.providers.multiplierTag', { value: p.price_multiplier })}</span>
                    )}
                  </span>
                }
              >
                <TestBadge provider={p} />
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon-xs" aria-label={t('settings.providers.edit')}>
                      <Ellipsis />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onSelect={() =>
                        setDialog({ provider: p, step: 'test', modelId: models.data?.find((m) => m.provider_id === p.id)?.id })
                      }
                    >
                      {t('settings.providers.test')}
                    </DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => setDialog({ provider: p, step: 'pick' })}>
                      {t('settings.providers.addModels')}
                    </DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => setDialog({ provider: p, step: 'form' })}>
                      {t('settings.providers.edit')}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onSelect={() => setRemoving({ kind: 'provider', item: p })}>
                      {t('settings.providers.remove')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </SettingRow>
            ))
          )}
        </SettingGroup>
      )}

      {models.data && (models.data.length > 0 || (providers.data?.length ?? 0) > 0) && (
        <SettingGroup title={t('settings.providers.models')}>
          {models.data.length === 0 && (
            <p className="px-4 py-6 text-center text-[13px] text-ink-muted">{t('settings.providers.noModels')}</p>
          )}
          {models.data.map((m) => {
            const prov = providerOf(m)
            const price = priceSummary(m.price, prov?.price_multiplier ?? 1)
            const isNarrator = narrator === m.id || fake
            return (
              <SettingRow
                key={m.id}
                label={m.display_name || m.name}
                hint={
                  <span className="tabular">
                    {prov && <span className="mr-3">{prov.name}</span>}
                    {t('settings.providers.window', { window: formatTokens(m.context_window), output: formatTokens(m.max_output) })}
                    {m.caps.vision && <span className="ml-3">{t('settings.providers.vision')}</span>}
                    {price && <span className="ml-3">{price}</span>}
                  </span>
                }
              >
                {light === m.id && !fake && (
                  <span
                    className="rounded-md bg-selected px-1.5 py-0.5 text-xs leading-4 text-ink-muted"
                    title={t('settings.providers.lightHint')}
                  >
                    {t('settings.providers.light')}
                  </span>
                )}
                {isNarrator ? (
                  <span className="rounded-md bg-gold-soft px-1.5 py-0.5 text-xs leading-4 text-gold-text">
                    {t('settings.providers.narrator')}
                  </span>
                ) : (
                  <Button variant="ghost" size="xs" onClick={() => updateSettings.mutate({ models: { narrator: m.id } })}>
                    {t('settings.providers.setNarrator')}
                  </Button>
                )}
                {!fake && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-xs" aria-label={t('settings.providers.edit')}>
                        <Ellipsis />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => prov && setDialog({ provider: prov, step: 'test', modelId: m.id })}>
                        {t('settings.providers.test')}
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => setEditing(m)}>{t('settings.providers.edit')}</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => updateSettings.mutate({ models: { light: light === m.id ? '' : m.id } })}>
                        {light === m.id ? t('settings.providers.unsetLight') : t('settings.providers.setLight')}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setRemoving({ kind: 'model', item: m })}>
                        {t('settings.providers.remove')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </SettingRow>
            )
          })}
        </SettingGroup>
      )}

      {dialog && (
        <ProviderDialog
          open
          onOpenChange={(o) => !o && setDialog(null)}
          provider={dialog.provider}
          step={dialog.step}
          testModelId={dialog.modelId}
        />
      )}
      {editing && <ModelDialog model={editing} onOpenChange={(o) => !o && setEditing(undefined)} />}
      <AlertDialog open={removing !== null} onOpenChange={(o) => !o && setRemoving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('settings.providers.removeTitle', {
                name: removing
                  ? removing.kind === 'provider'
                    ? removing.item.name
                    : removing.item.display_name || removing.item.name
                  : '',
              })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {removing?.kind === 'provider' ? t('settings.providers.removeProviderDesc') : t('settings.providers.removeModelDesc')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmRemove}>{t('common.delete')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

function TestBadge({ provider }: { provider: ProviderView }) {
  const { t } = useTranslation()
  const state = provider.test_ok === undefined ? 'untested' : provider.test_ok ? 'ok' : 'failed'
  return (
    <span
      className={cn(
        'rounded-md px-1.5 py-0.5 text-xs leading-4',
        state === 'ok' && 'bg-ok/10 text-ok',
        state === 'failed' && 'bg-danger/10 text-danger',
        state === 'untested' && 'bg-warn/10 text-warn',
      )}
    >
      {state === 'ok'
        ? t('settings.providers.testOk')
        : state === 'failed'
          ? t('settings.providers.testFailed')
          : t('settings.providers.untested')}
    </span>
  )
}
