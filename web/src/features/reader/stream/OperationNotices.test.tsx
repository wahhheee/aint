import '@/i18n'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { useReader } from '@/stores/reader'
import { OperationNotices } from './OperationNotices'

const { compact, inspect } = vi.hoisted(() => ({ compact: vi.fn(), inspect: vi.fn() }))
vi.mock('../ReaderContext', () => ({ useReaderContext: () => ({ saveId: 's', actions: { compact }, inspect }) }))
afterEach(() => {
  cleanup()
  sessionStorage.clear()
  vi.clearAllMocks()
})

it('恢复整理失败时可检查对应调用，只在原进度上重试', () => {
  const s = useReader.getState()
  s.reset('s')
  s.setConnection('open')
  s.dispatch({
    id: 2,
    type: 'session.snapshot',
    data: {
      turn: null,
      compacting: false,
      compact_failure: { reason: 'threshold', message: '模型拒答或触发了内容过滤。', attempt_id: 'job', retry_head: 'old' },
    },
  })
  const view = render(<OperationNotices headId="new" onRetry={vi.fn()} onContinue={vi.fn()} />)
  expect(screen.getByRole('alert').textContent).toContain('上一回合已保存')
  expect(screen.queryByRole('button', { name: '重试整理' })).toBeNull()
  view.rerender(<OperationNotices headId="old" onRetry={vi.fn()} onContinue={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '检查器' }))
  expect(inspect).toHaveBeenCalledWith('job')
  fireEvent.click(screen.getByRole('button', { name: '重试整理' }))
  expect(compact).toHaveBeenCalledWith('old', undefined)
})
