import type { DisplayItem } from '@/api/types'
import { parseMarkup, type Block, type Inline } from '@/markup/parse'

export interface StageCue {
  kind: string
  attrs: Record<string, string>
  key: string
}

/** 与正文共用解析器，转义或尚未完整到达的标记不会触发演出。 */
export function stageCues(turns: { id: string; items: DisplayItem[] }[], tags: string[]): StageCue[] {
  const out: StageCue[] = []
  for (const turn of turns) {
    let index = 0
    const walk = (nodes: (Block | Inline)[]) => {
      for (const n of nodes) {
        const kind = n.type === 'tag' ? n.name : n.type
        if (['scene', 'bgm', 'sfx'].includes(kind) && 'attrs' in n) out.push({ kind, attrs: n.attrs, key: `${turn.id}:${index++}` })
        if ('children' in n) walk(n.children)
      }
    }
    for (const item of turn.items) if (item.t === 'text') walk(parseMarkup(item.markup, { tags: new Set(tags), drafting: true }))
  }
  return out
}

export function lastSceneBackground(turns: DisplayItem[][]): string | undefined {
  return stageCues(
    turns.map((items, i) => ({ id: String(i), items })),
    ['scene'],
  ).findLast((c) => c.kind === 'scene' && c.attrs.bg)?.attrs.bg
}
