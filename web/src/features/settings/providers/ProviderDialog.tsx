/**
 * 添加 / 编辑供应商（《04-交互与界面设计》9.1）：
 *
 *   form  协议、名称、Base URL（预填官方地址）、API Key、价格倍率、额外请求头（折叠）
 *   pick  拉取模型列表勾选，或手动填写模型 ID（已知模型由后端按预设填好能力与官方价格）
 *   test  连接测试逐项结果
 *
 * 添加时依次走完三步；从列表进入时可以直接打开 pick（添加模型）或 test（测试连接）。
 * 每次打开都重新挂载（调用方在关闭时卸载），状态直接由 props 初始化。
 */
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Plus } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { useModels, useProviderMutations, useSettings, useUpdateSettings } from '@/api/queries'
import type { ProviderView } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import { defaultBaseUrl, formatHeaders, parseHeaders, PROTOCOLS, type RealProtocol } from './protocols'
import { TestReportView } from './TestReportView'

export type ProviderStep = 'form' | 'pick' | 'test'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 编辑或从列表进入 pick / test 时传入 */
  provider?: ProviderView
  step?: ProviderStep
  /** test 步骤使用的模型（已添加的模型 ID） */
  testModelId?: string
}

export function ProviderDialog({ open, onOpenChange, provider, step: initialStep = 'form', testModelId }: Props) {
  const { t } = useTranslation()
  const [step, setStep] = useState<ProviderStep>(initialStep)
  const [current, setCurrent] = useState<ProviderView | undefined>(provider)
  const [modelId, setModelId] = useState<string | undefined>(testModelId)
  const adding = !provider

  const title =
    step === 'pick'
      ? t('settings.providers.pick.title')
      : step === 'test'
        ? t('settings.providers.testing.title')
        : adding
          ? t('settings.providers.form.addTitle')
          : t('settings.providers.form.editTitle')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="title-book">{title}</DialogTitle>
          <DialogDescription className={cn(step !== 'pick' && 'sr-only')}>
            {step === 'pick' ? t('settings.providers.pick.desc') : title}
          </DialogDescription>
        </DialogHeader>
        {step === 'form' && (
          <ProviderForm
            provider={current}
            onSaved={(p) => {
              setCurrent(p)
              if (adding) setStep('pick')
              else onOpenChange(false)
            }}
            onCancel={() => onOpenChange(false)}
          />
        )}
        {step === 'pick' && current && (
          <ModelPicker
            provider={current}
            onDone={(firstAdded) => {
              if (firstAdded) {
                setModelId(firstAdded)
                setStep('test')
              } else onOpenChange(false)
            }}
          />
        )}
        {step === 'test' && current && <ProviderTest provider={current} modelId={modelId} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function ProviderForm({
  provider,
  onSaved,
  onCancel,
}: {
  provider?: ProviderView
  onSaved: (p: ProviderView) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const { createProvider, updateProvider } = useProviderMutations()
  const [protocol, setProtocol] = useState<RealProtocol>((provider?.protocol as RealProtocol) ?? 'anthropic')
  const [name, setName] = useState(provider?.name ?? '')
  const [baseUrl, setBaseUrl] = useState(provider?.base_url ?? defaultBaseUrl('anthropic'))
  const [apiKey, setApiKey] = useState('')
  const [multiplier, setMultiplier] = useState(String(provider?.price_multiplier ?? 1))
  const [headers, setHeaders] = useState(formatHeaders(provider?.headers))
  const [localError, setLocalError] = useState('')
  const mutation = provider ? updateProvider : createProvider

  const pickProtocol = (p: RealProtocol) => {
    // 还是上一个协议的默认地址时跟着换；用户改过就保留
    if (baseUrl === defaultBaseUrl(protocol) || baseUrl === '') setBaseUrl(defaultBaseUrl(p))
    setProtocol(p)
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setLocalError('')
    const h = parseHeaders(headers)
    const m = Number(multiplier)
    if (h === null) return setLocalError(t('settings.providers.form.headersHint'))
    if (!Number.isFinite(m) || m <= 0) return setLocalError(t('settings.providers.form.multiplierHint'))
    const input = { name, base_url: baseUrl, api_key: apiKey || undefined, headers: h, price_multiplier: m }
    if (provider) {
      updateProvider.mutate({ id: provider.id, input }, { onSuccess: onSaved })
    } else {
      createProvider.mutate({ ...input, protocol }, { onSuccess: onSaved })
    }
  }

  return (
    <form onSubmit={submit} className="space-y-6">
      <div className="space-y-2">
        <Label>{t('settings.providers.form.protocol')}</Label>
        <RadioGroup value={protocol} onValueChange={(v) => pickProtocol(v as RealProtocol)} disabled={!!provider} className="gap-2">
          {PROTOCOLS.map((p) => (
            <label
              key={p.id}
              className={cn(
                'flex cursor-pointer items-start gap-3 rounded-xl border border-rule px-4 py-3 transition-colors has-[[data-state=checked]]:border-rule-strong has-[[data-state=checked]]:bg-selected',
                provider && 'cursor-default opacity-70',
              )}
            >
              <RadioGroupItem value={p.id} className="mt-0.5" />
              <span className="min-w-0">
                <span className="block text-sm text-ink">{p.label}</span>
                <span className="mt-0.5 block text-xs leading-5 text-ink-faint">{t(`settings.providers.protocols.${p.id}`)}</span>
              </span>
            </label>
          ))}
        </RadioGroup>
      </div>

      <div className="space-y-2">
        <Label htmlFor="prov-name">{t('settings.providers.form.name')}</Label>
        <Input
          id="prov-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('settings.providers.form.namePlaceholder')}
          required
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="prov-url">{t('settings.providers.form.baseUrl')}</Label>
        <Input id="prov-url" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} className="font-mono text-[13px]" required />
        <p className="text-xs leading-5 text-ink-faint">{t('settings.providers.form.baseUrlHint')}</p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="prov-key">{t('settings.providers.form.apiKey')}</Label>
        <Input
          id="prov-key"
          type="password"
          autoComplete="off"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={provider ? `${provider.key_masked}（${t('settings.providers.form.apiKeyKeep')}）` : 'sk-…'}
          required={!provider}
          className="font-mono text-[13px]"
        />
        <p className="text-xs leading-5 text-ink-faint">{t('settings.providers.form.apiKeyHint')}</p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="prov-mult">{t('settings.providers.form.multiplier')}</Label>
        <Input
          id="prov-mult"
          inputMode="decimal"
          value={multiplier}
          onChange={(e) => setMultiplier(e.target.value)}
          className="tabular w-28"
        />
        <p className="text-xs leading-5 text-ink-faint">{t('settings.providers.form.multiplierHint')}</p>
      </div>

      <Collapsible defaultOpen={!!provider?.headers}>
        <CollapsibleTrigger className="group flex items-center gap-1 text-[13px] text-ink-muted hover:text-ink">
          <ChevronRight className="size-3.5 transition-transform group-data-[state=open]:rotate-90" />
          {t('settings.providers.form.advanced')}
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-2 pt-3">
          <Textarea
            value={headers}
            onChange={(e) => setHeaders(e.target.value)}
            rows={3}
            className="font-mono text-[13px]"
            placeholder="anthropic-beta: …"
          />
          <p className="text-xs leading-5 text-ink-faint">{t('settings.providers.form.headersHint')}</p>
        </CollapsibleContent>
      </Collapsible>

      {(localError || mutation.isError) && (
        <p role="alert" className="rounded-xl border border-danger/30 bg-danger/8 px-4 py-3 text-[13px] leading-6 text-ink">
          {localError || errorMessage(mutation.error)}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={mutation.isPending} className="min-w-24">
          {provider ? t('settings.providers.form.saveOnly') : t('settings.providers.form.save')}
        </Button>
      </DialogFooter>
    </form>
  )
}

function ModelPicker({ provider, onDone }: { provider: ProviderView; onDone: (firstAddedId?: string) => void }) {
  const { t } = useTranslation()
  const models = useModels()
  const settings = useSettings()
  const updateSettings = useUpdateSettings()
  const { createModel } = useProviderMutations()
  const remote = useQuery({
    queryKey: ['providers', provider.id, 'remote-models'],
    queryFn: () => api.providers.remoteModels(provider.id),
    retry: false,
    staleTime: 60_000,
  })
  const [filter, setFilter] = useState('')
  const [manual, setManual] = useState('')
  const [chosen, setChosen] = useState<string[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const existing = useMemo(
    () => new Set((models.data ?? []).filter((m) => m.provider_id === provider.id).map((m) => m.name)),
    [models.data, provider.id],
  )
  const ids = useMemo(() => {
    const list = (remote.data ?? []).map((m) => m.id)
    for (const c of chosen) if (!list.includes(c)) list.unshift(c)
    const f = filter.trim().toLowerCase()
    return f ? list.filter((id) => id.toLowerCase().includes(f)) : list
  }, [remote.data, chosen, filter])

  const toggle = (id: string) => setChosen((c) => (c.includes(id) ? c.filter((x) => x !== id) : [...c, id]))
  const addManual = () => {
    const id = manual.trim()
    if (id && !chosen.includes(id) && !existing.has(id)) setChosen((c) => [...c, id])
    setManual('')
  }

  const confirm = async () => {
    setBusy(true)
    setError('')
    let first: string | undefined
    try {
      for (const name of chosen) {
        const m = await createModel.mutateAsync({ provider_id: provider.id, name })
        first ??= m.id
      }
      // 还没有主笔时，把第一个添加的模型设为主笔，添加完就能开始游玩
      if (first && !settings.data?.models.narrator) await updateSettings.mutateAsync({ models: { narrator: first } })
      onDone(first)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      {remote.isPending && <p className="text-sm text-ink-muted">{t('settings.providers.pick.loading')}</p>}
      {remote.isError && <p className="text-[13px] leading-6 text-warn">{t('settings.providers.pick.listFailed')}</p>}
      {remote.data && remote.data.length > 0 && (
        <>
          <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('settings.providers.pick.filter')} />
          <ul className="max-h-64 divide-y divide-rule overflow-y-auto rounded-xl border border-rule">
            {ids.map((id) => {
              const has = existing.has(id)
              return (
                <li key={id}>
                  <label className={cn('flex items-center gap-3 px-4 py-2.5 text-sm', has ? 'text-ink-faint' : 'cursor-pointer text-ink')}>
                    <input
                      type="checkbox"
                      className="size-4 accent-[var(--color-primary)]"
                      disabled={has}
                      checked={has || chosen.includes(id)}
                      onChange={() => toggle(id)}
                    />
                    <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{id}</span>
                    {has && <span className="text-xs">{t('settings.providers.pick.added')}</span>}
                  </label>
                </li>
              )
            })}
          </ul>
        </>
      )}

      <div className="space-y-2">
        <Label htmlFor="pick-manual">{t('settings.providers.pick.manual')}</Label>
        <div className="flex gap-2">
          <Input
            id="pick-manual"
            value={manual}
            onChange={(e) => setManual(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                addManual()
              }
            }}
            placeholder={t('settings.providers.pick.manualPlaceholder')}
            className="font-mono text-[13px]"
          />
          <Button
            type="button"
            variant="outline"
            onClick={addManual}
            disabled={!manual.trim()}
            aria-label={t('settings.providers.pick.manual')}
          >
            <Plus />
          </Button>
        </div>
        {chosen.length > 0 && <p className="font-mono text-xs leading-5 break-all text-ink-muted">{chosen.join('、')}</p>}
      </div>

      {error && (
        <p role="alert" className="rounded-xl border border-danger/30 bg-danger/8 px-4 py-3 text-[13px] leading-6 text-ink">
          {error}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={() => onDone()}>
          {t('settings.providers.pick.skip')}
        </Button>
        <Button type="button" onClick={() => void confirm()} disabled={busy || chosen.length === 0} className="min-w-24">
          {t('settings.providers.pick.confirm', { count: chosen.length })}
        </Button>
      </DialogFooter>
    </div>
  )
}

function ProviderTest({ provider, modelId, onDone }: { provider: ProviderView; modelId?: string; onDone: () => void }) {
  const { t } = useTranslation()
  const models = useModels()
  const { testProvider } = useProviderMutations()
  const model = models.data?.find((m) => m.id === modelId)
  const { mutate } = testProvider
  const started = useRef('')

  // 每次测试都会真实调用模型（产生费用）：同一供应商与模型只自动测试一次（StrictMode 下 effect 会执行两遍）。
  useEffect(() => {
    const key = `${provider.id}/${modelId ?? ''}`
    if (started.current === key) return
    started.current = key
    mutate({ id: provider.id, model_id: modelId })
  }, [mutate, provider.id, modelId])

  return (
    <div className="space-y-4">
      <TestReportView report={testProvider.data} running={testProvider.isPending} model={model?.name} />
      {testProvider.isError && (
        <p role="alert" className="rounded-xl border border-danger/30 bg-danger/8 px-4 py-3 text-[13px] leading-6 text-ink">
          {errorMessage(testProvider.error)}
        </p>
      )}
      <DialogFooter>
        <Button
          type="button"
          variant="ghost"
          disabled={testProvider.isPending}
          onClick={() => testProvider.mutate({ id: provider.id, model_id: modelId })}
        >
          {t('settings.providers.testing.again')}
        </Button>
        <Button type="button" onClick={onDone} className="min-w-24">
          {t('settings.providers.testing.done')}
        </Button>
      </DialogFooter>
    </div>
  )
}
