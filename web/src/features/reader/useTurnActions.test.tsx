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

vi.mock('@/api/endpoints', () => ({ api: { saves: { submit: vi.fn(), regenerate: vi.fn() } } }))
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

describe('失败重试', () => {
  it('已保存中断回合从原父节点重新生成，草稿移除后仍保留重试来源', async () => {
    const a = actions()
    await a.current.choose({ id: 'a', label: '拆信' })
    useReader.getState().setPending(null)
    useReader.getState().dispatch({
      id: 1,
      type: 'turn.failed',
      data: { turn_id: 'failed', node_id: 'failed', parent_id: 'opening', code: 'network', message: '上游失败', recoverable: true },
    })
    expect(await a.current.retry()).toBe(true)
    expect(api.saves.submit).toHaveBeenCalledTimes(1)
    expect(api.saves.regenerate).toHaveBeenCalledWith('save', 'failed', undefined)
    expect(useReader.getState().pendingParent).toBe('opening')
  })
  it('未提交内容的普通回合重新提交原选项', async () => {
    const a = actions()
    await a.current.choose({ id: 'a', label: '拆信' })
    useReader
      .getState()
      .dispatch({ id: 1, type: 'turn.failed', data: { turn_id: 'failed', code: 'network', message: '上游失败', recoverable: true } })
    await a.current.retry()
    expect(api.saves.submit).toHaveBeenCalledTimes(2)
    expect(api.saves.regenerate).not.toHaveBeenCalled()
  })
  it('重新生成失败且未保存内容时仍重试同一分支', async () => {
    const a = actions()
    await a.current.regenerate('old', 'parent', { kind: 'free', text: '新输入' })
    useReader
      .getState()
      .dispatch({ id: 1, type: 'turn.failed', data: { turn_id: 'failed', code: 'network', message: '上游失败', recoverable: true } })
    await a.current.retry()
    expect(api.saves.regenerate).toHaveBeenLastCalledWith('save', 'old', { kind: 'free', text: '新输入' })
    expect(api.saves.submit).not.toHaveBeenCalled()
  })
})
