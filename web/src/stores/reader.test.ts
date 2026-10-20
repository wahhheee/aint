import { describe, expect, it } from 'vitest'

import type { AnyEvent } from '@/api/events'

import { reduceEvent } from './reader'

const empty = { draft: null, pending: null, failure: null, context: null }
const ev = (e: Omit<AnyEvent, 'id'>, id = 1) => ({ ...e, id }) as AnyEvent

describe('reduceEvent', () => {
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
})
