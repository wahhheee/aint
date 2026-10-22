/**
 * SSE 事件契约（与 internal/event/types.go 同步）。
 */
import type { ChoiceItem, DisplayItem, PanelRender, StateChange, TurnInput, Usage } from './types'

export interface EventMap {
  'turn.started': { turn_id: string; parent_id: string; input: TurnInput }
  'turn.text': { turn_id: string; seq: number; delta: string }
  'turn.tool': { turn_id: string; call_id: string; name: string; phase: 'start' | 'done' | 'error'; display?: string }
  'turn.display': { turn_id: string; item: DisplayItem }
  'turn.choices': { turn_id: string; choices: { t: 'choices'; prompt?: string; items: ChoiceItem[]; free?: boolean } }
  'turn.state': { turn_id: string; changes: StateChange[] }
  'turn.usage': { turn_id: string; usage: Usage; context: { used: number; threshold: number } }
  'turn.committed': { turn_id: string; node_id: string; status: 'committed' | 'interrupted' }
  'turn.failed': { turn_id: string; code: string; message: string; recoverable: boolean }
  panels: { panels: PanelRender[] }
  'compaction.started': { reason: string }
  'compaction.done': { node_id: string; stats: unknown }
  'compaction.failed': { message: string }
  'helper.progress': { turn_id: string; kind: 'research' | 'curate'; phase: string }
  'work.reloaded': { work_id: string; report: unknown }
  'save.titled': { save_id: string; title: string }
}

export type EventType = keyof EventMap

/** EventSource 需要逐个登记事件名。新增事件时在这里补充。 */
export const EVENT_TYPES: EventType[] = [
  'turn.started',
  'turn.text',
  'turn.tool',
  'turn.display',
  'turn.choices',
  'turn.state',
  'turn.usage',
  'turn.committed',
  'turn.failed',
  'panels',
  'compaction.started',
  'compaction.done',
  'compaction.failed',
  'helper.progress',
  'work.reloaded',
  'save.titled',
]

/** 带类型的事件（判别联合），便于在 switch 中收窄。 */
export type AnyEvent = { [K in EventType]: { type: K; id: number; data: EventMap[K] } }[EventType]
