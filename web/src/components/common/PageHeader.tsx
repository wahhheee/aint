/**
 * 页面骨架：外壳里的每个页面都以 PageHeader 开头，标题行固定 40px 高、与侧栏的品牌同一水平线，
 * 换页时标题永远在同一个位置。区块标题用 SectionHeading。
 */
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

export function PageHeader({
  title,
  description,
  actions,
  className,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
}) {
  return (
    <header className={cn('mb-10', className)}>
      <div className="flex min-h-10 items-center justify-between gap-6">
        <h1 className="title-book truncate text-[28px] leading-10">{title}</h1>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      {description && <p className="mt-2 max-w-2xl text-ink-muted">{description}</p>}
    </header>
  )
}

export function SectionHeading({
  children,
  count,
  actions,
  id,
}: {
  children: ReactNode
  count?: number
  actions?: ReactNode
  id?: string
}) {
  return (
    <div className="mb-5 flex h-8 items-center justify-between gap-4">
      <h2 id={id} className="flex items-baseline gap-2 text-[15px] font-semibold text-ink">
        {children}
        {count !== undefined && <span className="text-[13px] font-normal text-ink-faint tabular">{count}</span>}
      </h2>
      {actions}
    </div>
  )
}
