import type { LucideIcon } from "lucide-react"
import type * as React from "react"

import { cn } from "@/lib/utils"

function LeadingIcon({
  icon: Icon,
  className,
  iconClassName,
  ...props
}: React.ComponentProps<"span"> & {
  icon: LucideIcon
  iconClassName?: string
}) {
  return (
    <span
      data-slot="leading-icon"
      aria-hidden="true"
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-lg bg-muted",
        className
      )}
      {...props}
    >
      <Icon className={cn("size-4", iconClassName)} />
    </span>
  )
}

export { LeadingIcon }
