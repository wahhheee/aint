/**
 * 品牌标识：月洞门（纯黑的墙上一扇圆门，门里是黄昏、远山与一轮小月）。
 * 图形只有一份，就是 public/favicon.svg：浏览器标签页与界面里用的是同一个文件。
 */
import { Link } from 'react-router'

import { cn } from '@/lib/utils'

export function BrandMark({ className }: { className?: string }) {
  return <img src="/favicon.svg" alt="" aria-hidden draggable={false} className={cn('size-7 shrink-0 select-none', className)} />
}

/** 侧栏与窄屏顶栏的品牌：标识 + 字标，点击回到书架。 */
export function Brand() {
  return (
    <Link to="/" className="group inline-flex items-center gap-3 rounded-md outline-offset-4" aria-label="AINT · 书架">
      <BrandMark />
      <span className="font-serif text-[17px] font-semibold tracking-[0.28em] text-ink">AINT</span>
    </Link>
  )
}
