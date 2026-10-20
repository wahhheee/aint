import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * 提示条（《04-交互与界面设计》8.9）：正文列宽、浅底、左侧状态色竖线，不使用模态弹窗。
 * 用于：已停止、本回合已达上限、错误、回到更早回合后的分支提示等。
 */
export function Notice({
  level = 'info',
  children,
  actions,
}: {
  level?: 'info' | 'warn' | 'error'
  children: ReactNode
  actions?: ReactNode
}) {
  return (
    <div
      role={level === 'error' ? 'alert' : 'status'}
      className={cn(
        'my-6 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border-l-2 bg-paper-sunken px-4 py-2.5 font-sans text-[13px] text-ink-muted',
        level === 'error' ? 'border-danger' : level === 'warn' ? 'border-warn' : 'border-info',
      )}
    >
      <span className="min-w-0 flex-1">{children}</span>
      {actions && <span className="flex shrink-0 gap-2">{actions}</span>}
    </div>
  )
}

/** 低调的系统活动提示："· 翻阅了 2 份资料"、"· 正在翻阅资料……" */
export function QuietLine({ children }: { children: ReactNode }) {
  return <p className="my-3 font-sans text-xs text-ink-faint">· {children}</p>
}

/** 等待首个字：三点呼吸。 */
export function Waiting({ label }: { label: string }) {
  return (
    <p className="my-4 flex items-center gap-2 font-sans text-xs text-ink-faint" role="status" aria-live="polite">
      <span className="flex gap-1" aria-hidden>
        {[0, 1, 2].map((i) => (
          <span key={i} className="size-1 animate-pulse rounded-full bg-gold" style={{ animationDelay: `${i * 180}ms` }} />
        ))}
      </span>
      {label}
    </p>
  )
}
