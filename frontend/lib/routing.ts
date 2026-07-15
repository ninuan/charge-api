import type { UserRole } from "@/lib/types"

type RequiredRole = UserRole | "guest"

export function resolveHomeRoute(role: UserRole | null) {
  if (role === "admin") return "/admin"
  if (role === "user") return "/dashboard"

  return "/login"
}

export function resolveRoute(role: UserRole | null, requiredRole: RequiredRole) {
  if (requiredRole === "guest") {
    return role ? resolveHomeRoute(role) : null
  }

  if (!role) return "/login"

  return role === requiredRole ? null : resolveHomeRoute(role)
}

export function normalizeLegacyHash(hash: string) {
  const legacyPaths = new Set(["/login", "/register", "/dashboard", "/admin"])
  const path = hash.startsWith("#") ? hash.slice(1) : hash

  return legacyPaths.has(path) ? path : null
}
