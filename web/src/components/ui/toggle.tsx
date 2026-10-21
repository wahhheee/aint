import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"
import { Toggle as TogglePrimitive } from "radix-ui"

const toggleVariants = cva(
  "group/toggle inline-flex items-center justify-center gap-1.5 rounded-md font-medium whitespace-nowrap transition-[background-color,color,box-shadow] duration-150 outline-none focus-visible:ring-3 focus-visible:ring-ring/40 disabled:pointer-events-none disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default:
          "bg-transparent text-ink-muted hover:bg-hover hover:text-ink data-[state=on]:bg-selected data-[state=on]:text-ink",
        outline:
          "border border-rule-strong bg-transparent text-ink-muted hover:bg-hover hover:text-ink data-[state=on]:border-ink/40 data-[state=on]:bg-selected data-[state=on]:text-ink",
        // 分段控件中的一段：放在 ToggleGroup variant="segmented" 的槽里，选中的一段浮起
        segmented:
          "text-ink-muted hover:text-ink data-[state=on]:bg-segment data-[state=on]:text-ink data-[state=on]:shadow-segment",
      },
      size: {
        default: "h-8 min-w-8 px-3 text-[13px]",
        sm: "h-7 min-w-7 px-2.5 text-xs [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-9 min-w-9 px-3.5 text-sm",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Toggle({
  className,
  variant = "default",
  size = "default",
  ...props
}: React.ComponentProps<typeof TogglePrimitive.Root> &
  VariantProps<typeof toggleVariants>) {
  return (
    <TogglePrimitive.Root
      data-slot="toggle"
      className={cn(toggleVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Toggle, toggleVariants }
