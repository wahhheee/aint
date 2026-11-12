import '@/i18n'
import { StrictMode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ProviderDialog } from './ProviderDialog'
const mocks = vi.hoisted(() => ({ mutate: vi.fn(), create: vi.fn(), update: vi.fn() }))
vi.mock('@/api/queries', () => ({
  useModels: () => ({ data: [] }),
  useProviderMutations: () => ({
    testProvider: { mutate: mocks.mutate, isIdle: true, isPending: false },
    createProvider: { mutate: mocks.create, isPending: false },
    updateProvider: { mutate: mocks.update, isPending: false },
  }),
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
  mocks.create.mockClear()
  mocks.update.mockClear()
  vi.unstubAllGlobals()
})

it('Vertex AI 表单提交项目、区域与凭据，服务账号支持多行 JSON', () => {
  render(<ProviderDialog open onOpenChange={() => {}} />)
  fireEvent.click(screen.getByRole('radio', { name: /Google Vertex AI/ }))
  expect((screen.getByLabelText('Base URL') as HTMLInputElement).value).toBe('https://aiplatform.googleapis.com')
  fireEvent.change(screen.getByLabelText('名称'), { target: { value: '我的 Vertex' } })
  fireEvent.click(screen.getByRole('combobox', { name: '认证方式' }))
  fireEvent.click(screen.getByRole('option', { name: '服务账号 JSON（自动刷新令牌）' }))
  const secret = '{\n"type":"service_account"\n}'
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: secret } })
  expect(screen.getByLabelText('API Key').tagName).toBe('TEXTAREA')
  fireEvent.change(screen.getByLabelText(/Google Cloud 项目 ID/), { target: { value: 'my-project' } })
  fireEvent.change(screen.getByLabelText(/区域（/), { target: { value: 'us-central1' } })
  fireEvent.click(screen.getByRole('button', { name: '保存并继续' }))
  expect(mocks.create).toHaveBeenCalledWith(
    expect.objectContaining({
      protocol: 'vertex',
      api_key: secret,
      options: { auth_mode: 'service_account', project: 'my-project', location: 'us-central1' },
    }),
    expect.anything(),
  )
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
