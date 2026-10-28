import { expect, request } from '@playwright/test'
import { fileURLToPath } from 'node:url'

// 玩法回归使用独立夹具，通过普通导入流程装入测试服务。
export default async function setup() {
  const api = await request.newContext({ baseURL: 'http://127.0.0.1:7879' })
  try {
    const library = await api.get('/api/v1/works')
    expect(library.ok()).toBeTruthy()
    const initial = await library.text()
    expect(initial).toContain('aint.inverted-stars')
    expect(initial).toContain('aint.first-story')
    expect(initial).not.toContain('aint.misty-harbor')
    const response = await api.post('/api/v1/works/import', {
      data: { source: 'dir', path: fileURLToPath(new URL('../../testdata/works/misty-harbor', import.meta.url)) },
    })
    expect(response.ok()).toBeTruthy()
    const preview = await response.json()
    expect(preview.task).toBeTruthy()
    const confirmed = await api.post(`/api/v1/works/import/${preview.task}/confirm`)
    expect(confirmed.ok()).toBeTruthy()
  } finally {
    await api.dispose()
  }
}
