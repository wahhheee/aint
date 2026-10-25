/**
 * 模型设置（《04-交互与界面设计》9.2）：窗口、输出上限、能力、推理与缓存方式、价格。
 * 已知模型添加时已按内置预设填好；这里给需要修正的人用（如中转站的价格、能力差异）。
 */
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { errorMessage } from '@/api/errors'
import { useModelPreset, useProviderMutations } from '@/api/queries'
import type { Caps, Model, Price, Reasoning } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { parsePrice } from './protocols'

const LEVELS: Reasoning[] = ['off', 'low', 'medium', 'high']
const PRICE_FIELDS = ['input', 'cache_read', 'cache_write', 'cache_write_long', 'output'] as const
type PriceField = (typeof PRICE_FIELDS)[number]
const PRICE_LABEL: Record<PriceField, 'input' | 'cacheRead' | 'cacheWrite' | 'cacheWriteLong' | 'output'> = {
  input: 'input',
  cache_read: 'cacheRead',
  cache_write: 'cacheWrite',
  cache_write_long: 'cacheWriteLong',
  output: 'output',
}

/** 每次打开都重新挂载（调用方在关闭时卸载），表单状态直接由模型初始化。 */
export function ModelDialog({ model, onOpenChange }: { model: Model; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const { updateModel } = useProviderMutations()
  const preset = useModelPreset(model.id)
  const [displayName, setDisplayName] = useState(model.display_name ?? '')
  const [contextWindow, setContextWindow] = useState(String(model.context_window))
  const [maxOutput, setMaxOutput] = useState(String(model.max_output))
  const [caps, setCaps] = useState<Caps>({ ...model.caps, reasoning_map: { ...model.caps.reasoning_map } })
  const [currency, setCurrency] = useState(model.price?.currency || 'USD')
  const [prices, setPrices] = useState<Record<PriceField, string>>(
    () => Object.fromEntries(PRICE_FIELDS.map((f) => [f, model.price?.[f]?.toString() ?? ''])) as Record<PriceField, string>,
  )

  const setCap = <K extends keyof Caps>(k: K, v: Caps[K]) => setCaps((c) => ({ ...c, [k]: v }))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const price: Price = { currency }
    for (const f of PRICE_FIELDS) {
      const v = parsePrice(prices[f])
      if (v !== undefined) price[f] = v
    }
    const hasPrice = PRICE_FIELDS.some((f) => price[f] !== undefined)
    const map = Object.fromEntries(Object.entries(caps.reasoning_map ?? {}).filter(([, v]) => v && v.trim() !== ''))
    updateModel.mutate(
      {
        id: model.id,
        input: {
          display_name: displayName,
          context_window: Number(contextWindow) || undefined,
          max_output: Number(maxOutput) || undefined,
          caps: { ...caps, reasoning_map: map },
          ...(hasPrice ? { price } : { clear_price: true }),
        },
      },
      { onSuccess: () => onOpenChange(false) },
    )
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="title-book">{t('settings.providers.model.title')}</DialogTitle>
          <DialogDescription className="font-mono text-[13px]">{model.name}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-7">
          {preset.data && (
            <div className="space-y-2 rounded-xl border border-rule p-3">
              <p className="text-[13px] text-ink-muted">{t('settings.providers.model.presetHint', { revision: preset.data.revision })}</p>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    const p = preset.data!
                    setContextWindow(String(p.context_window))
                    setMaxOutput(String(p.max_output))
                    setCaps({ ...p.caps, reasoning_map: { ...p.caps.reasoning_map } })
                  }}
                >
                  {t('settings.providers.model.applyPresetCaps')}
                </Button>
                {preset.data.price && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      const price = preset.data!.price!
                      setCurrency(price.currency || 'USD')
                      setPrices(Object.fromEntries(PRICE_FIELDS.map((f) => [f, price[f]?.toString() ?? ''])) as Record<PriceField, string>)
                    }}
                  >
                    {t('settings.providers.model.applyPresetPrice')}
                  </Button>
                )}
              </div>
            </div>
          )}
          {preset.isError && (
            <p role="alert" className="text-[13px] text-danger">
              {errorMessage(preset.error)}
            </p>
          )}
          <div className="space-y-2">
            <Label htmlFor="model-name">{t('settings.providers.model.displayName')}</Label>
            <Input id="model-name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={model.name} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="model-ctx">{t('settings.providers.model.contextWindow')}</Label>
              <Input
                id="model-ctx"
                inputMode="numeric"
                value={contextWindow}
                onChange={(e) => setContextWindow(e.target.value)}
                className="tabular"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="model-out">{t('settings.providers.model.maxOutput')}</Label>
              <Input
                id="model-out"
                inputMode="numeric"
                value={maxOutput}
                onChange={(e) => setMaxOutput(e.target.value)}
                className="tabular"
              />
            </div>
          </div>

          <fieldset className="space-y-4">
            <legend className="mb-3 w-full border-b border-rule pb-2 text-[13px] font-medium text-ink-muted">
              {t('settings.providers.model.caps')}
            </legend>
            {(
              [
                ['vision', 'vision'],
                ['image_in_tool_result', 'imageInToolResult'],
                ['parallel_tools', 'parallelTools'],
              ] as const
            ).map(([k, label]) => (
              <div key={k} className="flex items-center justify-between gap-4">
                <Label htmlFor={`cap-${k}`}>{t(`settings.providers.model.${label}`)}</Label>
                <Switch id={`cap-${k}`} checked={caps[k]} onCheckedChange={(v) => setCap(k, v)} />
              </div>
            ))}
            <div className="flex items-center justify-between gap-4">
              <Label>{t('settings.providers.model.reasoning')}</Label>
              <Select value={caps.reasoning} onValueChange={(v) => setCap('reasoning', v as Caps['reasoning'])}>
                <SelectTrigger className="w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(['effort', 'budget', 'level', 'none'] as const).map((r) => (
                    <SelectItem key={r} value={r}>
                      {t(`settings.providers.model.styles.${r}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {caps.reasoning !== 'none' && (
              <div className="space-y-2">
                <Label>{t('settings.providers.model.reasoningMap')}</Label>
                <div className="grid grid-cols-4 gap-2">
                  {LEVELS.map((l) => (
                    <div key={l} className="space-y-1">
                      <span className="text-xs text-ink-faint">{t(`reasoning.${l}`)}</span>
                      <Input
                        value={caps.reasoning_map?.[l] ?? ''}
                        onChange={(e) => setCap('reasoning_map', { ...caps.reasoning_map, [l]: e.target.value })}
                        className="h-8 px-2 font-mono text-xs"
                      />
                    </div>
                  ))}
                </div>
                <p className="text-xs leading-5 text-ink-faint">{t('settings.providers.model.reasoningMapHint')}</p>
              </div>
            )}
            <div className="flex items-center justify-between gap-4">
              <Label>{t('settings.providers.model.cache')}</Label>
              <Select value={caps.cache} onValueChange={(v) => setCap('cache', v as Caps['cache'])}>
                <SelectTrigger className="w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(['breakpoints', 'implicit', 'none'] as const).map((c) => (
                    <SelectItem key={c} value={c}>
                      {t(`settings.providers.model.caches.${c}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </fieldset>

          <fieldset className="space-y-4">
            <legend className="mb-3 w-full border-b border-rule pb-2 text-[13px] font-medium text-ink-muted">
              {t('settings.providers.model.price')}
            </legend>
            <div className="flex items-center justify-between gap-4">
              <Label>{t('settings.providers.model.currency')}</Label>
              <Select value={currency} onValueChange={setCurrency}>
                <SelectTrigger className="w-28">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="USD">USD $</SelectItem>
                  <SelectItem value="CNY">CNY ¥</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-2 gap-x-4 gap-y-3">
              {PRICE_FIELDS.map((f) => (
                <div key={f} className="space-y-1">
                  <Label htmlFor={`price-${f}`} className="text-xs text-ink-muted">
                    {t(`settings.providers.model.${PRICE_LABEL[f]}`)}
                  </Label>
                  <Input
                    id={`price-${f}`}
                    inputMode="decimal"
                    value={prices[f]}
                    onChange={(e) => setPrices((p) => ({ ...p, [f]: e.target.value }))}
                    className="tabular h-8"
                  />
                </div>
              ))}
            </div>
            <p className="text-xs leading-5 text-ink-faint">{t('settings.providers.model.priceHint')}</p>
          </fieldset>

          {updateModel.isError && (
            <p role="alert" className="rounded-xl border border-danger/30 bg-danger/8 px-4 py-3 text-[13px] leading-6 text-ink">
              {errorMessage(updateModel.error)}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={updateModel.isPending} className="min-w-24">
              {t('settings.providers.form.saveOnly')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
