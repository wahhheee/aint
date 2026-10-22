/**
 * 导入预览的展示逻辑（纯函数）：问题排序与计数、与书架上已有作品的关系、可复制的文字报告。
 */
import type { ImportPreview, Issue } from '@/api/types'
import i18n from '@/i18n'

const LEVEL_ORDER: Record<Issue['level'], number> = { error: 0, warning: 1, info: 2 }

/** 按级别排序（error 在前），同级保持原顺序。 */
export function sortIssues(issues: Issue[]): Issue[] {
  return issues.toSorted((a, b) => LEVEL_ORDER[a.level] - LEVEL_ORDER[b.level])
}

export function countIssues(issues: Issue[]): Record<Issue['level'], number> {
  const n = { error: 0, warning: 0, info: 0 }
  for (const i of issues) n[i.level]++
  return n
}

/**
 * 与书架的关系：
 *   new          书架上没有这部作品
 *   newVersion   已有同 ID 作品，内容不同：作为新版本导入，现有存档不受影响
 *   sameVersion  已有内容完全相同的版本：确认后直接复用，不重复导入
 */
export type ShelfRelation = 'new' | 'newVersion' | 'sameVersion'

export function shelfRelation(p: ImportPreview): ShelfRelation {
  if (!p.existing) return 'new'
  return p.existing.same_version_id ? 'sameVersion' : 'newVersion'
}

/** 上传进度百分比（0–100 的整数）。总量未知时为 0。 */
export function progressPercent(loaded: number, total: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.floor((loaded / total) * 100))
}

/** 一条问题的位置："lore/lin.md:12"；没有文件时为空。 */
export function issueLocation(i: Issue): string {
  if (!i.path) return ''
  return i.line ? `${i.path}:${i.line}` : i.path
}

/** 可复制的文字报告：作者据此修正，玩家可以把它转给作者。 */
export function reportText(p: ImportPreview): string {
  const t = i18n.t.bind(i18n)
  const n = countIssues(p.report.issues)
  const head = [
    `${p.work.title || p.source.name || ''} ${p.work.version}${p.work.id ? `（${p.work.id}）` : ''}`.trim(),
    [t('import.counts.error', { n: n.error }), t('import.counts.warning', { n: n.warning }), t('import.counts.info', { n: n.info })].join(
      t('import.countSeparator'),
    ),
  ]
  const lines = sortIssues(p.report.issues).map((i) => {
    const parts = [`[${t(`work.levels.${i.level}`)}]`, i.code, issueLocation(i), i.message].filter(Boolean)
    return parts.join(' ') + (i.hint ? `（${i.hint}）` : '')
  })
  return [...head, '', ...lines].join('\n').trimEnd() + '\n'
}
