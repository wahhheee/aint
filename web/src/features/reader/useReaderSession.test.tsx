import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import type { StreamHandlers } from '@/api/sse'
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
})

it('等待首次快照再开放输入，重同步恢复空闲并刷新已提交数据', () => {
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
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['saves', 's'] })
  act(() => useReader.getState().resync())
  expect(stream.close).toHaveBeenCalledTimes(1)
  expect(useReader.getState().connection).toBe('connecting')
})
