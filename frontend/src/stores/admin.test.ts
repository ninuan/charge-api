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

function errorResponse(status: number, error: string) {
  return {
    ok: false,
    status,
    json: vi.fn().mockResolvedValue({ error })
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
        overview: {
          openIssues: 0,
          remoteSuccessRate: 100,
          activeUsers: 1,
          managedDevices: 0,
          offlinePorts: 0
        },
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
      .mockResolvedValueOnce(jsonResponse({
        checkedAt: "2026-07-09T00:00:00Z",
        charge: { state: "healthy", message: "服务正常" },
        database: { state: "healthy", message: "存储正常" },
        yyb: { state: "healthy", message: "扫码服务正常" }
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
    expect(admin.overview.remoteSuccessRate).toBe(100);
    expect(admin.health?.yyb.state).toBe("healthy");
  });

  it("keeps stats when the health request fails", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(jsonResponse({
        overview: {
          openIssues: 2,
          remoteSuccessRate: 75,
          activeUsers: 3,
          managedDevices: 4,
          offlinePorts: 1
        },
        users: [],
        hourly: [],
        daily: [],
        exceptions: []
      }))
      .mockResolvedValueOnce(errorResponse(503, "health unavailable")));

    const admin = useAdminStore();
    await admin.loadOverview();

    expect(admin.overview.openIssues).toBe(2);
    expect(admin.statsError).toBe("");
    expect(admin.health).toBeNull();
    expect(admin.healthError).toBe("health unavailable");
  });

  it("asks the backend to generate invite codes", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ id: "invite-1", code: "CHG-SERVER", enabled: true, createdAt: "2026-07-09T00:00:00Z", usedCount: 0 }))
      .mockResolvedValueOnce(jsonResponse([{ id: "invite-1", code: "CHG-SERVER", enabled: true, createdAt: "2026-07-09T00:00:00Z", usedCount: 0 }]));
    vi.stubGlobal("fetch", fetchMock);

    const admin = useAdminStore();
    await admin.createInvite();

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/admin/invites", expect.objectContaining({
      method: "POST",
      body: "{}"
    }));
    expect(admin.invites[0]?.code).toBe("CHG-SERVER");
  });

  it("loads a user page and prefetches the next page for immediate navigation", async () => {
    const firstPage = {
      items: [{
        user: { id: "user-1", username: "first", role: "user", enabled: true, createdAt: "2026-07-01T00:00:00Z", deviceLimit: 10, refreshEnabled: true },
        stats: trafficStats,
        dashboard: dashboardCounters,
        deviceIds: [],
        hasCookie: false,
        credential: { state: "unbound", bound: false, hasCredential: false },
        lastRefresh: refreshInfo
      }],
      page: 1,
      pageSize: 15,
      total: 16,
      totalPages: 2
    };
    const secondPage = { ...firstPage, items: [], page: 2, totalPages: 2 };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(firstPage))
      .mockResolvedValueOnce(jsonResponse(secondPage));
    vi.stubGlobal("fetch", fetchMock);

    const admin = useAdminStore();
    await (admin as any).loadUserPage();
    await Promise.resolve();

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/admin/users?page=1&pageSize=15&search=&account=all&credential=all&health=all", { credentials: "include" });
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/admin/users?page=2&pageSize=15&search=&account=all&credential=all&health=all", { credentials: "include" });
    expect((admin as any).userPage.items[0].user.username).toBe("first");
  });
});
