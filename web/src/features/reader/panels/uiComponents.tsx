/**
 * 面板内置组件（《03-作品格式规范》11.2）：ui-bar、ui-stat、ui-badge、ui-image、ui-button、ui-collapse、ui-tooltip。
 * 属性都来自作品模板（不可信），只当作文本或数字使用。
 */
import { ChevronRight } from 'lucide-react'
import { useState, type ReactNode } from 'react'

import { WorkIcon } from '@/components/common/WorkIcon'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { useReaderContext } from '../ReaderContext'

type Attrs = Record<string, string | undefined>
type UIComponent = (props: { attrs: Attrs; children?: ReactNode }) => ReactNode

/** 数值：图标 + 标签 + 右对齐的值 */
const UiStat: UIComponent = ({ attrs }) => (
  <div className="flex items-center gap-2 py-1 text-[13px]">
    {attrs.icon && <WorkIcon name={attrs.icon} className="size-3.5 text-ink-faint" />}
    <span className="text-ink-muted">{attrs.label}</span>
    <span className="ml-auto font-medium text-ink tabular">{attrs.value}</span>
  </div>
)

/** 进度条：数值变化时宽度过渡 300ms */
const UiBar: UIComponent = ({ attrs }) => {
  const value = Number(attrs.value) || 0
  const max = Number(attrs.max) || 100
  const pct = Math.max(0, Math.min(100, (value / max) * 100))
  return (
    <div
      className="flex items-center gap-3 py-1 text-[13px]"
      role="meter"
      aria-label={attrs.label}
      aria-valuenow={value}
      aria-valuemax={max}
    >
      <span className="w-14 shrink-0 truncate text-ink-muted">{attrs.label}</span>
      <span className="h-1 flex-1 overflow-hidden rounded-full bg-rule-strong">
        <span className="block h-full rounded-full bg-ink-muted transition-[width] duration-300 ease-out" style={{ width: `${pct}%` }} />
      </span>
    </div>
  )
}

const UiBadge: UIComponent = ({ attrs, children }) => (
  <span className="inline-flex items-center gap-1 rounded-md border border-rule px-1.5 py-0.5 text-xs text-ink-muted">
    {attrs.icon && <WorkIcon name={attrs.icon} className="size-3" />}
    {children}
  </span>
)

/** 按钮：执行作品命令（如 command="/follow 林舟"） */
const UiButton: UIComponent = ({ attrs, children }) => {
  const { runCommand } = useReaderContext()
  const command = attrs.command ? (attrs.command.startsWith('/') ? attrs.command : `/${attrs.command}`) : ''
  return (
    <Button
      variant="outline"
      size="xs"
      className="my-1"
      disabled={!command}
      onClick={() => {
        if (attrs.confirm && !window.confirm(attrs.confirm)) return // TODO(M2-8)：应用内确认框
        runCommand([command, attrs.args].filter(Boolean).join(' '))
      }}
    >
      {children}
    </Button>
  )
}

const UiCollapse: UIComponent = ({ attrs, children }) => {
  const [open, setOpen] = useState(attrs.open !== undefined && attrs.open !== 'false')
  return (
    <div className="py-1">
      <button type="button" onClick={() => setOpen(!open)} className="flex items-center gap-1 text-[13px] text-ink-muted hover:text-ink">
        <ChevronRight className={cn('size-3.5 transition-transform', open && 'rotate-90')} />
        {attrs.title}
      </button>
      {open && <div className="mt-1 pl-4">{children}</div>}
    </div>
  )
}

const UiTooltip: UIComponent = ({ attrs, children }) => (
  <Tooltip>
    <TooltipTrigger asChild>
      <span className="underline decoration-dotted underline-offset-4">{children}</span>
    </TooltipTrigger>
    <TooltipContent>{attrs.text}</TooltipContent>
  </Tooltip>
)

// TODO(M2-10)：ui-image（src 为素材引用 → /api/v1/assets/{hash}）
const UiImage: UIComponent = ({ attrs }) => <span className="text-xs text-ink-faint">［图片：{attrs.alt ?? attrs.src}］</span>

export const UI_COMPONENTS: Record<string, UIComponent> = {
  'ui-stat': UiStat,
  'ui-bar': UiBar,
  'ui-badge': UiBadge,
  'ui-button': UiButton,
  'ui-collapse': UiCollapse,
  'ui-tooltip': UiTooltip,
  'ui-image': UiImage,
}
