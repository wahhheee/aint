/**
 * 上下文（《04-交互与界面设计》9.4）：整理上下文的默认参数。存档设置可以单独覆盖（save.Settings）。
 */
import { useTranslation } from 'react-i18next'

import { useModels, useSettings, useSystemInfo, useUpdateSettings } from '@/api/queries'
import type { ContextSettings } from '@/api/types'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { formatTokens } from '@/lib/format'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

const BUDGETS = [32_000, 64_000, 128_000, 256_000]
const KEEPS = [6_000, 12_000, 24_000]

export function ContextSection() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const models = useModels()
  const system = useSystemInfo()
  const update = useUpdateSettings()
  if (!data) return null
  const ctx = data.context
  const lightModel = models.data?.find((m) => m.id === data.models.light)
  const patch = (c: Partial<ContextSettings>) => update.mutate({ context: c })

  const tokens = (key: 'budget' | 'keep_recent', values: number[]) => (
    <ToggleGroup type="single" variant="segmented" value={String(ctx[key])} onValueChange={(v) => v && patch({ [key]: Number(v) })}>
      {/* 当前值不在预设里时（直接改过设置）也显示出来 */}
      {(values.includes(ctx[key]) ? values : [...values, ctx[key]].sort((a, b) => a - b)).map((v) => (
        <ToggleGroupItem key={v} value={String(v)} className="tabular">
          {formatTokens(v)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )

  return (
    <section>
      <SectionTitle description={t('settings.context.intro')}>{t('settings.sections.context')}</SectionTitle>
      <SettingGroup>
        <SettingRow label={t('settings.context.budget')} hint={t('settings.context.budgetHint')}>
          {tokens('budget', BUDGETS)}
        </SettingRow>
        <SettingRow label={t('settings.context.keepRecent')} hint={t('settings.context.keepRecentHint')}>
          {tokens('keep_recent', KEEPS)}
        </SettingRow>
        <SettingRow
          label={t('settings.context.mode')}
          hint={
            lightModel
              ? t('settings.context.modeHint', { model: lightModel.display_name || lightModel.name })
              : t('settings.context.noLight')
          }
        >
          <ToggleGroup
            type="single"
            variant="segmented"
            value={ctx.mode}
            onValueChange={(v) => v && patch({ mode: v as ContextSettings['mode'] })}
          >
            <ToggleGroupItem value="inline">{t('settings.context.inline')}</ToggleGroupItem>
            <ToggleGroupItem value="separate" disabled={!lightModel && ctx.mode !== 'separate'}>
              {t('settings.context.separate')}
            </ToggleGroupItem>
          </ToggleGroup>
        </SettingRow>
        {(['max_calls', 'max_tool_calls', 'max_output'] as const).map((k) => (
          <SettingRow key={k} label={t(`playSettings.${k}`)} hint={t('playSettings.zeroHint')}>
            <Input
              key={`${k}-${ctx[k]}`}
              aria-label={t(`playSettings.${k}`)}
              type="number"
              min={0}
              max={k === 'max_calls' ? 50 : k === 'max_tool_calls' ? 200 : 128000}
              className="w-28"
              defaultValue={ctx[k] || ''}
              placeholder={system.data?.turn_defaults[k].toString()}
              onBlur={(e) => {
                if (e.currentTarget.validity.valid && Number(e.target.value) !== ctx[k]) patch({ [k]: Number(e.target.value) })
              }}
            />
          </SettingRow>
        ))}
        <SettingRow label={t('settings.context.toolHints')}>
          <Switch
            aria-label={t('settings.context.toolHints')}
            checked={data.reading.tool_hints}
            onCheckedChange={(v) => update.mutate({ reading: { tool_hints: v } })}
          />
        </SettingRow>
      </SettingGroup>
    </section>
  )
}
