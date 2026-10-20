/**
 * 品牌标识：一册微缩的线装书（磁青封面 + 题签），与书架上的作品封面同一语汇。
 */
import { Link } from 'react-router'

import { cn } from '@/lib/utils'

export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      aria-hidden
      className={cn('relative inline-block h-6 w-[18px] rounded-[1px_2px_2px_1px] bg-[#23345a] dark:bg-[#33487a]', className)}
    >
      <span className="absolute top-[3px] left-[3px] h-[14px] w-[5px] bg-[#ece6d6]" />
      <span className="absolute inset-y-0 right-[3px] w-px bg-black/30" />
    </span>
  )
}

/** 顶栏左侧的品牌：标识 + 字标，点击回到书架。 */
export function Brand() {
  return (
    <Link to="/" className="group inline-flex items-center gap-2.5 rounded-md outline-offset-4" aria-label="演义 · 书架">
      <BrandMark />
      <span className="font-serif text-[17px] font-semibold tracking-[0.32em] text-ink">演义</span>
    </Link>
  )
}
