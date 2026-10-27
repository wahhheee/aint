import { describe, expect, it } from 'vitest'

import type { AnyEvent } from '@/api/events'

import { reduceEvent, selectBusy, useReader } from './reader'

const empty = { draft: null, pending: null, pendingParent: null, failure: null, context: null, turnUsage: null, compacting: false }
const ev = (e: Omit<AnyEvent, 'id'>, id = 1) => ({ ...e, id }) as AnyEvent

describe('reduceEvent', () => {
  it('切换开场清除旧草稿、失败信息与回合用量，忽略旧终态事件', () => {
    let s = reduceEvent(empty, ev({ type: 'turn.started', data: { turn_id: 'old', parent_id: 'root', input: { kind: 'opening' } } }))
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 'old', seq: 1, delta: '旧开场' } }))
    s = reduceEvent(s, ev({ type: 'turn.committed', data: { turn_id: 'old', node_id: 'old', status: 'interrupted' } }))
    s = reduceEvent(s, ev({ type: 'timeline.changed', data: { head_id: 'new-root', reason: 'opening' } }))
    expect(s.draft).toBeNull()
    expect(s.failure).toBeNull()
    expect(s.turnUsage).toBeNull()
    s = reduceEvent(s, ev({ type: 'turn.committed', data: { turn_id: 'old', node_id: 'old', status: 'committed' } }))
    expect(s.draft).toBeNull()
  })
  it('中途进入恢复正文、工具与分支位置，后续增量只追加一次', () => {
    let s = reduceEvent(
      empty,
      ev({
        type: 'session.snapshot',
        data: {
          turn: {
            turn_id: 't',
            parent_id: 'older',
            input: { kind: 'free', text: '推门' },
            started: true,
            items: [{ t: 'text', markup: '雾' }],
            seq: 8,
            tools_running: 1,
          },
          compacting: false,
        },
      }),
    )
    expect(s.draft?.parentId).toBe('older')
    expect(s.draft?.toolsRunning).toBe(1)
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 't', seq: 8, delta: '雾' } }))
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 't', seq: 9, delta: '很浓' } }))
    expect(s.draft?.items).toEqual([{ t: 'text', markup: '雾很浓' }])
  })

  it('空闲快照清除丢失终态事件的草稿、排队和助手状态', () => {
    useReader.getState().reset('s')
    useReader.getState().dispatch(ev({ type: 'turn.started', data: { turn_id: 't', parent_id: 'p', input: { kind: 'continue' } } }))
    expect(selectBusy(useReader.getState())).toBe(true)
    useReader.getState().dispatch(ev({ type: 'session.snapshot', data: { turn: null, compacting: false } }))
    expect(selectBusy(useReader.getState())).toBe(false)
    expect(useReader.getState().draft).toBeNull()
    expect(useReader.getState().pendingTurnId).toBeUndefined()
  })

  it('恢复整理期间排队的输入并保留停止所需的回合 ID', () => {
    const s = reduceEvent(
      empty,
      ev({
        type: 'session.snapshot',
        data: {
          compacting: true,
          turn: { turn_id: 'q', parent_id: '', input: { kind: 'free', text: '继续' }, started: false, items: [], seq: 0, tools_running: 0 },
        },
      }),
    )
    expect(s.pending?.text).toBe('继续')
    expect(s.pendingTurnId).toBe('q')
    expect(s.compacting).toBe(true)
    expect(s.draft).toBeNull()
  })
  it('把正文增量拼成同一个正文项，并忽略重复补发的事件', () => {
    let s = reduceEvent(empty, ev({ type: 'turn.started', data: { turn_id: 't1', parent_id: 'p', input: { kind: 'free', text: '推门' } } }))
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 't1', seq: 1, delta: '雾' } }))
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 't1', seq: 2, delta: '很浓' } }))
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 't1', seq: 2, delta: '很浓' } }))
    expect(s.draft?.items).toEqual([{ t: 'text', markup: '雾很浓' }])
  })

  it('忽略其他回合的事件', () => {
    let s = reduceEvent(empty, ev({ type: 'turn.started', data: { turn_id: 't1', parent_id: 'p', input: { kind: 'opening' } } }))
    s = reduceEvent(s, ev({ type: 'turn.text', data: { turn_id: 'other', seq: 1, delta: 'x' } }))
    expect(s.draft?.items).toEqual([])
  })

  it('提交后记录节点 ID，等待时间线刷新', () => {
    let s = reduceEvent(empty, ev({ type: 'turn.started', data: { turn_id: 't1', parent_id: 'p', input: { kind: 'opening' } } }))
    s = reduceEvent(s, ev({ type: 'turn.committed', data: { turn_id: 't1', node_id: 't1', status: 'committed' } }))
    expect(s.draft?.status).toBe('committed')
    expect(s.draft?.nodeId).toBe('t1')
  })

  it('整理开始与结束切换整理状态，结束时用量换成整理后的估算', () => {
    let s = reduceEvent(
      { ...empty, context: { used: 9000, threshold: 8000 } },
      ev({ type: 'compaction.started', data: { reason: 'threshold' } }),
    )
    expect(s.compacting).toBe(true)
    s = reduceEvent(
      s,
      ev({
        type: 'compaction.done',
        data: {
          node_id: 'c1',
          stats: {
            reason: 'threshold',
            mode: 'inline',
            summary_from: 1,
            summary_to: 10,
            keep_from_turn: 11,
            summary_chars: 800,
            memory_writes: 2,
            before: 9000,
            after: 3000,
            calls: 3,
            duration_ms: 1000,
          },
        },
      }),
    )
    expect(s.compacting).toBe(false)
    expect(s.context).toEqual({ used: 3000, threshold: 8000 })
  })

  it('turn.usage 更新上下文用量与本回合的用量和费用，新回合开始时清空本回合用量', () => {
    const usage = { input_uncached: 120, cache_read: 7800, cache_write: 210, output: 260, reasoning: 40 }
    let s = reduceEvent(empty, ev({ type: 'turn.started', data: { turn_id: 't1', parent_id: 'p', input: { kind: 'opening' } } }))
    s = reduceEvent(
      s,
      ev({
        type: 'turn.usage',
        data: { turn_id: 't1', usage, costs: { CNY: 210000 }, unpriced: 0, context: { used: 8400, threshold: 12000 } },
      }),
    )
    expect(s.context).toEqual({ used: 8400, threshold: 12000 })
    expect(s.turnUsage).toEqual({ turnId: 't1', usage, costs: { CNY: 210000 }, unpriced: 0 })

    s = reduceEvent(s, ev({ type: 'turn.committed', data: { turn_id: 't1', node_id: 't1', status: 'committed' } }))
    expect(s.turnUsage?.turnId).toBe('t1')

    s = reduceEvent(s, ev({ type: 'turn.started', data: { turn_id: 't2', parent_id: 't1', input: { kind: 'free', text: '继续' } } }))
    expect(s.turnUsage).toBeNull()
    expect(s.context).toEqual({ used: 8400, threshold: 12000 })
  })
})

describe('回合操作恢复', () => {
  it('重写失败清除替换草稿并保留重试所需的原始要求', () => {
    const operation = { kind: 'rewrite' as const, node_id: 'old', instructions: '更含蓄', expected_head: 'edit' }
    let s = reduceEvent(
      empty,
      ev({
        type: 'session.snapshot',
        data: {
          compacting: false,
          turn: {
            turn_id: 'new',
            parent_id: 'root',
            input: { kind: 'free', text: '推门' },
            operation,
            started: true,
            items: [{ t: 'text', markup: '新稿' }],
            seq: 1,
            tools_running: 0,
          },
        },
      }),
    )
    expect(s.draft?.operation).toEqual(operation)
    s = reduceEvent(s, ev({ type: 'turn.failed', data: { turn_id: 'new', code: 'network', message: '失败', recoverable: true } }))
    expect(s.draft).toBeNull()
    expect(s.failure?.operation).toEqual(operation)
  })
  it('补选项提交不等待不可见的补充节点出现在正文列表', () => {
    const operation = { kind: 'choices' as const, node_id: 'story', expected_head: 'story' }
    let s = reduceEvent(
      empty,
      ev({ type: 'turn.started', data: { turn_id: 'extra', parent_id: 'story', input: { kind: 'continue' }, operation } }),
    )
    s = reduceEvent(s, ev({ type: 'turn.committed', data: { turn_id: 'extra', node_id: 'extra', status: 'committed' } }))
    expect(s.draft).toBeNull()
  })
})
