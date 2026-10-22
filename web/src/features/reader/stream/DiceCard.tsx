import { Dices } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { DisplayItem } from '@/api/types'

type DiceItem = Extract<DisplayItem, { t: 'dice' }>

/**
 * 骰子卡片（《02-技术方案》10.3）：掷骰原因、每颗骰子的点数、算式与合计。
 * 与提示条同宽同底，像夹在书页间的一张小笺；点数用等宽数字排成一行小方块。只有一颗骰子且没有加值时不重复写合计。
 */
export function DiceCard({ item }: { item: DiceItem }) {
  const { t } = useTranslation()
  const bonus = item.total - item.rolls.reduce((a, b) => a + b, 0)
  const negativeDice = bonus !== 0 && /-\d*d/.test(item.expr) // 含减去的骰子时，加值无法从点数直接推出，只显示算式
  return (
    <figure
      className="my-6 flex items-center gap-4 rounded-xl border border-rule bg-paper-raised/75 px-4 py-3 font-sans backdrop-blur-md"
      aria-label={t('reader.dice.label', { expr: item.expr, total: item.total })}
    >
      <Dices aria-hidden className="size-5 shrink-0 text-ink-faint" strokeWidth={1.5} />
      <div className="min-w-0 flex-1">
        <figcaption className="flex items-baseline gap-2 text-[13px] leading-5 text-ink-muted">
          <span className="min-w-0 truncate">{item.text || t('reader.dice.title')}</span>
          <span className="ml-auto shrink-0 font-mono text-xs text-ink-faint">{item.expr}</span>
        </figcaption>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {item.rolls.map((r, i) => (
            <span
              key={i}
              className="inline-flex h-7 min-w-7 items-center justify-center rounded-md border border-rule-strong bg-paper px-1.5 text-[13px] font-medium text-ink tabular"
            >
              {r}
            </span>
          ))}
          {bonus !== 0 && !negativeDice && (
            <span className="px-1 text-[13px] text-ink-muted tabular">{bonus > 0 ? `+${bonus}` : `−${-bonus}`}</span>
          )}
          {(item.rolls.length > 1 || bonus !== 0) && (
            <>
              <span className="ml-1 text-[13px] text-ink-faint">=</span>
              <span className="text-xl leading-none font-semibold text-ink tabular">{item.total}</span>
            </>
          )}
        </div>
      </div>
    </figure>
  )
}
