import type { LucideIcon } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"

export function MetricCard({ compact = false, label, value, detail, icon: Icon }: { compact?: boolean; label: string; value: number; detail: string; icon: LucideIcon }) {
  const summary = <div aria-label={`${label} 指标`} className={`flex items-start justify-between gap-3 ${compact ? "p-3" : "p-4"}`}><div><p className="text-sm text-muted-foreground">{label}</p><p className={`font-semibold tabular-nums ${compact ? "mt-1 text-xl" : "mt-2 text-2xl"}`}>{value}</p><p className="mt-1 text-xs text-muted-foreground">{detail}</p></div><span className="rounded-lg bg-muted p-2 text-muted-foreground"><Icon className="size-4" /></span></div>

  if (compact) return summary

  return <Card className="shadow-none" data-testid="metric-card"><CardContent className="p-0">{summary}</CardContent></Card>
}
