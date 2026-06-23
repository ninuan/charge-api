import type { UserRole } from "@/types/dashboard";

type RequiredRole = UserRole | "guest";

export function resolveHomeRoute(role: UserRole | null) {
  if (role === "admin") return "/admin";
  if (role === "user") return "/dashboard";
  return "/login";
}

export function resolveProtectedRoute(role: UserRole | null, requiredRole: RequiredRole) {
  if (requiredRole === "guest") {
    return role ? resolveHomeRoute(role) : null;
  }
  if (!role) return "/login";
  return role === requiredRole ? null : resolveHomeRoute(role);
}
