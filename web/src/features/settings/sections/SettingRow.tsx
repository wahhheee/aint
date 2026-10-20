import type { ReactNode } from 'react'

/** 设置项：左侧名称与说明，右侧控件。 */
export function SettingRow({ label, hint, children }: { label: ReactNode; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-8 gap-y-3 border-b border-rule py-4 last:border-b-0">
      <div className="min-w-0">
        <div className="text-[14px] text-ink">{label}</div>
        {hint && <div className="mt-0.5 max-w-md text-xs text-ink-faint">{hint}</div>}
      </div>
      <div className="flex shrink-0 items-center gap-3">{children}</div>
    </div>
  )
}

export function SectionTitle({ children, description }: { children: ReactNode; description?: ReactNode }) {
  return (
    <header className="mb-6">
      <h2 className="title-book text-xl">{children}</h2>
      {description && <p className="mt-2 max-w-prose text-ink-muted">{description}</p>}
    </header>
  )
}
