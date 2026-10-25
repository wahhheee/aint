import { test, expect } from '@playwright/test'

test('阅读、选项、调试、设置、目录与分支', async ({ page, request }) => {
  const created = await request.post('/api/v1/saves', {
    data: { work_id: 'yanyi.first-story', settings: { pace: 'short', reasoning: 'low' } },
  })
  expect(created.ok()).toBeTruthy()
  const save = await created.json()
  await request.patch('/api/v1/settings', { data: { reading: { typewriter: 'instant' }, theme: 'dark', onboarded: true } })
  const [fake] = await request.get('/api/v1/models').then((r) => r.json())
  await page.route('**/api/v1/models', (route) =>
    route.fulfill({
      json: [
        {
          ...fake,
          id: save.settings.narrator_model,
          display_name: '演示执笔模型',
          provider_id: 'provider-a',
          caps: { ...fake.caps, reasoning: 'effort' },
        },
        {
          ...fake,
          id: 'second-model',
          display_name: '演示执笔模型',
          provider_id: 'provider-b',
          caps: { ...fake.caps, reasoning: 'effort' },
        },
      ],
    }),
  )
  await page.route('**/api/v1/providers', (route) =>
    route.fulfill({
      json: [
        { id: 'provider-a', name: '供应商甲' },
        { id: 'provider-b', name: '供应商乙' },
      ],
    }),
  )
  let releaseEvents!: () => void
  const eventsReady = new Promise<void>((resolve) => {
    releaseEvents = resolve
  })
  let submitted = false
  page.on('request', (r) => {
    if (r.method() === 'POST' && r.url().endsWith('/turns')) submitted = true
  })
  await page.route(`**/saves/${save.id}/events`, async (route) => {
    await eventsReady
    await route.continue()
  })
  await page.goto(`/play/${save.id}`)
  await expect(page.getByRole('button', { name: '模型与推理' })).toBeVisible()
  expect(submitted).toBe(false)
  releaseEvents()
  const debug = page.getByRole('button', { name: '调试', exact: true })
  await debug.click()
  await expect(debug).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  const choice = page.locator('article button').filter({ hasText: '继续前行' }).last()
  await expect(choice).toBeVisible()
  const initial = await request.get(`/api/v1/saves/${save.id}/timeline`).then((r) => r.json())
  await choice.click()
  await expect(page.locator('[data-turn]')).toHaveCount(initial.nodes.length + 1)
  await expect(page.locator('.prose-book').last()).toContainText('远处的灯亮了')
  await expect(page.locator('.prose-book').last()).not.toContainText('<br/>')
  for (let n = 0; n < 4; n++) {
    await choice.click()
    await expect(page.locator('[data-turn]')).toHaveCount(initial.nodes.length + 2 + n)
  }
  const toolbar = page.getByRole('group', { name: '输入与游玩设置' })
  await toolbar.getByRole('button', { name: '模型与推理' }).click()
  await expect(page.getByRole('slider', { name: '推理强度' })).toBeVisible()
  await expect(page.getByRole('spinbutton')).toHaveCount(0)
  const modelSelect = page.getByRole('combobox', { name: '主笔模型' })
  await expect(modelSelect.locator('option')).toHaveText(['演示执笔模型 · 供应商甲', '演示执笔模型 · 供应商乙'])
  await modelSelect.selectOption('second-model')
  await page.getByRole('slider', { name: '推理强度' }).focus()
  await page.keyboard.press('ArrowRight')
  await page.screenshot({ path: `test-results/model-${test.info().project.name}.png`, fullPage: true })
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('alertdialog')).toContainText('文风')
  await page.getByRole('button', { name: '取消', exact: true }).click()
  const cancelled = await request.get(`/api/v1/saves/${save.id}`).then((r) => r.json())
  expect(cancelled.save.settings.narrator_model).toBe(save.settings.narrator_model)
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await page.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page.getByRole('slider', { name: '推理强度' })).toHaveCount(0)
  const settings = await request.get('/api/v1/settings').then((r) => r.json())
  const info = await request.get('/api/v1/system/info').then((r) => r.json())
  await toolbar.getByRole('button', { name: '上下文与成本' }).click()
  await expect(page.getByRole('slider', { name: '推理强度' })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: '主笔模型' })).toHaveCount(0)
  const budget = page.getByRole('spinbutton', { name: '整理阈值（token）', exact: true })
  await expect(budget).toHaveValue('')
  await expect(budget).toHaveAttribute('placeholder', String(settings.context.budget))
  const calls = page.getByRole('spinbutton', { name: '每回合模型调用上限' })
  await expect(calls).toHaveValue('')
  await expect(calls).toHaveAttribute('placeholder', String(settings.context.max_calls || info.turn_defaults.max_calls))
  await calls.fill('8')
  await page.screenshot({ path: `test-results/budget-${test.info().project.name}.png`, fullPage: true })
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(calls).toHaveCount(0)
  await toolbar.getByRole('button', { name: '篇幅（当前存档）', exact: true }).click()
  await page.getByRole('menuitem', { name: /长.*2000/ }).click()
  await expect(toolbar.getByRole('button', { name: '篇幅（当前存档）', exact: true })).toHaveText('长')
  const updated = await request.get(`/api/v1/saves/${save.id}`).then((r) => r.json())
  expect(updated.save.settings.max_calls).toBe(8)
  expect(updated.save.settings.pace).toBe('long')
  expect(updated.save.settings.narrator_model).toBe('second-model')
  expect(updated.save.settings.reasoning).toBe('medium')
  await page.reload()
  await expect(toolbar.getByRole('button', { name: '篇幅（当前存档）', exact: true })).toHaveText('长')
  if (test.info().project.name !== 'desktop') await page.getByRole('button', { name: '回合目录', exact: true }).click()
  const nav = page.getByRole('navigation', { name: '回合目录' })
  await expect(nav).toBeVisible()
  await nav.getByRole('button').first().click()
  await expect(page.getByRole('button', { name: '回到最新', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '回到最新', exact: true }).click()
  const dimensions = await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth }))
  expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.width)
  await page.screenshot({ path: `test-results/reader-${test.info().project.name}.png`, fullPage: true })
  const latest = page.locator('[data-turn]').last()
  await latest.getByRole('button', { name: '回合操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '重新生成', exact: true }).click()
  await expect(page.getByRole('button', { name: '上一个版本', exact: true }).last()).toBeVisible()
  await latest.getByRole('button', { name: '回合操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '修改正文', exact: true }).click()
  await page.getByRole('dialog').getByRole('textbox').fill('灯火映在水面上。')
  await page.getByRole('dialog').getByRole('button', { name: '保存', exact: true }).click()
  await expect(latest).toContainText('灯火映在水面上。')
  await latest.getByRole('button', { name: '上一个版本', exact: true }).click()
  await expect(latest).toContainText('远处的灯亮了')
  await request.patch(`/api/v1/saves/${save.id}`, { data: { settings: { keep_recent: 1 } } })
  const compact = await request.post(`/api/v1/saves/${save.id}/compact`)
  expect(compact.ok()).toBeTruthy()
  await expect
    .poll(async () => {
      const tl = await request.get(`/api/v1/saves/${save.id}/timeline`).then((r) => r.json())
      return tl.nodes.some((n: { kind: string }) => n.kind === 'compaction')
    })
    .toBeTruthy()
  await expect(page.getByText('正在整理记忆', { exact: true })).toHaveCount(0)

  await page.goto('/')
  await request.delete(`/api/v1/saves/${save.id}`)
})

test('后台任务默认关闭与模型设置入口', async ({ page, request }) => {
  await page.goto('/settings/background')
  await expect(page.getByRole('combobox', { name: '退出时生成标题' })).toHaveValue('off')
  await page.goto('/settings/providers')
  await expect(page.getByRole('heading', { name: '模型' }).first()).toBeVisible()
  const settings = await request.get('/api/v1/settings').then((r) => r.json())
  expect(settings.play.helpers).toBe(false)
})
