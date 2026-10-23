import type { DisplayItem, TurnInput } from '@/api/types'

/**
 * 回合显示项中的玩家输入，写回 TurnInput 的形状：重新生成时用它显示等待中的输入行（开场没有输入行）。
 * 只用于显示，提交时由服务端沿用原回合的输入。
 */
export function displayedInput(display: DisplayItem[]): TurnInput {
  const it = display.find((d): d is Extract<DisplayItem, { t: 'input' }> => d.t === 'input')
  if (!it) return { kind: 'opening' }
  if (it.kind === 'choice') return { kind: 'choice', choice_id: '', choice_label: it.text }
  return { kind: it.kind, text: it.text, mode: it.mode }
}
