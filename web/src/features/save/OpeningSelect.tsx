import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import type { OpeningSummary } from '@/api/types'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

/** 开局设置与阅读页共用的开场列表，保留作者排列顺序。 */
export function OpeningSelect({
  options,
  value,
  onChange,
  disabled = false,
}: {
  options: OpeningSummary[]
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}) {
  const id = useId()
  const { t } = useTranslation()
  return (
    <RadioGroup aria-label={t('openings.select')} value={value} onValueChange={onChange} disabled={disabled} className="gap-3">
      {options.map((op) => (
        <label
          key={op.id}
          htmlFor={`${id}-${op.id}`}
          className="flex cursor-pointer items-start gap-3 rounded-lg border border-rule p-3 has-[[data-state=checked]]:border-gold has-[[data-state=checked]]:bg-gold/5"
        >
          <RadioGroupItem id={`${id}-${op.id}`} value={op.id} className="mt-1" />
          <span className="min-w-0 text-sm leading-6">
            <span className="block font-medium">{op.title}</span>
            {op.description && <span className="block text-ink-muted">{op.description}</span>}
            {op.mode === 'generate' && <span className="block text-xs text-ink-faint">{t('openings.generated')}</span>}
          </span>
        </label>
      ))}
    </RadioGroup>
  )
}
