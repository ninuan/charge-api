import type { AdminUserSummary, TrafficStats } from "@/types/dashboard";

export function hasActiveAuthFailure(stats: TrafficStats) {
  if (!stats.authFailures || !stats.lastAuthFailureAt) return false;
  if (!stats.lastRemoteOkAt) return true;
  return new Date(stats.lastAuthFailureAt).getTime() > new Date(stats.lastRemoteOkAt).getTime();
}

export function hasCredentialRisk(summary: AdminUserSummary) {
  if (summary.deviceIds.length === 0) return false;
  return summary.credential.state === "unbound"
    || summary.credential.state === "sync_failed"
    || summary.credential.state === "expired";
}

export function hasAdminRisk(summary: AdminUserSummary) {
  return !summary.user.enabled
    || hasCredentialRisk(summary)
    || hasActiveAuthFailure(summary.stats)
    || summary.lastRefresh.failedDevices > 0
    || summary.dashboard.offlinePorts > 0;
}
