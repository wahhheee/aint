import { expect, test } from '@playwright/test'

test('倒悬的街角的三个开场与面板', async ({ page, request }, info) => {
  await request.patch('/api/v1/settings', { data: { onboarded: true, reading: { typewriter: 'instant' } } })
  for (const [opening, silver, location] of [
    ['alley-after-rain', 45, '灰羽街区 · 鸦羽巷7号'],
    ['floating-woods', 53, '深渊二层 · 失重浮岛林海'],
    ['black-thorn-night', 42, '灰羽街区 · 黑荆棘馆'],
  ] as const) {
    const response = await request.post('/api/v1/saves', {
      data: { work_id: 'aint.inverted-stars', opening_id: opening, form: { 'protagonist.name': '洛恩' } },
    })
    expect(response.ok()).toBeTruthy()
    const save = await response.json()
    const state = await request.get(`/api/v1/saves/${save.id}/state`).then((r) => r.json())
    expect(state.state.silver).toBe(silver)
    expect(state.state.location).toBe(location)
    await page.goto(`/play/${save.id}`)
    await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
    await expect(page.getByText(location, { exact: false }).first()).toBeVisible()
    await expect(page.getByText('面板渲染失败', { exact: false })).toHaveCount(0)
    await expect(page.locator('body')).toHaveJSProperty('scrollWidth', await page.locator('body').evaluate((e) => e.clientWidth))
    await page.screenshot({ path: info.outputPath(`${opening}.png`), fullPage: true })
  }
})
