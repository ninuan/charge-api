import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import AdminOverviewTab from "./AdminOverviewTab.vue";
import type { AdminOverview, AdminUserSummary, MetricPoint, SystemException } from "@/types/dashboard";

const overview: AdminOverview = {
  openIssues: 1,
  remoteSuccessRate: 98.6,
  activeUsers: 3,
  managedDevices: 4,
  offlinePorts: 2
};

const user: AdminUserSummary = {
  user: {
    id: "user-1", username: "alice", role: "user", enabled: true,
    createdAt: "2026-07-09T00:00:00Z", deviceLimit: 10, refreshEnabled: true
  },
  stats: {
    totalRequests: 3, refreshRequests: 2, remoteFetches: 2,
    cachedRefreshes: 0, failedRequests: 1, authFailures: 1,
    lastRequestAt: "2026-07-09T08:00:00Z"
  },
  dashboard: {
    pileCount: 1, portCount: 10, inUsePortCount: 1, idlePortCount: 8, offlinePorts: 1
  },
  deviceIds: ["2601201412385560001"],
  hasCookie: false,
  credential: { state: "expired", bound: true, hasCredential: false },
  snapshotUpdatedAt: "2026-07-09T08:00:00Z",
  lastRefresh: {
    minIntervalSeconds: 30, attemptedDevices: 1, successfulDevices: 0,
    failedDevices: 1, skippedDevices: 0, cached: true, partial: false
  }
};

const issue: SystemException = {
  id: "auth-user-1",
  userId: "user-1",
  username: "alice",
  type: "cookie_expired",
  level: "critical",
  message: "远端鉴权失败，登录凭据可能已失效",
  time: "2026-07-09T08:00:00Z"
};

const points: MetricPoint[] = [{
  time: "2026-07-09T08:00:00Z",
  requests: 3,
  remote: 2,
  cacheHits: 1,
  remoteOk: 1,
  remoteFailed: 1,
  cookieErrors: 1,
  activeUsers: 1
}];

describe("AdminOverviewTab", () => {
  it("renders operations health and emits the selected issue user", async () => {
    const wrapper = mount(AdminOverviewTab, {
      props: {
        overview,
        hourly: points,
        daily: points,
        issues: [issue],
        users: [user],
        statsLoading: false,
        statsError: "",
        lastUpdatedAt: "2026-07-09T08:00:00Z"
      },
      global: {
        stubs: {
          AdminTrendChart: { template: "<div>请求与成功率趋势</div>" }
        }
      }
    });

    expect(wrapper.text()).toContain("待处理异常");
    expect(wrapper.text()).toContain("远端成功率");
    expect(wrapper.text()).toContain("活跃用户");
    expect(wrapper.text()).toContain("受管设备");
    expect(wrapper.text()).toContain("离线充电口");
    expect(wrapper.find(".admin-health-strip").exists()).toBe(false);
    expect(wrapper.text()).toContain("请求与成功率趋势");
    expect(wrapper.text()).not.toContain("Traffic composition");
    expect(wrapper.text()).not.toContain("未配置 Cookie");
    expect(wrapper.get(".admin-account-head").text()).toContain("扫码凭据");
    expect(wrapper.get(".admin-account-head").text()).toContain("最近访问");

    const usersLink = wrapper.get("[data-open-users]");
    expect(usersLink.text()).toContain("全部用户");
    await usersLink.trigger("click");
    expect(wrapper.emitted("open-users")).toHaveLength(1);

    await wrapper.get('[data-issue-user="user-1"]').trigger("click");
    expect(wrapper.emitted("open-user")?.[0]).toEqual(["user-1"]);
  });

  it("shows useful empty states", () => {
    const wrapper = mount(AdminOverviewTab, {
      props: {
        overview: { ...overview, openIssues: 0 },
        hourly: [],
        daily: [],
        issues: [],
        users: [],
        statsLoading: false,
        statsError: "",
        lastUpdatedAt: undefined
      },
      global: {
        stubs: {
          AdminTrendChart: { template: "<div>暂无趋势数据</div>" }
        }
      }
    });

    expect(wrapper.text()).toContain("当前没有需要处理的异常");
    expect(wrapper.text()).toContain("暂无账户数据");
  });

  it("keeps operational metrics visible while health is rendered in the page header", async () => {
    const wrapper = mount(AdminOverviewTab, {
      props: {
        overview,
        hourly: points,
        daily: points,
        issues: [issue],
        users: [user],
        statsLoading: false,
        statsError: "",
        lastUpdatedAt: "2026-07-09T08:00:00Z"
      },
      global: {
        stubs: {
          AdminTrendChart: { template: "<div>请求与成功率趋势</div>" }
        }
      }
    });

    expect(wrapper.text()).toContain("远端成功率");
    expect(wrapper.text()).not.toContain("扫码服务检查失败");
    expect(wrapper.get('[data-issue-user="user-1"]').text()).toContain("查看");
  });
});
