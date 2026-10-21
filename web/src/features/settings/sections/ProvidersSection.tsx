/**
 * 模型供应商与模型（《04-交互与界面设计》9.1–9.3）。
 * TODO(M1-11 / M4-7)：添加向导（协议 → 名称 / Base URL / Key → 连接测试逐项结果 → 拉取模型列表）、
 * 模型卡片（窗口、最大输出、能力、价格）、模型角色（主笔 / 轻量）。
 */
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useModels, useProviders, useSettings, useSystemInfo } from '@/api/queries'
import { LoadError } from '@/components/common/LoadError'
import { Button } from '@/components/ui/button'
import { formatTokens } from '@/lib/format'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

const PROTOCOL_LABEL = { 'openai-responses': 'OpenAI Responses', anthropic: 'Anthropic Messages', gemini: 'Gemini', fake: '演示模型' }

export function ProvidersSection() {
  const { t } = useTranslation()
  const info = useSystemInfo()
  const providers = useProviders()
  const models = useModels()
  const settings = useSettings()
  const todo = () => toast(t('common.notImplemented'))

  return (
    <section>
      <SectionTitle description={t('settings.providers.intro')}>{t('settings.sections.providers')}</SectionTitle>

      {info.data?.fake_llm && (
        <p className="mb-8 flex gap-3 rounded-xl border border-rule bg-paper-raised px-4 py-3 text-[13px] leading-6 text-ink-muted">
          <span aria-hidden className="mt-2.5 size-1.5 shrink-0 rounded-full bg-gold" />
          {t('app.fakeHint')}
        </p>
      )}

      {providers.isError && <LoadError error={providers.error} />}
      {providers.data && (
        <SettingGroup
          title={t('settings.providers.services')}
          action={
            providers.data.length > 0 && (
              <Button variant="ghost" size="xs" onClick={todo}>
                <Plus />
                {t('settings.providers.add')}
              </Button>
            )
          }
        >
          {providers.data.length === 0 ? (
            <div className="flex flex-col items-center px-6 py-10 text-center">
              <p className="text-ink-muted">{t('settings.providers.empty')}</p>
              <Button className="mt-4" size="sm" onClick={todo}>
                <Plus />
                {t('settings.providers.add')}
              </Button>
            </div>
          ) : (
            providers.data.map((p) => (
              <SettingRow key={p.id} label={p.name} hint={PROTOCOL_LABEL[p.protocol]}>
                <span className="font-mono text-xs text-ink-faint">{p.key_masked}</span>
              </SettingRow>
            ))
          )}
        </SettingGroup>
      )}

      {models.data && models.data.length > 0 && (
        <SettingGroup title={t('settings.providers.models')}>
          {models.data.map((m) => (
            <SettingRow
              key={m.id}
              label={m.display_name || m.name}
              hint={
                <span className="tabular">
                  {t('settings.providers.window', { window: formatTokens(m.context_window), output: formatTokens(m.max_output) })}
                  {m.caps.vision && <span className="ml-3">{t('settings.providers.vision')}</span>}
                </span>
              }
            >
              {(settings.data?.models.narrator === m.id || info.data?.fake_llm) && (
                <span className="rounded-md bg-gold-soft px-1.5 py-0.5 text-xs leading-4 text-gold-text">
                  {t('settings.providers.narrator')}
                </span>
              )}
            </SettingRow>
          ))}
        </SettingGroup>
      )}
    </section>
  )
}
