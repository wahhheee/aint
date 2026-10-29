import { expect, test } from '@playwright/test'

test.beforeEach(async ({ request }) => {
  await request.patch('/api/v1/settings', { data: { onboarded: true, theme: 'dark', reading: { typewriter: 'instant' } } })
})

test('上下文档位、费用确认与持久化', async ({ page, request }, info) => {
  const original = await request.get('/api/v1/settings').then((r) => r.json())
  expect(original.context.budget).toBe(256000)
  expect(original.context.keep_recent).toBe(24000)
  const mutations: unknown[] = []
  page.on('request', (r) => {
    if (r.method() === 'PATCH' && r.url().endsWith('/settings')) mutations.push(r.postDataJSON())
  })
  try {
    await page.goto('/settings/context')
    const budget = page.getByRole('radiogroup', { name: '工作上下文预算' })
    const keep = page.getByRole('radiogroup', { name: '保留原文', exact: true })
    await expect(budget.getByRole('radio', { name: '256k', exact: true })).toBeChecked()
    await expect(keep.getByRole('radio', { name: '24k', exact: true })).toBeChecked()
    await expect(budget.getByRole('radio')).toHaveCount(5)
    await expect(keep.getByRole('radio')).toHaveCount(4)
    for (const control of [budget, keep]) {
      expect(
        await control.evaluate((el) => {
          const rect = el.getBoundingClientRect()
          return rect.left >= 0 && rect.right <= innerWidth
        }),
      ).toBe(true)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(0)
    await budget.scrollIntoViewIfNeeded()
    await page.screenshot({ path: info.outputPath('context-settings.png'), fullPage: true })

    await budget.getByRole('radio', { name: '512k', exact: true }).click()
    const confirm = page.getByRole('alertdialog')
    await expect(confirm).toContainText('超出 256K 的上下文进行双倍收费')
    expect(mutations).toHaveLength(0)
    await page.screenshot({ path: info.outputPath('context-confirm.png'), fullPage: true })
    await confirm.getByRole('button', { name: '取消', exact: true }).click()
    await expect(budget.getByRole('radio', { name: '256k', exact: true })).toBeChecked()
    expect(mutations).toHaveLength(0)

    await budget.getByRole('radio', { name: '512k', exact: true }).click()
    await page.keyboard.press('Escape')
    await expect(confirm).toHaveCount(0)
    expect(mutations).toHaveLength(0)

    await budget.getByRole('radio', { name: '512k', exact: true }).click()
    await confirm.getByRole('button', { name: '确定', exact: true }).click()
    await expect(budget.getByRole('radio', { name: '512k', exact: true })).toBeChecked()
    await keep.getByRole('radio', { name: '48k', exact: true }).click()
    await expect(keep.getByRole('radio', { name: '48k', exact: true })).toBeChecked()
    expect(mutations).toEqual([{ context: { budget: 512000 } }, { context: { keep_recent: 48000 } }])
    await page.reload()
    await expect(budget.getByRole('radio', { name: '512k', exact: true })).toBeChecked()
    await expect(keep.getByRole('radio', { name: '48k', exact: true })).toBeChecked()
    await expect(confirm).toHaveCount(0)

    await budget.getByRole('radio', { name: '256k', exact: true }).click()
    await expect(budget.getByRole('radio', { name: '256k', exact: true })).toBeChecked()
    await expect(confirm).toHaveCount(0)
    await budget.getByRole('radio', { name: '512k', exact: true }).click()
    await expect(confirm).toBeVisible()
    await confirm.getByRole('button', { name: '取消', exact: true }).click()
  } finally {
    await request.patch('/api/v1/settings', { data: { context: original.context } })
  }
})

test('开局与游玩保存高预算前确认费用，取消保留表单', async ({ page, request }) => {
  let createdCount = 0
  page.on('request', (r) => {
    if (r.method() === 'POST' && r.url().endsWith('/saves')) createdCount++
  })
  await page.goto('/works/aint.misty-harbor')
  await page.getByRole('button', { name: '开始新故事', exact: true }).click()
  const form = page.getByRole('dialog')
  await form.getByLabel('你的名字', { exact: true }).fill('阿宁')
  await form.getByText('上下文与成本', { exact: true }).click()
  const budget = page.getByRole('spinbutton', { name: '整理阈值（token）', exact: true })
  await expect(budget).toHaveAttribute('placeholder', '256000')
  await expect(page.getByRole('spinbutton', { name: '保留原文（token）', exact: true })).toHaveAttribute('placeholder', '24000')
  await budget.fill('512000')
  await form.getByRole('button', { name: '开始', exact: true }).click()
  const confirm = page.getByRole('alertdialog')
  await expect(confirm).toContainText('双倍收费')
  expect(createdCount).toBe(0)
  await confirm.getByRole('button', { name: '取消', exact: true }).click()
  await expect(budget).toHaveValue('512000')
  expect(createdCount).toBe(0)
  await form.getByRole('button', { name: '开始', exact: true }).click()
  await confirm.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page).toHaveURL(/\/play\//)
  const id = page.url().split('/').at(-1)!
  const savedBudget = async () => (await request.get(`/api/v1/saves/${id}`).then((r) => r.json())).save.settings.context_budget
  expect(await savedBudget()).toBe(512000)
  // 延迟设置写入，确认框关闭后仍须等待保存完成。
  await page.route(`**/api/v1/saves/${id}`, async (route) => {
    if (route.request().method() === 'PATCH') await new Promise((resolve) => setTimeout(resolve, 300))
    await route.continue()
  })

  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await budget.fill('256000')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect.poll(savedBudget).toBe(256000)
  await expect(budget).toHaveCount(0)

  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await budget.fill('512000')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(confirm).toContainText('双倍收费')
  expect(await savedBudget()).toBe(256000)
  await confirm.getByRole('button', { name: '取消', exact: true }).click()
  await expect(budget).toHaveValue('512000')
  expect(await savedBudget()).toBe(256000)
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await confirm.getByRole('button', { name: '确定', exact: true }).click()
  await expect(confirm).toHaveCount(0)
  await expect.poll(savedBudget).toBe(512000)
  await expect(budget).toHaveCount(0)

  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await page.getByRole('spinbutton', { name: '保留原文（token）', exact: true }).fill('48000')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(budget).toHaveCount(0)
  await expect(confirm).toHaveCount(0)
  await page.goto('/')
  await request.delete(`/api/v1/saves/${id}`)
})
