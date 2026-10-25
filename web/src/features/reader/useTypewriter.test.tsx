import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useTypewriter } from './useTypewriter'

beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

it('完整停顿标签暂停，跳过立即显示缓冲，切换回合重新开始', () => {
  const source = '甲<pause ms="1000"/>' + '乙'.repeat(20)
  const { result, rerender } = renderHook(({ key }) => useTypewriter(source.length, 'normal', key, source), { initialProps: { key: 'a' } })
  act(() => vi.advanceTimersByTime(500))
  expect(result.current.shown).toBe(source.indexOf('乙'))
  act(() => vi.advanceTimersByTime(300))
  expect(result.current.shown).toBe(source.indexOf('乙'))
  act(() => result.current.revealAll())
  expect(result.current.done).toBe(true)
  rerender({ key: 'b' })
  expect(result.current.shown).toBe(0)
})

it('即时阅读不等待停顿，转义停顿不触发', () => {
  const source = '甲\\<pause ms="5000"/>乙'
  const { result } = renderHook(() => useTypewriter(source.length, 'normal', 'a', source))
  act(() => vi.advanceTimersByTime(1500))
  expect(result.current.done).toBe(true)
  const instant = renderHook(() => useTypewriter(25, 'instant', 'b', '<pause ms="5000"/>'))
  expect(instant.result.current.shown).toBe(25)
})
