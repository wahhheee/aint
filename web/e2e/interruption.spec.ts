import { expect, test } from '@playwright/test'

test('未保存的中断在正文外提示，刷新可恢复且关闭不改存档', async ({ page, request }) => {
  await request.patch('/api/v1/settings', { data: { onboarded: true, reading: { typewriter: 'instant' } } })
  const { id } = await request.post('/api/v1/saves', { data: { work_id: 'aint.misty-harbor' } }).then((r) => r.json())
  const base = `/api/v1/saves/${id}`
  const original = await request.get(`${base}/timeline`).then((r) => r.json())
  const text = '港口被雾笼罩。\n\n'.repeat(180)
  const edit = await request.post(`${base}/nodes/${original.head_id}/rewrite`, { data: { text } })
  expect(edit.ok()).toBeTruthy()
  const { head_id: head } = await edit.json()
  await page.goto(`/play/${id}`)
  const scroll = page.locator('.reader-scroll')
  await expect(scroll).toBeVisible()
  await expect.poll(() => scroll.evaluate((el) => el.scrollHeight - el.clientHeight)).toBeGreaterThan(500)
  await scroll.evaluate((el) => el.scrollTo(0, 0))
  // 当前选项不匹配会在回合准备阶段失败，不生成节点。
  expect((await request.post(`${base}/turns`, { data: { kind: 'choice', choice_id: 'missing' } })).ok()).toBeTruthy()
  const notice = page.getByRole('alert').filter({ hasText: '本回合未保存' })
  await expect(notice).toBeInViewport()
  await expect(scroll.getByRole('alert')).toHaveCount(0)
  await page.reload()
  await expect(notice).toBeInViewport()
  await expect(notice).toContainText('选项')
  // 参数错误需要更换输入，不提供无效的原样重试。
  await expect(notice.getByRole('button', { name: '重试', exact: true })).toHaveCount(0)
  await notice.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(notice).toHaveCount(0)
  await page.reload()
  await expect(scroll).toBeVisible()
  await expect(notice).toHaveCount(0)
  expect((await request.get(`${base}/timeline`).then((r) => r.json())).head_id).toBe(head)
})
