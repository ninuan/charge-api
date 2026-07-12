import { createPinia } from "pinia";
import { config, flushPromises, shallowMount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { reactive } from "vue";
import AdminDashboardPage from "./AdminDashboardPage.vue";
import AdminConsoleShell from "@/components/admin/AdminConsoleShell.vue";
import type { AdminUserSummary } from "@/types/dashboard";

const route = reactive<{ query: Record<string, unknown> }>({ query: { tab: "unknown" } });
const replace = vi.fn();
let statsUsers: AdminUserSummary[] = [];

vi.mock("vue-router", () => ({
  useRoute: () => route,
  useRouter: () => ({ replace })
}));

function jsonResponse(body: unknown) {
  return {
    ok: true,
    json: vi.fn().mockResolvedValue(body)
  } as unknown as Response;
}

describe("AdminDashboardPage", () => {
  beforeEach(() => {
    config.global.renderStubDefaultSlot = true;
    replace.mockReset();
    route.query = { tab: "unknown" };
    statsUsers = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/admin/stats") {
        return jsonResponse({
          overview: { openIssues: 0, remoteSuccessRate: 0, activeUsers: 0, managedDevices: 0, offlinePorts: 0 },
          users: statsUsers, hourly: [], daily: [], exceptions: []
        });
      }
      if (url === "/api/admin/health") {
        return jsonResponse({
          checkedAt: "2026-07-09T00:00:00Z",
          charge: { state: "healthy", message: "服务正常" },
          database: { state: "healthy", message: "存储正常" },
          yyb: { state: "healthy", message: "扫码服务正常" }
        });
      }
      if (url === "/api/admin/settings") {
        return jsonResponse({
          openRegistration: true, inviteRequired: true, defaultDeviceLimit: 10,
          defaultRefreshEnabled: true, statsRetentionDays: 90
        });
      }
      if (url.startsWith("/api/admin/users?")) {
        return jsonResponse({ items: statsUsers, page: 1, pageSize: 15, total: statsUsers.length, totalPages: 1 });
      }
      return jsonResponse([]);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    config.global.renderStubDefaultSlot = false;
  });

  it("falls back to overview and writes selected tabs to the URL", async () => {
    const wrapper = shallowMount(AdminDashboardPage, {
      global: {
        plugins: [createPinia()],
        stubs: {
          AdminConsoleShell: {
            name: "AdminConsoleShell",
            props: { title: String, description: String, eyebrow: String },
            template: "<main><slot name='navigation' /><slot name='status' /><slot name='actions' /><slot /></main>"
          },
          AdminHealthStrip: { template: "<div data-header-health />" },
          AdminOverviewTab: true
        }
      }
    });
    await flushPromises();

    const shell = wrapper.findComponent(AdminConsoleShell);
    expect(shell.props("title")).toBe("运营总览");
    expect(shell.props("eyebrow")).toBe("Operations overview");
    expect(wrapper.find("[data-header-health]").exists()).toBe(true);
    expect(wrapper.get('[data-admin-tab="overview"]').attributes("aria-current")).toBe("page");
    expect(wrapper.get('[data-admin-tab="overview"]').text()).toBe("运营总览");
    expect(wrapper.get('[data-admin-tab="users"]').text()).toBe("用户管理");
    expect(wrapper.get('[data-admin-tab="settings"]').text()).toBe("系统设置");
    await wrapper.get('[data-admin-tab="users"]').trigger("click");

    expect(replace).toHaveBeenCalledWith({ query: { tab: "users" } });

    route.query = { tab: "settings" };
    await wrapper.vm.$nextTick();
    expect(shell.props("title")).toBe("系统设置");
  });

  it("uses the user query for list selections and drawer closing", async () => {
    statsUsers = [{
      user: {
        id: "user-1", username: "alice", role: "user", enabled: true,
        createdAt: "2026-07-01T00:00:00Z", deviceLimit: 10, refreshEnabled: true
      },
      stats: {
        totalRequests: 1, refreshRequests: 1, remoteFetches: 1,
        cachedRefreshes: 0, failedRequests: 0, authFailures: 0
      },
      dashboard: {
        pileCount: 1, portCount: 10, inUsePortCount: 0, idlePortCount: 10, offlinePorts: 0
      },
      deviceIds: ["device-1"],
      hasCookie: true,
      credential: { state: "healthy", bound: true, hasCredential: true },
      snapshotUpdatedAt: "2026-07-09T00:00:00Z",
      lastRefresh: {
        minIntervalSeconds: 30, attemptedDevices: 1, successfulDevices: 1,
        failedDevices: 0, skippedDevices: 0, cached: false, partial: false
      }
    }];
    route.query = { tab: "users" };

    const wrapper = shallowMount(AdminDashboardPage, {
      global: {
        plugins: [createPinia()],
        stubs: {
          AdminConsoleShell: {
            name: "AdminConsoleShell",
            props: { title: String, description: String, eyebrow: String },
            template: "<main><slot name='navigation' /><slot name='status' /><slot name='actions' /><slot /></main>"
          },
          AdminHealthStrip: true,
          AdminOverviewTab: true,
          AdminUsersTab: {
            props: ["page"],
            emits: ["select-user"],
            template: '<button data-select-user @click="$emit(\'select-user\', page.items[0])">选择用户</button>'
          },
          AdminUserDrawer: {
            props: ["user"],
            emits: ["close"],
            template: '<button data-close-user @click="$emit(\'close\')">{{ user && user.user.username }}</button>'
          }
        }
      }
    });
    await flushPromises();

    await wrapper.get("[data-select-user]").trigger("click");
    expect(replace).toHaveBeenLastCalledWith({ query: { tab: "users", user: "user-1" } });

    route.query = { tab: "users", user: "user-1" };
    await wrapper.vm.$nextTick();
    expect(wrapper.get("[data-close-user]").text()).toContain("alice");
    await wrapper.get("[data-close-user]").trigger("click");
    expect(replace).toHaveBeenLastCalledWith({ query: { tab: "users" } });
  });
});
