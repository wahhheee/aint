import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { errorMessage } from '@/api/errors'
import { useSettings, useUpdateSettings } from '@/api/queries'
import { confirmAction } from '@/components/common/confirm'
import { Switch } from '@/components/ui/switch'
import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

export function BackgroundSection() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
  if (!data) return null
  const patch = (play: Partial<typeof data.play>) => update.mutate({ play }, { onError: (err) => toast.error(errorMessage(err)) })
  return (
    <section>
      <SectionTitle description={t('background.intro')}>{t('settings.sections.background')}</SectionTitle>
      <SettingGroup>
        <SettingRow label={t('background.helpers')} hint={t('background.helperHint')}>
          <Switch
            aria-label={t('background.helpers')}
            checked={data.play.helpers}
            disabled={!data.models.light}
            onCheckedChange={(v) =>
              void (async () => {
                if (!v || (await confirmAction(t('background.helperCost')))) patch({ helpers: v })
              })()
            }
          />
        </SettingRow>
        <SettingRow label={t('background.title')} hint={t('background.titleHint')}>
          <select
            aria-label={t('background.title')}
            className="rounded-lg border border-rule bg-paper p-2 text-sm"
            value={data.play.title_on_exit}
            disabled={!data.models.light}
            onChange={(e) => {
              const v = e.target.value as 'off' | 'ask' | 'auto'
              void (async () => {
                if (v !== 'auto' || (await confirmAction(t('background.titleCost')))) patch({ title_on_exit: v })
              })()
            }}
          >
            {(['off', 'ask', 'auto'] as const).map((v) => (
              <option key={v} value={v}>
                {t(`background.${v}`)}
              </option>
            ))}
          </select>
        </SettingRow>
      </SettingGroup>
      {!data.models.light && <p className="mt-4 text-sm text-ink-muted">{t('background.noLight')}</p>}
    </section>
  )
}
