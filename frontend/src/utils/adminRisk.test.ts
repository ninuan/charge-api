import { describe, expect, it } from "vitest";
import { hasActiveAuthFailure, hasAdminRisk } from "./adminRisk";
import type { AdminUserSummary, TrafficStats } from "@/types/dashboard";

const baseStats: TrafficStats = {
  totalRequests: 0,
  refreshRequests: 0,
  remoteFetches: 0,
  cachedRefreshes: 0,
  failedRequests: 0,
  authFailures: 0
};

function summary(overrides: Partial<AdminUserSummary> = {}): AdminUserSummary {
  return {
    user: {
      id: "user-1",
      username: "alice",
      role: "user",
      enabled: true,
      createdAt: "2026-07-06T00:00:00Z",
      deviceLimit: 10,
      refreshEnabled: true
    },
    stats: baseStats,
    dashboard: {
      pileCount: 0,
      portCount: 0,
      inUsePortCount: 0,
      idlePortCount: 0,
      offlinePorts: 0
    },
    deviceIds: [],
    hasCookie: true,
    lastRefresh: {
      minIntervalSeconds: 30,
      attemptedDevices: 0,
      successfulDevices: 0,
      failedDevices: 0,
      skippedDevices: 0,
      cached: false,
      partial: false
    },
    ...overrides
  };
}

describe("admin risk helpers", () => {
  it("does not treat recovered auth failures as active risk", () => {
    const stats: TrafficStats = {
      ...baseStats,
      authFailures: 1,
      lastAuthFailureAt: "2026-07-06T08:00:00Z",
      lastRemoteOkAt: "2026-07-06T08:01:00Z"
    };

    expect(hasActiveAuthFailure(stats)).toBe(false);
    expect(hasAdminRisk(summary({ stats }))).toBe(false);
  });

  it("treats auth failures after the latest remote success as active risk", () => {
    const stats: TrafficStats = {
      ...baseStats,
      authFailures: 1,
      lastAuthFailureAt: "2026-07-06T08:02:00Z",
      lastRemoteOkAt: "2026-07-06T08:01:00Z"
    };

    expect(hasActiveAuthFailure(stats)).toBe(true);
    expect(hasAdminRisk(summary({ stats }))).toBe(true);
  });
});
