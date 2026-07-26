import type { LucideIcon } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

type MetricTone = "default" | "primary" | "success" | "warning" | "destructive"

// 图标底色按业务语义染色，让指标区不再是一排同样的灰方块。
const toneClasses: Record<MetricTone, string> = {
  default: "bg-muted text-muted-foreground",
  primary: "bg-primary/10 text-primary",
  success: "bg-success/15 text-success-foreground",
  warning: "bg-warning/15 text-warning-foreground",
  destructive: "bg-destructive/10 text-destructive",
}

export function MetricCard({ compact = false, label, value, detail, icon: Icon, tone = "default" }: { compact?: boolean; label: string; value: number | null; detail: string; icon: LucideIcon; tone?: MetricTone }) {
  // value 为 null 表示数据还没回来：显示骨架占位，而不是误导性的 0。
  const summary = <div aria-label={`${label} 指标`} className={`flex items-start justify-between gap-3 ${compact ? "p-3" : "p-4"}`}><div><p className="text-sm text-muted-foreground">{label}</p>{value === null ? <Skeleton className={compact ? "mt-1.5 h-6 w-12" : "mt-2.5 h-7 w-14"} /> : <p key={value} className={`font-semibold tabular-nums motion-safe:animate-in motion-safe:fade-in motion-safe:zoom-in-75 motion-safe:duration-300 ${compact ? "mt-1 text-xl" : "mt-2 text-2xl"}`}>{value}</p>}<p className="mt-1 text-xs text-muted-foreground">{detail}</p></div><span className={`rounded-lg p-2 ${toneClasses[tone]}`}><Icon className="size-4" /></span></div>

  if (compact) return summary

  return <Card className="shadow-xs" data-testid="metric-card"><CardContent className="p-0">{summary}</CardContent></Card>
}
