import { createContext, useMemo } from 'react'

import type { AssetRef, Character, TagDef } from '@/api/types'

/** 渲染所需的作品信息（来自 GET /saves/{id} 的 ui 字段）。 */
export interface MarkupContextValue {
  /** 渲染白名单：已启用的内置标签 + 作品标签名 */
  tags: ReadonlySet<string>
  workTags: ReadonlyMap<string, TagDef>
  /** who 属性 → 人物（按 ID、名字、别名都能找到） */
  characters: ReadonlyMap<string, Character>
  /** 素材引用（ID 或路径）→ 素材（<cg src>、<scene bg>、面板图片） */
  assets: ReadonlyMap<string, AssetRef>
}

export const MarkupContext = createContext<MarkupContextValue>({
  tags: new Set(),
  workTags: new Map(),
  characters: new Map(),
  assets: new Map(),
})

export const MarkupProvider = MarkupContext.Provider

export function useMarkupContext(
  builtinTags: string[],
  workTags: TagDef[],
  characters: Character[],
  assets: AssetRef[],
): MarkupContextValue {
  return useMemo(() => {
    const chars = new Map<string, Character>()
    for (const c of characters) {
      for (const key of [c.id, c.name, ...(c.aliases ?? [])]) chars.set(key, c)
    }
    return {
      tags: new Set([...builtinTags, ...workTags.map((t) => t.name)]),
      workTags: new Map(workTags.map((t) => [t.name, t])),
      characters: chars,
      assets: new Map(assets.flatMap((a) => [[a.path, a] as const, [a.id, a] as const])),
    }
  }, [builtinTags, workTags, characters, assets])
}

/** 图片素材引用（ID、路径或 asset:ID）→ 素材地址；不是作品中的图片时返回 undefined。 */
export function imageURL(ref: string | undefined, ctx: MarkupContextValue): string | undefined {
  if (!ref) return undefined
  const a = ctx.assets.get(ref.startsWith('asset:') ? ref.slice('asset:'.length) : ref)
  return a?.kind === 'image' ? a.url : undefined
}
