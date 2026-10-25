import { useMutation, useQueryClient } from '@tanstack/react-query'
import { SlidersHorizontal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk, useModels, useSettings } from '@/api/queries'
import type { Save, SaveSettings } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Switch } from '@/components/ui/switch'
import { Slider } from '@/components/ui/slider'
import { AdvancedPlaySettings } from '@/features/save/AdvancedPlaySettings'
import { selectBusy, useReader } from '@/stores/reader'

const LEVELS = ['off', 'low', 'medium', 'high'] as const
const selectClass = 'h-9 w-full rounded-lg border border-rule bg-paper px-2 text-sm text-ink'

/** 存档运行参数：空闲时保存，从下一回合生效。 */
export function PlaySettings({ save }: { save: Save }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={t('playSettings.title')}>
          <SlidersHorizontal />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="max-h-[80dvh] w-80 max-w-[calc(100vw-1rem)] overflow-y-auto">
        {open && <PlaySettingsForm save={save} onDone={() => setOpen(false)} />}
      </PopoverContent>
    </Popover>
  )
}

function PlaySettingsForm({ save, onDone }: { save: Save; onDone: () => void }) {
  const { t } = useTranslation()
  const models = useModels()
  const global = useSettings()
  const qc = useQueryClient()
  const busy = useReader(selectBusy)
  const compacting = useReader((s) => s.compacting)
  const [value, setValue] = useState(save.settings)
  const model = models.data?.find((m) => m.id === value.narrator_model)
  const update = useMutation({
    mutationFn: () => api.saves.settings(save.id, value),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.save(save.id) })
      onDone()
    },
    onError: (err) => toast.error(errorMessage(err)),
  })
  const patch = (v: Partial<SaveSettings>) => setValue((s) => ({ ...s, ...v }))
  const submit = async () => {
    if (value.narrator_model !== save.settings.narrator_model && !(await confirmAction(t('playSettings.switchWarning')))) return
    if (value.helpers && !save.settings.helpers && !(await confirmAction(t('background.helperCost')))) return
    update.mutate()
  }
  return (
    <div className="space-y-5">
      <div>
        <h3 className="font-serif text-base">{t('playSettings.title')}</h3>
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
              {m.display_name || m.name}
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
      <label className="flex items-center justify-between gap-3 text-sm">
        <span>{t('background.helpers')}</span>
        <Switch checked={value.helpers} disabled={!global.data?.models.light} onCheckedChange={(v) => patch({ helpers: v })} />
      </label>
      <AdvancedPlaySettings value={value} onChange={patch} />
      <Button className="w-full" onClick={() => void submit()} disabled={busy || compacting || update.isPending}>
        {t('common.save')}
      </Button>
    </div>
  )
}
