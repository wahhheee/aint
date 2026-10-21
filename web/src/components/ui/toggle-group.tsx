"use client"

import * as React from "react"
import { type VariantProps } from "class-variance-authority"
import { cn } from "cn"
import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui"

import { toggleVariants } from "@/components/ui/toggle"

const ToggleGroupContext = React.createContext<
  VariantProps<typeof toggleVariants> & {
    orientation?: "horizontal" | "vertical"
  }
>({
  size: "default",
  variant: "default",
  orientation: "horizontal",
})

/**
 * 一组切换按钮。variant="segmented" 是分段控件：一条凹槽里并排几段，选中的一段浮起；
 * 每段等宽（取最宽的一段），切换时控件本身的宽度不变。
 */
function ToggleGroup({
  className,
  variant,
  size,
  orientation = "horizontal",
  children,
  ...props
}: React.ComponentProps<typeof ToggleGroupPrimitive.Root> &
  VariantProps<typeof toggleVariants> & {
    orientation?: "horizontal" | "vertical"
  }) {
  return (
    <ToggleGroupPrimitive.Root
      data-slot="toggle-group"
      data-variant={variant}
      data-size={size}
      data-orientation={orientation}
      className={cn(
        "group/toggle-group flex w-fit flex-row items-center gap-1 data-vertical:flex-col data-vertical:items-stretch",
        "data-[variant=segmented]:grid data-[variant=segmented]:auto-cols-fr data-[variant=segmented]:grid-flow-col data-[variant=segmented]:gap-0.5 data-[variant=segmented]:rounded-lg data-[variant=segmented]:bg-paper-sunken data-[variant=segmented]:p-0.5",
        className
      )}
      {...props}
    >
      <ToggleGroupContext.Provider value={{ variant, size, orientation }}>
        {children}
      </ToggleGroupContext.Provider>
    </ToggleGroupPrimitive.Root>
  )
}

function ToggleGroupItem({
  className,
  children,
  variant = "default",
  size = "default",
  ...props
}: React.ComponentProps<typeof ToggleGroupPrimitive.Item> &
  VariantProps<typeof toggleVariants>) {
  const context = React.useContext(ToggleGroupContext)

  return (
    <ToggleGroupPrimitive.Item
      data-slot="toggle-group-item"
      data-variant={context.variant || variant}
      data-size={context.size || size}
      className={cn(
        "shrink-0 focus:z-10 focus-visible:z-10",
        toggleVariants({
          variant: context.variant || variant,
          size: context.size || size,
        }),
        className
      )}
      {...props}
    >
      {children}
    </ToggleGroupPrimitive.Item>
  )
}

export { ToggleGroup, ToggleGroupItem }
