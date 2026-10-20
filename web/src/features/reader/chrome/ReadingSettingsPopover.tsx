/**
 * 阅读页内的快捷阅读设置：外观、字号、版心宽度、文字出现方式。完整设置在设置页。
 * 滑块拖动时只改本地值，松手后才写回后端（避免频繁请求）。
 */
import { Type } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useSettings, useUpdateSettings } from '@/api/queries'
import type { ReadingSettings, ThemePreference } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Slider } from '@/components/ui/slider'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function ReadingSettingsPopover() {
  const { t } = useTranslation()
  const { data } = useSettings()
  const update = useUpdateSettings()
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
      <PopoverContent align="end" className="w-72 space-y-5">
        <Row label={t('settings.reading.theme')}>
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={data.theme}
            onValueChange={(v) => v && update.mutate({ theme: v as ThemePreference })}
          >
            {(['light', 'dark', 'system'] as const).map((v) => (
              <ToggleGroupItem key={v} value={v} className="px-2.5 text-xs">
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
          />
        </Row>
        <Row label={t('settings.reading.typewriter')}>
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={data.reading.typewriter}
            onValueChange={(v) => v && patchReading({ typewriter: v as ReadingSettings['typewriter'] })}
          >
            {(['instant', 'normal', 'slow'] as const).map((v) => (
              <ToggleGroupItem key={v} value={v} className="px-2.5 text-xs">
                {t(`settings.reading.typewriters.${v}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </Row>
      </PopoverContent>
    </Popover>
  )
}

function Row({ label, value, children }: { label: string; value?: number; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-[13px] text-ink-muted">
        {label}
        {value !== undefined && <span className="ml-2 text-ink tabular">{value}</span>}
      </span>
      {children}
    </div>
  )
}
