import { useTranslation } from 'react-i18next'
import { useSettings, useUpdateSettings } from '@/api/queries'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

export function AudioSection() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
  if (!data) return null
  return (
    <section>
      <SectionTitle description={t('audio.intro')}>{t('settings.sections.audio')}</SectionTitle>
      <SettingGroup>
        <SettingRow label={t('audio.muted')}>
          <Switch
            aria-label={t('audio.muted')}
            checked={data.audio.muted}
            onCheckedChange={(muted) => update.mutate({ audio: { muted } })}
          />
        </SettingRow>
        {(['master', 'bgm', 'sfx'] as const).map((k) => (
          <SettingRow key={k} label={t(`audio.${k}`)}>
            <Slider
              className="w-40"
              aria-label={t(`audio.${k}`)}
              min={0}
              max={1}
              step={0.05}
              defaultValue={[data.audio[k]]}
              onValueCommit={([v]) => update.mutate({ audio: { [k]: v } })}
            />
          </SettingRow>
        ))}
      </SettingGroup>
    </section>
  )
}
