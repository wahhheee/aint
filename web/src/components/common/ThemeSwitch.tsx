/**
 * 外观切换：纸页 / 夜读 / 跟随系统，三枚图标的分段控件（侧栏底部、阅读页的阅读设置）。
 * 先在本地切换（立即生效），再写回后端设置。
 */
import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useUpdateSettings } from '@/api/queries'
import type { ThemePreference } from '@/api/types'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

const THEME_ICONS: Record<ThemePreference, LucideIcon> = { light: Sun, dark: Moon, system: Monitor }

export function ThemeSwitch() {
  const { t } = useTranslation()
  const update = useUpdateSettings()
  const preference = useTheme((s) => s.preference)
  const setPreference = useTheme((s) => s.setPreference)

  return (
    <div role="radiogroup" aria-label={t('settings.reading.theme')} className="flex shrink-0 gap-0.5 rounded-lg bg-paper-sunken p-0.5">
      {(['light', 'dark', 'system'] as const).map((v) => {
        const Icon = THEME_ICONS[v]
        const active = preference === v
        return (
          <Tooltip key={v}>
            <TooltipTrigger asChild>
              <button
                type="button"
                role="radio"
                aria-checked={active}
                aria-label={t(`settings.reading.themes.${v}`)}
                onClick={() => {
                  setPreference(v)
                  update.mutate({ theme: v })
                }}
                className={cn(
                  'flex size-7 items-center justify-center rounded-md text-ink-faint transition-colors hover:text-ink',
                  active && 'bg-segment text-ink shadow-segment',
                )}
              >
                <Icon className="size-3.5" strokeWidth={1.75} />
              </button>
            </TooltipTrigger>
            <TooltipContent>{t(`settings.reading.themes.${v}`)}</TooltipContent>
          </Tooltip>
        )
      })}
    </div>
  )
}
