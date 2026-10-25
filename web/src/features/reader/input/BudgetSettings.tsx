import { SlidersHorizontal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useSettings } from '@/api/queries'
import type { Save, SaveSettings } from '@/api/types'
import { confirmAction, useConfirmStore } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Switch } from '@/components/ui/switch'
import { AdvancedPlaySettings } from '@/features/save/AdvancedPlaySettings'
import { useSaveSettings } from '@/features/save/useSaveSettings'

/** 预算与后台助手独立于模型选择，篇幅由输入栏直接设置。 */
export function BudgetSettings({ save }: { save: Save }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="shrink-0"
          aria-label={t('playSettings.advanced')}
          title={t('playSettings.advanced')}
        >
          <SlidersHorizontal />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        onInteractOutside={(e) => {
          if (useConfirmStore.getState().req) e.preventDefault()
        }}
        align="end"
        side="top"
        className="max-h-[70dvh] w-80 max-w-[calc(100vw-1rem)] overflow-y-auto"
      >
        {open && <BudgetSettingsForm save={save} onDone={() => setOpen(false)} />}
      </PopoverContent>
    </Popover>
  )
}

function BudgetSettingsForm({ save, onDone }: { save: Save; onDone: () => void }) {
  const { t } = useTranslation()
  const global = useSettings()
  const [patch, setPatch] = useState<Partial<SaveSettings>>({})
  const value = { ...save.settings, ...patch }
  const { update, disabled } = useSaveSettings(save.id, onDone)
  const submit = async () => {
    if (value.helpers && !save.settings.helpers && !(await confirmAction(t('background.helperCost')))) return
    update.mutate(patch)
  }
  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <div>
        <h3 className="font-serif text-base">{t('playSettings.advanced')}</h3>
        <p className="mt-1 text-xs leading-5 text-ink-muted">{t('playSettings.hint')}</p>
      </div>
      <AdvancedPlaySettings expanded value={value} onChange={(v) => setPatch((p) => ({ ...p, ...v }))} />
      <label className="flex items-center justify-between gap-3 text-sm">
        <span>{t('background.helpers')}</span>
        <Switch
          checked={value.helpers}
          disabled={!global.data?.models.light}
          onCheckedChange={(helpers) => setPatch((p) => ({ ...p, helpers }))}
        />
      </label>
      <Button type="submit" className="w-full" disabled={disabled}>
        {t('common.save')}
      </Button>
    </form>
  )
}
