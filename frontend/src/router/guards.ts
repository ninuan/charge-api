import type { UserRole } from "@/types/dashboard";

type RequiredRole = UserRole | "guest";

type AuthResolutionOptions = {
  initialized: boolean;
  role: UserRole | null;
  requiredRole: RequiredRole;
  fetchMe: () => Promise<{ role?: UserRole } | null>;
};

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

export async function resolveProtectedRouteAfterAuth(options: AuthResolutionOptions) {
  let role = options.role;
  if (!options.initialized) {
    try {
      role = (await options.fetchMe())?.role ?? null;
    } catch {
      role = null;
    }
  }
  return resolveProtectedRoute(role, options.requiredRole);
}

export function toNavigationGuardResult(redirect: string | null) {
  return redirect ? { path: redirect, replace: true } : true;
}
