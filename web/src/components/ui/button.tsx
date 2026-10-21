import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"
import { Slot } from "radix-ui"

// 尺寸固定在 4px 网格上：xs 28 / sm 32 / default 36 / lg 40；图标按钮同高的正方形。
const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center rounded-lg border border-transparent bg-clip-padding font-medium whitespace-nowrap transition-[background-color,border-color,color,opacity] duration-150 outline-none select-none focus-visible:ring-3 focus-visible:ring-ring/40 disabled:pointer-events-none disabled:opacity-45 aria-invalid:border-destructive [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/88",
        outline:
          "border-rule-strong bg-transparent text-ink hover:bg-hover aria-expanded:bg-hover",
        secondary:
          "bg-paper-sunken text-ink hover:bg-selected aria-expanded:bg-selected",
        ghost:
          "text-ink-muted hover:bg-hover hover:text-ink aria-expanded:bg-hover aria-expanded:text-ink",
        destructive:
          "bg-destructive/10 text-destructive hover:bg-destructive/16 focus-visible:ring-destructive/30",
        link: "text-ink underline decoration-rule-strong underline-offset-4 hover:decoration-ink",
      },
      size: {
        default: "h-9 gap-2 px-4 text-sm",
        xs: "h-7 gap-1.5 rounded-md px-2.5 text-xs [&_svg:not([class*='size-'])]:size-3.5",
        sm: "h-8 gap-1.5 px-3 text-[13px] [&_svg:not([class*='size-'])]:size-4",
        lg: "h-10 gap-2 px-5 text-sm",
        icon: "size-9",
        "icon-xs": "size-7 rounded-md [&_svg:not([class*='size-'])]:size-3.5",
        "icon-sm": "size-8",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot.Root : "button"

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
