import '@/i18n'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/api/endpoints'
import type { Command } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { useReader } from '@/stores/reader'

import { useTurnActions } from './useTurnActions'

vi.mock('@/api/endpoints', () => ({ api: { saves: { submit: vi.fn() } } }))
vi.mock('@/components/common/confirm', () => ({ confirmAction: vi.fn() }))

const follow: Command = {
  name: 'follow',
  title: '跟随',
  description: '',
  args: [{ name: 'who', required: true }],
  present: ['slash', 'button'],
  ai_visible: true,
  confirm: '切换跟随人物？',
}

beforeEach(() => {
  vi.resetAllMocks()
  useReader.getState().reset('save')
  vi.mocked(api.saves.submit).mockResolvedValue({ turn_id: 'turn' })
  vi.mocked(confirmAction).mockResolvedValue(true)
})
afterEach(cleanup)

function actions() {
  const client = new QueryClient()
  return renderHook(() => useTurnActions('save', [follow]), {
    wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>,
  }).result
}

describe('选项提交', () => {
  it('普通选项提交选项 ID', async () => {
    expect(await actions().current.choose({ id: 'a', label: '拆信' })).toBe(true)
    expect(api.saves.submit).toHaveBeenCalledWith('save', { kind: 'choice', choice_id: 'a' })
  })

  it('命令选项保留选项 ID，同时解析参数并确认命令', async () => {
    expect(await actions().current.choose({ id: 'b', label: '跟着林舟', command: '/follow 林舟' })).toBe(true)
    expect(confirmAction).toHaveBeenCalledWith('切换跟随人物？')
    expect(api.saves.submit).toHaveBeenCalledWith('save', {
      kind: 'command',
      choice_id: 'b',
      command: { name: 'follow', args: { who: '林舟' } },
    })
    expect(useReader.getState().pending?.choice_id).toBe('b')
  })

  it('取消命令确认时不提交选择', async () => {
    vi.mocked(confirmAction).mockResolvedValue(false)
    expect(await actions().current.choose({ id: 'b', label: '跟着林舟', command: '/follow 林舟' })).toBe(false)
    expect(api.saves.submit).not.toHaveBeenCalled()
    expect(useReader.getState().pending).toBeNull()
  })

  it('内置命令选项保留选项 ID', async () => {
    expect(await actions().current.choose({ id: 'b', label: '继续观察', command: '/continue' })).toBe(true)
    expect(api.saves.submit).toHaveBeenCalledWith('save', { kind: 'continue', choice_id: 'b' })
  })

  it('输入栏独立使用命令时不附带选项 ID', async () => {
    expect(await actions().current.submitText('/follow 林舟')).toBe(true)
    expect(api.saves.submit).toHaveBeenCalledWith('save', {
      kind: 'command',
      command: { name: 'follow', args: { who: '林舟' } },
    })
  })
})
