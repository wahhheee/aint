/**
 * 阅读与外观（《04-交互与界面设计》9.6）：右侧预览区实时展示示例正文（含样式标签与选项）。
 */
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useSettings, useUpdateSettings } from '@/api/queries'
import type { ReadingSettings, ThemePreference } from '@/api/types'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useReadingStyle } from '@/features/reader/useReadingStyle'

import { SectionTitle, SettingRow } from './SettingRow'

export function ReadingSection() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
  // 拖动中的临时值：拖动时即时预览，松手后写回后端并清除
  const [drag, setDrag] = useState<Partial<ReadingSettings>>({})
  const local = data ? { ...data.reading, ...drag } : undefined
  const style = useReadingStyle(local)
  if (!data || !local) return null

  const patch = (r: Partial<ReadingSettings>) => update.mutate({ reading: r }, { onSettled: () => setDrag({}) })
  const choice = <K extends keyof ReadingSettings>(
    key: K,
    values: readonly ReadingSettings[K][],
    label: (v: ReadingSettings[K]) => string,
  ) => (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={String(local[key])}
      onValueChange={(v) => v && patch({ [key]: v } as Partial<ReadingSettings>)}
    >
      {values.map((v) => (
        <ToggleGroupItem key={String(v)} value={String(v)} className="px-3 text-xs">
          {label(v)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
  const slider = (key: 'font_size' | 'line_height' | 'max_width', min: number, max: number, step: number) => (
    <>
      <span className="w-10 text-right text-xs text-ink-muted tabular">{local[key]}</span>
      <Slider
        className="w-40"
        min={min}
        max={max}
        step={step}
        value={[local[key]]}
        onValueChange={([v]) => setDrag({ [key]: v })}
        onValueCommit={([v]) => patch({ [key]: v })}
      />
    </>
  )

  return (
    <section>
      <SectionTitle>{t('settings.sections.reading')}</SectionTitle>

      {/* 预览 */}
      <div
        className="work-root mb-8 rounded-2xl border border-rule bg-paper px-8 py-7"
        style={style}
        aria-label={t('settings.reading.preview')}
      >
        <div className="prose-book mx-auto max-w-[var(--reading-width)]">
          <p>{t('settings.reading.previewText')}</p>
          <p>
            <span className="mk-say" style={{ '--speaker-color': '#3c8d6e' } as React.CSSProperties}>
              <span className="mk-speaker">林舟</span>「你也收到了那封信？」
            </span>
          </p>
          <p>
            <span className="font-bold text-[#a8770e] dark:text-[#f2c14e]">老陈在午夜前从未离开过柜台。</span>
          </p>
        </div>
      </div>

      <SettingRow label={t('settings.reading.theme')}>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={data.theme}
          onValueChange={(v) => v && update.mutate({ theme: v as ThemePreference })}
        >
          {(['light', 'dark', 'system'] as const).map((v) => (
            <ToggleGroupItem key={v} value={v} className="px-3 text-xs">
              {t(`settings.reading.themes.${v}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </SettingRow>
      <SettingRow label={t('settings.reading.font')}>
        {choice('font_family', ['work', 'serif', 'sans'] as const, (v) => t(`settings.reading.fonts.${v}`))}
      </SettingRow>
      <SettingRow label={t('settings.reading.fontSize')}>{slider('font_size', 14, 24, 1)}</SettingRow>
      <SettingRow label={t('settings.reading.lineHeight')}>{slider('line_height', 1.5, 2.4, 0.1)}</SettingRow>
      <SettingRow label={t('settings.reading.width')}>{slider('max_width', 32, 52, 1)}</SettingRow>
      <SettingRow label={t('settings.reading.indent')}>
        {choice('indent', ['work', 'on', 'off'] as const, (v) => t(`settings.reading.indents.${v}`))}
      </SettingRow>
      <SettingRow label={t('settings.reading.typewriter')}>
        {choice('typewriter', ['instant', 'normal', 'slow'] as const, (v) => t(`settings.reading.typewriters.${v}`))}
      </SettingRow>
      <SettingRow label={t('settings.reading.motion')}>
        {choice('reduce_motion', ['system', 'on', 'off'] as const, (v) => t(`settings.reading.motions.${v}`))}
      </SettingRow>
      <SettingRow label={t('settings.reading.useWorkTheme')}>
        <Switch checked={data.use_work_theme} onCheckedChange={(v) => update.mutate({ use_work_theme: v })} />
      </SettingRow>
      <SettingRow label={t('settings.reading.toolHints')}>
        <Switch checked={local.tool_hints} onCheckedChange={(v) => patch({ tool_hints: v })} />
      </SettingRow>
    </section>
  )
}
