import { expect, test } from '@playwright/test'

test.beforeEach(async ({ request }) => {
  await request.patch('/api/v1/settings', { data: { onboarded: true, reading: { typewriter: 'instant' }, theme: 'dark' } })
})

test.afterEach(async ({ request }) => {
  await request.patch('/api/v1/settings', { data: { play: { auto_debug: true } } })
})

test('默认记录覆盖生成式开场，检查器位于用量环右侧且保留回合入口', async ({ page, request }, info) => {
  const settings = await request.get('/api/v1/settings').then((r) => r.json())
  expect(settings.play.auto_debug).toBe(true)
  const save = await request.post('/api/v1/saves', { data: { work_id: 'aint.first-story' } }).then((r) => r.json())
  let releaseCapture!: () => void
  const captureReady = new Promise<void>((resolve) => {
    releaseCapture = resolve
  })
  let submitted = false
  page.on('request', (r) => {
    if (r.method() === 'POST' && r.url().endsWith('/turns')) submitted = true
  })
  await page.route(`**/saves/${save.id}/debug`, async (route) => {
    if (route.request().method() === 'PATCH') await captureReady
    await route.continue()
  })
  const captureRequest = page.waitForRequest((r) => r.method() === 'PATCH' && r.url().endsWith('/debug'))
  await page.goto(`/play/${save.id}`)
  await captureRequest
  expect(submitted).toBe(false)
  releaseCapture()
  const debug = page.getByRole('button', { name: '调试', exact: true })
  await expect(debug).toHaveAttribute('aria-pressed', 'true')
  const choice = page.locator('article button').filter({ hasText: '继续前行' }).last()
  await expect(choice).toBeVisible()
  const status = await request.get(`/api/v1/saves/${save.id}/debug`).then((r) => r.json())
  expect(status.calls.length).toBeGreaterThan(0)
  const inspector = page.locator('header').getByRole('button', { name: '检查器', exact: true })
  await expect(inspector).toBeVisible()
  const placement = await inspector.evaluate((button) => {
    const ring = button.previousElementSibling!.getBoundingClientRect()
    const icon = button.getBoundingClientRect()
    const separator = button.nextElementSibling!.getBoundingClientRect()
    return { ringRight: ring.right, iconLeft: icon.left, iconRight: icon.right, separatorLeft: separator.left }
  })
  expect(placement.iconLeft).toBeGreaterThanOrEqual(placement.ringRight)
  expect(placement.separatorLeft).toBeGreaterThanOrEqual(placement.iconRight)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('debug-toolbar.png'), fullPage: true })
  await inspector.click()
  await expect(page.getByRole('dialog', { name: '游玩调试' })).toBeVisible()
  await page.keyboard.press('Escape')
  await debug.click()
  await expect(debug).toHaveAttribute('aria-pressed', 'false')
  await choice.click()
  await expect(page.locator('[data-turn]')).toHaveCount(2)
  await expect(debug).toHaveAttribute('aria-pressed', 'false')
  const paused = await request.get(`/api/v1/saves/${save.id}/debug`).then((r) => r.json())
  expect(paused.calls).toHaveLength(status.calls.length)
  await page.locator('[data-turn]').first().getByRole('button', { name: '回合操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '检查器', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '游玩调试' })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.goto('/')
  await page.goto(`/play/${save.id}`)
  await expect(debug).toHaveAttribute('aria-pressed', 'true')
  const ring = inspector.locator('xpath=preceding-sibling::button[1]')
  await ring.click()
  await expect(page.getByRole('dialog').getByRole('button', { name: '检查器', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '立即整理', exact: true })).toBeVisible()
})

test('自动记录设置持久化，关闭后可手动记录并查看空检查器', async ({ page, request }, info) => {
  await page.goto('/settings/developer')
  const autoDebug = page.getByRole('switch', { name: '游玩时自动开启调试记录', exact: true })
  await expect(autoDebug).toBeChecked()
  await page.screenshot({ path: info.outputPath('debug-settings.png'), fullPage: true })
  await autoDebug.click()
  await expect(autoDebug).not.toBeChecked()
  await page.reload()
  await expect(autoDebug).not.toBeChecked()
  const save = await request.post('/api/v1/saves', { data: { work_id: 'aint.first-story' } }).then((r) => r.json())
  await page.goto(`/play/${save.id}`)
  const debug = page.getByRole('button', { name: '调试', exact: true })
  const choice = page.locator('article button').filter({ hasText: '继续前行' }).last()
  await expect(choice).toBeVisible()
  await expect(debug).toHaveAttribute('aria-pressed', 'false')
  const status = await request.get(`/api/v1/saves/${save.id}/debug`).then((r) => r.json())
  expect(status.calls).toHaveLength(0)
  await page.locator('header').getByRole('button', { name: '检查器', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '游玩调试' })).toBeVisible()
  await page.keyboard.press('Escape')
  await debug.click()
  await expect(debug).toHaveAttribute('aria-pressed', 'true')
  await choice.click()
  await expect(page.locator('[data-turn]')).toHaveCount(2)
  await expect
    .poll(async () =>
      request
        .get(`/api/v1/saves/${save.id}/debug`)
        .then((r) => r.json())
        .then((s) => s.calls.length),
    )
    .toBeGreaterThan(0)
  await page.reload()
  await expect(debug).toHaveAttribute('aria-pressed', 'true')
  await debug.click()
  await expect(debug).toHaveAttribute('aria-pressed', 'false')
  await page.reload()
  await expect(debug).toHaveAttribute('aria-pressed', 'false')
})

test('自动记录失败提示错误且仍可游玩与手动重试', async ({ page, request }) => {
  const save = await request.post('/api/v1/saves', { data: { work_id: 'aint.first-story' } }).then((r) => r.json())
  await page.route(`**/saves/${save.id}/debug`, async (route) => {
    if (route.request().method() === 'PATCH')
      await route.fulfill({ status: 500, json: { error: { code: 'INTERNAL', message: '记录暂不可用' } } })
    else await route.continue()
  })
  await page.goto(`/play/${save.id}`)
  await expect(page.getByText('记录暂不可用', { exact: true })).toBeVisible()
  await expect(page.locator('article button').filter({ hasText: '继续前行' }).last()).toBeVisible()
  const debug = page.getByRole('button', { name: '调试', exact: true })
  await expect(debug).toHaveAttribute('aria-pressed', 'false')
  await page.unroute(`**/saves/${save.id}/debug`)
  await debug.click()
  await expect(debug).toHaveAttribute('aria-pressed', 'true')
})
