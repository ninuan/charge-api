import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminUsers } from "@/components/admin-users"
import type { AdminUserSummary } from "@/lib/types"

vi.mock("@/lib/admin-api", () => ({
  adminApi: {
    updateUser: vi.fn(),
    removeUser: vi.fn(),
  },
}))

function summary(
  id: string,
  username: string,
  role: "admin" | "user"
): AdminUserSummary {
  return {
    user: {
      id,
      username,
      role,
      enabled: true,
      createdAt: "2026-07-27T00:00:00Z",
      deviceLimit: 10,
      refreshEnabled: true,
    },
    stats: {
      totalRequests: 0,
      refreshRequests: 0,
      remoteFetches: 0,
      cachedRefreshes: 0,
      failedRequests: 0,
      authFailures: 0,
    },
    dashboard: {
      pileCount: 0,
      portCount: 0,
      inUsePortCount: 0,
      idlePortCount: 0,
      offlinePorts: 0,
    },
    deviceIds: [],
    hasCookie: false,
    credential: {
      state: "unbound",
      bound: false,
      hasCredential: false,
    },
    snapshotUpdatedAt: "2026-07-27T00:00:00Z",
    lastRefresh: {
      minIntervalSeconds: 60,
      attemptedDevices: 0,
      successfulDevices: 0,
      failedDevices: 0,
      skippedDevices: 0,
      cached: false,
      partial: false,
    },
  }
}

describe("AdminUsers", () => {
  afterEach(cleanup)

  it("protects the current admin and does not report no-device accounts as risk", () => {
    render(
      <AdminUsers
        currentUserId="admin-id"
        query={{
          page: 1,
          pageSize: 15,
          search: "",
          account: "all",
          credential: "all",
          health: "all",
        }}
        page={{
          items: [
            summary("admin-id", "admin", "admin"),
            summary("user-id", "new-user", "user"),
          ],
          page: 1,
          pageSize: 15,
          total: 2,
          totalPages: 1,
        }}
        load={vi.fn().mockResolvedValue(undefined)}
        selectedUserId={null}
        onSelectedUserChange={vi.fn()}
      />
    )

    expect(screen.getByRole("table", { name: "用户目录" })).toBeInTheDocument()
    expect(screen.getAllByText("当前账号")).toHaveLength(2)
    expect(screen.getAllByText("无需扫码")).toHaveLength(2)
    expect(screen.getAllByText("待添加设备")).toHaveLength(2)
    expect(screen.queryByText("需要关注")).not.toBeInTheDocument()
    expect(
      screen.getAllByRole("button", { name: "管理用户 admin" })
    ).toHaveLength(2)
    expect(
      screen.getAllByRole("button", { name: "管理用户 new-user" })
    ).toHaveLength(2)
  })
})
