import '@/i18n'

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { DebugCall } from '@/api/types'

import { CodeBlock } from './DebugContent'
import { Overview, ToolsView } from './DebugViews'

afterEach(cleanup)

describe('调试内容展示', () => {
  it('原文可精确复制，格式化需要主动切换，响应中的 HTML 不执行', async () => {
    const copy = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } })
    const raw = '{ "html": "<img src=x onerror=alert(1)>" }\n'
    const view = render(<CodeBlock label="请求体" value={raw} />)
    expect(view.container.querySelector('pre')?.textContent).toBe(raw)
    expect(view.container.querySelector('img')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '复制请求体' }))
    expect(copy).toHaveBeenCalledWith(raw)
    fireEvent.click(screen.getByRole('button', { name: '格式化 JSON' }))
    expect(view.container.querySelector('pre')?.textContent).toBe(JSON.stringify(JSON.parse(raw), null, 2))
  })

  it('中断时保留已返回的推理，未执行的工具不会显示为成功', () => {
    const call: DebugCall = {
      id: 'c',
      turn_id: 't',
      turn_no: 1,
      model: 'test',
      protocol: 'fake',
      purpose: 'narrate',
      started: '',
      duration_ms: 25,
      status: 'error',
      error: '中断',
      stop: '',
      usage: { input_uncached: 0, cache_read: 0, cache_write: 0, output: 0, reasoning: 0 },
      tool_count: 0,
      http_count: 0,
      revision: 1,
      truncated: false,
      steps: [
        { kind: 'reasoning', at_ms: 10, text: '返回的明文思考' },
        { kind: 'tool', at_ms: 20, text: '', call: { id: 'read1', name: 'read', args: { path: '/memory/a.md' } } },
      ],
      tools: [],
      results: [],
      http: [],
    }
    const view = render(<Overview call={call} />)
    expect(screen.getByText('返回的明文思考')).toBeTruthy()
    expect(screen.getByText('未执行')).toBeTruthy()
    expect(screen.queryByText('已执行')).toBeNull()
    view.rerender(<ToolsView call={call} imageURL={(b) => b} />)
    expect(screen.getByText('未执行')).toBeTruthy()
    expect(view.container.textContent).toContain('/memory/a.md')
  })
})
