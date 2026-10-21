/**
 * 设置页的版式部件。每个分区的结构相同：
 *
 *   SectionTitle    分区标题 + 一句说明
 *   SettingGroup    一组设置：可选的小标题 + 圆角容器，行与行之间是发丝线
 *     SettingRow    左侧名称与说明，右侧控件（控件宽度固定，数值变化不推动版面）
 */
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

export function SectionTitle({ children, description }: { children: ReactNode; description?: ReactNode }) {
  return (
    <header className="mb-8">
      <h2 className="title-book text-xl leading-8">{children}</h2>
      {description && <p className="mt-1.5 max-w-[60ch] text-[13px] leading-6 text-ink-muted">{description}</p>}
    </header>
  )
}

export function SettingGroup({
  title,
  action,
  children,
  className,
}: {
  title?: ReactNode
  action?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('mb-8 last:mb-0', className)}>
      {(title || action) && (
        <div className="mb-2 flex h-7 items-center justify-between gap-4 px-1">
          <h3 className="text-[13px] font-medium text-ink-muted">{title}</h3>
          {action}
        </div>
      )}
      <div className="divide-y divide-rule rounded-xl border border-rule bg-paper-raised">{children}</div>
    </section>
  )
}

export function SettingRow({ label, hint, children }: { label: ReactNode; hint?: ReactNode; children?: ReactNode }) {
  return (
    <div className="flex min-h-14 flex-wrap items-center justify-between gap-x-6 gap-y-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="text-sm leading-5 text-ink">{label}</div>
        {hint && <div className="mt-0.5 text-xs leading-5 text-ink-faint">{hint}</div>}
      </div>
      {children && <div className="flex shrink-0 items-center gap-3">{children}</div>}
    </div>
  )
}
