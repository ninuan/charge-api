import type { LucideIcon } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

export function MetricCard({ compact = false, label, value, detail, icon: Icon }: { compact?: boolean; label: string; value: number | null; detail: string; icon: LucideIcon }) {
  // value 为 null 表示数据还没回来：显示骨架占位，而不是误导性的 0。
  const summary = <div aria-label={`${label} 指标`} className={`flex items-start justify-between gap-3 ${compact ? "p-3" : "p-4"}`}><div><p className="text-sm text-muted-foreground">{label}</p>{value === null ? <Skeleton className={compact ? "mt-1.5 h-6 w-12" : "mt-2.5 h-7 w-14"} /> : <p className={`font-semibold tabular-nums ${compact ? "mt-1 text-xl" : "mt-2 text-2xl"}`}>{value}</p>}<p className="mt-1 text-xs text-muted-foreground">{detail}</p></div><span className="rounded-lg bg-muted p-2 text-muted-foreground"><Icon className="size-4" /></span></div>

  if (compact) return summary

  return <Card className="shadow-none" data-testid="metric-card"><CardContent className="p-0">{summary}</CardContent></Card>
}
