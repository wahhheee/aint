import { expect, test, type Page } from '@playwright/test'

test('不再提示分别记忆模型与预算，刷新和其他存档仍生效', async ({ page, request, browser }) => {
  const original = await request.get('/api/v1/settings').then((r) => r.json())
  const saves: string[] = []
  const otherContext = await browser.newContext()
  try {
    await request.patch('/api/v1/settings', {
      data: {
        onboarded: true,
        theme: 'dark',
        dismissed_warnings: { model_switch: false, high_context_budget: false },
      },
    })
    for (let i = 0; i < 2; i++) {
      const save = await request.post('/api/v1/saves', { data: { work_id: 'aint.misty-harbor' } }).then((r) => r.json())
      saves.push(save.id)
    }
    const [fake] = await request.get('/api/v1/models').then((r) => r.json())
    const configureModels = (target: Page) =>
      target.route('**/api/v1/models', (route) =>
        route.fulfill({
          json: [
            { ...fake, display_name: '当前模型', caps: { ...fake.caps, reasoning: 'effort' } },
            { ...fake, id: 'other-model', display_name: '另一模型', caps: { ...fake.caps, reasoning: 'effort' } },
          ],
        }),
      )
    const switchModel = async (target: Page) => {
      await target.getByRole('button', { name: '模型与推理', exact: true }).click()
      await target.getByRole('button', { name: '选择主笔模型', exact: true }).click()
      await target.getByRole('button', { name: '另一模型', exact: true }).click()
      await target.getByRole('slider', { name: '推理强度' }).waitFor()
      await target.getByRole('button', { name: '模型与推理', exact: true }).click()
    }
    const changeBudget = async (target: Page) => {
      await target.getByRole('button', { name: '上下文与成本', exact: true }).click()
      await target.getByRole('spinbutton', { name: '整理阈值（token）', exact: true }).fill('512000')
      await target.getByRole('button', { name: '上下文与成本', exact: true }).click()
    }
    await configureModels(page)
    await page.goto(`/play/${saves[0]}`)
    const other = await otherContext.newPage()
    await configureModels(other)
    await other.goto(page.url().replace(saves[0], saves[1]))
    await expect(other.getByRole('button', { name: '模型与推理', exact: true })).toBeVisible()
    await switchModel(page)
    const confirm = page.getByRole('alertdialog')
    await expect(confirm).toContainText('文风')
    await confirm.getByRole('checkbox', { name: '不再提示' }).check()
    await confirm.getByRole('button', { name: '确定', exact: true }).click()
    await expect(confirm).toHaveCount(0)
    await expect(page.getByRole('slider')).toHaveCount(0)
    let settings = await request.get('/api/v1/settings').then((r) => r.json())
    expect(settings.dismissed_warnings).toEqual({ model_switch: true, high_context_budget: false })

    await changeBudget(page)
    await expect(confirm).toContainText('双倍收费')
    await confirm.getByRole('checkbox', { name: '不再提示' }).check()
    await page.keyboard.press('Escape')
    await expect(confirm).toHaveCount(0)
    await expect(page.getByRole('spinbutton')).toHaveCount(0)
    settings = await request.get('/api/v1/settings').then((r) => r.json())
    expect(settings.dismissed_warnings.high_context_budget).toBe(false)
    await changeBudget(page)
    await confirm.getByRole('checkbox', { name: '不再提示' }).check()
    await confirm.getByRole('button', { name: '确定', exact: true }).click()
    await expect(confirm).toHaveCount(0)
    await expect(page.getByRole('spinbutton')).toHaveCount(0)
    await page.reload()

    await switchModel(other)
    await expect(other.getByRole('slider')).toHaveCount(0)
    await expect(other.getByRole('alertdialog')).toHaveCount(0)
    await changeBudget(other)
    await expect(other.getByRole('spinbutton')).toHaveCount(0)
    await expect(other.getByRole('alertdialog')).toHaveCount(0)
    const saved = await request.get(`/api/v1/saves/${saves[1]}`).then((r) => r.json())
    expect(saved.save.settings.narrator_model).toBe('other-model')
    expect(saved.save.settings.context_budget).toBe(512000)

    await other.goto(page.url().replace(`/play/${saves[0]}`, '/settings/context'))
    await other.getByRole('radio', { name: '512k', exact: true }).click()
    await expect(other.getByRole('radio', { name: '512k', exact: true })).toBeChecked()
    await expect(other.getByRole('alertdialog')).toHaveCount(0)
  } finally {
    await otherContext.close()
    await page.goto('/')
    for (const id of saves) await request.delete(`/api/v1/saves/${id}`)
    await request.patch('/api/v1/settings', { data: { dismissed_warnings: original.dismissed_warnings, context: original.context } })
  }
})
