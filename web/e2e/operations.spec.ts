import { expect, test, type Frame, type Page } from '@playwright/test'

import type { Timeline } from '../src/api/types'

/** 暂存下次导航的首次时间线请求，用操作前的快照模拟晚到的旧响应。 */
async function delayOldTimeline(page: Page, old: Timeline) {
  const released = Promise.withResolvers<void>()
  let navigated = false
  let held = 0
  const onNavigation = (frame: Frame) => {
    if (frame === page.mainFrame()) navigated = true
  }
  page.on('framenavigated', onNavigation)
  await page.route('**/api/v1/saves/*/timeline?*', async (route) => {
    if (navigated && held === 0) {
      held++
      await released.promise
      await route.fulfill({ json: old })
    } else {
      await route.continue()
    }
  })
  return {
    held: () => held,
    release: async () => {
      page.off('framenavigated', onNavigation)
      released.resolve()
      await page.unrouteAll({ behavior: 'wait' })
    },
  }
}

test('开场提交期间首次时间线仍在加载时显示回合菜单', async ({ page, request }) => {
  await request.patch('/api/v1/settings', { data: { reading: { typewriter: 'instant' }, theme: 'light', onboarded: true } })
  const created = await request.post('/api/v1/saves', { data: { work_id: 'aint.first-story' } })
  expect(created.ok()).toBeTruthy()
  const save = await created.json()
  const old = await request.get(`/api/v1/saves/${save.id}/timeline`).then((r) => r.json())
  const delayed = await delayOldTimeline(page, old)
  try {
    await page.goto(`/play/${save.id}`)
    await expect.poll(delayed.held).toBeGreaterThan(0)
    const turn = page.locator('[data-turn]')
    await expect(turn).toHaveCount(1)
    await expect(turn.getByRole('button', { name: '回合操作', exact: true })).toBeEnabled()
    await delayed.release()
    await turn.getByRole('button', { name: '回合操作', exact: true }).click()
    await expect(page.getByRole('menuitem', { name: '按要求重写', exact: true })).toBeVisible()
  } finally {
    await delayed.release()
  }
})

test('重写原位替换、刷新后取消恢复、补选项与选择续写', async ({ page, request }, info) => {
  await request.patch('/api/v1/settings', { data: { reading: { typewriter: 'instant' }, theme: 'light', onboarded: true } })
  const created = await request.post('/api/v1/saves', { data: { work_id: 'aint.first-story' } })
  expect(created.ok()).toBeTruthy()
  const save = await created.json()
  await page.goto(`/play/${save.id}`)
  await expect(page.locator('article button').filter({ hasText: '继续前行' }).last()).toBeVisible()
  await page.locator('[data-turn]').last().getByRole('button', { name: '回合操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '按要求重写', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('textbox').fill('放慢脚步，多写雨声。')
  await page.screenshot({ path: info.outputPath('rewrite-dialog.png'), fullPage: true })
  await dialog.getByRole('button', { name: '按要求重写', exact: true }).click()
  await expect(page.getByRole('button', { name: '停止', exact: true })).toBeVisible()
  await expect(page.getByText('雨声还在继续。', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('button', { name: '停止', exact: true })).toBeVisible()
  await expect(page.getByText('雨声还在继续。', { exact: true })).toBeVisible()
  await expect(page.locator('.prose-book').filter({ hasText: '远处的灯亮了' })).toHaveCount(0)
  await page.getByRole('button', { name: '停止', exact: true }).click()
  await expect(page.locator('.prose-book').filter({ hasText: '远处的灯亮了' })).toHaveCount(1)
  await expect(page.getByRole('button', { name: '停止', exact: true })).toHaveCount(0)

  const input = page.getByRole('textbox', { name: '写下你的行动……' })
  await input.fill('/rewrite 放慢脚步，多写雨声。')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('[data-turn]')).toHaveCount(1)
  await expect(page.getByRole('button', { name: '生成选项', exact: true })).toBeVisible()
  await expect(page.locator('.prose-book').filter({ hasText: '那封信还在口袋里' })).toHaveCount(1)
  await expect(page.locator('.prose-book').filter({ hasText: '远处的灯亮了' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '上一个版本', exact: true })).toBeVisible()
  const before = await request.get(`/api/v1/saves/${save.id}/state`).then((r) => r.json())
  const old = await request.get(`/api/v1/saves/${save.id}/timeline`).then((r) => r.json())
  await input.fill('/choices')
  const [submitted] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/choices')),
    page.getByRole('button', { name: '发送', exact: true }).click(),
  ])
  expect(submitted.status()).toBe(202)
  const choice = page.locator('article button').filter({ hasText: '走近那盏灯' }).last()
  const delayed = await delayOldTimeline(page, old)
  try {
    await page.reload()
    await expect.poll(delayed.held).toBeGreaterThan(0)
    await expect(choice).toBeVisible()
  } finally {
    await delayed.release()
  }
  await expect(page.locator('.prose-book').filter({ hasText: '那封信还在口袋里' })).toHaveCount(1)
  await expect(page.locator('[data-turn]')).toHaveCount(1)
  await expect(page.getByText('好的，我来生成选项。', { exact: false })).toHaveCount(0)
  const after = await request.get(`/api/v1/saves/${save.id}/state`).then((r) => r.json())
  expect(after.state).toEqual(before.state)
  await page.screenshot({ path: info.outputPath('requested-choices.png'), fullPage: true })
  expect(await page.locator('body').evaluate((e) => e.scrollWidth <= innerWidth)).toBe(true)
  await choice.click()
  await expect(page.locator('[data-turn]')).toHaveCount(2)
  await expect(page.locator('article button').filter({ hasText: '继续前行' }).last()).toBeVisible()
})
