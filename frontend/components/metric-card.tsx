"use client"

import type { LucideIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

type MetricTone = "default" | "primary" | "success" | "warning" | "destructive"

// 图标底色按业务语义染色，让指标区不再是一排同样的灰方块。
const toneClasses: Record<MetricTone, string> = {
  default: "bg-muted text-muted-foreground",
  primary: "bg-primary/10 text-primary",
  success: "bg-success/15 text-success-foreground",
  warning: "bg-warning/15 text-warning-foreground",
  destructive: "bg-destructive/10 text-destructive",
}

function AnimatedMetricValue({ value }: { value: number }) {
  const previousValue = useRef(value)
  const [displayValue, setDisplayValue] = useState(value)

  useEffect(() => {
    const startValue = previousValue.current
    previousValue.current = value

    if (startValue === value) return

    if (
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      const reducedMotionFrame = requestAnimationFrame(() =>
        setDisplayValue(value)
      )
      return () => cancelAnimationFrame(reducedMotionFrame)
    }

    const startedAt = performance.now()
    const duration = 220
    let frame = 0
    const finish = window.setTimeout(
      () => setDisplayValue(value),
      duration + 40
    )
    const tick = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / duration)
      const eased = 1 - Math.pow(1 - progress, 3)
      setDisplayValue(Math.round(startValue + (value - startValue) * eased))
      if (progress < 1) frame = requestAnimationFrame(tick)
      else window.clearTimeout(finish)
    }
    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
      window.clearTimeout(finish)
    }
  }, [value])

  return displayValue
}

export function MetricCard({
  compact = false,
  label,
  value,
  suffix,
  detail,
  icon: Icon,
  tone = "default",
  className,
}: {
  compact?: boolean
  label: string
  value: number | null
  suffix?: string
  detail: string
  icon: LucideIcon
  tone?: MetricTone
  className?: string
}) {
  // value 为 null 表示数据还没回来：显示骨架占位，而不是误导性的 0。
  const summary = (
    <div
      aria-label={`${label} 指标`}
      className={cn(
        "flex items-start justify-between gap-3",
        compact ? "p-3" : "p-4",
        className
      )}
    >
      <div>
        <p className="text-sm text-muted-foreground">{label}</p>
        {value === null ? (
          <Skeleton
            className={compact ? "mt-1.5 h-6 w-12" : "mt-2.5 h-7 w-14"}
          />
        ) : (
          <p
            className={`font-semibold tabular-nums ${compact ? "mt-1 text-xl" : "mt-2 text-2xl"}`}
          >
            <AnimatedMetricValue value={value} />
            {suffix && <span className="ml-0.5 text-[0.65em]">{suffix}</span>}
          </p>
        )}
        <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
      </div>
      <span className={`rounded-lg p-2 ${toneClasses[tone]}`}>
        <Icon className="size-4" />
      </span>
    </div>
  )

  if (compact) return summary

  return (
    <Card className="shadow-xs" data-testid="metric-card">
      <CardContent className="p-0">{summary}</CardContent>
    </Card>
  )
}
