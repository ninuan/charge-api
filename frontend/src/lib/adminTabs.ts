export type AdminTab = "overview" | "users" | "settings";

export function resolveAdminTab(value: unknown): AdminTab {
  return value === "overview" || value === "users" || value === "settings"
    ? value
    : "overview";
}

export function adminTabQuery(tab: AdminTab) {
  return { tab };
}
