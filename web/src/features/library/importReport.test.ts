import { describe, expect, it } from 'vitest'

import type { ImportPreview, Issue } from '@/api/types'
import { formatBytes } from '@/lib/format'

import { countIssues, issueLocation, progressPercent, reportText, shelfRelation, sortIssues } from './importReport'

const issues: Issue[] = [
  { level: 'warning', code: 'W-NO-DESC', path: 'state.yaml', message: '状态字段 day 缺少 desc', hint: '写清含义与范围' },
  { level: 'error', code: 'E-REF', path: 'lore/lin.md', line: 12, message: '引用了不存在的文档：chen' },
  { level: 'info', code: 'I-TOKENS', message: '常驻上下文约 9.8k token' },
  { level: 'error', code: 'E-LFS', path: 'assets/cover.png', message: '这是 Git LFS 指针文件，不是文件的实际内容' },
]

function preview(over: Partial<ImportPreview> = {}): ImportPreview {
  return {
    task: 'task-1',
    source: { type: 'zip', name: 'misty.zip' },
    work: { id: 'someone.misty', title: '雾港来信', version: '0.3.0', tags: [], authors: [], requires: { vision: 'none' } },
    tree_hash: 'sha256:abc',
    files: 12,
    size: 2048,
    report: { issues: [] },
    ...over,
  }
}

describe('校验报告', () => {
  it('按级别排序，同级保持原顺序', () => {
    expect(sortIssues(issues).map((i) => i.code)).toEqual(['E-REF', 'E-LFS', 'W-NO-DESC', 'I-TOKENS'])
    expect(issues[0].code).toBe('W-NO-DESC') // 不修改原数组
  })

  it('计数与位置', () => {
    expect(countIssues(issues)).toEqual({ error: 2, warning: 1, info: 1 })
    expect(issueLocation(issues[1])).toBe('lore/lin.md:12')
    expect(issueLocation(issues[0])).toBe('state.yaml')
    expect(issueLocation(issues[2])).toBe('')
  })

  it('可复制的文字报告', () => {
    expect(reportText(preview({ task: undefined, report: { issues } }))).toBe(
      [
        '雾港来信 0.3.0（someone.misty）',
        '2 处错误，1 条警告，1 条提示',
        '',
        '[错误] E-REF lore/lin.md:12 引用了不存在的文档：chen',
        '[错误] E-LFS assets/cover.png 这是 Git LFS 指针文件，不是文件的实际内容',
        '[警告] W-NO-DESC state.yaml 状态字段 day 缺少 desc（写清含义与范围）',
        '[提示] I-TOKENS 常驻上下文约 9.8k token',
        '',
      ].join('\n'),
    )
  })
})

describe('shelfRelation', () => {
  it('书架上没有、有同 ID 作品、有相同版本', () => {
    expect(shelfRelation(preview())).toBe('new')
    expect(shelfRelation(preview({ existing: { title: '雾港来信', latest_version: '0.2.1' } }))).toBe('newVersion')
    expect(
      shelfRelation(preview({ existing: { title: '雾港来信', latest_version: '0.3.0', same_version_id: 'v1', same_version: '0.3.0' } })),
    ).toBe('sameVersion')
  })
})

describe('progressPercent / formatBytes', () => {
  it('进度百分比', () => {
    expect(progressPercent(0, 0)).toBe(0)
    expect(progressPercent(1, 3)).toBe(33)
    expect(progressPercent(5, 4)).toBe(100)
  })

  it('文件大小', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(812)).toBe('812 B')
    expect(formatBytes(2048)).toBe('2 KB')
    expect(formatBytes(12.34 * 1024)).toBe('12.3 KB')
    expect(formatBytes(512 * 1024 * 1024)).toBe('512 MB')
    expect(formatBytes(1.5 * 1024 ** 3)).toBe('1.5 GB')
  })
})
