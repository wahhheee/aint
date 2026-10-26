import { expect, test, type Page } from '@playwright/test'

async function switchOpening(page: Page, title: string) {
  await page.getByRole('button', { name: '切换开场', exact: true }).click()
  await page.getByRole('dialog').getByRole('radio', { name: title, exact: false }).click()
}

test.beforeEach(async ({ request }) => {
  await request.patch('/api/v1/settings', { data: { reading: { typewriter: 'instant' }, theme: 'light', onboarded: true } })
})

test('开局选择、静态与生成式来回切换、重进及跨页面同步', async ({ page, request, context }, info) => {
  await page.goto('/works/aint.misty-harbor')
  await page.getByRole('button', { name: '开始新故事', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('radio', { name: /邮局醒来/ })).toBeChecked()
  await dialog.getByRole('radio', { name: /乘船入港/ }).click()
  await dialog.getByLabel('你的名字', { exact: true }).fill('阿宁')
  await dialog.getByRole('button', { name: '开始', exact: true }).click()
  await expect(page).toHaveURL(/\/play\//)
  const id = page.url().split('/').at(-1)!
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
  let current = await request.get(`/api/v1/saves/${id}`).then((r) => r.json())
  expect(current.save.opening_id).toBe('arriving')
  expect(current.save.opening_locked).toBe(false)
  const generatedHead = current.save.head_node_id
  const other = await context.newPage()
  await other.goto(page.url())
  await switchOpening(page, '邮局醒来')
  await expect(page.getByText('阿宁——信封上', { exact: false })).toBeVisible()
  await expect(other.getByText('阿宁——信封上', { exact: false })).toBeVisible()
  const state = await request.get(`/api/v1/saves/${id}/state`).then((r) => r.json())
  expect(state.state.trust['林舟']).toBe(2)
  await page.reload()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
  await page.screenshot({ path: info.outputPath('opening-static.png'), fullPage: true })
  await page.getByRole('button', { name: '切换开场', exact: true }).click()
  await page.screenshot({ path: info.outputPath('opening-picker.png'), fullPage: true })
  await page
    .getByRole('dialog')
    .getByRole('radio', { name: /乘船入港/ })
    .click()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
  await expect
    .poll(async () => {
      current = await request.get(`/api/v1/saves/${id}`).then((r) => r.json())
      return current.needs_opening || !!current.running_turn_id
    })
    .toBe(false)
  expect(current.save.head_node_id).not.toBe(generatedHead)
  expect(current.save.opening_locked).toBe(false)
  const timeline = await request.get(`/api/v1/saves/${id}/timeline`).then((r) => r.json())
  expect(timeline.nodes).toHaveLength(1)
  await other.close()
})

test('中断首轮可取消或确认切换，场外对话完成后锁定', async ({ page, request }) => {
  const response = await request.post('/api/v1/saves', { data: { work_id: 'aint.misty-harbor' } })
  const save = await response.json()
  await page.goto(`/play/${save.id}`)
  const input = page.getByRole('textbox', { name: '写下你的行动……' })
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
  await input.fill('/ooc 连接测试')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.getByText('重连测试正文', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '停止', exact: true }).click()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
  await switchOpening(page, '乘船入港')
  await expect(page.getByRole('alertdialog')).toContainText('清除当前未完成的互动')
  await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.getByRole('dialog').getByRole('radio', { name: /邮局醒来/ })).toBeChecked()
  await page
    .getByRole('dialog')
    .getByRole('radio', { name: /乘船入港/ })
    .click()
  await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toBeEnabled()
  await expect(page.getByText('重连测试正文', { exact: false })).toHaveCount(0)
  await input.fill('/ooc 谢谢')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toHaveCount(0, { timeout: 15000 })
  await page.reload()
  await expect(page.getByRole('button', { name: '切换开场', exact: true })).toHaveCount(0)
  const current = await request.get(`/api/v1/saves/${save.id}`).then((r) => r.json())
  expect(current.save.opening_locked).toBe(true)
  expect(
    (
      await request.put(`/api/v1/saves/${save.id}/opening`, {
        data: { opening_id: 'post-office', expected_head_id: current.save.head_node_id },
      })
    ).status(),
  ).toBe(409)
})
