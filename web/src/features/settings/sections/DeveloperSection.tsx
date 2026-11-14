import { useTranslation } from 'react-i18next'

import { useSettings, useUpdateSettings } from '@/api/queries'
import { Switch } from '@/components/ui/switch'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

/** 开发者选项：状态编辑工具与游玩调试记录的默认行为。 */
export function DeveloperSection() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
  if (!data) return null
  return (
    <section>
      <SectionTitle description={t('settings.developer.intro')}>{t('settings.sections.developer')}</SectionTitle>
      <SettingGroup>
        <SettingRow label={t('settings.developer.autoDebug')} hint={t('settings.developer.autoDebugHint')}>
          <Switch
            checked={data.play.auto_debug}
            onCheckedChange={(auto_debug) => update.mutate({ play: { auto_debug } })}
            aria-label={t('settings.developer.autoDebug')}
          />
        </SettingRow>
        <SettingRow label={t('settings.developer.enable')} hint={t('settings.developer.enableHint')}>
          <Switch
            checked={data.developer}
            onCheckedChange={(developer) => update.mutate({ developer })}
            aria-label={t('settings.developer.enable')}
          />
        </SettingRow>
      </SettingGroup>
    </section>
  )
}
