/**
 * 斜杠命令面板：列出平台内置命令与作品命令（名称、参数提示、说明）。
 * 焦点始终留在输入框，由输入框处理上下键与 Tab/Enter（见 InputDock）。
 */
import { WorkIcon } from '@/components/common/WorkIcon'
import { cn } from '@/lib/utils'

import type { PaletteEntry } from './paletteEntries'

export function CommandPalette({
  entries,
  active,
  onPick,
  onHover,
}: {
  entries: PaletteEntry[]
  active: number
  onPick: (e: PaletteEntry) => void
  onHover: (i: number) => void
}) {
  return (
    <div
      role="listbox"
      aria-label="命令"
      className="absolute inset-x-4 bottom-full z-20 mb-2 overflow-hidden rounded-xl bg-popover p-1 text-sm shadow-overlay sm:inset-x-6"
    >
      {entries.map((e, i) => (
        <button
          key={e.name}
          type="button"
          role="option"
          aria-selected={i === active}
          onMouseEnter={() => onHover(i)}
          onMouseDown={(ev) => {
            ev.preventDefault() // 保持输入框焦点
            onPick(e)
          }}
          className={cn('flex w-full items-baseline gap-3 rounded-lg px-3 py-2 text-left', i === active && 'bg-selected')}
        >
          <span className="w-4 shrink-0 self-center text-ink-faint">{e.icon ? <WorkIcon name={e.icon} className="size-3.5" /> : null}</span>
          <span className="shrink-0 font-mono text-[13px] text-ink">{e.signature}</span>
          <span className="min-w-0 truncate text-xs text-ink-muted">{e.description}</span>
          {e.builtin && <span className="ml-auto shrink-0 text-[11px] text-ink-faint">内置</span>}
        </button>
      ))}
    </div>
  )
}
