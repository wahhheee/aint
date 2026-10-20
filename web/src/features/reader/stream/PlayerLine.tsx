import { useTranslation } from 'react-i18next'

import type { DisplayItem } from '@/api/types'
import { cn } from '@/lib/utils'

import { useReaderContext } from '../ReaderContext'

type InputItem = Extract<DisplayItem, { t: 'input' }>

/**
 * 玩家输入行：不用聊天气泡，而是一行"行动记录"——左侧泥金细线，小字号，带模式标签。
 * 场外对话以淡色呈现，表明它不属于故事。
 */
export function PlayerLine({ item }: { item: InputItem }) {
  const { t } = useTranslation()
  const { ui } = useReaderContext()
  const isOOC = item.kind === 'ooc'
  // 输入模式按作品的样式标签着色（如「推论」用 hypo 的颜色）
  const modeStyle = ui.commands.find((c) => c.mode_label === item.mode || c.title === item.mode)?.mode_style

  return (
    <p
      className={cn(
        'mb-6 flex items-baseline gap-2.5 border-l-2 border-gold/70 py-0.5 pl-3.5 font-sans text-[13.5px] leading-relaxed',
        isOOC ? 'border-ink-faint/40 text-ink-faint' : 'text-ink-muted',
      )}
    >
      <span className="shrink-0 text-ink-faint">
        {isOOC ? t('reader.ooc') : t('reader.you')}
        {item.mode && <span className={cn('ml-1', modeStyle && `mk-${modeStyle}`)}>（{item.mode}）</span>}
      </span>
      <span className="min-w-0 break-words whitespace-pre-wrap text-ink">{item.text}</span>
    </p>
  )
}
