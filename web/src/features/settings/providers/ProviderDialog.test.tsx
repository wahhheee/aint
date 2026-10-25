import '@/i18n'
import { StrictMode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { ProviderDialog } from './ProviderDialog'
const mocks = vi.hoisted(() => ({ mutate: vi.fn() }))
vi.mock('@/api/queries', () => ({
  useModels: () => ({ data: [] }),
  useProviderMutations: () => ({ testProvider: { mutate: mocks.mutate, isIdle: true, isPending: false } }),
}))
afterEach(() => {
  cleanup()
  mocks.mutate.mockClear()
})
it('StrictMode 和重新渲染都不自动消耗额度；明确点击后才测试', () => {
  const view = (
    <StrictMode>
      <ProviderDialog
        open
        onOpenChange={() => {}}
        step="test"
        provider={{
          id: 'p',
          name: '测试',
          protocol: 'anthropic',
          base_url: 'https://example.com',
          key_masked: '***',
          created_at: 0,
          price_multiplier: 1,
        }}
      />
    </StrictMode>
  )
  const result = render(view)
  result.rerender(view)
  expect(mocks.mutate).not.toHaveBeenCalled()
  expect(screen.getByText(/消耗少量额度/)).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '开始测试' }))
  expect(mocks.mutate).toHaveBeenCalledTimes(1)
})
it('跳过测试不调用模型', () => {
  const close = vi.fn()
  render(
    <ProviderDialog
      open
      onOpenChange={close}
      step="test"
      provider={{
        id: 'p',
        name: '测试',
        protocol: 'anthropic',
        base_url: 'https://example.com',
        key_masked: '***',
        created_at: 0,
        price_multiplier: 1,
      }}
    />,
  )
  fireEvent.click(screen.getByRole('button', { name: '跳过测试' }))
  expect(mocks.mutate).not.toHaveBeenCalled()
  expect(close).toHaveBeenCalledWith(false)
})
