import { useTranslation } from 'react-i18next'
import { useSettings, useSystemInfo } from '@/api/queries'
import type { SaveSettings } from '@/api/types'
import { Input } from '@/components/ui/input'

const selectClass = 'h-9 w-full rounded-lg border border-rule bg-paper px-2 text-sm text-ink'

/** 开局与游玩共用的存档参数；零预算沿用全局设置。 */
export function AdvancedPlaySettings({
  value,
  onChange,
  expanded = false,
}: {
  value: Partial<SaveSettings>
  onChange: (v: Partial<SaveSettings>) => void
  expanded?: boolean
}) {
  const { t } = useTranslation()
  const global = useSettings()
  const system = useSystemInfo()
  const defaults = {
    context_budget: global.data?.context.budget,
    keep_recent: global.data?.context.keep_recent,
    max_calls: global.data?.context.max_calls || system.data?.turn_defaults.max_calls,
    max_tool_calls: global.data?.context.max_tool_calls || system.data?.turn_defaults.max_tool_calls,
    max_output: global.data?.context.max_output || system.data?.turn_defaults.max_output,
  }
  const fields = (
    <div className="space-y-3 text-sm">
      {(['context_budget', 'keep_recent', 'max_calls', 'max_tool_calls', 'max_output'] as const).map((k) => (
        <label key={k} className="block space-y-2">
          <span>{t(`playSettings.${k}`)}</span>
          <Input
            type="number"
            min={0}
            max={k === 'max_calls' ? 50 : k === 'max_tool_calls' ? 200 : k === 'max_output' ? 128000 : 2000000}
            value={value[k] || ''}
            placeholder={defaults[k]?.toString()}
            onChange={(e) => onChange({ [k]: Number(e.target.value) })}
          />
        </label>
      ))}
      <p className="text-xs text-ink-faint">{t('playSettings.zeroHint')}</p>
      <label className="block space-y-2">
        <span>{t('settings.context.mode')}</span>
        <select
          className={selectClass}
          value={value.compact_mode ?? ''}
          onChange={(e) => onChange({ compact_mode: e.target.value as SaveSettings['compact_mode'] })}
        >
          <option value="">{t('playSettings.inherit')}</option>
          <option value="inline">{t('settings.context.inline')}</option>
          <option value="separate">{t('settings.context.separate')}</option>
        </select>
      </label>
      <label className="block space-y-2">
        <span>{t('playSettings.cache')}</span>
        <select
          className={selectClass}
          value={value.cache_ttl || 'short'}
          onChange={(e) => onChange({ cache_ttl: e.target.value as 'short' | 'long' })}
        >
          <option value="short">{t('playSettings.short')}</option>
          <option value="long">{t('playSettings.long')}</option>
        </select>
      </label>
    </div>
  )
  return expanded ? (
    fields
  ) : (
    <details className="text-sm">
      <summary className="cursor-pointer text-ink-muted">{t('playSettings.advanced')}</summary>
      <div className="mt-3">{fields}</div>
    </details>
  )
}
