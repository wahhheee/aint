import { useTranslation } from 'react-i18next'

import { useSystemInfo } from '@/api/queries'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

/** 关于：版本、提示词版本、平台、调试开关的状态（反馈问题时一并提供）。 */
export function AboutSection() {
  const { t } = useTranslation()
  const { data } = useSystemInfo()
  if (!data) return null
  const b = data.build
  const value = (v: string) => <span className="font-mono text-xs text-ink-muted select-all">{v}</span>
  return (
    <section>
      <SectionTitle description={t('settings.about.intro')}>{t('settings.sections.about')}</SectionTitle>
      <SettingGroup>
        <SettingRow label={t('settings.about.version')}>
          {value(`${b.version}${b.commit ? ` (${b.commit}${b.modified ? '*' : ''})` : ''}`)}
        </SettingRow>
        <SettingRow label={t('settings.about.prompts')}>{value(data.prompt_version)}</SettingRow>
        <SettingRow label={t('settings.about.platform')}>{value(`${b.platform}  ${b.go_version}`)}</SettingRow>
        <SettingRow label={t('settings.about.trace')}>
          <span className="text-[13px] text-ink-muted">{data.trace_llm ? t('settings.about.traceOn') : t('settings.about.traceOff')}</span>
        </SettingRow>
      </SettingGroup>
    </section>
  )
}
