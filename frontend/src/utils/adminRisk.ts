import type { AdminUserSummary, TrafficStats } from "@/types/dashboard";

export function hasActiveAuthFailure(stats: TrafficStats) {
  if (!stats.authFailures || !stats.lastAuthFailureAt) return false;
  if (!stats.lastRemoteOkAt) return true;
  return new Date(stats.lastAuthFailureAt).getTime() > new Date(stats.lastRemoteOkAt).getTime();
}

export function hasAdminRisk(summary: AdminUserSummary) {
  return !summary.hasCookie || hasActiveAuthFailure(summary.stats) || summary.stats.failedRequests > 0;
}
