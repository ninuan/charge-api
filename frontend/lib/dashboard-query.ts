import type { PortFilter } from "@/lib/dashboard-filters"

const portFilters = new Set<PortFilter>(["all", "idle", "charging", "offline"])

export type DashboardQuery = {
  search: string
  filter: PortFilter
}

export function parseDashboardQuery(search: string): DashboardQuery {
  const params = new URLSearchParams(search)
  const filter = params.get("status")

  return {
    search: params.get("q")?.trim() ?? "",
    filter:
      filter && portFilters.has(filter as PortFilter)
        ? (filter as PortFilter)
        : "all",
  }
}
