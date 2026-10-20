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

import { SectionTitle } from './SettingRow'

const PROTOCOL_LABEL = { 'openai-responses': 'OpenAI Responses', anthropic: 'Anthropic Messages', gemini: 'Gemini', fake: '演示模型' }

export function ProvidersSection() {
  const { t } = useTranslation()
  const info = useSystemInfo()
  const providers = useProviders()
  const models = useModels()
  const settings = useSettings()

  return (
    <section>
      <SectionTitle description={t('settings.providers.intro')}>{t('settings.sections.providers')}</SectionTitle>

      {info.data?.fake_llm && (
        <p className="mb-6 rounded-lg border-l-2 border-gold bg-paper-sunken px-4 py-3 text-[13px] text-ink-muted">{t('app.fakeHint')}</p>
      )}

      {providers.isError && <LoadError error={providers.error} />}
      {providers.data && providers.data.length === 0 && (
        <div className="rounded-xl border border-dashed border-rule px-6 py-10 text-center">
          <p className="text-ink-muted">{t('settings.providers.empty')}</p>
          <Button className="mt-4" onClick={() => toast(t('common.notImplemented'))}>
            <Plus />
            {t('settings.providers.add')}
          </Button>
        </div>
      )}
      {providers.data && providers.data.length > 0 && (
        <ul className="divide-y divide-rule">
          {providers.data.map((p) => (
            <li key={p.id} className="flex items-baseline gap-4 py-3">
              <span className="font-medium">{p.name}</span>
              <span className="text-xs text-ink-muted">{PROTOCOL_LABEL[p.protocol]}</span>
              <span className="ml-auto font-mono text-xs text-ink-faint">{p.key_masked}</span>
            </li>
          ))}
        </ul>
      )}

      {models.data && models.data.length > 0 && (
        <div className="mt-10">
          <h3 className="mb-3 font-serif text-[13px] tracking-[0.16em] text-ink-muted">{t('settings.providers.models')}</h3>
          <ul className="divide-y divide-rule">
            {models.data.map((m) => (
              <li key={m.id} className="flex items-baseline gap-4 py-3">
                <span>{m.display_name || m.name}</span>
                <span className="text-xs text-ink-faint tabular">
                  {formatTokens(m.context_window)} / {formatTokens(m.max_output)}
                </span>
                {m.caps.vision && <span className="text-xs text-ink-muted">看图</span>}
                {(settings.data?.models.narrator === m.id || info.data?.fake_llm) && (
                  <span className="ml-auto text-xs text-gold-text">{t('settings.providers.narrator')}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
