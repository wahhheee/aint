/**
 * 阅读页的实时状态：进行中的回合草稿、连接状态、上下文用量。
 *
 * 数据分工：
 *   - 已提交的回合 → 时间线接口（TanStack Query），是权威版本；
 *   - 进行中的回合 → 这里的 draft，由 SSE 事件驱动；提交后时间线刷新，草稿随之移除。
 *
 * 事件处理写成纯函数 reduceEvent（便于单元测试与排查"某个事件为什么没生效"）。
 */
import { create } from 'zustand'

import type { AnyEvent } from '@/api/events'
import type { StreamStatus } from '@/api/sse'
import type { DisplayItem, TurnInput } from '@/api/types'

export interface TurnFailure {
  turnId: string
  code: string
  message: string
  recoverable: boolean
}

export interface DraftTurn {
  turnId: string
  /** 父节点：重新生成、修改输入时是较早的节点，正文流中它之后的已提交回合先隐去 */
  parentId: string
  input: TurnInput
  /** 与提交后的显示项结构一致，渲染时可以复用同一套组件 */
  items: DisplayItem[]
  /** 已收到的最大 turn.text 序号（检测丢失） */
  seq: number
  /** 进行中的工具调用数（显示"正在翻阅资料"） */
  toolsRunning: number
  status: 'streaming' | 'committed' | 'interrupted'
  /** 提交后的节点 ID（= turnId），等时间线包含它时移除草稿 */
  nodeId?: string
}

export interface ReaderState {
  saveId: string | null
  draft: DraftTurn | null
  /** 已提交输入、尚未收到 turn.started（显示等待态，防止重复提交） */
  pending: TurnInput | null
  /** 重新生成、修改输入时的父节点（等待期间正文流中它之后的回合先隐去） */
  pendingParent: string | null
  failure: TurnFailure | null
  context: { used: number; threshold: number } | null
  /** 正在整理上下文（顶栏显示"正在整理记忆"，此时提交的输入排队） */
  compacting: boolean
  connection: StreamStatus

  reset: (saveId: string) => void
  setPending: (input: TurnInput | null, parentId?: string) => void
  /** 用呈现信息初始化上下文用量与整理状态（打开存档时） */
  init: (context: { used: number; threshold: number }, compacting: boolean) => void
  setConnection: (s: StreamStatus) => void
  dispatch: (ev: AnyEvent) => void
  clearDraft: () => void
  clearFailure: () => void
}

type Snapshot = Pick<ReaderState, 'draft' | 'pending' | 'pendingParent' | 'failure' | 'context' | 'compacting'>

/** 处理一条 SSE 事件，返回新状态（纯函数）。 */
export function reduceEvent(s: Snapshot, ev: AnyEvent): Snapshot {
  switch (ev.type) {
    case 'turn.started':
      return {
        ...s,
        pending: null,
        pendingParent: null,
        failure: null,
        draft: {
          turnId: ev.data.turn_id,
          parentId: ev.data.parent_id,
          input: ev.data.input,
          items: [],
          seq: 0,
          toolsRunning: 0,
          status: 'streaming',
        },
      }
    case 'turn.text': {
      const d = s.draft
      if (!d || d.turnId !== ev.data.turn_id) return s
      if (ev.data.seq <= d.seq) return s // 续传补发的重复事件
      const items = [...d.items]
      const last = items[items.length - 1]
      if (last?.t === 'text') items[items.length - 1] = { t: 'text', markup: last.markup + ev.data.delta }
      else items.push({ t: 'text', markup: ev.data.delta })
      return { ...s, draft: { ...d, items, seq: ev.data.seq } }
    }
    case 'turn.tool': {
      const d = s.draft
      if (!d || d.turnId !== ev.data.turn_id || ev.data.display !== 'subtle') return s
      const delta = ev.data.phase === 'start' ? 1 : -1
      return { ...s, draft: { ...d, toolsRunning: Math.max(0, d.toolsRunning + delta) } }
    }
    case 'turn.display': {
      const d = s.draft
      if (!d || d.turnId !== ev.data.turn_id) return s
      return { ...s, draft: { ...d, items: [...d.items, ev.data.item] } }
    }
    case 'turn.choices': {
      const d = s.draft
      if (!d || d.turnId !== ev.data.turn_id) return s
      return { ...s, draft: { ...d, items: [...d.items, ev.data.choices] } }
    }
    case 'turn.usage':
      return { ...s, context: ev.data.context }
    case 'turn.committed': {
      const d = s.draft
      if (!d || d.turnId !== ev.data.turn_id) return s
      return { ...s, draft: { ...d, status: ev.data.status, nodeId: ev.data.node_id, toolsRunning: 0 } }
    }
    case 'compaction.started':
      return { ...s, compacting: true }
    case 'compaction.done':
      return { ...s, compacting: false, context: s.context && { ...s.context, used: ev.data.stats.after } }
    case 'compaction.failed':
      return { ...s, compacting: false }
    case 'turn.failed':
      return {
        ...s,
        pending: null,
        pendingParent: null,
        // 后端在有内容时先发 committed 再发 failed；仍在 streaming 说明这一回合没有留下任何内容，草稿可以移除
        draft: s.draft?.turnId === ev.data.turn_id && s.draft.status === 'streaming' ? null : s.draft,
        failure: {
          turnId: ev.data.turn_id,
          code: ev.data.code,
          message: ev.data.message,
          recoverable: ev.data.recoverable,
        },
      }
    default:
      return s
  }
}

export const useReader = create<ReaderState>((set) => ({
  saveId: null,
  draft: null,
  pending: null,
  pendingParent: null,
  failure: null,
  context: null,
  compacting: false,
  connection: 'connecting',

  reset: (saveId) =>
    set({
      saveId,
      draft: null,
      pending: null,
      pendingParent: null,
      failure: null,
      context: null,
      compacting: false,
      connection: 'connecting',
    }),
  setPending: (pending, parentId) => set({ pending, pendingParent: pending ? (parentId ?? null) : null, failure: null }),
  init: (context, compacting) => set({ context, compacting }),
  setConnection: (connection) => set({ connection }),
  dispatch: (ev) => set((s) => reduceEvent(s, ev)),
  clearDraft: () => set({ draft: null }),
  clearFailure: () => set({ failure: null }),
}))

/** 正在生成（发送按钮变为停止）。 */
export const selectBusy = (s: ReaderState) => s.pending !== null || s.draft?.status === 'streaming'
