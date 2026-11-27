import type { UserRole } from "@/lib/types"

export function resolveHomeRoute(role: UserRole | null) {
  if (role === "admin") return "/admin"
  if (role === "user") return "/dashboard"

  return "/login"
}

export function normalizeLegacyHash(hash: string) {
  const legacyPaths = new Set(["/login", "/register", "/dashboard", "/admin"])
  const path = hash.startsWith("#") ? hash.slice(1) : hash

  return legacyPaths.has(path) ? path : null
}
