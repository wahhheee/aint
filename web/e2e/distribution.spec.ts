import { test, expect, type APIRequestContext } from '@playwright/test'
import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

function fixture(id: string) {
  const dir = mkdtempSync(join(tmpdir(), 'yanyi-work-'))
  mkdirSync(join(dir, 'prompts'))
  writeFileSync(join(dir, 'work.yaml'), `format: 1\nid: ${id}\nversion: 1.0.0\ntitle: 创作测试\n`)
  writeFileSync(join(dir, 'prompts/narrator.md'), '与玩家共同讲述一个故事。')
  writeFileSync(join(dir, 'prompts/opening.md'), '旅程开始了。')
  writeFileSync(join(dir, 'state.yaml'), 'fields:\n  old: {type: string, default: 旧值}\n')
  return dir
}

async function importDirectory(request: APIRequestContext, path: string) {
  const response = await request.post('/api/v1/works/import', { data: { source: 'dir', path } })
  expect(response.ok()).toBeTruthy()
  const preview = await response.json()
  expect(preview.task).toBeTruthy()
  const confirmed = await request.post(`/api/v1/works/import/${preview.task}/confirm`)
  expect(confirmed.ok()).toBeTruthy()
  return confirmed.json()
}

test('开发目录预览、重载、版本导出和删除', async ({ page, request }, info) => {
  const id = `test.dev-${info.project.name}`
  const dir = fixture(id)
  await request.patch('/api/v1/settings', { data: { onboarded: true, theme: 'light' } })
  await page.goto('/')
  await page.getByRole('button', { name: '导入作品', exact: true }).click()
  await page.getByRole('tab', { name: '开发目录', exact: true }).click()
  await page.getByLabel('本机目录', { exact: true }).fill(dir)
  await page.getByRole('button', { name: '解析', exact: true }).click()
  await expect(page.getByText('创作测试', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '导入', exact: true }).click()
  await page.getByRole('button', { name: '查看作品', exact: true }).click()
  await expect(page.getByRole('heading', { name: '创作测试', exact: true })).toBeVisible()
  writeFileSync(join(dir, 'work.yaml'), `format: 1\nid: ${id}\nversion: 1.0.1\ntitle: 热重载已生效\n`)
  await expect(page.getByRole('heading', { name: '热重载已生效' })).toBeVisible({ timeout: 10000 })
  await page.getByRole('tab', { name: '版本', exact: true }).click()
  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: '导出 zip' }).click()
  expect((await download).suggestedFilename()).toMatch(/\.zip$/)
  await expect(page.locator('body')).toHaveJSProperty('scrollWidth', await page.locator('body').evaluate((e) => e.clientWidth))
  await page.screenshot({ path: info.outputPath('versions.png'), fullPage: true })
  await page.getByRole('button', { name: '删除作品', exact: true }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click()
  await expect(page).toHaveURL('/')
  expect((await request.get(`/api/v1/works/${id}`)).status()).toBe(404)
})

test('存档升级与迁移记录', async ({ page, request }, info) => {
  const id = `test.upgrade-${info.project.name}`
  const dir = fixture(id)
  await request.patch('/api/v1/settings', { data: { onboarded: true, reading: { typewriter: 'instant' } } })
  const old = await importDirectory(request, dir)
  const created = await request.post('/api/v1/saves', { data: { work_id: id, version_id: old.version_id } })
  expect(created.ok()).toBeTruthy()
  const save = await created.json()
  writeFileSync(join(dir, 'work.yaml'), `format: 1\nid: ${id}\nversion: 2.0.0\ntitle: 创作测试\nchangelog: 新增旅程记录。\n`)
  writeFileSync(join(dir, 'state.yaml'), 'fields:\n  added: {type: number, default: 42}\n')
  const next = await importDirectory(request, dir)
  await page.goto(`/works/${id}`)
  await page.getByRole('button', { name: '更多操作' }).click()
  await page.getByRole('menuitem', { name: '升级作品版本' }).click()
  await expect(page.getByRole('dialog')).toContainText('产生费用')
  await expect(page.getByRole('dialog')).toContainText('新增旅程记录。')
  await page.getByRole('button', { name: '开始整理与升级' }).click()
  await expect(page).toHaveURL(`/play/${save.id}`)
  await expect
    .poll(async () => (await request.get(`/api/v1/saves/${save.id}`).then((r) => r.json())).save.work_version_id)
    .toBe(next.version_id)
  await page.goto(`/works/${id}`)
  await page.getByRole('button', { name: '更多操作' }).click()
  await page.getByRole('menuitem', { name: '状态迁移记录' }).click()
  await expect(page.getByRole('dialog')).toContainText('旧值')
  await page.screenshot({ path: info.outputPath('migration.png') })
  expect((await request.delete(`/api/v1/works/${id}/versions/${old.version_id}`)).status()).toBe(409)
  await request.delete(`/api/v1/saves/${save.id}`)
  expect((await request.delete(`/api/v1/works/${id}`)).ok()).toBeTruthy()
})

test('设置密码、登录与撤销其他设备', async ({ page, request, browser }, info) => {
  const password = 'test-access-password'
  await request.patch('/api/v1/settings', { data: { onboarded: true, theme: 'light' } })
  const other = await browser.newContext()
  try {
    await page.goto('/settings/access')
    await page.getByLabel('新密码（至少 8 位）', { exact: true }).fill(password)
    await page.getByLabel('再次输入新密码', { exact: true }).fill(password)
    await page
      .locator('form')
      .filter({ has: page.getByLabel('再次输入新密码') })
      .getByRole('button', { name: '保存', exact: true })
      .click()
    await expect(page.getByRole('heading', { name: '进入演义' })).toBeVisible()
    const device = await other.newPage()
    await device.goto(page.url())
    await expect(device.getByRole('heading', { name: '进入演义' })).toBeVisible()
    await page.screenshot({ path: info.outputPath('login.png'), fullPage: true })
    await page.getByLabel('访问密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await expect(page.getByRole('button', { name: '退出所有设备' })).toBeVisible()
    await device.getByLabel('访问密码', { exact: true }).fill(password)
    await device.getByRole('button', { name: '登录', exact: true }).click()
    await expect(device.getByRole('button', { name: '退出所有设备' })).toBeVisible()
    await page.getByRole('button', { name: '退出所有设备' }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click()
    await expect(page.getByRole('heading', { name: '进入演义' })).toBeVisible()
    await expect(device.getByRole('heading', { name: '进入演义' })).toBeVisible({ timeout: 20000 })
  } finally {
    await request.post('/api/v1/auth/login', { data: { password } })
    const cleared = await request.put('/api/v1/auth/password', { data: { current_password: password, password: '' } })
    expect(cleared.ok()).toBeTruthy()
    await other.close()
  }
})
