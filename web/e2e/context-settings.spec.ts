import { expect, test } from '@playwright/test'

test.beforeEach(async ({ request }) => {
  await request.patch('/api/v1/settings', { data: { onboarded: true, theme: 'dark', reading: { typewriter: 'instant' } } })
})

test('上下文档位、费用确认与持久化', async ({ page, request }) => {
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

    await budget.getByRole('radio', { name: '512k', exact: true }).click()
    const confirm = page.getByRole('alertdialog')
    await expect(confirm).toContainText('超出 256K 的上下文进行双倍收费')
    expect(mutations).toHaveLength(0)
    await confirm.getByRole('button', { name: '取消', exact: true }).click()
    await expect(budget.getByRole('radio', { name: '256k', exact: true })).toBeChecked()
    expect(mutations).toHaveLength(0)

    await budget.getByRole('radio', { name: '512k', exact: true }).click()
    await expect(confirm).toBeVisible()
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

test('高预算确认取消时开局保留表单，游玩丢弃配置并关闭面板', async ({ page, request }) => {
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
  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await expect.poll(savedBudget).toBe(256000)
  await expect(budget).toHaveCount(0)

  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await budget.fill('512000')
  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await expect(confirm).toContainText('双倍收费')
  expect(await savedBudget()).toBe(256000)
  await confirm.getByRole('button', { name: '取消', exact: true }).click()
  await expect(confirm).toHaveCount(0)
  await expect(budget).toHaveCount(0)
  expect(await savedBudget()).toBe(256000)
  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await expect(budget).toHaveValue('256000')
  await budget.fill('512000')
  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await confirm.getByRole('button', { name: '确定', exact: true }).click()
  await expect(confirm).toHaveCount(0)
  await expect.poll(savedBudget).toBe(512000)
  await expect(budget).toHaveCount(0)

  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await page.getByRole('spinbutton', { name: '保留原文（token）', exact: true }).fill('48000')
  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await expect(budget).toHaveCount(0)
  await expect(confirm).toHaveCount(0)
  await page.goto('/')
  await request.delete(`/api/v1/saves/${id}`)
})

test('离开游玩设置时校验输入，保存失败保留草稿并可重试', async ({ page, request }) => {
  const created = await request.post('/api/v1/saves', { data: { work_id: 'aint.misty-harbor' } })
  const { id } = await created.json()
  let attempts = 0
  let fail = true
  await page.route(`**/api/v1/saves/${id}`, async (route) => {
    if (route.request().method() === 'PATCH') {
      attempts++
      if (fail) {
        await route.fulfill({ status: 503, json: { error: { code: 'internal', message: '设置暂时无法保存' } } })
        return
      }
    }
    await route.continue()
  })
  await page.goto(`/play/${id}`)
  const trigger = page.getByRole('button', { name: '上下文与成本', exact: true })
  const calls = page.getByRole('spinbutton', { name: '每回合模型调用上限', exact: true })
  await trigger.click()
  await calls.fill('51')
  await trigger.click()
  await expect(calls).toHaveValue('51')
  expect(await calls.evaluate((el: HTMLInputElement) => el.validity.rangeOverflow)).toBe(true)
  expect(attempts).toBe(0)

  await calls.fill('8')
  await page.mouse.click(4, 4)
  await expect(page.getByText('设置暂时无法保存', { exact: true })).toBeVisible()
  await expect(calls).toBeEnabled()
  await expect(calls).toHaveValue('8')
  expect(attempts).toBe(1)
  fail = false
  await page.getByRole('button', { name: '模型与推理', exact: true }).click()
  await expect(page.getByRole('button', { name: '选择主笔模型', exact: true })).toBeVisible()
  expect(attempts).toBe(2)
  const saved = await request.get(`/api/v1/saves/${id}`).then((r) => r.json())
  expect(saved.save.settings.max_calls).toBe(8)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: '选择主笔模型', exact: true })).toHaveCount(0)
  expect(attempts).toBe(2)
  await page.mouse.move(4, 4)
  await expect(page.getByText('设置暂时无法保存', { exact: true })).toHaveCount(0)
  await trigger.click()
  await expect(calls).toHaveValue('8')
  await calls.fill('9')
  await page.keyboard.press('Escape')
  await expect(calls).toHaveCount(0)
  await page.reload()
  await trigger.click()
  await expect(calls).toHaveValue('9')
  await page.goto('/')
  await request.delete(`/api/v1/saves/${id}`)
})

test('发送先确认并等待游玩设置保存，取消时保留输入且重复点击只发起一个回合', async ({ page, request }) => {
  const created = await request.post('/api/v1/saves', { data: { work_id: 'aint.misty-harbor' } })
  const { id } = await created.json()
  let release!: () => void
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  const order: string[] = []
  let budgetAtSubmit = 0
  await page.route(`**/api/v1/saves/${id}`, async (route) => {
    if (route.request().method() === 'PATCH') {
      order.push('settings')
      await gate
    }
    await route.continue()
  })
  await page.route(`**/api/v1/saves/${id}/turns`, async (route) => {
    order.push('turn')
    budgetAtSubmit = (await request.get(`/api/v1/saves/${id}`).then((r) => r.json())).save.settings.context_budget
    await route.continue()
  })
  await page.goto(`/play/${id}`)
  const input = page.getByRole('textbox', { name: '写下你的行动……', exact: true })
  const send = page.getByRole('button', { name: '发送', exact: true })
  await input.fill('继续向前')
  await expect(send).toBeEnabled()
  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  const budget = page.getByRole('spinbutton', { name: '整理阈值（token）', exact: true })
  await budget.fill('512000')
  await send.click()
  const confirm = page.getByRole('alertdialog')
  await expect(confirm).toContainText('双倍收费')
  await confirm.getByRole('button', { name: '取消', exact: true }).click()
  await expect(confirm).toHaveCount(0)
  await expect(budget).toHaveCount(0)
  await expect(input).toHaveValue('继续向前')
  expect(order).toEqual([])

  await page.getByRole('button', { name: '上下文与成本', exact: true }).click()
  await expect(budget).toHaveValue('')
  await budget.fill('512000')
  await send.click()
  await confirm.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '正在保存' })).toBeVisible()
  await expect.poll(() => order).toEqual(['settings'])
  await send.click()
  expect(order).toEqual(['settings'])
  release()
  await expect(input).toHaveValue('')
  await expect(budget).toHaveCount(0)
  await expect(page.locator('article button').filter({ hasText: '继续前行' }).last()).toBeVisible()
  expect(order).toEqual(['settings', 'turn'])
  expect(budgetAtSubmit).toBe(512000)
  await page.goto('/')
  await request.delete(`/api/v1/saves/${id}`)
})
