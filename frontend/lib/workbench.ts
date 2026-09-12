import type { Pile, Port } from "@/lib/types"
import type { PortFilter } from "@/lib/dashboard-filters"

export function portCounts(pile: Pile) {
  return pile.ports.reduce(
    (count, port) => {
      count[port.status] += 1
      return count
    },
    { idle: 0, in_use: 0, offline: 0 }
  )
}

export function selectWorkbenchPiles(
  piles: Pile[],
  search: string,
  filter: PortFilter
) {
  const query = search.trim().toLocaleLowerCase()
  const portQuery = /^\d{1,2}$/.test(query) ? Number(query) : null
  const wanted =
    filter === "charging" ? "in_use" : filter === "all" ? null : filter
  return piles.flatMap((pile) => {
    const matched =
      !query ||
      (portQuery === null &&
        `${pile.name} ${pile.number} ${pile.id} ${pile.address}`
          .toLocaleLowerCase()
          .includes(query))
    const ports = pile.ports.filter(
      (port) =>
        (matched || port.id === portQuery) &&
        (!wanted || port.status === wanted) &&
        (wanted !== "idle" || pile.online)
    )
    if (
      !ports.length &&
      !(matched && filter === "all" && pile.ports.length === 0)
    )
      return []
    return [{ pile, portIds: ports.map((port) => port.id) }]
  })
}

export function portTime(port: Port, kind: "used" | "remaining") {
  if (port.status !== "in_use") return "—"
  if (kind === "remaining") return port.remainingText?.trim() || "平台未提供"
  if (port.usedText?.trim()) return port.usedText.trim()
  if (!Number.isFinite(port.usedSeconds) || port.usedSeconds < 0)
    return "平台未提供"
  const minutes = Math.floor(port.usedSeconds / 60)
  return minutes < 60
    ? `${minutes} 分钟`
    : `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分钟`
}

export function formatSnapshotTime(value?: string, date = false) {
  if (!value || !Number.isFinite(Date.parse(value))) return "尚未读取"
  return new Intl.DateTimeFormat("zh-CN", {
    ...(date ? ({ month: "numeric", day: "numeric" } as const) : {}),
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value))
}

export function dashboardHref(pileId?: string, portId?: number | null) {
  const params = new URLSearchParams()
  if (pileId) params.set("pile", pileId)
  if (portId && portId > 0) params.set("port", String(portId))
  return `/dashboard${params.size ? `?${params}` : ""}`
}

export function updatePageQuery(
  values: Record<string, string | number | null | undefined>,
  replace = false
) {
  const params = new URLSearchParams(window.location.search)
  for (const [key, value] of Object.entries(values)) {
    if (value === null || value === undefined || value === "")
      params.delete(key)
    else params.set(key, String(value))
  }
  const url = `${window.location.pathname}${params.size ? `?${params}` : ""}${window.location.hash}`
  // Next.js synchronizes useSearchParams for external history updates. Passing
  // its internal state back makes it treat this as its own update and skip sync.
  if (replace) window.history.replaceState(null, "", url)
  else window.history.pushState(null, "", url)
}
