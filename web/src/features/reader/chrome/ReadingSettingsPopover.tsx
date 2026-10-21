/**
 * 阅读页内的快捷阅读设置：外观、字号、版心宽度、文字出现方式。完整设置在设置页。
 * 滑块拖动时只改本地值，松手后才写回后端（避免频繁请求）。
 */
import { ChevronRight, Type } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { useSettings, useUpdateSettings } from '@/api/queries'
import type { ReadingSettings, ThemePreference } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Slider } from '@/components/ui/slider'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useTheme } from '@/lib/theme'

export function ReadingSettingsPopover() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
  const preference = useTheme((s) => s.preference)
  const setPreference = useTheme((s) => s.setPreference)
  // 拖动中的临时值；松手后写回后端并清除（显示值 = 临时值 ?? 服务端值）
  const [drag, setDrag] = useState<{ font_size?: number; max_width?: number }>({})
  if (!data) return null
  const size = drag.font_size ?? data.reading.font_size
  const width = drag.max_width ?? data.reading.max_width

  const patchReading = (r: Partial<ReadingSettings>) => update.mutate({ reading: r }, { onSettled: () => setDrag({}) })

  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={t('reader.readingSettings')}>
              <Type />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{t('reader.readingSettings')}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" className="w-80 gap-0 p-0">
        <p className="border-b border-rule px-4 py-3 text-[13px] font-medium text-ink">{t('reader.readingSettings')}</p>
        <div className="space-y-4 p-4">
          <Row label={t('settings.reading.theme')}>
            <ToggleGroup
              type="single"
              variant="segmented"
              size="sm"
              value={preference}
              onValueChange={(v) => {
                if (!v) return
                setPreference(v as ThemePreference)
                update.mutate({ theme: v as ThemePreference })
              }}
            >
              {(['light', 'dark', 'system'] as const).map((v) => (
                <ToggleGroupItem key={v} value={v}>
                  {t(`settings.reading.themes.${v}`)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </Row>
          <Row label={t('settings.reading.fontSize')} value={size}>
            <Slider
              className="w-32"
              min={14}
              max={24}
              step={1}
              value={[size]}
              onValueChange={([v]) => setDrag({ font_size: v })}
              onValueCommit={([v]) => patchReading({ font_size: v })}
              aria-label={t('settings.reading.fontSize')}
            />
          </Row>
          <Row label={t('settings.reading.width')} value={width}>
            <Slider
              className="w-32"
              min={32}
              max={52}
              step={1}
              value={[width]}
              onValueChange={([v]) => setDrag({ max_width: v })}
              onValueCommit={([v]) => patchReading({ max_width: v })}
              aria-label={t('settings.reading.width')}
            />
          </Row>
          <Row label={t('settings.reading.typewriter')}>
            <ToggleGroup
              type="single"
              variant="segmented"
              size="sm"
              value={data.reading.typewriter}
              onValueChange={(v) => v && patchReading({ typewriter: v as ReadingSettings['typewriter'] })}
            >
              {(['instant', 'normal', 'slow'] as const).map((v) => (
                <ToggleGroupItem key={v} value={v}>
                  {t(`settings.reading.typewriters.${v}`)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </Row>
        </div>
        <Link
          to="/settings/reading"
          className="flex h-10 items-center justify-between border-t border-rule px-4 text-[13px] text-ink-muted transition-colors hover:bg-hover hover:text-ink"
        >
          {t('reader.allReadingSettings')}
          <ChevronRight className="size-3.5" />
        </Link>
      </PopoverContent>
    </Popover>
  )
}

function Row({ label, value, children }: { label: string; value?: number; children: React.ReactNode }) {
  return (
    <div className="flex h-8 items-center justify-between gap-3">
      <span className="flex items-baseline gap-2 text-[13px] text-ink-muted">
        {label}
        {value !== undefined && <span className="w-5 text-xs text-ink tabular">{value}</span>}
      </span>
      {children}
    </div>
  )
}
