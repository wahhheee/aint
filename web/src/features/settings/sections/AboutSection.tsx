import { useTranslation } from 'react-i18next'

import { useSystemInfo } from '@/api/queries'

import { SectionTitle, SettingRow } from './SettingRow'

/** 关于：版本、提示词版本、平台、调试开关的状态（反馈问题时一并提供）。 */
export function AboutSection() {
  const { t } = useTranslation()
  const { data } = useSystemInfo()
  if (!data) return null
  const b = data.build
  return (
    <section>
      <SectionTitle>{t('settings.sections.about')}</SectionTitle>
      <SettingRow label={t('settings.about.version')}>
        <span className="font-mono text-[13px] text-ink-muted">
          {b.version}
          {b.commit && ` (${b.commit}${b.modified ? '*' : ''})`}
        </span>
      </SettingRow>
      <SettingRow label={t('settings.about.prompts')}>
        <span className="font-mono text-[13px] text-ink-muted">{data.prompt_version}</span>
      </SettingRow>
      <SettingRow label={t('settings.about.platform')}>
        <span className="font-mono text-[13px] text-ink-muted">
          {b.platform} · {b.go_version}
        </span>
      </SettingRow>
      <SettingRow label={t('settings.about.trace')}>
        <span className="text-[13px] text-ink-muted">{data.trace_llm ? t('settings.about.traceOn') : t('settings.about.traceOff')}</span>
      </SettingRow>
    </section>
  )
}
