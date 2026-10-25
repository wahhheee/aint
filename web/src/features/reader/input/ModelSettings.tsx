import { ChevronDown } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useModels, useProviders } from '@/api/queries'
import type { Save, SaveSettings } from '@/api/types'
import { confirmAction, useConfirmStore } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Slider } from '@/components/ui/slider'
import { useSaveSettings } from '@/features/save/useSaveSettings'
import { modelLabel } from '@/lib/modelLabel'

const LEVELS = ['off', 'low', 'medium', 'high'] as const
const selectClass = 'h-9 w-full rounded-lg border border-rule bg-paper px-2 text-sm text-ink'

/** 存档运行参数：空闲时保存，从下一回合生效。 */
export function ModelSettings({ save }: { save: Save }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const models = useModels()
  const providers = useProviders()
  const model = models.data?.find((m) => m.id === save.settings.narrator_model)
  const label = model ? modelLabel(model, providers.data) : save.settings.narrator_model
  const provider = providers.data?.find((p) => p.id === model?.provider_id)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="min-w-0 shrink justify-start gap-1 px-2 font-normal text-ink-muted"
          aria-label={t('playSettings.model')}
          title={label}
        >
          <span className="min-w-0 text-left leading-tight">
            <span className="block truncate">{model?.display_name || model?.name || save.settings.narrator_model}</span>
            <span className="block truncate text-[10px] text-ink-faint">
              {provider?.name || model?.provider_id} · {t('newGame.reasoning')} {t(`reasoning.${save.settings.reasoning}`)}
            </span>
          </span>
          <ChevronDown className="size-3 shrink-0" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        onInteractOutside={(e) => {
          if (useConfirmStore.getState().req) e.preventDefault()
        }}
        align="start"
        side="top"
        className="max-h-[80dvh] w-80 max-w-[calc(100vw-1rem)] overflow-y-auto"
      >
        {open && <ModelSettingsForm save={save} onDone={() => setOpen(false)} />}
      </PopoverContent>
    </Popover>
  )
}

function ModelSettingsForm({ save, onDone }: { save: Save; onDone: () => void }) {
  const { t } = useTranslation()
  const models = useModels()
  const providers = useProviders()
  const { update, disabled } = useSaveSettings(save.id, onDone)
  const [value, setValue] = useState(save.settings)
  const model = models.data?.find((m) => m.id === value.narrator_model)
  const patch = (v: Partial<SaveSettings>) => setValue((s) => ({ ...s, ...v }))
  const submit = async () => {
    if (value.narrator_model !== save.settings.narrator_model && !(await confirmAction(t('playSettings.switchWarning')))) return
    update.mutate({ narrator_model: value.narrator_model, reasoning: value.reasoning })
  }
  return (
    <div className="space-y-5">
      <div>
        <h3 className="font-serif text-base">{t('playSettings.model')}</h3>
        <p className="mt-1 text-xs leading-5 text-ink-muted">{t('playSettings.hint')}</p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="play-model">{t('newGame.model')}</Label>
        <select
          id="play-model"
          className={selectClass}
          value={value.narrator_model}
          onChange={(e) => patch({ narrator_model: e.target.value })}
        >
          {!models.data?.some((m) => m.id === value.narrator_model) && <option value={value.narrator_model}>{value.narrator_model}</option>}
          {models.data?.map((m) => (
            <option key={m.id} value={m.id}>
              {modelLabel(m, providers.data)}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-3">
        <div className="flex justify-between">
          <Label>{t('newGame.reasoning')}</Label>
          <span className="text-sm text-gold-text">{t(`reasoning.${value.reasoning}`)}</span>
        </div>
        <Slider
          aria-label={t('newGame.reasoning')}
          aria-valuetext={t(`reasoning.${value.reasoning}`)}
          min={0}
          max={3}
          step={1}
          value={[Math.max(0, LEVELS.indexOf(value.reasoning))]}
          onValueChange={([v]) => patch({ reasoning: LEVELS[v] })}
          disabled={model?.caps.reasoning === 'none'}
        />
        <div className="flex justify-between text-xs text-ink-faint">
          {LEVELS.map((l) => (
            <span key={l}>{t(`reasoning.${l}`)}</span>
          ))}
        </div>
        {model?.caps.reasoning === 'none' && <p className="text-xs text-ink-muted">{t('playSettings.noReasoning')}</p>}
      </div>
      <Button className="w-full" onClick={() => void submit()} disabled={disabled}>
        {t('common.save')}
      </Button>
    </div>
  )
}
