/**
 * 面板内置组件（《03-作品格式规范》11.2）：ui-bar、ui-stat、ui-badge、ui-image、ui-button、ui-collapse、ui-tooltip。
 * 属性都来自作品模板（不可信），只当作文本或数字使用。
 */
import { ChevronRight } from 'lucide-react'
import { useContext, useState, type ReactNode } from 'react'

import { confirmAction } from '@/components/common/confirm'
import { WorkIcon } from '@/components/common/WorkIcon'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { imageURL, MarkupContext } from '@/markup/context'

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
      onClick={async () => {
        if (attrs.confirm && !(await confirmAction(attrs.confirm))) return
        runCommand([command, attrs.args].filter(Boolean).join(' '), !!attrs.confirm)
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

const IMAGE_SIZES: Record<string, string> = { sm: 'max-w-16', md: 'max-w-40', lg: 'max-w-full' }

/** 图片：src 为素材引用（ID、路径或 asset:ID）；size 为 sm / md / lg（默认 lg，宽度不超过面板）。找不到素材时显示说明文字。 */
const UiImage: UIComponent = ({ attrs }) => {
  const url = imageURL(attrs.src, useContext(MarkupContext))
  if (!url) return <span className="text-xs text-ink-faint">［{attrs.alt || attrs.src}］</span>
  return (
    <img
      src={url}
      alt={attrs.alt ?? ''}
      loading="lazy"
      className={cn('my-1.5 block h-auto w-full rounded-md border border-rule', IMAGE_SIZES[attrs.size ?? 'lg'] ?? IMAGE_SIZES.lg)}
    />
  )
}

export const UI_COMPONENTS: Record<string, UIComponent> = {
  'ui-stat': UiStat,
  'ui-bar': UiBar,
  'ui-badge': UiBadge,
  'ui-button': UiButton,
  'ui-collapse': UiCollapse,
  'ui-tooltip': UiTooltip,
  'ui-image': UiImage,
}
