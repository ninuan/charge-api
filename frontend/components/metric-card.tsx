import type { LucideIcon } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"

export function MetricCard({ label, value, detail, icon: Icon }: { label: string; value: number; detail: string; icon: LucideIcon }) {
  return <Card className="shadow-none"><CardContent className="flex items-start justify-between gap-3 p-5"><div><p className="text-sm text-muted-foreground">{label}</p><p className="mt-3 text-3xl font-semibold tabular-nums">{value}</p><p className="mt-2 text-xs text-muted-foreground">{detail}</p></div><span className="rounded-lg bg-muted p-2 text-muted-foreground"><Icon className="size-5" /></span></CardContent></Card>
}
