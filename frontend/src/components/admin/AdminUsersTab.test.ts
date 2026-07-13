import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { describe, expect, it } from "vitest";
import AdminUserDrawer from "./AdminUserDrawer.vue";
import AdminUsersTab from "./AdminUsersTab.vue";
import type { AdminUserSummary, CredentialState } from "@/types/dashboard";

function makeUser(
  id: string,
  credentialState: CredentialState,
  overrides: Partial<AdminUserSummary["user"]> = {}
): AdminUserSummary {
  return {
    user: {
      id,
      username: `${credentialState}-user`,
      role: "user",
      enabled: true,
      createdAt: "2026-07-01T08:00:00Z",
      deviceLimit: 10,
      refreshEnabled: true,
      ...overrides
    },
    stats: {
      totalRequests: 12,
      refreshRequests: 8,
      remoteFetches: 6,
      cachedRefreshes: 2,
      failedRequests: credentialState === "expired" ? 1 : 0,
      authFailures: credentialState === "expired" ? 1 : 0,
      lastRequestAt: "2026-07-09T08:00:00Z",
      lastRemoteFetchAt: "2026-07-09T07:58:00Z"
    },
    dashboard: {
      pileCount: 1,
      portCount: 10,
      inUsePortCount: 2,
      idlePortCount: 8,
      offlinePorts: 0
    },
    deviceIds: ["2601201412385560001"],
    hasCookie: credentialState === "healthy",
    credential: {
      state: credentialState,
      bound: credentialState !== "unbound",
      hasCredential: credentialState === "healthy",
      lastCheckedAt: "2026-07-09T07:58:00Z"
    },
    snapshotUpdatedAt: "2026-07-09T08:00:00Z",
    recoveryDiagnostics: [],
    lastRefresh: {
      lastRemoteAt: "2026-07-09T07:58:00Z",
      minIntervalSeconds: 30,
      attemptedDevices: 1,
      successfulDevices: credentialState === "expired" ? 0 : 1,
      failedDevices: credentialState === "expired" ? 1 : 0,
      skippedDevices: 0,
      cached: false,
      partial: false
    }
  };
}

const users = [
  makeUser("healthy", "healthy"),
  makeUser("expired", "expired"),
  makeUser("disabled", "waiting_device", { enabled: false })
];

const firstPage = { items: users, page: 1, pageSize: 15, total: 18, totalPages: 2 };
const firstQuery = { page: 1, pageSize: 15, search: "", account: "all" as const, credential: "all" as const, health: "all" as const };

const dialogStubs = {
  Dialog: { template: "<div><slot /></div>" },
  DialogContent: { template: "<section><slot /></section>" },
  DialogHeader: { template: "<header><slot /></header>" },
  DialogTitle: { template: "<h2><slot /></h2>" },
  DialogDescription: { template: "<p><slot /></p>" },
  DialogFooter: { template: "<footer><slot /></footer>" }
};

describe("AdminUsersTab", () => {
  it("sends filters to the server and keeps destructive actions out of the main list", async () => {
    const wrapper = mount(AdminUsersTab, { props: { page: firstPage, query: firstQuery } });

    expect(wrapper.text()).toContain("账户状态");
    expect(wrapper.text()).toContain("扫码凭据");
    expect(wrapper.text()).toContain("当前健康");
    expect(wrapper.text()).toContain("最近访问");
    expect(wrapper.text()).not.toContain("重置密码");
    expect(wrapper.text()).not.toContain("删除用户");
    expect(wrapper.find(".hidden.overflow-x-auto.lg\\:block").exists()).toBe(true);
    expect(wrapper.find(".admin-user-card").exists()).toBe(true);

    const filterToggle = wrapper.get("[data-mobile-filter-toggle]");
    expect(filterToggle.attributes("aria-expanded")).toBe("false");
    await filterToggle.trigger("click");
    expect(filterToggle.attributes("aria-expanded")).toBe("true");
    expect(wrapper.get(".admin-user-filter-options").classes()).toContain("is-open");

    await wrapper.get('[aria-label="筛选凭据状态"]').setValue("expired");

    expect(wrapper.emitted("query-change")?.[0]).toEqual([{ credential: "expired", page: 1 }]);
  });

  it("emits the selected user from the single row action", async () => {
    const wrapper = mount(AdminUsersTab, { props: { page: firstPage, query: firstQuery } });

    await wrapper.get('[data-user-detail="healthy"]').trigger("click");

    expect(wrapper.emitted("select-user")?.[0]).toEqual([users[0]]);
  });

  it("delegates page changes instead of filtering a full user list in the browser", async () => {
    const wrapper = mount(AdminUsersTab, {
      props: {
        page: { items: users.slice(0, 2), page: 1, pageSize: 2, total: 5, totalPages: 3 },
        query: { page: 1, pageSize: 2, search: "", account: "all", credential: "all", health: "all" }
      } as never
    });

    await wrapper.get("[data-user-page-next]").trigger("click");

    expect(wrapper.emitted("page-change")?.[0]).toEqual([2]);
  });
});

describe("AdminUserDrawer", () => {
  it("shows detailed health and protected account operations", () => {
    const wrapper = mount(AdminUserDrawer, {
      props: {
        user: users[1],
        currentUserId: "admin-1"
      },
      global: {
        plugins: [createPinia()],
        stubs: dialogStubs
      }
    });

    expect(wrapper.text()).toContain("设备额度");
    expect(wrapper.text()).toContain("远端刷新权限");
    expect(wrapper.text()).toContain("最近请求");
    expect(wrapper.text()).toContain("最近远端请求");
    expect(wrapper.text()).toContain("当前问题");
    expect(wrapper.text()).toContain("重置密码");
    expect(wrapper.text()).toContain("删除用户");
    const drawer = wrapper.get(".admin-user-drawer");
    expect(drawer.classes()).toContain("h-dvh");
    expect(drawer.classes()).toContain("w-full");
    expect(drawer.classes()).toContain("sm:max-w-xl");
  });

  it("shows a sanitized credential recovery timeline instead of raw upstream errors", () => {
    const user = makeUser("diagnostic", "sync_failed");
    user.recoveryDiagnostics = [
      {
        code: "remote_auth_rejected",
        message: "远端拒绝原登录凭据，开始自动恢复",
        deviceSuffix: "6001",
        statusCode: 403,
        at: "2026-07-13T13:24:32Z"
      },
      {
        code: "mocele_autologin_missing_info",
        message: "自动登录未返回必要的 info 凭据",
        deviceSuffix: "6001",
        at: "2026-07-13T13:24:35Z"
      }
    ];
    const wrapper = mount(AdminUserDrawer, {
      props: { user, currentUserId: "admin-1" },
      global: { plugins: [createPinia()], stubs: dialogStubs }
    });

    expect(wrapper.text()).toContain("凭据恢复诊断");
    expect(wrapper.text()).toContain("远端拒绝原登录凭据，开始自动恢复");
    expect(wrapper.text()).toContain("设备尾号 6001");
    expect(wrapper.text()).toContain("HTTP 403");
    expect(wrapper.text()).not.toContain("Cookie=");
  });
});
