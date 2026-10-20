import type { DisplayItem } from '@/api/types'

const SCENE_BG = /<scene\b[^>]*?\bbg="([^"]+)"/g

/** 找出一组回合中最后一次切换到的背景素材 ID。 */
export function lastSceneBackground(turns: DisplayItem[][]): string | undefined {
  let bg: string | undefined
  for (const items of turns) {
    for (const it of items) {
      if (it.t !== 'text') continue
      for (const m of it.markup.matchAll(SCENE_BG)) bg = m[1]
    }
  }
  return bg
}
