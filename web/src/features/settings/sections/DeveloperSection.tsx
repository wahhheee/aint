import { useTranslation } from 'react-i18next'

import { useSettings, useUpdateSettings } from '@/api/queries'
import { Switch } from '@/components/ui/switch'

import { SectionTitle, SettingRow } from './SettingRow'

/** 开发者选项（《04-交互与界面设计》9.9，D-31）。默认关闭：普通玩家看不到调试入口。 */
export function DeveloperSection() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
  if (!data) return null
  return (
    <section>
      <SectionTitle description={t('settings.developer.intro')}>{t('settings.sections.developer')}</SectionTitle>
      <SettingRow label={t('settings.developer.enable')} hint={t('settings.developer.enableHint')}>
        <Switch
          checked={data.developer}
          onCheckedChange={(developer) => update.mutate({ developer })}
          aria-label={t('settings.developer.enable')}
        />
      </SettingRow>
    </section>
  )
}
