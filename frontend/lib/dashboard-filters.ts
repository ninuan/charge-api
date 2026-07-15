import type { Pile, PortStatus } from "@/lib/types"

export type PortFilter = "all" | "idle" | "charging" | "offline"

const statusMap: Record<PortFilter, PortStatus | null> = { all: null, idle: "idle", charging: "in_use", offline: "offline" }

export function filterPiles(piles: Pile[], search: string, filter: PortFilter) {
  const query = search.trim().toLowerCase()
  const isPortNumberQuery = /^\d{1,2}$/.test(query)
  const requiredStatus = statusMap[filter]

  return piles.flatMap((pile) => {
    const pileMatches = !query || (!isPortNumberQuery && `${pile.name} ${pile.number} ${pile.address} ${pile.id}`.toLowerCase().includes(query))
    const ports = pile.ports.filter((port) => (pileMatches || (isPortNumberQuery && port.id === Number(query))) && (!requiredStatus || port.status === requiredStatus))
    return ports.length ? [{ pile, portIds: ports.map((port) => port.id) }] : []
  })
}
