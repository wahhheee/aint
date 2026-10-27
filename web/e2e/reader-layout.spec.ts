import { test, expect, type Page } from '@playwright/test'

/** 提示、正文与输入共享中轴与宽度，滚动条不能挤偏版心。 */
async function expectCentered(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => {
        const rail = document.querySelector('.reader-rail')!.getBoundingClientRect()
        const columns = [...document.querySelectorAll('.reader-column')].map((el) => el.getBoundingClientRect())
        return Math.max(
          ...columns.map((r) => Math.abs(r.x + r.width / 2 - rail.x - rail.width / 2)),
          ...columns.map((r) => Math.abs(r.width - columns[0].width)),
          document.documentElement.scrollWidth - innerWidth,
        )
      }),
    )
    .toBeLessThan(1)
}

test('阅读版心随两侧栏自适应，目录支持桌面折叠与手机侧滑', async ({ page, request }, info) => {
  const created = await request.post('/api/v1/saves', { data: { work_id: 'aint.misty-harbor' } })
  expect(created.ok()).toBeTruthy()
  const save = await created.json()
  await request.patch('/api/v1/settings', {
    data: { reading: { typewriter: 'instant', reduce_motion: 'system' }, theme: 'dark', onboarded: true },
  })
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto(`/play/${save.id}`)
  await expect(page.locator('article').last()).toBeVisible()
  const toggle = page.getByRole('button', { name: '回合目录', exact: true })
  const nav = page.getByRole('navigation', { name: '回合目录' })
  const panelToggle = page.getByRole('button', { name: '面板', exact: true })

  if (info.project.name === 'desktop') {
    for (const width of [2048, 1600, 1280, 1200]) {
      await page.setViewportSize({ width, height: 1000 })
      for (const panels of [true, false]) {
        const sidebar = page.locator('.reader-panels')
        if ((await sidebar.getAttribute('data-open')) !== String(panels)) await panelToggle.click()
        for (const index of [true, false]) {
          if ((await toggle.getAttribute('aria-expanded')) !== String(index)) await toggle.click()
          await expect(page.locator('.reader-index')).toHaveCSS('width', index ? '208px' : '0px')
          await expect(sidebar).toHaveCSS('width', panels ? '320px' : '0px')
          await expectCentered(page)
          if (!index) await expect(nav).toHaveCount(0)
          if (width === 1600) await page.screenshot({ path: `test-results/layout-${width}-${panels}-${index}.png`, animations: 'disabled' })
        }
      }
    }

    // 展开过程中也保持同轴，宽度经过中间值，避免直接跳变。
    const halfway = await toggle.evaluate(async (button) => {
      ;(button as HTMLButtonElement).click()
      await new Promise(requestAnimationFrame)
      await new Promise(requestAnimationFrame)
      await new Promise(requestAnimationFrame)
      return document.querySelector('.reader-index')!.getBoundingClientRect().width
    })
    expect(halfway).toBeGreaterThan(0)
    expect(halfway).toBeLessThan(208)
    await expectCentered(page)
    await expect(page.locator('.reader-index')).toHaveCSS('width', '208px')
    await nav.getByRole('button').first().click()
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await toggle.click()
    await page.reload()
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await expectCentered(page)

    // 缩到平板后使用独立的抽屉状态，返回桌面仍记住收起偏好。
    await page.setViewportSize({ width: 900, height: 1000 })
    await toggle.click()
    await expect(page.getByRole('dialog', { name: '回合目录' })).toBeVisible()
    await page.setViewportSize({ width: 1280, height: 1000 })
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await expectCentered(page)

    await page.emulateMedia({ reducedMotion: 'reduce' })
    await toggle.click()
    await expect(page.locator('.reader-index')).toHaveCSS('width', '208px')
    const durations = await page.locator('.reader-index').evaluate((el) => getComputedStyle(el).transitionDuration)
    expect(durations.split(',').every((duration) => parseFloat(duration) <= 0.001)).toBe(true)
  } else {
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await expectCentered(page)
    await toggle.click()
    const dialog = page.getByRole('dialog', { name: '回合目录' })
    await expect(dialog).toBeVisible()
    await expect(nav.getByRole('textbox')).toBeFocused()
    await nav.getByRole('textbox').fill('不存在的回合内容')
    await expect(nav).toContainText('没有匹配的已加载回合')
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(toggle).toBeFocused()
    await toggle.click()
    await nav.getByRole('textbox').fill('')
    await page.screenshot({ path: `test-results/layout-index-${info.project.name}.png`, animations: 'disabled', scale: 'css' })
    await nav.getByRole('button').first().click()
    await expect(dialog).toHaveCount(0)
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await expectCentered(page)
    await page.screenshot({ path: `test-results/layout-${info.project.name}.png`, animations: 'disabled', scale: 'css' })
  }
  await page.goto('/')
  await request.delete(`/api/v1/saves/${save.id}`)
})
