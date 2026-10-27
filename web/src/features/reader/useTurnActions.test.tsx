import '@/i18n'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { ApiError } from '@/api/errors'
import type { Command, Timeline } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { useReader } from '@/stores/reader'

import { useTurnActions } from './useTurnActions'

vi.mock('@/api/endpoints', () => ({
  api: { saves: { submit: vi.fn(), regenerate: vi.fn(), cancel: vi.fn(), revise: vi.fn(), requestChoices: vi.fn() } },
}))
vi.mock('sonner', () => ({ toast: Object.assign(vi.fn(), { error: vi.fn() }) }))
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
  useReader.getState().setConnection('open')
  vi.mocked(api.saves.submit).mockResolvedValue({ turn_id: 'turn' })
  vi.mocked(confirmAction).mockResolvedValue(true)
})

describe('会话恢复', () => {
  it('其他页面已启动或完成自动开场时同步会话，不显示失败提示', async () => {
    const version = useReader.getState().resyncVersion
    vi.mocked(api.saves.submit).mockRejectedValue(new ApiError('conflict', '存档已变化，请刷新后重试', 409))
    expect(await actions().current.submit({ kind: 'opening', expected_head_id: 'root' })).toBe(false)
    expect(toast.error).not.toHaveBeenCalled()
    expect(useReader.getState().resyncVersion).toBe(version + 1)
    expect(useReader.getState().pending).toBeNull()
  })
  it('提交冲突显示原因并请求完整同步，不清空待发送内容', async () => {
    const version = useReader.getState().resyncVersion
    vi.mocked(api.saves.submit).mockRejectedValue(new ApiError('conflict', '上一回合还在进行中', 409, { turn_id: 'other' }))
    expect(await actions().current.submitText('推门')).toBe(false)
    expect(toast.error).toHaveBeenCalledWith('上一回合还在进行中')
    expect(useReader.getState().resyncVersion).toBe(version + 1)
    expect(useReader.getState().connection).toBe('reconnecting')
  })

  it('可停止从快照恢复的排队输入', async () => {
    useReader
      .getState()
      .dispatch({ id: 1, type: 'turn.pending', data: { turn_id: 'queued', parent_id: '', input: { kind: 'free', text: '继续' } } })
    await actions().current.stop()
    expect(api.saves.cancel).toHaveBeenCalledWith('save', 'queued')
  })

  it('事件连接尚未同步时不提交新回合', async () => {
    useReader.getState().setConnection('connecting')
    expect(await actions().current.submitText('推门')).toBe(false)
    expect(api.saves.submit).not.toHaveBeenCalled()
    expect(toast).toHaveBeenCalled()
  })
})
afterEach(cleanup)

function actions(timeline?: Timeline) {
  const client = new QueryClient()
  return renderHook(() => useTurnActions('save', [follow], timeline), {
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

const operationTimeline: Timeline = {
  save_id: 'save',
  head_id: 'edit',
  has_more: false,
  has_later: false,
  nodes: [
    {
      id: 'story',
      parent_id: 'root',
      kind: 'turn',
      status: 'committed',
      turn_no: 1,
      created_at: 0,
      display: [
        { t: 'input', kind: 'free', text: '看看摊位' },
        { t: 'text', markup: '雨夜集市。' },
      ],
      versions: { index: 1, count: 1 },
    },
  ],
}

describe('回合操作命令', () => {
  it('重写保留原输入，向操作接口发送要求和当前进度', async () => {
    const a = actions(operationTimeline)
    expect(await a.current.submitText('/rewrite 暂时不要揭露身份')).toBe(true)
    expect(api.saves.revise).toHaveBeenCalledWith('save', 'story', '暂时不要揭露身份', 'edit')
    expect(api.saves.submit).not.toHaveBeenCalled()
    expect(useReader.getState().pending?.text).toBe('看看摊位')
    expect(useReader.getState().pendingParent).toBe('root')
  })
  it('补选项不裁掉当前正文，重试保持操作种类', async () => {
    const a = actions(operationTimeline)
    await a.current.submitText('/choices')
    expect(api.saves.requestChoices).toHaveBeenCalledWith('save', 'story', 'edit')
    expect(useReader.getState().pendingParent).toBeNull()
    useReader.getState().setPending(null)
    await a.current.retry()
    expect(api.saves.requestChoices).toHaveBeenCalledTimes(2)
    expect(api.saves.submit).not.toHaveBeenCalled()
  })
})
