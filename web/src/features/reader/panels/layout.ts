import type { PanelMeta, PanelRender } from '@/api/types'

/** 取某个位置上、且有渲染结果的面板（渲染结果为空的面板不显示），按 order 排序。 */
export function byPlace(meta: PanelMeta[], rendered: PanelRender[] | undefined, place: PanelMeta['place']) {
  const html = new Map((rendered ?? []).map((p) => [p.id, p]))
  return meta
    .filter((m) => m.place === place && html.has(m.id))
    .sort((a, b) => a.order - b.order)
    .map((m) => ({ meta: m, render: html.get(m.id)! }))
}

export function hasSidebarPanels(meta: PanelMeta[], rendered?: PanelRender[]) {
  return byPlace(meta, rendered, 'sidebar').length > 0
}
