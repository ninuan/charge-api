import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAdminStore } from "./admin";

const trafficStats = {
  totalRequests: 0,
  refreshRequests: 0,
  remoteFetches: 0,
  cachedRefreshes: 0,
  failedRequests: 0,
  authFailures: 0
};

const dashboardCounters = {
  pileCount: 0,
  portCount: 0,
  inUsePortCount: 0,
  idlePortCount: 0,
  offlinePorts: 0
};

const refreshInfo = {
  minIntervalSeconds: 30,
  attemptedDevices: 0,
  successfulDevices: 0,
  failedDevices: 0,
  skippedDevices: 0,
  cached: false,
  partial: false
};

const settings = {
  openRegistration: true,
  inviteRequired: true,
  defaultDeviceLimit: 10,
  defaultRefreshEnabled: true,
  statsRetentionDays: 90
};

function jsonResponse(body: unknown) {
  return {
    ok: true,
    json: vi.fn().mockResolvedValue(body)
  } as unknown as Response;
}

describe("admin store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.unstubAllGlobals();
  });

  it("normalizes nullable list fields from admin APIs", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(jsonResponse({
        users: [{
          user: {
            id: "user-1",
            username: "wang",
            role: "user",
            enabled: true,
            createdAt: "2026-06-24T00:00:00Z",
            deviceLimit: 10,
            refreshEnabled: true
          },
          stats: trafficStats,
          dashboard: dashboardCounters,
          deviceIds: null,
          hasCookie: true,
          lastRefresh: refreshInfo
        }],
        hourly: null,
        daily: null,
        exceptions: null
      }))
      .mockResolvedValueOnce(jsonResponse(settings))
      .mockResolvedValueOnce(jsonResponse(null)));

    const admin = useAdminStore();
    await admin.load();

    expect(admin.users).toHaveLength(1);
    expect(admin.users[0].deviceIds).toEqual([]);
    expect(admin.hourly).toEqual([]);
    expect(admin.daily).toEqual([]);
    expect(admin.exceptions).toEqual([]);
    expect(admin.invites).toEqual([]);
    expect(admin.totals.devices).toBe(0);
  });
});
