/**
 * 阅读页的实时状态：进行中的回合草稿、连接状态、上下文用量、本回合用量与费用。
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
import type { DisplayItem, TurnInput, Usage } from '@/api/types'

export interface TurnFailure {
  turnId: string
  nodeId?: string
  parentId?: string
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

/** 本回合到目前为止的用量与费用（turn.usage；每次模型调用结束时更新） */
export interface TurnUsage {
  turnId: string
  usage: Usage
  /** 按货币分别累计的费用（百万分之一货币单位） */
  costs: Record<string, number>
  /** 未配置价格的调用次数 */
  unpriced: number
}

export interface ReaderState {
  saveId: string | null
  draft: DraftTurn | null
  /** 已提交输入、尚未收到 turn.started（显示等待态，防止重复提交） */
  pending: TurnInput | null
  /** 重新生成、修改输入时的父节点（等待期间正文流中它之后的回合先隐去） */
  pendingParent: string | null
  /** 服务端已接受的排队或准备中回合，可在开始生成前停止 */
  pendingTurnId?: string
  failure: TurnFailure | null
  context: { used: number; threshold: number } | null
  /** 最近一个回合的用量：新回合开始时清空，本页面没有进行过回合时为 null */
  turnUsage: TurnUsage | null
  /** 正在整理上下文（顶栏显示"正在整理记忆"，此时提交的输入排队） */
  helper?: string
  compacting: boolean
  connection: StreamStatus

  reset: (saveId: string) => void
  setPending: (input: TurnInput | null, parentId?: string) => void
  /** 用呈现信息初始化上下文用量；实时会话状态以事件流为准 */
  init: (context: { used: number; threshold: number }) => void
  resyncVersion: number
  resync: () => void
  setConnection: (s: StreamStatus) => void
  dispatch: (ev: AnyEvent) => void
  clearDraft: () => void
  clearFailure: () => void
}

type Snapshot = Pick<
  ReaderState,
  'draft' | 'pending' | 'pendingParent' | 'pendingTurnId' | 'failure' | 'context' | 'turnUsage' | 'compacting' | 'helper'
>

/** 处理一条 SSE 事件，返回新状态（纯函数）。 */
export function reduceEvent(s: Snapshot, ev: AnyEvent): Snapshot {
  switch (ev.type) {
    case 'session.snapshot': {
      const { turn, compacting, helper, usage } = ev.data
      return {
        ...s,
        draft: turn?.started
          ? {
              turnId: turn.turn_id,
              parentId: turn.parent_id,
              input: turn.input,
              items: turn.items,
              seq: turn.seq,
              toolsRunning: turn.tools_running,
              status: 'streaming',
            }
          : null,
        pending: turn && !turn.started ? turn.input : null,
        pendingParent: turn && !turn.started ? turn.parent_id || null : null,
        pendingTurnId: turn && !turn.started ? turn.turn_id : undefined,
        failure: null,
        compacting,
        helper,
        turnUsage: usage ? { turnId: usage.turn_id, usage: usage.usage, costs: usage.costs, unpriced: usage.unpriced } : null,
        context: usage?.context ?? s.context,
      }
    }
    case 'turn.pending':
      return {
        ...s,
        draft: null,
        pending: ev.data.input,
        pendingParent: ev.data.parent_id || null,
        pendingTurnId: ev.data.turn_id,
        failure: null,
        helper: undefined,
        turnUsage: null,
      }
    case 'helper.progress':
      return { ...s, helper: ev.data.phase === 'start' ? ev.data.kind : undefined }
    case 'turn.started':
      return {
        ...s,
        helper: undefined,
        pending: null,
        pendingParent: null,
        pendingTurnId: undefined,
        failure: null,
        turnUsage: null,
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
    case 'turn.usage': {
      const { turn_id, usage, costs, unpriced, context } = ev.data
      return { ...s, context, turnUsage: { turnId: turn_id, usage, costs, unpriced } }
    }
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
      if (s.pendingTurnId && s.pendingTurnId !== ev.data.turn_id) return s
      if (s.draft?.status === 'streaming' && s.draft.turnId !== ev.data.turn_id) return s
      return {
        ...s,
        helper: undefined,
        pending: null,
        pendingParent: null,
        pendingTurnId: undefined,
        // 后端在有内容时先发 committed 再发 failed；仍在 streaming 说明这一回合没有留下任何内容，草稿可以移除
        draft: s.draft?.turnId === ev.data.turn_id && s.draft.status === 'streaming' ? null : s.draft,
        failure: {
          turnId: ev.data.turn_id,
          nodeId: ev.data.node_id,
          parentId: ev.data.parent_id,
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
  turnUsage: null,
  compacting: false,
  connection: 'connecting',
  resyncVersion: 0,
  resync: () => set((s) => ({ resyncVersion: s.resyncVersion + 1, connection: 'reconnecting' })),

  reset: (saveId) =>
    set({
      saveId,
      draft: null,
      pending: null,
      pendingParent: null,
      pendingTurnId: undefined,
      failure: null,
      context: null,
      turnUsage: null,
      compacting: false,
      helper: undefined,
      connection: 'connecting',
    }),
  setPending: (pending, parentId) =>
    set({ pending, pendingParent: pending ? (parentId ?? null) : null, pendingTurnId: undefined, failure: null }),
  init: (context) => set((s) => ({ context: s.turnUsage ? s.context : context })),
  setConnection: (connection) => set({ connection }),
  dispatch: (ev) => set((s) => reduceEvent(s, ev)),
  clearDraft: () => set({ draft: null }),
  clearFailure: () => set({ failure: null }),
}))

/** 正在生成（发送按钮变为停止）。 */
export const selectBusy = (s: ReaderState) => s.pending !== null || s.draft?.status === 'streaming'
