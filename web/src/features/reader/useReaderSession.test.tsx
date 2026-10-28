import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/api/endpoints'
import type { AnyEvent } from '@/api/events'
import { useTimeline } from '@/api/queries'
import type { StreamHandlers } from '@/api/sse'
import type { Timeline } from '@/api/types'
import { selectBusy, useReader } from '@/stores/reader'

import { useReaderSession } from './useReaderSession'

const stream = vi.hoisted(() => ({ handlers: null as StreamHandlers | null, close: vi.fn() }))
vi.mock('@/api/sse', () => ({
  openEventStream: (_: string, handlers: StreamHandlers) => {
    stream.handlers = handlers
    return stream.close
  },
}))
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

it('等待首次快照再开放输入，重同步恢复空闲并刷新已提交数据', async () => {
  const qc = new QueryClient()
  const invalidate = vi.spyOn(qc, 'invalidateQueries')
  renderHook(() => useReaderSession('s'), {
    wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>,
  })
  act(() => stream.handlers!.onStatus!('open'))
  expect(useReader.getState().connection).toBe('connecting')
  act(() =>
    stream.handlers!.onEvent({
      id: 9,
      type: 'session.snapshot',
      data: {
        compacting: false,
        turn: {
          turn_id: 't',
          parent_id: 'p',
          input: { kind: 'free', text: '推门' },
          started: true,
          items: [{ t: 'text', markup: '雾' }],
          seq: 4,
          tools_running: 0,
        },
      },
    }),
  )
  expect(useReader.getState().connection).toBe('open')
  expect(selectBusy(useReader.getState())).toBe(true)
  act(() => stream.handlers!.onResync!())
  expect(useReader.getState().connection).toBe('reconnecting')
  act(() => stream.handlers!.onEvent({ id: 20, type: 'session.snapshot', data: { turn: null, compacting: false } }))
  expect(selectBusy(useReader.getState())).toBe(false)
  expect(useReader.getState().connection).toBe('open')
  await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ['saves', 's'] }))
  act(() => useReader.getState().resync())
  expect(stream.close).toHaveBeenCalledTimes(1)
  expect(useReader.getState().connection).toBe('connecting')
})

const snapshot: AnyEvent = { id: 1, type: 'session.snapshot', data: { turn: null, compacting: false } }
const committed: AnyEvent = { id: 2, type: 'turn.committed', data: { turn_id: 'new', node_id: 'new', status: 'committed' } }

function timeline(id: string, hasMore = false): Timeline {
  return {
    save_id: 's',
    head_id: id,
    has_more: hasMore,
    has_later: false,
    nodes: [
      {
        id,
        parent_id: 'root',
        kind: 'turn',
        status: 'committed',
        turn_no: 1,
        created_at: 0,
        display: [{ t: 'text', markup: id }],
        versions: { index: 0, count: 1 },
      },
    ],
  }
}

function readerTimeline() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return renderHook(
    () => {
      const query = useTimeline('s')
      useReaderSession('s')
      return query
    },
    { wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider> },
  )
}

function pendingTimeline() {
  let resolve!: (value: Timeline) => void
  const promise = new Promise<Timeline>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

it.each([snapshot, committed])('$type 在首次查询期间到达时读取新时间线，晚到的旧响应不覆盖结果', async (event) => {
  const first = pendingTimeline()
  const fetch = vi
    .spyOn(api.saves, 'timeline')
    .mockImplementationOnce(() => first.promise)
    .mockResolvedValue(timeline('new'))
  const { result } = readerTimeline()
  expect(fetch).toHaveBeenCalledTimes(1)

  act(() => stream.handlers!.onEvent(event))
  await waitFor(() => expect(result.current.data?.head_id).toBe('new'))
  await act(async () => first.resolve({ ...timeline('root'), nodes: [] }))
  expect(result.current.data?.nodes.map((node) => node.id)).toEqual(['new'])
})

it('连续提交事件分别刷新时间线，先前响应晚到时保留最新提交', async () => {
  const first = pendingTimeline()
  const second = pendingTimeline()
  const fetch = vi
    .spyOn(api.saves, 'timeline')
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise)
    .mockResolvedValue(timeline('latest'))
  const { result } = readerTimeline()

  act(() => stream.handlers!.onEvent(committed))
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  act(() => stream.handlers!.onEvent({ ...committed, id: 3 }))
  await waitFor(() => expect(result.current.data?.head_id).toBe('latest'))
  await act(async () => {
    second.resolve(timeline('new'))
    first.resolve(timeline('old'))
  })
  expect(result.current.data?.head_id).toBe('latest')
})

it('切换开场后清空旧分页，只显示新开场的时间线', async () => {
  vi.spyOn(api.saves, 'timeline')
    .mockResolvedValueOnce(timeline('recent', true))
    .mockResolvedValueOnce(timeline('earlier'))
    .mockResolvedValue(timeline('opening'))
  const { result } = readerTimeline()
  await waitFor(() => expect(result.current.data?.head_id).toBe('recent'))
  await act(async () => {
    await result.current.fetchNextPage()
  })
  await waitFor(() => expect(result.current.data?.nodes.map((node) => node.id)).toEqual(['earlier', 'recent']))

  act(() => stream.handlers!.onEvent({ id: 3, type: 'timeline.changed', data: { head_id: 'opening', reason: 'opening' } }))
  await waitFor(() => expect(result.current.data?.nodes.map((node) => node.id)).toEqual(['opening']))
  expect(result.current.hasNextPage).toBe(false)
})
