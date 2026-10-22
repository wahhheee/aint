import '@/i18n'

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ImportPreview } from '@/api/types'

import { ImportDialog } from './ImportDialog'
import type { ImportStage, WorkImport } from './useWorkImport'

afterEach(cleanup)

const basePreview: ImportPreview = {
  task: 'task-1',
  source: { type: 'zip', name: 'misty.zip' },
  work: {
    id: 'someone.misty',
    title: '雾港来信',
    version: '0.3.0',
    tags: ['悬疑'],
    authors: [{ name: '某人' }],
    content_rating: 'teen',
    requires: { vision: 'none' },
  },
  tree_hash: 'sha256:abc',
  files: 12,
  size: 2048,
  report: { issues: [] },
}

function fakeImport(stage: ImportStage, over: Partial<WorkImport> = {}): WorkImport {
  return {
    stage,
    busy: false,
    confirming: false,
    confirmError: null,
    start: vi.fn(),
    startFrom: vi.fn(),
    confirm: vi.fn(),
    reset: vi.fn(),
    ...over,
  }
}

function show(imp: WorkImport) {
  render(
    <MemoryRouter>
      <ImportDialog open onOpenChange={() => {}} imp={imp} />
    </MemoryRouter>,
  )
}

describe('ImportDialog', () => {
  it('选择：五个标签页，文件与文件夹可选择，其余说明尚未实现', () => {
    show(fakeImport({ step: 'select' }))
    for (const name of ['文件', '文件夹', 'GitHub', '链接', '开发目录']) expect(screen.getByRole('tab', { name })).toBeTruthy()
    expect(screen.getByRole('button', { name: '选择 zip 包' })).toBeTruthy()
  })

  it('选择 zip 文件后开始导入', () => {
    const imp = fakeImport({ step: 'select' })
    show(imp)
    const input = document.querySelector<HTMLInputElement>('input[type=file]')!
    const zip = new File(['PK'], 'misty.zip', { type: 'application/zip' })
    fireEvent.change(input, { target: { files: [zip] } })
    expect(imp.start).toHaveBeenCalledWith({ kind: 'zip', file: zip })
  })

  it('上传进度', () => {
    show(fakeImport({ step: 'uploading', name: 'misty.zip', loaded: 512, total: 2048 }))
    expect(screen.getAllByText('正在上传 misty.zip').length).toBeGreaterThan(0)
    expect(screen.getByText('512 B / 2 KB')).toBeTruthy()
  })

  it('检查结果：摘要、已有版本、可以导入', () => {
    const imp = fakeImport({
      step: 'preview',
      preview: { ...basePreview, existing: { title: '雾港来信', latest_version: '0.2.1' } },
    })
    show(imp)
    expect(screen.getAllByText('雾港来信').length).toBeGreaterThan(0)
    expect(screen.getByText(/someone\.misty · 作者 某人 · 青少年/)).toBeTruthy()
    expect(screen.getByText(/当前最新 v0\.2\.1/)).toBeTruthy()
    expect(screen.getByText('没有发现问题')).toBeTruthy()
    const confirm = screen.getByRole('button', { name: '导入' })
    fireEvent.click(confirm)
    expect(imp.confirm).toHaveBeenCalled()
  })

  it('有错误时列出问题并禁止导入', () => {
    show(
      fakeImport({
        step: 'preview',
        preview: {
          ...basePreview,
          task: undefined,
          report: {
            issues: [
              { level: 'warning', code: 'W-NO-DESC', path: 'state.yaml', message: '状态字段 day 缺少 desc' },
              { level: 'error', code: 'E-REF', path: 'lore/lin.md', line: 12, message: '引用了不存在的文档：chen' },
            ],
          },
        },
      }),
    )
    expect(screen.getByText('1 处错误')).toBeTruthy()
    expect(screen.getByText('lore/lin.md:12')).toBeTruthy()
    expect(screen.getByText(/作品有 1 处错误，无法导入/)).toBeTruthy()
    expect((screen.getByRole('button', { name: '导入' }) as HTMLButtonElement).disabled).toBe(true)
    // 错误排在警告前面
    const items = screen.getAllByRole('listitem').map((li) => li.textContent ?? '')
    expect(items[0]).toContain('E-REF')
  })

  it('完成：可以查看作品', () => {
    show(
      fakeImport({
        step: 'done',
        preview: basePreview,
        result: { work_id: 'someone.misty', version_id: 'v1', created: true, report: { issues: [] } },
      }),
    )
    expect(screen.getByText('《雾港来信》v0.3.0 已放上书架')).toBeTruthy()
    expect(screen.getByRole('button', { name: '查看作品' })).toBeTruthy()
  })

  it('失败：显示原因与请求编号', () => {
    const imp = fakeImport({ step: 'failed', message: '路径中不允许 ..：../evil.txt', requestId: 'abc123' })
    show(imp)
    expect(screen.getByRole('alert').textContent).toContain('路径中不允许 ..')
    expect(screen.getByText('abc123')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '重新选择' }))
    expect(imp.reset).toHaveBeenCalled()
  })
})
