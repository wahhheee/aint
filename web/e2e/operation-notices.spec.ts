import { expect, test, type Locator } from '@playwright/test'
import type { EventMap } from '../src/api/events'

type Snapshot = EventMap['session.snapshot']

test('中断与整理提示对齐输入框，并提供适用于当前进度的操作', async ({ page, request }, info) => {
  test.setTimeout(90000)
  const desktop = info.project.name === 'desktop'
  if (desktop) await page.setViewportSize({ width: 1398, height: 820 })
  await request.patch('/api/v1/settings', {
    data: {
      onboarded: true,
      theme: info.project.name === 'mobile' ? 'light' : 'dark',
      reading: { typewriter: 'instant' },
      play: { auto_debug: true },
    },
  })
  const created = await request.post('/api/v1/saves', { data: { work_id: 'aint.inverted-stars', opening_id: 'alley-after-rain' } })
  expect(created.ok()).toBe(true)
  const { id } = await created.json()
  const base = `/api/v1/saves/${id}`
  // 在浏览器接收事件的边界重放终止快照；时间线、调试记录与整理重试使用测试服务。
  await page.addInitScript(() => {
    class ReaderEvents extends EventSource {
      constructor(url: string, init?: EventSourceInit) {
        super(url, init)
        if (url.includes('/saves/') && url.endsWith('/events')) {
          window.addEventListener('test:snapshot', (ev) => {
            if (this.readyState === EventSource.OPEN)
              this.dispatchEvent(new MessageEvent('session.snapshot', { data: JSON.stringify((ev as CustomEvent).detail) }))
          })
        }
      }
    }
    window.EventSource = ReaderEvents
  })
  await page.goto(`/play/${id}`)
  await expect(page.locator('article').first()).toBeVisible()
  if (desktop) {
    const index = page.getByRole('button', { name: '回合目录', exact: true })
    if ((await index.getAttribute('aria-expanded')) === 'true') await index.click()
  }
  const dock = page.locator('.reader-column').filter({ has: page.locator('textarea') })
  const scroll = page.locator('.reader-scroll')
  const expectNoticeLayout = async (notice: Locator) => {
    await expect(notice).toBeInViewport({ ratio: 0.99 })
    expect(await notice.evaluate((el) => el.closest('.reader-scroll') === null)).toBe(true)
    const n = (await notice.boundingBox())!
    const d = (await dock.boundingBox())!
    expect(Math.abs(n.x - d.x)).toBeLessThan(1)
    expect(Math.abs(n.width - d.width)).toBeLessThan(1)
    expect(n.y + n.height).toBeLessThanOrEqual(d.y)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  const replay = (data: Partial<Snapshot>) =>
    page.evaluate((s) => window.dispatchEvent(new CustomEvent('test:snapshot', { detail: s })), {
      turn: null,
      compacting: false,
      ...data,
    })
  const positionStory = () => scroll.evaluate((el) => el.scrollTo(0, el.scrollHeight * 0.28))
  const input = page.getByRole('textbox', { name: '写下你的行动……' })
  await input.fill('/ooc 查看这封信')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.getByText('重连测试正文', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: '停止', exact: true }).click()
  const stopped = page.getByRole('status', { name: '已停止生成', exact: true })
  await expect(stopped).toContainText('已保留')
  await positionStory()
  await expectNoticeLayout(stopped)
  await stopped.getByRole('button', { name: '检查器' }).click()
  await expect(page.getByRole('dialog', { name: '游玩调试' })).toBeVisible()
  await expect(page.getByRole('dialog').getByRole('button', { name: '查看所有回合' })).toBeVisible()
  await expect(page.getByRole('dialog')).toContainText('重连测试正文')
  await page.keyboard.press('Escape')
  await stopped.getByRole('button', { name: '关闭' }).click()
  await expect(stopped).toHaveCount(0)

  const { head_id: head } = await request.get(`${base}/timeline`).then((r) => r.json())
  const partial: EventMap['turn.failed'] = {
    turn_id: 'filter',
    node_id: head,
    code: 'refusal',
    message: '模型拒答或触发了内容过滤。',
    recoverable: true,
  }
  const compact: EventMap['compaction.failed'] = { attempt_id: 'compact', retry_head: head, reason: 'manual', message: partial.message }
  const rewrite = { kind: 'rewrite', node_id: head, expected_head: head } as const
  const cases: [Partial<Snapshot>, string, string[]][] = [
    [{ failure: partial }, '生成中断 · 正文已保留', ['重新生成']],
    [
      { failure: { ...partial, code: 'budget', message: '本回合已达时长上限，可以继续或重试。' } },
      '生成中断 · 正文已保留',
      ['继续', '重新生成'],
    ],
    [
      { failure: { ...partial, node_id: undefined, code: 'protocol', message: '模型响应没有正常结束，请在检查器中查看停止原因。' } },
      '本回合未保存',
      ['重试'],
    ],
    [
      {
        failure: { ...partial, node_id: undefined, code: 'invalid_argument', recoverable: false, message: '该选项不在当前回合的选项中。' },
      },
      '本回合未保存',
      [],
    ],
    [{ failure: { ...partial, node_id: undefined, code: 'canceled', message: '已停止。' } }, '已停止生成', ['重试']],
    [{ compact_failure: compact }, '整理未完成', ['重试整理']],
    [{ compact_failure: { ...compact, reason: 'threshold' } }, '自动整理未完成 · 上一回合已保存', ['重试整理']],
    [{ compact_failure: { ...compact, retry_head: 'older' } }, '整理未完成', []],
    [
      {
        failure: {
          ...partial,
          node_id: undefined,
          operation: { ...rewrite, kind: 'choices' },
          message: '选项没有生成，当前正文和状态保持不变。模型拒答或触发了内容过滤。',
        },
      },
      '未能补充选项',
      ['重试'],
    ],
    [
      {
        failure: { ...partial, node_id: undefined, operation: rewrite, message: '重写没有完成，已恢复原版。模型拒答或触发了内容过滤。' },
        compact_failure: { attempt_id: 'staged', message: '重写未采用，暂存的整理结果已丢弃。' },
      },
      '重写未完成',
      ['重试'],
    ],
    [{ failure: partial, compacting: true }, '生成中断 · 正文已保留', ['重新生成']],
    [
      {
        failure: {
          ...partial,
          node_id: undefined,
          code: 'canceled',
          operation: rewrite,
          recoverable: false,
          message: '已取消，当前正文和状态保持不变。',
        },
      },
      '已停止生成',
      [],
    ],
  ]
  for (const [state, title, commands] of cases) {
    await replay(state)
    const notice = page.getByRole(state.failure?.code === 'canceled' ? 'status' : 'alert', { name: title, exact: true })
    await expect(notice).toBeVisible()
    if (state.failure && state.failure.code !== 'canceled') await expect(notice).toContainText(state.failure.message)
    if (state.compact_failure) {
      const compactNotice = page.getByRole('alert', {
        name: state.compact_failure.reason === 'threshold' ? '自动整理未完成 · 上一回合已保存' : '整理未完成',
        exact: true,
      })
      await expect(compactNotice).toContainText(state.compact_failure.message)
      await expect(compactNotice).toContainText('记忆与状态未变')
    }
    await expect(notice.getByRole('button')).toHaveCount(commands.length + 2)
    for (const name of commands) {
      const button = notice.getByRole('button', { name, exact: true })
      if (state.compacting) await expect(button).toBeDisabled()
      else await expect(button).toBeEnabled()
    }
    await positionStory()
    await expectNoticeLayout(notice)
  }
  await replay({})
  // 整理至少保留最近两个回合，先生成足够的历史供摘要使用。
  for (const action of ['展开信纸', '走到窗边细读']) {
    const before = await request.get(`${base}/timeline`).then((r) => r.json())
    await input.fill(action)
    await page.getByRole('button', { name: '发送', exact: true }).click()
    await expect
      .poll(() =>
        request
          .get(`${base}/timeline`)
          .then((r) => r.json())
          .then((s) => s.head_id),
      )
      .not.toBe(before.head_id)
  }
  expect((await request.patch(base, { data: { settings: { keep_recent: 1 } } })).ok()).toBe(true)
  const timeline = () => request.get(`${base}/timeline`).then((r) => r.json())
  await replay({ compact_failure: { ...compact, retry_head: (await timeline()).head_id } })
  await page.getByRole('button', { name: '重试整理', exact: true }).click()
  const redo = page.getByRole('button', { name: '重新整理', exact: true })
  await expect(redo).toBeVisible()
  const before = await request.get(`${base}/timeline`).then((r) => r.json())
  await redo.click()
  await expect
    .poll(() =>
      request
        .get(`${base}/timeline`)
        .then((r) => r.json())
        .then((s) => s.head_id),
    )
    .not.toBe(before.head_id)
  await input.fill('收起信纸')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('article').last()).toContainText('停下观察')
  await expect(redo).toHaveCount(0)
  await expect(page.getByText('上下文已整理', { exact: false }).last()).toBeVisible()

  // 没有开启采集、也没有历史调用时，提示不提供检查器入口。
  await request.patch('/api/v1/settings', { data: { play: { auto_debug: false } } })
  try {
    const fresh = await request.post('/api/v1/saves', { data: { work_id: 'aint.inverted-stars' } }).then((r) => r.json())
    await page.goto(`/play/${fresh.id}`)
    await expect(page.locator('article').first()).toBeVisible()
    const base = `/api/v1/saves/${fresh.id}`
    expect((await request.post(`${base}/turns`, { data: { kind: 'choice', choice_id: 'missing' } })).ok()).toBe(true)
    const notice = page.getByRole('alert', { name: '本回合未保存' })
    await expect(notice.getByRole('button')).toHaveCount(1)
    await positionStory()
    await expectNoticeLayout(notice)
  } finally {
    await request.patch('/api/v1/settings', { data: { play: { auto_debug: true } } })
  }
})
