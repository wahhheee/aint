import '@/i18n'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ChoiceList } from './ChoiceList'

const state = vi.hoisted(() => ({ choose: vi.fn(), timed: true }))
vi.mock('../ReaderContext', () => ({
  useReaderContext: () => ({
    choose: state.choose,
    focusInput: vi.fn(),
    ui: {},
    reading: { timed_choices: state.timed, reduce_motion: 'on' },
  }),
}))
const item = { t: 'choices' as const, timeout_seconds: 1, default_choice: 'a', items: [{ id: 'a', label: '继续' }] }
beforeEach(() => {
  vi.useFakeTimers()
  state.choose.mockReset().mockResolvedValue(false)
  state.timed = true
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})
it('到期只提交一次，取消确认后保留手动选择', async () => {
  render(<ChoiceList item={item} />)
  await act(async () => {
    vi.advanceTimersByTime(1100)
  })
  await act(async () => {
    vi.advanceTimersByTime(10)
  })
  expect(state.choose).toHaveBeenCalledTimes(1)
  expect((screen.getByRole('button', { name: /继续/ }) as HTMLButtonElement).disabled).toBe(false)
  await act(async () => {
    vi.advanceTimersByTime(2000)
  })
  expect(state.choose).toHaveBeenCalledTimes(1)
})
it('焦点停留在选项时暂停，关闭限时偏好时不自动选', async () => {
  render(<ChoiceList item={item} />)
  fireEvent.focus(screen.getByRole('button', { name: /继续/ }))
  await act(async () => {
    vi.advanceTimersByTime(2000)
  })
  expect(state.choose).not.toHaveBeenCalled()
  cleanup()
  state.timed = false
  render(<ChoiceList item={item} />)
  await act(async () => {
    vi.advanceTimersByTime(2000)
  })
  expect(state.choose).not.toHaveBeenCalled()
})
