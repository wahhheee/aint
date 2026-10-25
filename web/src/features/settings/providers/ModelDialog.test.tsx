import '@/i18n'

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Model, ModelPreset } from '@/api/types'

import { ModelDialog } from './ModelDialog'

const mocks = vi.hoisted(() => ({ mutate: vi.fn(), preset: null as ModelPreset | null }))
vi.mock('@/api/queries', () => ({
  useProviderMutations: () => ({ updateModel: { mutate: mocks.mutate } }),
  useModelPreset: () => ({ data: mocks.preset }),
}))
beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe = vi.fn()
      unobserve = vi.fn()
      disconnect = vi.fn()
    },
  )
})
afterEach(() => {
  cleanup()
  mocks.mutate.mockClear()
  vi.unstubAllGlobals()
})

const model: Model = {
  id: 'm',
  provider_id: 'p',
  name: 'claude-sonnet-5-5',
  display_name: '我的主笔',
  context_window: 9000,
  max_output: 1000,
  caps: {
    tools: true,
    vision: false,
    parallel_tools: false,
    image_in_tool_result: false,
    reasoning: 'none',
    cache: 'none',
    image_max_edge: 1568,
  },
  price: { input: 99, output: 100, currency: 'CNY' },
}
const preset: ModelPreset = {
  revision: '2026-10-25',
  context_window: 1000000,
  max_output: 128000,
  caps: { ...model.caps, vision: true, reasoning: 'effort', cache: 'breakpoints', reasoning_map: { off: 'between_tools' } },
  price: { input: 2, output: 10, currency: 'USD' },
}

describe('模型预设', () => {
  it('能力应用到表单，保存时保留名称与中转站报价', () => {
    mocks.preset = preset
    render(<ModelDialog model={model} onOpenChange={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: '应用预设能力参数' }))
    expect(mocks.mutate).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(mocks.mutate.mock.calls[0][0].input).toEqual(
      expect.objectContaining({
        display_name: '我的主笔',
        context_window: 1000000,
        max_output: 128000,
        caps: preset.caps,
        price: model.price,
      }),
    )
  })
  it('官方价格独立填入，不改能力参数', () => {
    mocks.preset = preset
    render(<ModelDialog model={model} onOpenChange={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: '填入官方价格' }))
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(mocks.mutate.mock.calls[0][0].input).toEqual(expect.objectContaining({ context_window: 9000, price: preset.price }))
  })
  it('未知模型可照常手动编辑', () => {
    mocks.preset = null
    render(<ModelDialog model={model} onOpenChange={() => {}} />)
    expect(screen.queryByRole('button', { name: '应用预设能力参数' })).toBeNull()
    expect(screen.getByLabelText('显示名称')).toBeTruthy()
  })
})
