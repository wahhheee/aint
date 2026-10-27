/**
 * 面板布局（《03-作品格式规范》11.1）：sidebar 右侧栏、topbar 正文上方细条、float 角落小卡、drawer 抽屉（PanelSheet.tsx）。
 * 面板的静态布局来自 GET /saves/{id}（ui.panels），内容来自 /panels 与 SSE panels 事件，按 id 对应。
 */
import type { PanelMeta, PanelRender } from '@/api/types'
import { cn } from '@/lib/utils'

import { byPlace } from './layout'
import { PanelHTML } from './PanelHTML'

/** 右侧栏：面板之间用标题与留白分隔，而不是一张张卡片。 */
export function SidebarPanels({ meta, rendered, className }: { meta: PanelMeta[]; rendered?: PanelRender[]; className?: string }) {
  const panels = byPlace(meta, rendered, 'sidebar')
  return (
    <div className={cn('space-y-8', className)}>
      {panels.map(({ meta: m, render }) => (
        <section key={m.id} aria-label={m.title ?? m.id}>
          {m.title && (
            <h3 className="mb-3 flex h-5 items-center gap-3 font-serif text-[13px] font-semibold tracking-[0.14em] text-ink-muted after:h-px after:flex-1 after:bg-rule">
              {m.title}
            </h3>
          )}
          <PanelHTML html={render.html} />
        </section>
      ))}
    </div>
  )
}

/** 顶栏面板：正文上方一句话的局势 */
export function TopbarPanels({ meta, rendered }: { meta: PanelMeta[]; rendered?: PanelRender[] }) {
  const panels = byPlace(meta, rendered, 'topbar')
  if (panels.length === 0) return null
  return (
    <div className="reader-column relative z-10 flex shrink-0 justify-center pb-1">
      <div className="max-w-full truncate rounded-full border border-rule bg-paper-raised/70 px-4 py-1 text-center font-serif text-[13px] leading-5 text-ink-muted backdrop-blur-md [&_p]:m-0">
        {panels.map(({ meta: m, render }) => (
          <PanelHTML key={m.id} html={render.html} />
        ))}
      </div>
    </div>
  )
}

/** 悬浮面板：角落的小卡片（浮层可以有阴影） */
export function FloatPanels({ meta, rendered }: { meta: PanelMeta[]; rendered?: PanelRender[] }) {
  const panels = byPlace(meta, rendered, 'float')
  if (panels.length === 0) return null
  return (
    <div className="pointer-events-none absolute right-5 bottom-28 z-10 hidden flex-col items-end gap-2 md:flex">
      {panels.map(({ meta: m, render }) => (
        <div
          key={m.id}
          className="pointer-events-auto rounded-xl border border-rule bg-paper-overlay/85 px-3.5 py-2 font-serif text-[13px] text-ink shadow-overlay backdrop-blur-md [&_p]:m-0"
        >
          <PanelHTML html={render.html} />
        </div>
      ))}
    </div>
  )
}
